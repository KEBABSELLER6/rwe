package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/KEBABSELLER6/rwe/internal/manifest"
)

type Synced struct {
	WorkspaceID string
	Hash        string
	Files       int
	Uploaded    int
	Created     bool
}

func (c *Client) Sync(ctx context.Context, dir string) (Synced, error) {
	files, err := ListFiles(dir)
	if err != nil {
		return Synced{}, err
	}
	tree, err := BuildManifest(dir, files)
	if err != nil {
		return Synced{}, err
	}

	id, err := loadWorkspaceID(dir)
	if err != nil {
		return Synced{}, err
	}
	created := id == ""
	if created {
		if id, err = c.createWorkspace(ctx, dir, files); err != nil {
			return Synced{}, err
		}
	}

	missing, err := c.syncManifest(ctx, id, tree)
	if isStatus(err, http.StatusNotFound) {
		created = true
		if id, err = c.createWorkspace(ctx, dir, files); err != nil {
			return Synced{}, err
		}
		missing, err = c.syncManifest(ctx, id, tree)
	}
	if err != nil {
		return Synced{}, err
	}

	uploaded, err := c.uploadMissing(ctx, dir, id, tree.Files, missing)
	if err != nil {
		return Synced{}, err
	}

	return Synced{WorkspaceID: id, Hash: tree.Files.Hash(), Files: len(tree.Files), Uploaded: uploaded, Created: created}, nil
}

func (c *Client) createWorkspace(ctx context.Context, dir string, files []string) (string, error) {
	pr, pw := io.Pipe()
	go func() { pw.CloseWithError(writeTarball(pw, dir, files)) }()
	defer pr.Close()

	var res struct {
		ID string `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/workspaces", "application/gzip", pr, &res); err != nil {
		return "", err
	}
	if err := saveWorkspaceID(dir, res.ID); err != nil {
		return "", err
	}
	return res.ID, nil
}

func (c *Client) syncManifest(ctx context.Context, id string, tree Tree) ([]string, error) {
	var res struct {
		Missing []string `json:"missing"`
	}
	err := c.postJSON(ctx, "/workspaces/"+id+"/sync", map[string]any{"files": tree.Files, "sizes": tree.Sizes, "executable": tree.Executable}, &res)
	return res.Missing, err
}

func (c *Client) uploadMissing(ctx context.Context, dir, id string, m manifest.Manifest, missing []string) (int, error) {
	sent := map[string]bool{}
	remaining := 0
	for _, path := range missing {
		sum := m[path]
		if sent[sum] {
			continue
		}
		sent[sum] = true

		f, err := os.Open(filepath.Join(dir, path))
		if err != nil {
			return 0, err
		}
		var res struct {
			Remaining int `json:"remaining"`
		}
		err = c.do(ctx, http.MethodPut, "/workspaces/"+id+"/blobs/"+sum, "application/octet-stream", f, &res)
		f.Close()
		if err != nil {
			return 0, err
		}
		remaining = res.Remaining
	}

	if remaining != 0 {
		return 0, fmt.Errorf("server still waiting for %d file(s); a file probably changed during sync, retry", remaining)
	}
	return len(sent), nil
}

func stateFile(dir string) string {
	return filepath.Join(dir, stateDir, "workspace")
}

func loadWorkspaceID(dir string) (string, error) {
	raw, err := os.ReadFile(stateFile(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return strings.TrimSpace(string(raw)), err
}

func saveWorkspaceID(dir, id string) error {
	if err := os.MkdirAll(filepath.Join(dir, stateDir), 0o755); err != nil {
		return err
	}
	return os.WriteFile(stateFile(dir), []byte(id+"\n"), 0o644)
}
