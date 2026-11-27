package service

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"uuid"
)

func (s *WorkspaceService) Create(ctx context.Context, body io.Reader) (Workspace, error) {
	id := uuid.NewV7()

	tmp, err := os.MkdirTemp(s.dir, ".tmp-")
	if err != nil {
		return Workspace{}, err
	}
	defer os.RemoveAll(tmp)

	filesDir := filepath.Join(tmp, filesDirName)
	if err := os.Mkdir(filesDir, 0o755); err != nil {
		return Workspace{}, err
	}

	m, err := extract(ctx, body, filesDir, s.limits)
	if err != nil {
		return Workspace{}, err
	}

	if err := writeJSON(filepath.Join(tmp, manifestFile), m); err != nil {
		return Workspace{}, err
	}

	if err := os.Rename(tmp, s.wsDir(id)); err != nil {
		return Workspace{}, err
	}

	return Workspace{ID: id, ManifestHash: m.Hash()}, nil
}
