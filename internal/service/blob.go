package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"uuid"

	"kebabseller6/rwe/internal/manifest"
)

func (s *WorkspaceService) PutBlob(ctx context.Context, id uuid.UUID, sum string, body io.Reader) (remaining int, err error) {
	if !manifest.Sha256Hex.MatchString(sum) {
		return 0, fmt.Errorf("%w: invalid sha256 %q", ErrInvalidInput, sum)
	}

	target, err := manifest.Load(s.targetPath(id))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, fmt.Errorf("%w: workspace %s has no pending sync", ErrNotFound, id)
	}
	if err != nil {
		return 0, err
	}

	paths := pathsWithSum(target, sum)
	if len(paths) == 0 {
		return 0, fmt.Errorf("%w: blob %s was not requested", ErrInvalidInput, sum)
	}

	blob, err := s.receiveBlob(s.wsDir(id), sum, body)
	if err != nil {
		return 0, err
	}
	defer os.Remove(blob)

	if err := ctx.Err(); err != nil {
		return 0, err
	}

	s.manifestMu.Lock()
	defer s.manifestMu.Unlock()

	if err := s.checkNotBusy(id); err != nil {
		return 0, err
	}

	target, err = manifest.Load(s.targetPath(id))
	if err != nil {
		return 0, err
	}
	paths = pathsWithSum(target, sum)
	if len(paths) == 0 {
		return 0, fmt.Errorf("%w: blob %s is no longer requested", ErrInvalidInput, sum)
	}

	current, err := s.loadManifest(id)
	if err != nil {
		return 0, err
	}

	root, err := os.OpenRoot(s.filesDir(id))
	if err != nil {
		return 0, err
	}
	defer root.Close()

	for _, p := range paths {
		if err := placeFile(root, blob, p); err != nil {
			return 0, fmt.Errorf("place %q: %w", p, err)
		}
		current[p] = sum
	}

	if err := writeJSON(s.manifestPath(id), current); err != nil {
		return 0, err
	}

	for p, want := range target {
		if current[p] != want {
			remaining++
		}
	}
	return remaining, nil
}

func pathsWithSum(m manifest.Manifest, sum string) []string {
	var paths []string
	for p, s := range m {
		if s == sum {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	return paths
}

func (s *WorkspaceService) receiveBlob(wsDir, sum string, body io.Reader) (string, error) {
	tmp, err := os.CreateTemp(wsDir, ".blob-*")
	if err != nil {
		return "", err
	}
	name := tmp.Name()

	h := sha256.New()
	limited := io.LimitReader(body, s.limits.MaxTotalBytes+1)
	n, copyErr := io.Copy(io.MultiWriter(tmp, h), limited)
	closeErr := tmp.Close()

	switch {
	case copyErr != nil:
		err = fmt.Errorf("%w: reading blob: %w", ErrInvalidInput, copyErr)
	case closeErr != nil:
		err = closeErr
	case n > s.limits.MaxTotalBytes:
		err = fmt.Errorf("%w: blob exceeds %d bytes", ErrTooLarge, s.limits.MaxTotalBytes)
	case hex.EncodeToString(h.Sum(nil)) != sum:
		err = fmt.Errorf("%w: content does not match sha256 %s", ErrInvalidInput, sum)
	}
	if err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}
