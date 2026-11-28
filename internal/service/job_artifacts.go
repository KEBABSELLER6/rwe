package service

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"uuid"
)

const (
	artifactsDirName = "artifacts"
	maxArtifacts     = 16
)

type Artifact struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

func validateArtifacts(paths []string) ([]string, error) {
	if len(paths) > maxArtifacts {
		return nil, fmt.Errorf("%w: at most %d artifacts", ErrInvalidInput, maxArtifacts)
	}
	clean := make([]string, 0, len(paths))
	for _, p := range paths {
		c, err := cleanName(p)
		if err != nil {
			return nil, err
		}
		if c == "." {
			return nil, fmt.Errorf("%w: artifact %q would be the whole workspace", ErrInvalidInput, p)
		}
		clean = append(clean, c)
	}

	sorted := slices.Sorted(slices.Values(clean))
	for i := 1; i < len(sorted); i++ {
		if prev := sorted[i-1]; sorted[i] == prev || strings.HasPrefix(sorted[i], prev+"/") {
			return nil, fmt.Errorf("%w: artifacts %q and %q overlap", ErrInvalidInput, prev, sorted[i])
		}
	}
	return clean, nil
}

func (s *JobService) collectArtifacts(jobDir string, paths []string, log io.Writer) []Artifact {
	if len(paths) == 0 {
		return nil
	}
	root, err := os.OpenRoot(jobDir)
	if err != nil {
		fmt.Fprintf(log, "rwe: collect artifacts: %v\n", err)
		return nil
	}
	defer root.Close()

	var collected []Artifact
	for _, p := range paths {
		dst := path.Join(artifactsDirName, p)
		err := moveArtifact(root, path.Join(filesDirName, p), dst)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			fmt.Fprintf(log, "rwe: artifact %s not found\n", p)
			continue
		case err != nil:
			fmt.Fprintf(log, "rwe: artifact %s: %v\n", p, err)
			continue
		}
		size, err := treeSize(root.FS(), dst)
		if err != nil {
			fmt.Fprintf(log, "rwe: artifact %s: %v\n", p, err)
		}
		collected = append(collected, Artifact{Path: p, Size: size})
	}
	return collected
}

func moveArtifact(root *os.Root, src, dst string) error {
	if _, err := root.Lstat(src); err != nil {
		return err
	}
	if err := root.MkdirAll(path.Dir(dst), 0o755); err != nil {
		return err
	}
	return root.Rename(src, dst)
}

func treeSize(fsys fs.FS, name string) (int64, error) {
	var size int64
	err := fs.WalkDir(fsys, name, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func (s *JobService) OpenArtifacts(id uuid.UUID) (*os.Root, error) {
	job, err := s.Get(id)
	switch {
	case err != nil:
		return nil, err
	case !job.Status.Finished():
		return nil, fmt.Errorf("%w: job %s has not finished", ErrConflict, id)
	case len(job.Artifacts) == 0:
		return nil, fmt.Errorf("%w: job %s has no artifacts", ErrNotFound, id)
	}

	root, err := os.OpenRoot(filepath.Join(s.dir, id.String(), artifactsDirName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: artifacts of job %s have expired", ErrGone, id)
	}
	return root, err
}

func WriteTarGz(w io.Writer, fsys fs.FS) error {
	zw := gzip.NewWriter(w)
	tw := tar.NewWriter(zw)
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || name == "." {
			return err
		}
		if !d.IsDir() && !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = name
		hdr.Uid, hdr.Gid, hdr.Uname, hdr.Gname = 0, 0, "", ""
		if d.IsDir() {
			hdr.Name += "/"
			return tw.WriteHeader(hdr)
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		f, err := fsys.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	return errors.Join(err, tw.Close(), zw.Close())
}
