package service

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"time"
	"uuid"
)

const pruneInterval = time.Hour

func (s *JobService) prune(now time.Time) {
	s.pruneLogs(now)
	s.pruneArtifacts(now)
	s.pruneCaches(now)
}

func (s *JobService) pruneLogs(now time.Time) {
	for _, id := range s.finishedBefore(now, s.limits.LogRetention) {
		if err := os.Remove(s.logPath(id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("remove expired job log", "job", id, "err", err)
		}
	}
}

func (s *JobService) pruneArtifacts(now time.Time) {
	for _, id := range s.finishedBefore(now, s.limits.ArtifactRetention) {
		if err := forceRemoveAll(s.artifactsDir(id)); err != nil {
			slog.Warn("remove expired job artifacts", "job", id, "err", err)
		}
	}
}

func (s *JobService) finishedBefore(now time.Time, retention time.Duration) []uuid.UUID {
	if retention <= 0 {
		return nil
	}
	cutoff := now.Add(-retention)

	s.mu.Lock()
	defer s.mu.Unlock()
	var expired []uuid.UUID
	for id, job := range s.jobs {
		if job.Status.Finished() && job.FinishedAt.Before(cutoff) {
			expired = append(expired, id)
		}
	}
	return expired
}
