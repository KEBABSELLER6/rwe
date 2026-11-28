package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
	"uuid"
)

func (s *JobService) Start(ctx context.Context) {
	s.prune(time.Now())
	prune := time.NewTicker(pruneInterval)
	defer prune.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-prune.C:
			s.prune(now)
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
	spec, artifacts := job.spec, job.artifactPaths
	s.saveOrWarn(job)
	s.mu.Unlock()

	phases := newPhaseTimer()
	code, collected, err := s.runOnCopy(ctx, id, spec, artifacts, phases)
	phases.log(id)
	s.workspaces.Release(job.WorkspaceID)

	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.saveOrWarn(job)
	job.FinishedAt = time.Now()
	job.DurationMS = job.FinishedAt.Sub(job.StartedAt).Milliseconds()
	job.PhasesMS = phases.ms
	job.Artifacts = collected
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

func (s *JobService) runOnCopy(ctx context.Context, id uuid.UUID, spec RunSpec, artifacts []string, t *phaseTimer) (int, []Artifact, error) {
	filesDir := s.jobFilesDir(id)
	defer func() {
		forceRemoveAll(filesDir)
		t.done("cleanup")
	}()
	if err := copyTree(filesDir, spec.FilesDir); err != nil {
		return -1, nil, fmt.Errorf("copy workspace for job: %w", err)
	}
	if err := prepareCaches(spec.Caches, filesDir); err != nil {
		return -1, nil, err
	}
	t.done("copy")
	spec.FilesDir = filesDir
	return s.runWithLog(ctx, id, spec, artifacts, t)
}

func (s *JobService) runWithLog(ctx context.Context, id uuid.UUID, spec RunSpec, artifacts []string, t *phaseTimer) (int, []Artifact, error) {
	log, err := os.OpenFile(s.logPath(id), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return -1, nil, err
	}
	code, runErr := s.runner.Run(ctx, spec, log)
	t.done("run")
	collected := s.collectArtifacts(filepath.Dir(s.logPath(id)), artifacts, log)
	t.done("artifacts")
	return code, collected, errors.Join(runErr, log.Close())
}

type phaseTimer struct {
	last  time.Time
	attrs []any
	ms    map[string]int64
}

func newPhaseTimer() *phaseTimer {
	return &phaseTimer{last: time.Now(), ms: map[string]int64{}}
}

func (t *phaseTimer) done(phase string) {
	now := time.Now()
	t.attrs = append(t.attrs, phase, now.Sub(t.last).Round(time.Millisecond))
	t.ms[phase] = now.Sub(t.last).Milliseconds()
	t.last = now
}

func (t *phaseTimer) log(id uuid.UUID) {
	slog.Info("job phases", append([]any{"job", id}, t.attrs...)...)
}
