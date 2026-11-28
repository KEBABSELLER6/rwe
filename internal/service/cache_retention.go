package service

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"
	"uuid"
)

func (s *JobService) pruneCaches(now time.Time) {
	workspaces := filepath.Join(s.cachesDir, "workspaces")
	for _, e := range readDirOrWarn(workspaces) {
		id, err := uuid.Parse(e.Name())
		if err != nil || s.workspaces.Exists(id) {
			continue
		}
		removeCache(filepath.Join(workspaces, e.Name()))
	}

	if s.limits.CacheRetention <= 0 {
		return
	}
	cutoff := now.Add(-s.limits.CacheRetention)
	shared := filepath.Join(s.cachesDir, "shared")
	for _, e := range readDirOrWarn(shared) {
		info, err := e.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		removeCache(filepath.Join(shared, e.Name()))
	}
}

func readDirOrWarn(dir string) []os.DirEntry {
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		slog.Warn("list caches", "dir", dir, "err", err)
	}
	return entries
}

func removeCache(dir string) {
	slog.Info("delete unused cache", "dir", dir)
	if err := forceRemoveAll(dir); err != nil {
		slog.Warn("delete cache", "dir", dir, "err", err)
	}
}
