package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kebabseller6/rwe/internal/manifest"
)

type entry struct {
	name     string
	typ      byte
	body     string
	mode     int64
	linkname string
}

func makeTarGz(t *testing.T, entries []entry) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		hdr := &tar.Header{Name: e.name, Typeflag: typ, Mode: mode, Size: int64(len(e.body)), Linkname: e.linkname}
		if typ != tar.TypeReg {
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf
}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestCreate_OK(t *testing.T) {
	dir := t.TempDir()
	s, err := NewWorkspaceService(dir)
	if err != nil {
		t.Fatal(err)
	}

	body := makeTarGz(t, []entry{
		{name: "./", typ: tar.TypeDir},
		{name: "./go.mod", body: "module x\n"},
		{name: "cmd/server/main.go", body: "package main\n"}, // parent dir not listed
		{name: "run.sh", body: "#!/bin/sh\n", mode: 0o4755},  // setuid must be dropped
	})

	ws, err := s.Create(t.Context(), body)
	if err != nil {
		t.Fatal(err)
	}

	files := filepath.Join(dir, ws.ID.String(), "files")
	got, err := os.ReadFile(filepath.Join(files, "cmd/server/main.go"))
	if err != nil || string(got) != "package main\n" {
		t.Fatalf("main.go = %q, %v", got, err)
	}

	fi, err := os.Stat(filepath.Join(files, "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 || fi.Mode()&os.ModeSetuid != 0 {
		t.Errorf("run.sh mode = %v, want 0755 without setuid", fi.Mode())
	}

	m, err := extractedManifest(filepath.Join(dir, ws.ID.String()))
	if err != nil {
		t.Fatal(err)
	}
	if m["go.mod"] != sha("module x\n") || len(m) != 3 {
		t.Errorf("manifest = %v", m)
	}
	if ws.ManifestHash != m.Hash() || len(ws.ManifestHash) != 64 {
		t.Errorf("manifest hash = %q", ws.ManifestHash)
	}

	assertNoTempDirs(t, dir)
}

func TestCreate_Rejects(t *testing.T) {
	tests := []struct {
		name    string
		body    func(t *testing.T) *bytes.Buffer
		limits  *Limits
		wantErr error
	}{
		{
			name:    "not gzip",
			body:    func(*testing.T) *bytes.Buffer { return bytes.NewBufferString("hello") },
			wantErr: ErrInvalidInput,
		},
		{
			name:    "parent escape",
			body:    func(t *testing.T) *bytes.Buffer { return makeTarGz(t, []entry{{name: "../escape", body: "x"}}) },
			wantErr: ErrInvalidInput,
		},
		{
			name:    "nested escape",
			body:    func(t *testing.T) *bytes.Buffer { return makeTarGz(t, []entry{{name: "a/../../escape", body: "x"}}) },
			wantErr: ErrInvalidInput,
		},
		{
			name:    "absolute path",
			body:    func(t *testing.T) *bytes.Buffer { return makeTarGz(t, []entry{{name: "/etc/passwd", body: "x"}}) },
			wantErr: ErrInvalidInput,
		},
		{
			name: "symlink",
			body: func(t *testing.T) *bytes.Buffer {
				return makeTarGz(t, []entry{{name: "link", typ: tar.TypeSymlink, linkname: "/etc"}})
			},
			wantErr: ErrInvalidInput,
		},
		{
			name: "duplicate",
			body: func(t *testing.T) *bytes.Buffer {
				return makeTarGz(t, []entry{{name: "a", body: "1"}, {name: "./a", body: "2"}})
			},
			wantErr: ErrInvalidInput,
		},
		{
			name: "too many bytes",
			body: func(t *testing.T) *bytes.Buffer {
				return makeTarGz(t, []entry{{name: "big", body: strings.Repeat("x", 11)}})
			},
			limits:  &Limits{MaxTotalBytes: 10, MaxFiles: 10},
			wantErr: ErrTooLarge,
		},
		{
			name: "too many files",
			body: func(t *testing.T) *bytes.Buffer {
				return makeTarGz(t, []entry{{name: "a"}, {name: "b"}, {name: "c"}})
			},
			limits:  &Limits{MaxTotalBytes: 100, MaxFiles: 2},
			wantErr: ErrTooLarge,
		},
		{
			name: "truncated",
			body: func(t *testing.T) *bytes.Buffer {
				full := makeTarGz(t, []entry{{name: "a", body: strings.Repeat("x", 4096)}})
				return bytes.NewBuffer(full.Bytes()[:full.Len()/2])
			},
			wantErr: ErrInvalidInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			s, err := NewWorkspaceService(dir)
			if err != nil {
				t.Fatal(err)
			}
			if tt.limits != nil {
				s.limits = *tt.limits
			}

			_, err = s.Create(t.Context(), tt.body(t))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}

			// A failed upload must leave nothing behind.
			left, _ := os.ReadDir(dir)
			if len(left) != 0 {
				t.Errorf("leftover entries in %s: %v", dir, left)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape")); err == nil {
				t.Error("file escaped the workspace")
			}
		})
	}
}

func assertNoTempDirs(t *testing.T, dir string) {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("temp dir left behind: %s", e.Name())
		}
	}
}

func extractedManifest(wsDir string) (manifest.Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(wsDir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m manifest.Manifest
	return m, json.Unmarshal(raw, &m)
}
