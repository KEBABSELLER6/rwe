package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
	"uuid"
)

func (s *JobService) Start(ctx context.Context) {
	s.pruneLogs(time.Now())
	prune := time.NewTicker(pruneInterval)
	defer prune.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-prune.C:
			s.pruneLogs(now)
		case id := <-s.queue:
			s.run(ctx, id)
		}
	}
}

func (s *JobService) run(ctx context.Context, id uuid.UUID) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	s.mu.Lock()
	job := s.jobs[id]
	if job.Status == JobCancelled {
		s.mu.Unlock()
		return
	}
	job.Status = JobRunning
	job.StartedAt = time.Now()
	job.cancel = cancel
	spec := job.spec
	s.saveOrWarn(job)
	s.mu.Unlock()

	code, err := s.runOnCopy(ctx, id, spec)
	s.workspaces.Release(job.WorkspaceID)

	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.saveOrWarn(job)
	job.FinishedAt = time.Now()
	job.DurationMS = job.FinishedAt.Sub(job.StartedAt).Milliseconds()
	switch {
	case err != nil && errors.Is(context.Cause(ctx), errCancelled):
		job.Status = JobCancelled
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
	defer forceRemoveAll(filesDir)
	if err := os.CopyFS(filesDir, os.DirFS(spec.FilesDir)); err != nil {
		return -1, fmt.Errorf("copy workspace for job: %w", err)
	}
	if err := prepareCaches(spec.Caches, filesDir); err != nil {
		return -1, err
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
