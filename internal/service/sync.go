package service

import (
	"context"
	"fmt"
	"os"
	"path"
	"slices"
	"uuid"

	"kebabseller6/rwe/internal/manifest"
)

type syncTarget struct {
	Files manifest.Manifest `json:"files"`
	Sizes map[string]int64  `json:"sizes"`
}

func (s *WorkspaceService) Sync(ctx context.Context, id uuid.UUID, want manifest.Manifest, sizes map[string]int64) ([]string, error) {
	s.manifestMu.Lock()
	defer s.manifestMu.Unlock()

	if err := s.checkNotBusy(id); err != nil {
		return nil, err
	}

	current, err := s.loadManifest(id)
	if err != nil {
		return nil, err
	}

	if err := validateManifest(want, s.limits); err != nil {
		return nil, err
	}

	root, err := os.OpenRoot(s.filesDir(id))
	if err != nil {
		return nil, err
	}
	defer root.Close()

	if err := checkTotalSize(root, current, want, sizes, s.limits); err != nil {
		return nil, err
	}

	for p := range current {
		if _, keep := want[p]; keep {
			continue
		}
		if err := removeWithEmptyParents(root, p); err != nil {
			return nil, fmt.Errorf("remove %q: %w", p, err)
		}
		delete(current, p)
	}

	if err := writeJSON(s.manifestPath(id), current); err != nil {
		return nil, err
	}

	missing := []string{}
	for p, sum := range want {
		if current[p] != sum {
			missing = append(missing, p)
		}
	}
	slices.Sort(missing)

	if err := writeJSON(s.targetPath(id), syncTarget{Files: want, Sizes: sizes}); err != nil {
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
		if !manifest.Sha256Hex.MatchString(sum) {
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

func checkTotalSize(root *os.Root, current, want manifest.Manifest, sizes map[string]int64, lim Limits) error {
	var total int64
	for p, sum := range want {
		size, declared := sizes[p]
		if current[p] == sum {
			fi, err := root.Stat(p)
			if err != nil {
				return err
			}
			size = fi.Size()
		} else if !declared || size < 0 {
			return fmt.Errorf("%w: missing or negative size for %q", ErrInvalidInput, p)
		}
		if size > lim.MaxTotalBytes-total {
			return fmt.Errorf("%w: workspace would exceed %d bytes", ErrTooLarge, lim.MaxTotalBytes)
		}
		total += size
	}
	return nil
}
