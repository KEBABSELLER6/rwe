package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"uuid"

	"kebabseller6/rwe/internal/manifest"
)

func syncTo(t *testing.T, s *WorkspaceService, id uuid.UUID, want manifest.Manifest) {
	t.Helper()
	if _, err := s.Sync(t.Context(), id, want); err != nil {
		t.Fatal(err)
	}
}

func TestPutBlob(t *testing.T) {
	s, wsDir, id := newSyncedWorkspace(t)
	files := filepath.Join(wsDir, "files")

	want := manifest.Manifest{
		"keep.txt":       sha("keep"),
		"change.txt":     sha("new"),
		"copy/one.txt":   sha("same"),
		"copy/two.txt":   sha("same"),
		"deep/a/b/c.txt": sha("deep"),
	}
	syncTo(t, s, id, want)

	uploads := []struct {
		content       string
		wantRemaining int
	}{
		{"new", 3},
		{"same", 1},
		{"deep", 0},
	}
	for _, u := range uploads {
		remaining, err := s.PutBlob(t.Context(), id, sha(u.content), strings.NewReader(u.content))
		if err != nil {
			t.Fatalf("PutBlob(%q): %v", u.content, err)
		}
		if remaining != u.wantRemaining {
			t.Errorf("PutBlob(%q) remaining = %d, want %d", u.content, remaining, u.wantRemaining)
		}
	}

	for p, content := range map[string]string{
		"change.txt":     "new",
		"copy/one.txt":   "same",
		"copy/two.txt":   "same",
		"deep/a/b/c.txt": "deep",
	} {
		got, err := os.ReadFile(filepath.Join(files, p))
		if err != nil || string(got) != content {
			t.Errorf("%s = %q, %v; want %q", p, got, err, content)
		}
	}

	current, err := manifest.Load(filepath.Join(wsDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if current.Hash() != want.Hash() {
		t.Errorf("manifest.json = %v, want %v", current, want)
	}

	assertNoBlobTemps(t, wsDir)
}

func TestPutBlob_KeepsExecutableBit(t *testing.T) {
	dir := t.TempDir()
	s, err := NewWorkspaceService(dir)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := s.Create(t.Context(), makeTarGz(t, []entry{{name: "run.sh", body: "v1", mode: 0o755}}))
	if err != nil {
		t.Fatal(err)
	}

	syncTo(t, s, ws.ID, manifest.Manifest{"run.sh": sha("v2")})
	if _, err := s.PutBlob(t.Context(), ws.ID, sha("v2"), strings.NewReader("v2")); err != nil {
		t.Fatal(err)
	}

	fi, err := os.Stat(filepath.Join(dir, ws.ID.String(), "files", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Errorf("run.sh mode = %v, want 0755", fi.Mode().Perm())
	}
}

func TestPutBlob_Parallel(t *testing.T) {
	s, wsDir, id := newSyncedWorkspace(t)

	want := manifest.Manifest{}
	contents := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	for _, c := range contents {
		want["parallel/"+c] = sha(c)
	}
	syncTo(t, s, id, want)

	var wg sync.WaitGroup
	for _, c := range contents {
		wg.Go(func() {
			if _, err := s.PutBlob(t.Context(), id, sha(c), strings.NewReader(c)); err != nil {
				t.Errorf("PutBlob(%q): %v", c, err)
			}
		})
	}
	wg.Wait()

	current, err := manifest.Load(filepath.Join(wsDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if current.Hash() != want.Hash() {
		t.Errorf("lost updates: manifest.json = %v", current)
	}
}

func TestPutBlob_Rejects(t *testing.T) {
	tests := []struct {
		name    string
		sum     string
		body    string
		limit   int64
		wantErr error
	}{
		{name: "invalid sha", sum: "nothex", body: "new", wantErr: ErrInvalidInput},
		{name: "content mismatch", sum: sha("new"), body: "evil", wantErr: ErrInvalidInput},
		{name: "not requested", sum: sha("other"), body: "other", wantErr: ErrInvalidInput},
		{name: "too large", sum: sha("new"), body: "new", limit: 2, wantErr: ErrTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, wsDir, id := newSyncedWorkspace(t)
			syncTo(t, s, id, manifest.Manifest{"change.txt": sha("new")})
			if tt.limit > 0 {
				s.limits.MaxTotalBytes = tt.limit
			}

			_, err := s.PutBlob(t.Context(), id, tt.sum, strings.NewReader(tt.body))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}

			got, _ := os.ReadFile(filepath.Join(wsDir, "files", "change.txt"))
			if string(got) != "old" {
				t.Errorf("change.txt = %q, want unchanged %q", got, "old")
			}
			assertNoBlobTemps(t, wsDir)
		})
	}
}

func TestPutBlob_NoPendingSync(t *testing.T) {
	s, _, id := newSyncedWorkspace(t)

	_, err := s.PutBlob(t.Context(), id, sha("x"), strings.NewReader("x"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}

	_, err = s.PutBlob(t.Context(), uuid.NewV7(), sha("x"), strings.NewReader("x"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown workspace: err = %v, want ErrNotFound", err)
	}
}

func assertNoBlobTemps(t *testing.T, wsDir string) {
	t.Helper()
	filepath.WalkDir(wsDir, func(p string, d os.DirEntry, err error) error {
		if err == nil && (strings.HasPrefix(d.Name(), ".blob-") || strings.HasPrefix(d.Name(), ".rwe-tmp-")) {
			t.Errorf("temp file left behind: %s", p)
		}
		return nil
	})
}
