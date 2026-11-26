package service

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"uuid"

	"kebabseller6/rwe/internal/manifest"
)

const (
	filesDirName = "files"
	manifestFile = "manifest.json"
	targetFile   = "target.json"
)

type Limits struct {
	MaxTotalBytes int64
	MaxFiles      int
}

var DefaultLimits = Limits{
	MaxTotalBytes: 200 << 20,
	MaxFiles:      20_000,
}

type Workspace struct {
	ID           uuid.UUID `json:"id"`
	ManifestHash string    `json:"manifest_hash"`
}

type WorkspaceService struct {
	dir        string
	limits     Limits
	manifestMu sync.Mutex
	busy       map[uuid.UUID]bool
}

func NewWorkspaceService(dir string) (*WorkspaceService, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create workspace dir: %w", err)
	}
	return &WorkspaceService{dir: dir, limits: DefaultLimits, busy: map[uuid.UUID]bool{}}, nil
}

func (s *WorkspaceService) wsDir(id uuid.UUID) string {
	return filepath.Join(s.dir, id.String())
}

func (s *WorkspaceService) filesDir(id uuid.UUID) string {
	return filepath.Join(s.wsDir(id), filesDirName)
}

func (s *WorkspaceService) manifestPath(id uuid.UUID) string {
	return filepath.Join(s.wsDir(id), manifestFile)
}

func (s *WorkspaceService) targetPath(id uuid.UUID) string {
	return filepath.Join(s.wsDir(id), targetFile)
}

func (s *WorkspaceService) loadManifest(id uuid.UUID) (manifest.Manifest, error) {
	m, err := manifest.Load(s.manifestPath(id))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: workspace %s", ErrNotFound, id)
	}
	return m, err
}
