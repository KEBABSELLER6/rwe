package service

import (
	"errors"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"uuid"

	"github.com/KEBABSELLER6/rwe/internal/manifest"
)

func newSyncedWorkspace(t *testing.T) (*WorkspaceService, string, uuid.UUID) {
	t.Helper()
	dir := t.TempDir()
	s, err := NewWorkspaceService(dir, DefaultWorkspaceLimits.Retention)
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

	want, sizes := manifestOf(map[string]string{
		"keep.txt":    "keep",
		"change.txt":  "new",
		"half/b.txt":  "b",
		"new/file.go": "package new",
	})

	missing, err := s.Sync(t.Context(), id, want, sizes, nil)
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

	target, err := s.loadTarget(id)
	if err != nil {
		t.Fatal(err)
	}
	if target.Files.Hash() != want.Hash() || !maps.Equal(target.Sizes, sizes) {
		t.Errorf("target.json = %+v, want files %v, sizes %v", target, want, sizes)
	}
}

func TestSync_NoChanges(t *testing.T) {
	s, wsDir, id := newSyncedWorkspace(t)
	current, err := manifest.Load(filepath.Join(wsDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}

	missing, err := s.Sync(t.Context(), id, current, nil, nil)
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

			_, err := s.Sync(t.Context(), id, tt.want, nil, nil)
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

func TestSync_ChangesModeWithoutUpload(t *testing.T) {
	s, wsDir, id := newSyncedWorkspace(t)
	current, err := manifest.Load(filepath.Join(wsDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(wsDir, "files", "keep.txt")

	for _, step := range []struct {
		executable []string
		wantPerm   os.FileMode
	}{
		{[]string{"keep.txt"}, 0o755},
		{nil, 0o644},
	} {
		missing, err := s.Sync(t.Context(), id, current, nil, step.executable)
		if err != nil {
			t.Fatal(err)
		}
		if len(missing) != 0 {
			t.Errorf("missing = %v, want none for a mode-only change", missing)
		}
		assertPerm(t, keep, step.wantPerm)
	}
}

func TestSync_RejectsUnknownExecutable(t *testing.T) {
	s, _, id := newSyncedWorkspace(t)

	_, err := s.Sync(t.Context(), id, manifest.Manifest{}, nil, []string{"keep.txt"})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestSync_TooManyFiles(t *testing.T) {
	s, _, id := newSyncedWorkspace(t)
	s.limits.MaxFiles = 1

	_, err := s.Sync(t.Context(), id, manifest.Manifest{"a": sha("a"), "b": sha("b")}, nil, nil)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestSync_TotalSize(t *testing.T) {
	big := map[string]int64{"new.bin": 90}
	tests := []struct {
		name    string
		want    manifest.Manifest
		sizes   map[string]int64
		wantErr error
	}{
		{"fits with kept files", manifest.Manifest{"keep.txt": sha("keep"), "new.bin": sha("x")}, big, nil},
		{"kept files count", manifest.Manifest{"keep.txt": sha("keep"), "half/b.txt": sha("b"), "new.bin": sha("x")}, map[string]int64{"new.bin": 96}, ErrTooLarge},
		{"one file too big", manifest.Manifest{"new.bin": sha("x")}, map[string]int64{"new.bin": 101}, ErrTooLarge},
		{"same blob at many paths", manifest.Manifest{"a": sha("x"), "b": sha("x")}, map[string]int64{"a": 60, "b": 60}, ErrTooLarge},
		{"no overflow", manifest.Manifest{"a": sha("x"), "b": sha("y")}, map[string]int64{"a": 50, "b": math.MaxInt64}, ErrTooLarge},
		{"size missing", manifest.Manifest{"new.bin": sha("x")}, nil, ErrInvalidInput},
		{"negative size", manifest.Manifest{"new.bin": sha("x")}, map[string]int64{"new.bin": -1}, ErrInvalidInput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, wsDir, id := newSyncedWorkspace(t)
			s.limits.MaxTotalBytes = 100
			before, _ := os.ReadFile(filepath.Join(wsDir, "manifest.json"))

			_, err := s.Sync(t.Context(), id, tt.want, tt.sizes, nil)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}

			after, _ := os.ReadFile(filepath.Join(wsDir, "manifest.json"))
			if tt.wantErr != nil && string(before) != string(after) {
				t.Error("rejected sync modified the workspace")
			}
		})
	}
}

func TestSync_UnknownWorkspace(t *testing.T) {
	s, err := NewWorkspaceService(t.TempDir(), DefaultWorkspaceLimits.Retention)
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.Sync(t.Context(), uuid.NewV7(), manifest.Manifest{}, nil, nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
