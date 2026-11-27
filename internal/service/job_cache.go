package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"uuid"
)

const (
	WorkspaceMount = "/workspace"
	maxCaches      = 8
)

type CacheMount struct {
	HostDir string
	Path    string
}

func validateCaches(paths []string) error {
	if len(paths) > maxCaches {
		return fmt.Errorf("%w: at most %d caches", ErrInvalidInput, maxCaches)
	}
	seen := map[string]bool{}
	for _, p := range paths {
		switch {
		case !path.IsAbs(p) || path.Clean(p) != p:
			return fmt.Errorf("%w: cache %q must be a clean absolute path", ErrInvalidInput, p)
		case p == "/" || p == WorkspaceMount:
			return fmt.Errorf("%w: cache %q would hide the whole filesystem or workspace", ErrInvalidInput, p)
		case strings.ContainsAny(p, ":,"):
			return fmt.Errorf("%w: cache %q must not contain ':' or ','", ErrInvalidInput, p)
		case seen[p]:
			return fmt.Errorf("%w: cache %q listed twice", ErrInvalidInput, p)
		}
		seen[p] = true
	}
	return nil
}

func (s *JobService) cacheMounts(workspaceID uuid.UUID, paths []string) []CacheMount {
	mounts := make([]CacheMount, 0, len(paths))
	for _, p := range paths {
		sum := sha256.Sum256([]byte(p))
		key := hex.EncodeToString(sum[:8])
		dir := filepath.Join(s.cachesDir, "shared", key)
		if inWorkspace(p) {
			dir = filepath.Join(s.cachesDir, "workspaces", workspaceID.String(), key)
		}
		mounts = append(mounts, CacheMount{HostDir: dir, Path: p})
	}
	return mounts
}

func inWorkspace(p string) bool {
	return strings.HasPrefix(p, WorkspaceMount+"/")
}

func prepareCaches(mounts []CacheMount, filesDir string) error {
	for _, m := range mounts {
		if err := os.MkdirAll(m.HostDir, 0o755); err != nil {
			return fmt.Errorf("create cache dir: %w", err)
		}
		if rel, ok := strings.CutPrefix(m.Path, WorkspaceMount+"/"); ok {
			if err := os.MkdirAll(filepath.Join(filesDir, filepath.FromSlash(rel)), 0o755); err != nil {
				return fmt.Errorf("create cache mount point: %w", err)
			}
		}
	}
	return nil
}
