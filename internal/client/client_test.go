package client

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kebabseller6/rwe/internal/handler"
	"kebabseller6/rwe/internal/service"
)

type lsRunner struct{}

func (lsRunner) Run(ctx context.Context, spec service.RunSpec, logs io.Writer) (int, error) {
	entries, err := os.ReadDir(spec.FilesDir)
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		fmt.Fprintln(logs, e.Name())
	}
	fmt.Fprint(logs, "no trailing newline")
	return 3, nil
}

func newTestClient(t *testing.T) *Client {
	t.Helper()
	data := t.TempDir()
	ws, err := service.NewWorkspaceService(filepath.Join(data, "workspaces"))
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := service.NewJobService(filepath.Join(data, "jobs"), ws, lsRunner{}, service.DefaultJobLimits)
	if err != nil {
		t.Fatal(err)
	}
	go jobs.Start(t.Context())

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(handler.NewRouter(ws, jobs, logger))
	t.Cleanup(srv.Close)
	return New(srv.URL)
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunSyncsAndStreamsLogs(t *testing.T) {
	c := newTestClient(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.txt": "a", "sub/b.txt": "b"})

	var lines []string
	job, err := c.Run(t.Context(), dir, JobSpec{Image: "img", Command: []string{"ls"}}, func(l string) { lines = append(lines, l) })
	if err != nil {
		t.Fatal(err)
	}

	if job.Status != JobFailed || job.ExitCode == nil || *job.ExitCode != 3 {
		t.Fatalf("job = %+v, want failed with exit 3", job)
	}
	want := []string{"a.txt", "sub", "no trailing newline"}
	if !slices.Equal(lines, want) {
		t.Fatalf("lines = %q, want %q", lines, want)
	}
}

func TestSyncUploadsOnlyChanges(t *testing.T) {
	c := newTestClient(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"keep.txt": "keep", "edit.txt": "v1", "gone.txt": "x"})

	first, err := c.Sync(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if first.Uploaded != 0 || first.Files != 3 {
		t.Fatalf("first sync = %+v, want 3 files sent via create, nothing uploaded", first)
	}

	writeFiles(t, dir, map[string]string{"edit.txt": "v2", "new.txt": "v2"})
	os.Remove(filepath.Join(dir, "gone.txt"))

	second, err := c.Sync(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if second.WorkspaceID != first.WorkspaceID || second.Uploaded != 1 || second.Files != 3 {
		t.Fatalf("second sync = %+v, want same workspace, one deduplicated upload, 3 files", second)
	}
}

func TestSyncFilesOver20MiB(t *testing.T) {
	c := newTestClient(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"big.bin": string(randomBytes(t, 21<<20))})

	if _, err := c.Sync(t.Context(), dir); err != nil {
		t.Fatalf("create: %v", err)
	}

	writeFiles(t, dir, map[string]string{"big.bin": string(randomBytes(t, 21<<20))})
	synced, err := c.Sync(t.Context(), dir)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if synced.Uploaded != 1 {
		t.Fatalf("sync = %+v, want the big file uploaded", synced)
	}
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func TestSyncRecreatesUnknownWorkspace(t *testing.T) {
	c := newTestClient(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.txt": "a"})
	stale := "00000000-0000-7000-8000-000000000000"
	if err := saveWorkspaceID(dir, stale); err != nil {
		t.Fatal(err)
	}

	synced, err := c.Sync(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	saved, _ := loadWorkspaceID(dir)
	if synced.WorkspaceID == stale || saved != synced.WorkspaceID {
		t.Fatalf("workspace %q (saved %q), want a fresh one", synced.WorkspaceID, saved)
	}
}

func TestListFilesSkipsStateAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.txt": "a", ".rwe/workspace": "id", "d/b.txt": "b"})
	if err := os.Symlink("a.txt", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}

	files, err := ListFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(files)
	if got := strings.Join(files, ","); got != "a.txt,d/b.txt" {
		t.Fatalf("files = %s", got)
	}
}
