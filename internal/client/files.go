package client

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/KEBABSELLER6/rwe/internal/manifest"
)

const stateDir = ".rwe"

func ListFiles(dir string) ([]string, error) {
	candidates, err := gitFiles(dir)
	if err != nil {
		candidates, err = walkFiles(dir)
		if err != nil {
			return nil, err
		}
	}

	var files []string
	for _, f := range candidates {
		if skipped(f) {
			continue
		}
		info, err := os.Lstat(filepath.Join(dir, f))
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, f)
	}
	return files, nil
}

func skipped(path string) bool {
	return path == stateDir || strings.HasPrefix(path, stateDir+"/") || strings.HasPrefix(path, ".git/")
}

func gitFiles(dir string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00"), nil
}

func walkFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == stateDir {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	return files, err
}

type Tree struct {
	Files      manifest.Manifest
	Sizes      map[string]int64
	Executable []string
}

func BuildManifest(dir string, files []string) (Tree, error) {
	tree := Tree{Files: manifest.Manifest{}, Sizes: map[string]int64{}, Executable: []string{}}
	for _, f := range files {
		sum, size, executable, err := hashFile(filepath.Join(dir, f))
		if err != nil {
			return Tree{}, err
		}
		tree.Files[f] = sum
		tree.Sizes[f] = size
		if executable {
			tree.Executable = append(tree.Executable, f)
		}
	}
	return tree, nil
}

func hashFile(path string) (sum string, size int64, executable bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, false, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", 0, false, err
	}
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, false, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, info.Mode()&0o111 != 0, nil
}

func writeTarball(w io.Writer, dir string, files []string) error {
	zw := gzip.NewWriter(w)
	tw := tar.NewWriter(zw)

	for _, f := range files {
		if err := addFile(tw, dir, f); err != nil {
			return err
		}
	}

	if err := tw.Close(); err != nil {
		return err
	}
	return zw.Close()
}

func addFile(tw *tar.Writer, dir, name string) error {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	info, err := os.Lstat(filepath.Join(dir, name))
	if err != nil {
		return err
	}

	hdr := &tar.Header{
		Typeflag: tar.TypeReg,
		Name:     name,
		Mode:     int64(info.Mode().Perm()),
		Size:     int64(len(data)),
		ModTime:  info.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = tw.Write(data)
	return err
}
