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

func (s *JobService) save(job *Job) error {
	return writeFileJSON(s.statePath(job.ID), job)
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
		job, err := s.loadJob(id)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			slog.Warn("skip unreadable job state", "job", id, "err", err)
			continue
		}
		s.jobs[id] = job
	}
	return nil
}

func (s *JobService) loadJob(id uuid.UUID) (*Job, error) {
	raw, err := os.ReadFile(s.statePath(id))
	if err != nil {
		return nil, err
	}
	var job Job
	if err := json.Unmarshal(raw, &job); err != nil {
		return nil, err
	}
	if job.ID != id {
		return nil, fmt.Errorf("state file has id %s", job.ID)
	}
	if !job.Status.Finished() {
		job.Status = JobError
		job.Error = errServerRestarted.Error()
		job.FinishedAt = time.Now()
		forceRemoveAll(s.jobFilesDir(id))
		s.saveOrWarn(&job)
	}
	return &job, nil
}
