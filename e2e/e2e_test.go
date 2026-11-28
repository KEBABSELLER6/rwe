package e2e

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const image = "busybox:1.37"

var (
	buildOnce sync.Once
	binDir    string
	buildErr  error
)

func requireDocker(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("e2e tests skipped in -short mode")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skipf("docker not available: %v", err)
	}
}

func binaries(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		binDir, buildErr = os.MkdirTemp("", "rwe-e2e-")
		steps := [][]string{
			{"go", "build", "-o", filepath.Join(binDir, "server"), "../cmd/server"},
			{"go", "build", "-o", filepath.Join(binDir, "rwe"), "../cmd/rwe"},
			{"docker", "pull", "--quiet", image},
		}
		for _, step := range steps {
			if buildErr != nil {
				return
			}
			if out, err := exec.Command(step[0], step[1:]...).CombinedOutput(); err != nil {
				buildErr = fmt.Errorf("%s: %w\n%s", strings.Join(step, " "), err, out)
			}
		}
	})
	if buildErr != nil {
		t.Fatalf("prepare e2e: %v", buildErr)
	}
	return binDir
}

func TestMain(m *testing.M) {
	code := m.Run()
	if binDir != "" {
		os.RemoveAll(binDir)
	}
	os.Exit(code)
}

var startedAddr = regexp.MustCompile(`server started .*addr=(\S+)`)

func startServer(t *testing.T) string {
	t.Helper()
	cmd := exec.Command(filepath.Join(binaries(t), "server"), "-addr", "127.0.0.1:0", "-data", t.TempDir())
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	drained := make(chan struct{})
	addr := make(chan string, 1)
	go func() {
		defer close(drained)
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			logs.WriteString(sc.Text())
			logs.WriteByte('\n')
			if m := startedAddr.FindStringSubmatch(sc.Text()); m != nil {
				addr <- m[1]
			}
		}
	}()
	t.Cleanup(func() {
		cmd.Process.Signal(syscall.SIGINT)
		cmd.Wait()
		<-drained
		if t.Failed() {
			t.Logf("server log:\n%s", logs.String())
		}
	})

	select {
	case a := <-addr:
		return "http://" + a
	case <-time.After(10 * time.Second):
		t.Fatal("server did not start within 10s")
		return ""
	}
}

type project struct {
	t      *testing.T
	dir    string
	server string
}

func newProject(t *testing.T, config string, files map[string]string) project {
	t.Helper()
	requireDocker(t)
	p := project{t: t, dir: t.TempDir(), server: startServer(t)}
	files[".rwe.json"] = config
	for name, content := range files {
		path := filepath.Join(p.dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func (p project) run(args ...string) (stdout, stderr string, code int) {
	p.t.Helper()
	home := p.t.TempDir()
	cmd := exec.Command(filepath.Join(binaries(p.t), "rwe"), append([]string{"run"}, args...)...)
	cmd.Dir = p.dir
	cmd.Env = append(os.Environ(), "RWE_URL="+p.server, "HOME="+home, "XDG_CONFIG_HOME="+home)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		p.t.Fatal(err)
	}
	return out.String(), errOut.String(), cmd.ProcessState.ExitCode()
}

func TestRunStreamsOutputAndPassesExitCode(t *testing.T) {
	p := newProject(t, `{"image":"`+image+`"}`, map[string]string{"hello.txt": "hello from the laptop\n"})

	stdout, stderr, code := p.run("--", "sh", "-c", "cat hello.txt; exit 3")
	if code != 3 {
		t.Errorf("exit code = %d, want 3\nstderr: %s", code, stderr)
	}
	if stdout != "hello from the laptop\n" {
		t.Errorf("stdout = %q, want the file content", stdout)
	}
	if !strings.Contains(stderr, "failed (exit 3)") {
		t.Errorf("stderr = %q, want a failed (exit 3) summary", stderr)
	}
}

func TestJobCannotChangeWorkspace(t *testing.T) {
	p := newProject(t, `{"image":"`+image+`"}`, map[string]string{"a.txt": "original\n", "b.txt": "kept\n"})

	if _, stderr, code := p.run("--", "sh", "-c", "echo changed > a.txt; rm b.txt; touch new.txt"); code != 0 {
		t.Fatalf("first job exit code = %d\nstderr: %s", code, stderr)
	}
	stdout, stderr, code := p.run("--", "sh", "-c", "cat a.txt b.txt; ls")
	if code != 0 {
		t.Fatalf("second job exit code = %d\nstderr: %s", code, stderr)
	}
	if !strings.HasPrefix(stdout, "original\nkept\n") || strings.Contains(stdout, "new.txt") {
		t.Errorf("second job saw a changed workspace:\n%s", stdout)
	}
}

func TestArtifactsComeBack(t *testing.T) {
	p := newProject(t, `{"image":"`+image+`"}`, map[string]string{"a.txt": "a\n"})

	if _, stderr, code := p.run("-a", "out", "--", "sh", "-c", "mkdir out; echo built > out/result.txt"); code != 0 {
		t.Fatalf("exit code = %d\nstderr: %s", code, stderr)
	}
	matches, err := filepath.Glob(filepath.Join(p.dir, ".rwe", "artifacts", "*", "out", "result.txt"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("artifact files = %v, %v; want one result.txt", matches, err)
	}
	if raw, err := os.ReadFile(matches[0]); err != nil || string(raw) != "built\n" {
		t.Errorf("result.txt = %q, %v; want built", raw, err)
	}
}

func TestCacheSurvivesBetweenJobs(t *testing.T) {
	p := newProject(t, `{"image":"`+image+`","caches":["/tmp/cache"]}`, map[string]string{"a.txt": "a\n"})

	if _, stderr, code := p.run("--", "sh", "-c", "echo warm > /tmp/cache/state"); code != 0 {
		t.Fatalf("first job exit code = %d\nstderr: %s", code, stderr)
	}
	stdout, stderr, code := p.run("--", "cat", "/tmp/cache/state")
	if code != 0 || stdout != "warm\n" {
		t.Errorf("second job = %q (exit %d), want warm\nstderr: %s", stdout, code, stderr)
	}
}
