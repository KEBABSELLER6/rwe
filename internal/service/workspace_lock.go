package service

import (
	"fmt"
	"uuid"
)

func (s *WorkspaceService) Acquire(id uuid.UUID, expectedHash string) (filesDir string, err error) {
	s.manifestMu.Lock()
	defer s.manifestMu.Unlock()

	if err := s.checkNotBusy(id); err != nil {
		return "", err
	}

	current, err := s.loadManifest(id)
	if err != nil {
		return "", err
	}

	if current.Hash() != expectedHash {
		return "", fmt.Errorf("%w: workspace content does not match expected_hash", ErrConflict)
	}

	if err := s.touch(id); err != nil {
		return "", err
	}
	s.busy[id] = true
	return s.filesDir(id), nil
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
