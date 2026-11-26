package service

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"uuid"

	"kebabseller6/rwe/internal/manifest"
)

func (s *WorkspaceService) Acquire(id uuid.UUID, expectedHash string) (filesDir string, err error) {
	s.manifestMu.Lock()
	defer s.manifestMu.Unlock()

	if err := s.checkNotBusy(id); err != nil {
		return "", err
	}

	wsDir := filepath.Join(s.dir, id.String())
	current, err := manifest.Load(filepath.Join(wsDir, "manifest.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%w: workspace %s", ErrNotFound, id)
	}
	if err != nil {
		return "", err
	}

	if current.Hash() != expectedHash {
		return "", fmt.Errorf("%w: workspace content does not match expected_hash", ErrConflict)
	}

	s.busy[id] = true
	return filepath.Join(wsDir, "files"), nil
}

func (s *WorkspaceService) Release(id uuid.UUID) {
	s.manifestMu.Lock()
	defer s.manifestMu.Unlock()
	delete(s.busy, id)
}

func (s *WorkspaceService) checkNotBusy(id uuid.UUID) error {
	if s.busy[id] {
		return fmt.Errorf("%w: workspace %s has a queued or running job", ErrConflict, id)
	}
	return nil
}
