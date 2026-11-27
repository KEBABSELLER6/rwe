package service

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	"kebabseller6/rwe/internal/manifest"
)

func extract(ctx context.Context, r io.Reader, dst string, lim WorkspaceLimits) (manifest.Manifest, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("%w: not a gzip stream: %w", ErrInvalidInput, err)
	}
	defer zr.Close()

	root, err := os.OpenRoot(dst)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	tr := tar.NewReader(zr)
	m := manifest.Manifest{}
	var total int64

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: reading tar: %w", ErrInvalidInput, err)
		}

		if hdr.Typeflag == tar.TypeXGlobalHeader {
			continue
		}

		name, err := cleanName(hdr.Name)
		if err != nil {
			return nil, err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if name == "." {
				continue
			}
			if err := root.MkdirAll(name, 0o755); err != nil {
				return nil, fmt.Errorf("mkdir %q: %w", name, err)
			}

		case tar.TypeReg:
			if name == "." {
				return nil, fmt.Errorf("%w: file entry with empty name", ErrInvalidInput)
			}
			if len(m) >= lim.MaxFiles {
				return nil, fmt.Errorf("%w: more than %d files", ErrTooLarge, lim.MaxFiles)
			}

			total += hdr.Size
			if hdr.Size < 0 || total > lim.MaxTotalBytes {
				return nil, fmt.Errorf("%w: unpacked size exceeds %d bytes", ErrTooLarge, lim.MaxTotalBytes)
			}

			sum, err := writeFile(root, name, hdr, tr)
			if err != nil {
				return nil, err
			}
			m[name] = sum

		default:
			return nil, fmt.Errorf("%w: unsupported entry %q (type %q)", ErrInvalidInput, hdr.Name, hdr.Typeflag)
		}
	}

	return m, nil
}

func writeFile(root *os.Root, name string, hdr *tar.Header, r io.Reader) (string, error) {
	if dir := path.Dir(name); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("mkdir %q: %w", dir, err)
		}
	}

	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm(hdr.Mode&0o111 != 0))
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("%w: duplicate entry %q", ErrInvalidInput, name)
		}
		return "", fmt.Errorf("create %q: %w", name, err)
	}

	h := sha256.New()

	_, copyErr := io.Copy(io.MultiWriter(f, h), r)
	closeErr := f.Close()
	if copyErr != nil {
		return "", fmt.Errorf("%w: writing %q: %w", ErrInvalidInput, name, copyErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close %q: %w", name, closeErr)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
