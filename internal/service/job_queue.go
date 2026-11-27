package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
	"uuid"

	"kebabseller6/rwe/internal/manifest"
)

func (s *JobService) Submit(req JobRequest) (Job, error) {
	if err := s.validate(&req); err != nil {
		return Job{}, err
	}

	filesDir, err := s.workspaces.Acquire(req.WorkspaceID, req.ExpectedHash)
	if err != nil {
		return Job{}, err
	}

	job, err := s.newJob(req, filesDir)
	if err != nil {
		s.workspaces.Release(req.WorkspaceID)
		return Job{}, err
	}

	select {
	case s.queue <- job.ID:
		return job, nil
	default:
		s.forget(job.ID)
		s.workspaces.Release(req.WorkspaceID)
		return Job{}, fmt.Errorf("%w: job queue is full", ErrConflict)
	}
}

func (s *JobService) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-s.queue:
			s.run(ctx, id)
		}
	}
}

func (s *JobService) validate(req *JobRequest) error {
	switch {
	case !manifest.Sha256Hex.MatchString(req.ExpectedHash):
		return fmt.Errorf("%w: expected_hash must be a sha256", ErrInvalidInput)
	case req.Image == "":
		return fmt.Errorf("%w: image is required", ErrInvalidInput)
	case len(req.Command) == 0:
		return fmt.Errorf("%w: command is required", ErrInvalidInput)
	case req.Timeout < 0 || req.Timeout > s.limits.MaxTimeout:
		return fmt.Errorf("%w: timeout must be between 0 and %s", ErrInvalidInput, s.limits.MaxTimeout)
	case req.Memory < 0 || req.CPUs < 0:
		return fmt.Errorf("%w: limits must not be negative", ErrInvalidInput)
	}
	if req.Timeout == 0 {
		req.Timeout = s.limits.DefaultTimeout
	}
	return nil
}

func (s *JobService) newJob(req JobRequest, filesDir string) (Job, error) {
	id := uuid.NewV7()
	if err := os.MkdirAll(filepath.Dir(s.logPath(id)), 0o755); err != nil {
		return Job{}, err
	}
	if err := os.WriteFile(s.logPath(id), nil, 0o644); err != nil {
		return Job{}, err
	}

	job := &Job{
		ID:          id,
		WorkspaceID: req.WorkspaceID,
		Image:       req.Image,
		Command:     slices.Clone(req.Command),
		Status:      JobQueued,
		CreatedAt:   time.Now(),
		spec: RunSpec{
			Name:     "rwe-" + id.String(),
			Image:    req.Image,
			Command:  slices.Clone(req.Command),
			FilesDir: filesDir,
			Timeout:  req.Timeout,
			Memory:   req.Memory,
			CPUs:     req.CPUs,
		},
	}

	s.mu.Lock()
	s.jobs[id] = job
	s.mu.Unlock()
	return *job, nil
}

func (s *JobService) forget(id uuid.UUID) {
	s.mu.Lock()
	delete(s.jobs, id)
	s.mu.Unlock()
	os.RemoveAll(filepath.Dir(s.logPath(id)))
}

func (s *JobService) run(ctx context.Context, id uuid.UUID) {
	s.mu.Lock()
	job := s.jobs[id]
	job.Status = JobRunning
	job.StartedAt = time.Now()
	spec := job.spec
	s.mu.Unlock()

	code, err := s.runOnCopy(ctx, id, spec)
	s.workspaces.Release(job.WorkspaceID)

	s.mu.Lock()
	defer s.mu.Unlock()
	job.FinishedAt = time.Now()
	job.DurationMS = job.FinishedAt.Sub(job.StartedAt).Milliseconds()
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		job.Status = JobTimeout
		job.Error = fmt.Sprintf("timed out after %s", spec.Timeout)
	case err != nil:
		job.Status = JobError
		job.Error = err.Error()
	case code == 0:
		job.Status = JobPassed
		job.ExitCode = &code
	default:
		job.Status = JobFailed
		job.ExitCode = &code
	}
}

func (s *JobService) runOnCopy(ctx context.Context, id uuid.UUID, spec RunSpec) (int, error) {
	filesDir := s.jobFilesDir(id)
	defer os.RemoveAll(filesDir)
	if err := os.CopyFS(filesDir, os.DirFS(spec.FilesDir)); err != nil {
		return -1, fmt.Errorf("copy workspace for job: %w", err)
	}
	spec.FilesDir = filesDir
	return s.runWithLog(ctx, id, spec)
}

func (s *JobService) runWithLog(ctx context.Context, id uuid.UUID, spec RunSpec) (int, error) {
	log, err := os.OpenFile(s.logPath(id), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return -1, err
	}
	code, runErr := s.runner.Run(ctx, spec, log)
	return code, errors.Join(runErr, log.Close())
}
