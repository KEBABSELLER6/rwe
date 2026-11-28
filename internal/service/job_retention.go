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
	s.pruneCaches(now)
}

func (s *JobService) pruneLogs(now time.Time) {
	if s.limits.LogRetention <= 0 {
		return
	}
	cutoff := now.Add(-s.limits.LogRetention)

	s.mu.Lock()
	var expired []uuid.UUID
	for id, job := range s.jobs {
		if job.Status.Finished() && job.FinishedAt.Before(cutoff) {
			expired = append(expired, id)
		}
	}
	s.mu.Unlock()

	for _, id := range expired {
		if err := os.Remove(s.logPath(id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("remove expired job log", "job", id, "err", err)
		}
	}
}
