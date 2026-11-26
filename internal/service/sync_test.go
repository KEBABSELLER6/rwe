package service

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"uuid"

	"kebabseller6/rwe/internal/manifest"
)

func newSyncedWorkspace(t *testing.T) (*WorkspaceService, string, uuid.UUID) {
	t.Helper()
	dir := t.TempDir()
	s, err := NewWorkspaceService(dir)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := s.Create(t.Context(), makeTarGz(t, []entry{
		{name: "keep.txt", body: "keep"},
		{name: "change.txt", body: "old"},
		{name: "gone/deep/a.txt", body: "a"},
		{name: "half/b.txt", body: "b"},
		{name: "half/c.txt", body: "c"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	return s, filepath.Join(dir, ws.ID.String()), ws.ID
}

func TestSync(t *testing.T) {
	s, wsDir, id := newSyncedWorkspace(t)
	files := filepath.Join(wsDir, "files")

	want := manifest.Manifest{
		"keep.txt":    sha("keep"),
		"change.txt":  sha("new"),
		"half/b.txt":  sha("b"),
		"new/file.go": sha("package new"),
	}

	missing, err := s.Sync(t.Context(), id, want)
	if err != nil {
		t.Fatal(err)
	}

	if wantMissing := []string{"change.txt", "new/file.go"}; !slices.Equal(missing, wantMissing) {
		t.Errorf("missing = %v, want %v", missing, wantMissing)
	}

	for _, gone := range []string{"gone", "half/c.txt"} {
		if _, err := os.Stat(filepath.Join(files, gone)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s still exists (err=%v)", gone, err)
		}
	}
	if _, err := os.Stat(filepath.Join(files, "half/b.txt")); err != nil {
		t.Errorf("half/b.txt removed: %v", err)
	}

	current, err := manifest.Load(filepath.Join(wsDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	wantCurrent := manifest.Manifest{
		"keep.txt":   sha("keep"),
		"change.txt": sha("old"),
		"half/b.txt": sha("b"),
	}
	if current.Hash() != wantCurrent.Hash() {
		t.Errorf("manifest.json = %v, want %v", current, wantCurrent)
	}

	target, err := manifest.Load(filepath.Join(wsDir, "target.json"))
	if err != nil {
		t.Fatal(err)
	}
	if target.Hash() != want.Hash() {
		t.Errorf("target.json = %v, want %v", target, want)
	}
}

func TestSync_NoChanges(t *testing.T) {
	s, wsDir, id := newSyncedWorkspace(t)
	current, err := manifest.Load(filepath.Join(wsDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}

	missing, err := s.Sync(t.Context(), id, current)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		t.Errorf("missing = %v, want none", missing)
	}
}

func TestSync_Rejects(t *testing.T) {
	good := sha("x")
	tests := []struct {
		name    string
		want    manifest.Manifest
		wantErr error
	}{
		{"parent escape", manifest.Manifest{"../x": good}, ErrInvalidInput},
		{"absolute", manifest.Manifest{"/etc/passwd": good}, ErrInvalidInput},
		{"non-canonical", manifest.Manifest{"./a": good}, ErrInvalidInput},
		{"trailing slash", manifest.Manifest{"a/": good}, ErrInvalidInput},
		{"empty path", manifest.Manifest{"": good}, ErrInvalidInput},
		{"bad sha", manifest.Manifest{"a": "nothex"}, ErrInvalidInput},
		{"uppercase sha", manifest.Manifest{"a": strings.ToUpper(good)}, ErrInvalidInput},
		{"file and dir", manifest.Manifest{"a": good, "a/b": good}, ErrInvalidInput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, wsDir, id := newSyncedWorkspace(t)
			before, _ := os.ReadFile(filepath.Join(wsDir, "manifest.json"))

			_, err := s.Sync(t.Context(), id, tt.want)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}

			after, _ := os.ReadFile(filepath.Join(wsDir, "manifest.json"))
			if string(before) != string(after) {
				t.Error("rejected sync modified the workspace")
			}
		})
	}
}

func TestSync_TooManyFiles(t *testing.T) {
	s, _, id := newSyncedWorkspace(t)
	s.limits.MaxFiles = 1

	_, err := s.Sync(t.Context(), id, manifest.Manifest{"a": sha("a"), "b": sha("b")})
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestSync_UnknownWorkspace(t *testing.T) {
	s, err := NewWorkspaceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.Sync(t.Context(), uuid.NewV7(), manifest.Manifest{})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
