package service

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"uuid"
)

type Workspace struct {
	ID           uuid.UUID `json:"id"`
	ManifestHash string    `json:"manifest_hash"`
}

type WorkspaceService struct {
	dir    string
	limits Limits
}

func NewWorkspaceService(dir string) (*WorkspaceService, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create workspace dir: %w", err)
	}
	return &WorkspaceService{dir: dir, limits: DefaultLimits}, nil
}

func (s *WorkspaceService) Create(ctx context.Context, body io.Reader) (Workspace, error) {
	id := uuid.NewV7()

	tmp, err := os.MkdirTemp(s.dir, ".tmp-")
	if err != nil {
		return Workspace{}, err
	}
	defer os.RemoveAll(tmp)

	filesDir := filepath.Join(tmp, "files")
	if err := os.Mkdir(filesDir, 0o755); err != nil {
		return Workspace{}, err
	}

	m, err := extract(ctx, body, filesDir, s.limits)
	if err != nil {
		return Workspace{}, err
	}

	raw, err := json.Marshal(m)
	if err != nil {
		return Workspace{}, err
	}
	if err := os.WriteFile(filepath.Join(tmp, "manifest.json"), raw, 0o644); err != nil {
		return Workspace{}, err
	}

	if err := os.Rename(tmp, filepath.Join(s.dir, id.String())); err != nil {
		return Workspace{}, err
	}

	return Workspace{ID: id, ManifestHash: m.Hash()}, nil
}
