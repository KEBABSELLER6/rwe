package service

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"uuid"

	"kebabseller6/rwe/internal/manifest"
)

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (s *WorkspaceService) Sync(ctx context.Context, id uuid.UUID, want manifest.Manifest) ([]string, error) {
	s.manifestMu.Lock()
	defer s.manifestMu.Unlock()

	wsDir := filepath.Join(s.dir, id.String())

	current, err := manifest.Load(filepath.Join(wsDir, "manifest.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: workspace %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}

	if err := validateManifest(want, s.limits); err != nil {
		return nil, err
	}

	root, err := os.OpenRoot(filepath.Join(wsDir, "files"))
	if err != nil {
		return nil, err
	}
	defer root.Close()

	for p := range current {
		if _, keep := want[p]; keep {
			continue
		}
		if err := removeWithEmptyParents(root, p); err != nil {
			return nil, fmt.Errorf("remove %q: %w", p, err)
		}
		delete(current, p)
	}

	if err := writeJSON(filepath.Join(wsDir, "manifest.json"), current); err != nil {
		return nil, err
	}

	missing := []string{}
	for p, sum := range want {
		if current[p] != sum {
			missing = append(missing, p)
		}
	}
	slices.Sort(missing)

	if err := writeJSON(filepath.Join(wsDir, "target.json"), want); err != nil {
		return nil, err
	}

	return missing, nil
}

func validateManifest(m manifest.Manifest, lim Limits) error {
	if len(m) > lim.MaxFiles {
		return fmt.Errorf("%w: more than %d files", ErrTooLarge, lim.MaxFiles)
	}
	for p, sum := range m {
		clean, err := cleanName(p)
		if err != nil {
			return err
		}
		if clean != p || clean == "." {
			return fmt.Errorf("%w: non-canonical path %q", ErrInvalidInput, p)
		}
		if !sha256Hex.MatchString(sum) {
			return fmt.Errorf("%w: invalid sha256 for %q", ErrInvalidInput, p)
		}
		for dir := path.Dir(p); dir != "."; dir = path.Dir(dir) {
			if _, isFile := m[dir]; isFile {
				return fmt.Errorf("%w: %q is both a file and a directory", ErrInvalidInput, dir)
			}
		}
	}
	return nil
}

func removeWithEmptyParents(root *os.Root, p string) error {
	if err := root.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for dir := path.Dir(p); dir != "."; dir = path.Dir(dir) {
		if root.Remove(dir) != nil {
			break
		}
	}
	return nil
}

func writeJSON(name string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(name), ".tmp-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	_, writeErr := tmp.Write(raw)
	closeErr := tmp.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), name)
}
