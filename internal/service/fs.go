package service

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

func cleanName(name string) (string, error) {
	clean := path.Clean(name)
	if clean == "." {
		return ".", nil
	}
	if !filepath.IsLocal(filepath.FromSlash(clean)) {
		return "", fmt.Errorf("%w: unsafe path %q", ErrInvalidInput, name)
	}
	return clean, nil
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

func placeFile(root *os.Root, src, dst string) error {
	if dir := path.Dir(dst); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	perm := os.FileMode(0o644)
	if fi, err := root.Stat(dst); err == nil {
		perm = fi.Mode().Perm()
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmpName := path.Join(path.Dir(dst), ".rwe-tmp-"+path.Base(dst))
	out, err := root.OpenFile(tmpName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		root.Remove(tmpName)
		return err
	}

	return root.Rename(tmpName, dst)
}

func forceRemoveAll(dir string) error {
	if err := os.RemoveAll(dir); err == nil {
		return nil
	}
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			os.Chmod(p, 0o700)
		}
		return nil
	})
	return os.RemoveAll(dir)
}
