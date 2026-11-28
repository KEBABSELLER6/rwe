package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/KEBABSELLER6/rwe/internal/service"
)

const (
	testImage = "busybox:1.37"
	testOwner = "rwe-test"
)

func requireDocker(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("docker tests skipped in -short mode")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skipf("docker not available: %v", err)
	}
}

func spec(filesDir string, cmd ...string) service.RunSpec {
	return service.RunSpec{
		Name:     "rwe-test-" + uuid.NewV7().String(),
		Image:    testImage,
		Command:  cmd,
		FilesDir: filesDir,
		Timeout:  time.Minute,
	}
}

func TestDocker_RunsInWorkspace(t *testing.T) {
	requireDocker(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	code, err := NewDocker(testOwner).Run(t.Context(), spec(dir, "sh", "-c", "cat in.txt; echo oops >&2; echo built > out.txt"), &logs)
	if err != nil {
		t.Fatalf("Run: %v\nlogs:\n%s", err, logs.String())
	}
	if code != 0 {
		t.Fatalf("exit code = %d\nlogs:\n%s", code, logs.String())
	}
	if !strings.Contains(logs.String(), "hello") || !strings.Contains(logs.String(), "oops") {
		t.Errorf("logs missing stdout or stderr:\n%s", logs.String())
	}

	out := filepath.Join(dir, "out.txt")
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("container output not visible on host: %v", err)
	}
	if err := os.Remove(out); err != nil {
		t.Errorf("server user cannot delete container output: %v", err)
	}
}

func TestDocker_ExitCode(t *testing.T) {
	requireDocker(t)

	code, err := NewDocker(testOwner).Run(t.Context(), spec(t.TempDir(), "sh", "-c", "exit 3"), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
}

func TestDocker_TimeoutRemovesContainer(t *testing.T) {
	requireDocker(t)
	s := spec(t.TempDir(), "sleep", "300")
	s.Timeout = 3 * time.Second

	start := time.Now()
	_, err := NewDocker(testOwner).Run(t.Context(), s, &bytes.Buffer{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if took := time.Since(start); took > 30*time.Second {
		t.Errorf("timeout took %v", took)
	}

	out, _ := exec.Command("docker", "ps", "--all", "--quiet", "--filter", "name="+s.Name).Output()
	if len(bytes.TrimSpace(out)) != 0 {
		t.Errorf("container %s still exists", s.Name)
	}
}

func TestDocker_BadImageIsError(t *testing.T) {
	requireDocker(t)
	s := spec(t.TempDir(), "true")
	s.Image = "rwe-does-not-exist.invalid/nope:1"

	var logs bytes.Buffer
	_, err := NewDocker(testOwner).Run(t.Context(), s, &logs)
	if err == nil {
		t.Fatalf("expected error for missing image, logs:\n%s", logs.String())
	}
}

func TestDocker_CacheMountArgs(t *testing.T) {
	s := spec("files", "true")
	s.Caches = []service.CacheMount{{HostDir: "/data/caches/shared/abc", Path: "/tmp/.cache"}}
	args, err := runArgs(s, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(args, " "), "--volume /data/caches/shared/abc:/tmp/.cache") {
		t.Errorf("args = %v, want the cache volume", args)
	}
}

func TestDocker_CachesAreWritableAndKept(t *testing.T) {
	requireDocker(t)
	files, shared, inWorkspace := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(files, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := spec(files, "sh", "-c", "echo a > /tmp/.cache/x && echo b > node_modules/y")
	s.Caches = []service.CacheMount{
		{HostDir: shared, Path: "/tmp/.cache"},
		{HostDir: inWorkspace, Path: service.WorkspaceMount + "/node_modules"},
	}

	var logs bytes.Buffer
	if code, err := NewDocker(testOwner).Run(t.Context(), s, &logs); err != nil || code != 0 {
		t.Fatalf("Run = %d, %v\nlogs:\n%s", code, err, logs.String())
	}
	for _, p := range []string{filepath.Join(shared, "x"), filepath.Join(inWorkspace, "y")} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("cache file not kept on host: %v", err)
		}
	}
}

func TestDocker_CommandExitCodesDockerAlsoUses(t *testing.T) {
	requireDocker(t)
	for _, want := range []int{125, 126, 127} {
		s := spec(t.TempDir(), "sh", "-c", fmt.Sprintf("exit %d", want))

		var logs bytes.Buffer
		code, err := NewDocker(testOwner).Run(t.Context(), s, &logs)
		if err != nil || code != want {
			t.Errorf("Run = %d, %v; want %d, nil\nlogs:\n%s", code, err, want, logs.String())
		}
		if exec.Command("docker", "inspect", s.Name).Run() == nil {
			t.Errorf("container %s still exists", s.Name)
		}
	}
}

func TestDocker_MissingCommandIsExit127(t *testing.T) {
	requireDocker(t)
	s := spec(t.TempDir(), "/does-not-exist")

	var logs bytes.Buffer
	code, err := NewDocker(testOwner).Run(t.Context(), s, &logs)
	if err != nil || code != 127 {
		t.Fatalf("Run = %d, %v; want 127, nil like a shell\nlogs:\n%s", code, err, logs.String())
	}
}

func TestDocker_FailedStartIsError(t *testing.T) {
	requireDocker(t)
	s := spec(t.TempDir(), "true")
	s.Caches = []service.CacheMount{{HostDir: filepath.Join(t.TempDir(), "file"), Path: "/proc/nope"}}
	if err := os.WriteFile(s.Caches[0].HostDir, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	code, err := NewDocker(testOwner).Run(t.Context(), s, &logs)
	if err == nil {
		t.Fatalf("Run = %d, nil; want an error for a container that cannot start\nlogs:\n%s", code, logs.String())
	}
	if exec.Command("docker", "inspect", s.Name).Run() == nil {
		t.Errorf("container %s still exists", s.Name)
	}
}

func TestDocker_RemoveStaleRemovesOnlyOwnContainers(t *testing.T) {
	requireDocker(t)
	owner, other := "rwe-test-"+uuid.NewV7().String(), "rwe-test-"+uuid.NewV7().String()
	start := func(owner string) string {
		t.Helper()
		out, err := exec.Command("docker", "run", "--detach", "--label", ownerLabel+"="+owner, testImage, "sleep", "60").Output()
		if err != nil {
			t.Fatalf("start container: %v", err)
		}
		id := strings.TrimSpace(string(out))
		t.Cleanup(func() { exec.Command("docker", "rm", "--force", id).Run() })
		return id
	}
	mine, theirs := start(owner), start(other)

	removed, err := NewDocker(owner).RemoveStale(t.Context())
	if err != nil || removed != 1 {
		t.Fatalf("RemoveStale = %d, %v; want 1, nil", removed, err)
	}
	if exec.Command("docker", "inspect", mine).Run() == nil {
		t.Errorf("own container %s still exists", mine)
	}
	if err := exec.Command("docker", "inspect", theirs).Run(); err != nil {
		t.Errorf("other owner's container removed: %v", err)
	}
}
