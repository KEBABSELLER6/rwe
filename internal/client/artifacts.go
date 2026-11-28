package client

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
)

type Artifact struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

func ArtifactsDir(dir, jobID string) string {
	return filepath.Join(dir, stateDir, "artifacts", jobID)
}

func (c *Client) FetchArtifacts(ctx context.Context, dir string, job Job) (string, error) {
	if len(job.Artifacts) == 0 {
		return "", nil
	}
	resp, err := c.send(ctx, http.MethodGet, "/jobs/"+job.ID+"/artifacts", "", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	dst := ArtifactsDir(dir, job.ID)
	if err := os.RemoveAll(filepath.Dir(dst)); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", err
	}
	if err := untar(resp.Body, dst); err != nil {
		return "", fmt.Errorf("extract artifacts: %w", err)
	}
	return dst, nil
}

func untar(r io.Reader, dst string) error {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(dst)
	if err != nil {
		return err
	}
	defer root.Close()

	tr := tar.NewReader(zr)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := path.Clean(hdr.Name)
		if !filepath.IsLocal(filepath.FromSlash(name)) {
			return fmt.Errorf("unsafe path %q", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			err = root.MkdirAll(name, 0o755)
		case tar.TypeReg:
			err = writeEntry(root, name, os.FileMode(hdr.Mode)&0o755, tr)
		default:
			err = fmt.Errorf("unsupported entry %q", hdr.Name)
		}
		if err != nil {
			return err
		}
	}
}

func writeEntry(root *os.Root, name string, perm os.FileMode, r io.Reader) error {
	if err := root.MkdirAll(path.Dir(name), 0o755); err != nil {
		return err
	}
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm|0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, r)
	return errors.Join(copyErr, f.Close())
}
