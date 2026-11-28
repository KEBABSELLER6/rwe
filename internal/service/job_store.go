package service

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"
	"uuid"
)

const jobStateFile = "job.json"

var errServerRestarted = errors.New("server restarted before the job finished")

func (s *JobService) statePath(id uuid.UUID) string {
	return filepath.Join(s.dir, id.String(), jobStateFile)
}

type jobState struct {
	Job
	Requeue *requeueSpec `json:"requeue,omitempty"`
}

type requeueSpec struct {
	ExpectedHash string   `json:"expected_hash"`
	TimeoutMS    int64    `json:"timeout_ms"`
	Memory       int64    `json:"memory,omitzero"`
	CPUs         float64  `json:"cpus,omitzero"`
	Artifacts    []string `json:"artifacts,omitempty"`
}

func (s *JobService) save(job *Job) error {
	state := jobState{Job: *job}
	if job.Status == JobQueued {
		state.Requeue = &requeueSpec{
			ExpectedHash: job.expectedHash,
			TimeoutMS:    job.spec.Timeout.Milliseconds(),
			Memory:       job.spec.Memory,
			CPUs:         job.spec.CPUs,
			Artifacts:    job.artifactPaths,
		}
	}
	return writeFileJSON(s.statePath(job.ID), state)
}

func (s *JobService) saveOrWarn(job *Job) {
	if err := s.save(job); err != nil {
		slog.Warn("save job state", "job", job.ID, "err", err)
	}
}

func (s *JobService) load() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("read jobs dir: %w", err)
	}
	for _, entry := range entries {
		id, err := uuid.Parse(entry.Name())
		if err != nil || !entry.IsDir() {
			continue
		}
		state, err := s.loadState(id)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			slog.Warn("skip unreadable job state", "job", id, "err", err)
			continue
		}
		job := &state.Job
		s.jobs[id] = job
		if job.Status.Finished() {
			continue
		}
		if err := s.requeue(job, state.Requeue); err != nil {
			job.Status = JobError
			job.Error = err.Error()
			job.FinishedAt = time.Now()
			forceRemoveAll(s.jobFilesDir(id))
			s.saveOrWarn(job)
			continue
		}
		slog.Info("requeued job after restart", "job", id)
	}
	return nil
}

func (s *JobService) loadState(id uuid.UUID) (*jobState, error) {
	raw, err := os.ReadFile(s.statePath(id))
	if err != nil {
		return nil, err
	}
	var state jobState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	if state.ID != id {
		return nil, fmt.Errorf("state file has id %s", state.ID)
	}
	return &state, nil
}

func (s *JobService) requeue(job *Job, spec *requeueSpec) error {
	if job.Status != JobQueued || spec == nil {
		return errServerRestarted
	}
	req := JobRequest{
		WorkspaceID:  job.WorkspaceID,
		ExpectedHash: spec.ExpectedHash,
		Image:        job.Image,
		Command:      job.Command,
		Timeout:      time.Duration(spec.TimeoutMS) * time.Millisecond,
		Memory:       spec.Memory,
		CPUs:         spec.CPUs,
		Caches:       job.Caches,
		Artifacts:    spec.Artifacts,
	}
	filesDir, err := s.workspaces.Acquire(req.WorkspaceID, req.ExpectedHash)
	if err != nil {
		return fmt.Errorf("%w; requeue: %w", errServerRestarted, err)
	}
	s.prepare(job, req, filesDir)
	select {
	case s.queue <- job.ID:
		return nil
	default:
		s.workspaces.Release(req.WorkspaceID)
		return fmt.Errorf("%w; requeue: job queue is full", errServerRestarted)
	}
}
