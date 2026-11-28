package service

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
	"uuid"
)

const deletingPrefix = ".del-"

func (s *WorkspaceService) StartPruning(ctx context.Context) {
	s.prune(time.Now())
	ticker := time.NewTicker(pruneInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.prune(now)
		}
	}
}

func (s *WorkspaceService) touch(id uuid.UUID) error {
	now := time.Now()
	return os.Chtimes(s.wsDir(id), now, now)
}

func (s *WorkspaceService) prune(now time.Time) {
	if s.limits.Retention <= 0 {
		return
	}
	for _, id := range s.detachExpired(now.Add(-s.limits.Retention)) {
		slog.Info("delete unused workspace", "workspace", id)
	}

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		slog.Warn("list workspaces", "err", err)
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), deletingPrefix) {
			if err := forceRemoveAll(filepath.Join(s.dir, e.Name())); err != nil {
				slog.Warn("delete workspace files", "dir", e.Name(), "err", err)
			}
		}
	}
}

func (s *WorkspaceService) detachExpired(cutoff time.Time) []uuid.UUID {
	s.manifestMu.Lock()
	defer s.manifestMu.Unlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		slog.Warn("list workspaces", "err", err)
		return nil
	}
	var detached []uuid.UUID
	for _, e := range entries {
		id, err := uuid.Parse(e.Name())
		if err != nil || !e.IsDir() || s.busy[id] {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.Rename(s.wsDir(id), filepath.Join(s.dir, deletingPrefix+e.Name())); err != nil {
			slog.Warn("detach unused workspace", "workspace", id, "err", err)
			continue
		}
		detached = append(detached, id)
	}
	return detached
}
