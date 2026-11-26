package service

import (
	"fmt"
	"os"
	"sync"
	"uuid"
)

type Workspace struct {
	ID           uuid.UUID `json:"id"`
	ManifestHash string    `json:"manifest_hash"`
}

type WorkspaceService struct {
	dir        string
	limits     Limits
	manifestMu sync.Mutex
}

func NewWorkspaceService(dir string) (*WorkspaceService, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create workspace dir: %w", err)
	}
	return &WorkspaceService{dir: dir, limits: DefaultLimits}, nil
}
