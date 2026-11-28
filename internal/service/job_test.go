package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/KEBABSELLER6/rwe/internal/manifest"
)

type fakeRunner struct {
	run func(ctx context.Context, spec RunSpec, logs io.Writer) (int, error)

	mu    sync.Mutex
	specs []RunSpec
}

func (f *fakeRunner) Run(ctx context.Context, spec RunSpec, logs io.Writer) (int, error) {
	f.mu.Lock()
	f.specs = append(f.specs, spec)
	f.mu.Unlock()
	return f.run(ctx, spec, logs)
}

type jobEnv struct {
	ws     *WorkspaceService
	jobs   *JobService
	runner *fakeRunner
	wsID   uuid.UUID
	wsDir  string
	hash   string
}

func newJobEnv(t *testing.T, run func(context.Context, RunSpec, io.Writer) (int, error)) *jobEnv {
	t.Helper()
	ws, wsDir, wsID := newSyncedWorkspace(t)
	current, err := manifest.Load(filepath.Join(wsDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}

	runner := &fakeRunner{run: run}
	jobs, err := NewJobService(t.TempDir(), t.TempDir(), ws, runner, DefaultJobLimits)
	if err != nil {
		t.Fatal(err)
	}
	stopped := make(chan struct{})
	go func() {
		jobs.Start(t.Context())
		close(stopped)
	}()
	t.Cleanup(func() { <-stopped })

	return &jobEnv{ws: ws, jobs: jobs, runner: runner, wsID: wsID, wsDir: wsDir, hash: current.Hash()}
}

func (e *jobEnv) request() JobRequest {
	return JobRequest{WorkspaceID: e.wsID, ExpectedHash: e.hash, Image: "golang:1.27", Command: []string{"make", "test"}}
}

func waitFinished(t *testing.T, s *JobService, id uuid.UUID) Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, err := s.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status.Finished() {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s did not finish", id)
	return Job{}
}

func exitWith(code int, err error, output string) func(context.Context, RunSpec, io.Writer) (int, error) {
	return func(_ context.Context, _ RunSpec, logs io.Writer) (int, error) {
		io.WriteString(logs, output)
		return code, err
	}
}

func TestJob_Statuses(t *testing.T) {
	tests := []struct {
		name       string
		code       int
		err        error
		wantStatus JobStatus
		wantExit   *int
	}{
		{"passed", 0, nil, JobPassed, new(0)},
		{"failed", 2, nil, JobFailed, new(2)},
		{"timeout", -1, fmt.Errorf("stopped: %w", context.DeadlineExceeded), JobTimeout, nil},
		{"error", -1, errors.New("docker failed"), JobError, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newJobEnv(t, exitWith(tt.code, tt.err, "output\n"))

			submitted, err := env.jobs.Submit(env.request())
			if err != nil {
				t.Fatal(err)
			}
			if submitted.Status != JobQueued {
				t.Errorf("submitted status = %s, want queued", submitted.Status)
			}

			job := waitFinished(t, env.jobs, submitted.ID)
			if job.Status != tt.wantStatus {
				t.Errorf("status = %s, want %s (error: %s)", job.Status, tt.wantStatus, job.Error)
			}
			if (job.ExitCode == nil) != (tt.wantExit == nil) || (job.ExitCode != nil && *job.ExitCode != *tt.wantExit) {
				t.Errorf("exit code = %v, want %v", job.ExitCode, tt.wantExit)
			}
			if job.StartedAt.IsZero() || job.FinishedAt.IsZero() {
				t.Error("start/finish time not set")
			}

			if _, err := env.ws.Sync(t.Context(), env.wsID, manifest.Manifest{}, nil, nil); err != nil {
				t.Errorf("workspace not released after job: %v", err)
			}
		})
	}
}

func TestJob_RunSpec(t *testing.T) {
	env := newJobEnv(t, exitWith(0, nil, ""))
	req := env.request()
	req.Memory = 1 << 30

	job, err := env.jobs.Submit(req)
	if err != nil {
		t.Fatal(err)
	}
	waitFinished(t, env.jobs, job.ID)

	spec := env.runner.specs[0]
	if spec.Name != "rwe-"+job.ID.String() || spec.Image != "golang:1.27" || !slices.Equal(spec.Command, []string{"make", "test"}) {
		t.Errorf("spec = %+v", spec)
	}
	if strings.HasPrefix(spec.FilesDir, env.wsDir) {
		t.Errorf("FilesDir = %s, want a per-job copy, not the workspace files dir", spec.FilesDir)
	}
	if spec.Timeout != DefaultJobLimits.DefaultTimeout || spec.Memory != 1<<30 {
		t.Errorf("timeout/memory = %s/%d", spec.Timeout, spec.Memory)
	}
}

func TestJob_WorkspaceBusyWhileQueuedOrRunning(t *testing.T) {
	release := make(chan struct{})
	env := newJobEnv(t, func(ctx context.Context, _ RunSpec, _ io.Writer) (int, error) {
		<-release
		return 0, nil
	})

	job, err := env.jobs.Submit(env.request())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := env.ws.Sync(t.Context(), env.wsID, manifest.Manifest{}, nil, nil); !errors.Is(err, ErrConflict) {
		t.Errorf("sync during job: err = %v, want ErrConflict", err)
	}
	if _, err := env.jobs.Submit(env.request()); !errors.Is(err, ErrConflict) {
		t.Errorf("second job on busy workspace: err = %v, want ErrConflict", err)
	}

	close(release)
	waitFinished(t, env.jobs, job.ID)

	if _, err := env.jobs.Submit(env.request()); err != nil {
		t.Errorf("submit after job finished: %v", err)
	}
}

func TestJob_RunsOnCopyOfWorkspace(t *testing.T) {
	var jobFiles string
	env := newJobEnv(t, func(_ context.Context, spec RunSpec, _ io.Writer) (int, error) {
		jobFiles = spec.FilesDir
		got, err := os.ReadFile(filepath.Join(spec.FilesDir, "keep.txt"))
		if err != nil || string(got) != "keep" {
			return -1, fmt.Errorf("job copy keep.txt = %q, %v", got, err)
		}
		if err := os.WriteFile(filepath.Join(spec.FilesDir, "change.txt"), []byte("rewritten"), 0o644); err != nil {
			return -1, err
		}
		return 0, os.WriteFile(filepath.Join(spec.FilesDir, "out.bin"), []byte("artifact"), 0o644)
	})

	job, err := env.jobs.Submit(env.request())
	if err != nil {
		t.Fatal(err)
	}
	if done := waitFinished(t, env.jobs, job.ID); done.Status != JobPassed {
		t.Fatalf("status = %s, error: %s", done.Status, done.Error)
	}

	files := filepath.Join(env.wsDir, "files")
	if got, _ := os.ReadFile(filepath.Join(files, "change.txt")); string(got) != "old" {
		t.Errorf("workspace change.txt = %q, want it untouched by the job", got)
	}
	if _, err := os.Stat(filepath.Join(files, "out.bin")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("job output leaked into the workspace: %v", err)
	}
	if _, err := os.Stat(jobFiles); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("job copy %s not removed: %v", jobFiles, err)
	}
	next, err := env.jobs.Submit(env.request())
	if err != nil {
		t.Fatalf("expected_hash no longer matches after job: %v", err)
	}
	waitFinished(t, env.jobs, next.ID)
}

func TestJob_CopyKeepsModTimes(t *testing.T) {
	old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	env := newJobEnv(t, func(_ context.Context, spec RunSpec, _ io.Writer) (int, error) {
		info, err := os.Stat(filepath.Join(spec.FilesDir, "keep.txt"))
		if err != nil {
			return -1, err
		}
		if !info.ModTime().Equal(old) {
			return -1, fmt.Errorf("job copy keep.txt mtime = %s, want %s", info.ModTime(), old)
		}
		return 0, nil
	})
	if err := os.Chtimes(filepath.Join(env.wsDir, "files", "keep.txt"), time.Time{}, old); err != nil {
		t.Fatal(err)
	}

	job, err := env.jobs.Submit(env.request())
	if err != nil {
		t.Fatal(err)
	}
	if done := waitFinished(t, env.jobs, job.ID); done.Status != JobPassed {
		t.Fatalf("status = %s, error: %s", done.Status, done.Error)
	}
}

func TestJob_RemovesReadOnlyCopy(t *testing.T) {
	var jobFiles string
	env := newJobEnv(t, func(_ context.Context, spec RunSpec, _ io.Writer) (int, error) {
		jobFiles = spec.FilesDir
		locked := filepath.Join(spec.FilesDir, "locked", "inner")
		if err := os.MkdirAll(locked, 0o755); err != nil {
			return -1, err
		}
		if err := os.WriteFile(filepath.Join(locked, "f"), nil, 0o644); err != nil {
			return -1, err
		}
		for _, dir := range []string{locked, filepath.Dir(locked)} {
			if err := os.Chmod(dir, 0o500); err != nil {
				return -1, err
			}
		}
		return 0, os.Chmod(spec.FilesDir, 0o000)
	})

	job, err := env.jobs.Submit(env.request())
	if err != nil {
		t.Fatal(err)
	}
	waitFinished(t, env.jobs, job.ID)

	if _, err := os.Lstat(jobFiles); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("read-only job copy %s not removed: %v", jobFiles, err)
	}
}

func TestJob_SubmitRejects(t *testing.T) {
	env := newJobEnv(t, exitWith(0, nil, ""))

	tests := []struct {
		name    string
		modify  func(*JobRequest)
		wantErr error
	}{
		{"hash mismatch", func(r *JobRequest) { r.ExpectedHash = strings.Repeat("0", 64) }, ErrConflict},
		{"invalid hash", func(r *JobRequest) { r.ExpectedHash = "nope" }, ErrInvalidInput},
		{"no image", func(r *JobRequest) { r.Image = "" }, ErrInvalidInput},
		{"no command", func(r *JobRequest) { r.Command = nil }, ErrInvalidInput},
		{"timeout too long", func(r *JobRequest) { r.Timeout = 2 * time.Hour }, ErrInvalidInput},
		{"negative memory", func(r *JobRequest) { r.Memory = -1 }, ErrInvalidInput},
		{"unknown workspace", func(r *JobRequest) { r.WorkspaceID = uuid.NewV7() }, ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := env.request()
			tt.modify(&req)
			if _, err := env.jobs.Submit(req); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if _, err := env.ws.Sync(t.Context(), env.wsID, manifest.Manifest{"keep.txt": sha("keep")}, nil, nil); err != nil {
				t.Errorf("rejected submit left workspace locked: %v", err)
			}
			env.hash = manifest.Manifest{"keep.txt": sha("keep")}.Hash()
		})
	}
}

func TestJob_QueueFull(t *testing.T) {
	jobs, err := NewJobService(t.TempDir(), t.TempDir(), nil, nil, DefaultJobLimits)
	if err != nil {
		t.Fatal(err)
	}

	var envs []*jobEnv
	for range DefaultJobLimits.QueueSize + 1 {
		env := newJobEnv(t, nil)
		envs = append(envs, env)
	}
	locker := &multiLocker{byID: map[uuid.UUID]*WorkspaceService{}}
	for _, env := range envs {
		locker.byID[env.wsID] = env.ws
	}
	jobs.workspaces = locker

	for i, env := range envs {
		_, err := jobs.Submit(env.request())
		if i < DefaultJobLimits.QueueSize && err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
		if i == DefaultJobLimits.QueueSize {
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("submit beyond queue size: err = %v, want ErrConflict", err)
			}
			if _, err := env.ws.Sync(t.Context(), env.wsID, manifest.Manifest{}, nil, nil); err != nil {
				t.Errorf("rejected job left workspace locked: %v", err)
			}
		}
	}
}

type multiLocker struct {
	byID map[uuid.UUID]*WorkspaceService
}

func (m *multiLocker) Acquire(id uuid.UUID, hash string) (string, error) {
	return m.byID[id].Acquire(id, hash)
}

func (m *multiLocker) Release(id uuid.UUID) { m.byID[id].Release(id) }

func (m *multiLocker) Exists(id uuid.UUID) bool { return m.byID[id] != nil }

func TestJob_FollowLogs(t *testing.T) {
	step := make(chan struct{})
	env := newJobEnv(t, func(_ context.Context, _ RunSpec, logs io.Writer) (int, error) {
		io.WriteString(logs, "line 1\nline ")
		<-step
		io.WriteString(logs, "2\nno newline at end")
		return 0, nil
	})

	job, err := env.jobs.Submit(env.request())
	if err != nil {
		t.Fatal(err)
	}

	lines := make(chan string, 10)
	done := make(chan error, 1)
	go func() {
		done <- env.jobs.FollowLogs(t.Context(), job.ID, func(line []byte) error {
			lines <- string(line)
			return nil
		})
	}()

	if got := <-lines; got != "line 1" {
		t.Fatalf("first line = %q", got)
	}
	close(step)

	if err := <-done; err != nil {
		t.Fatal(err)
	}
	close(lines)
	var rest []string
	for l := range lines {
		rest = append(rest, l)
	}
	if want := []string{"line 2", "no newline at end"}; !slices.Equal(rest, want) {
		t.Errorf("remaining lines = %q, want %q", rest, want)
	}
}

func TestJob_FollowLogsOfFinishedJob(t *testing.T) {
	env := newJobEnv(t, exitWith(0, nil, "a\nb\n"))
	job, err := env.jobs.Submit(env.request())
	if err != nil {
		t.Fatal(err)
	}
	waitFinished(t, env.jobs, job.ID)

	var got []string
	err = env.jobs.FollowLogs(t.Context(), job.ID, func(line []byte) error {
		got = append(got, string(line))
		return nil
	})
	if err != nil || !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("lines = %q, err = %v", got, err)
	}

	if err := env.jobs.FollowLogs(t.Context(), uuid.NewV7(), nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown job: err = %v, want ErrNotFound", err)
	}
}

func TestJob_MaxMemory(t *testing.T) {
	env := newJobEnv(t, exitWith(0, nil, ""))
	env.jobs.limits.MaxMemory = 2 << 30

	job, err := env.jobs.Submit(env.request())
	if err != nil {
		t.Fatal(err)
	}
	waitFinished(t, env.jobs, job.ID)
	if got := env.runner.specs[0].Memory; got != 2<<30 {
		t.Errorf("memory = %d, want the server cap as default", got)
	}

	req := env.request()
	req.Memory = 3 << 30
	if _, err := env.jobs.Submit(req); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("over cap: err = %v, want ErrInvalidInput", err)
	}
}

func TestJob_CancelRunning(t *testing.T) {
	started := make(chan struct{})
	env := newJobEnv(t, func(ctx context.Context, _ RunSpec, _ io.Writer) (int, error) {
		close(started)
		<-ctx.Done()
		return -1, fmt.Errorf("stopped: %w", ctx.Err())
	})

	job, err := env.jobs.Submit(env.request())
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if _, err := env.jobs.Cancel(job.ID); err != nil {
		t.Fatal(err)
	}

	if got := waitFinished(t, env.jobs, job.ID); got.Status != JobCancelled {
		t.Errorf("status = %s, want cancelled", got.Status)
	}
	if _, err := env.ws.Sync(t.Context(), env.wsID, manifest.Manifest{}, nil, nil); err != nil {
		t.Errorf("workspace not released after cancel: %v", err)
	}
}

func TestJob_CancelQueued(t *testing.T) {
	env := newJobEnv(t, exitWith(0, nil, ""))
	jobs, err := NewJobService(t.TempDir(), t.TempDir(), env.ws, env.runner, DefaultJobLimits)
	if err != nil {
		t.Fatal(err)
	}

	job, err := jobs.Submit(env.request())
	if err != nil {
		t.Fatal(err)
	}
	if got, err := jobs.Cancel(job.ID); err != nil || got.Status != JobCancelled {
		t.Fatalf("cancel = %s, %v; want cancelled", got.Status, err)
	}
	if _, err := env.ws.Sync(t.Context(), env.wsID, manifest.Manifest{}, nil, nil); err != nil {
		t.Errorf("workspace not released after cancel: %v", err)
	}

	jobs.run(t.Context(), job.ID)
	if len(env.runner.specs) != 0 {
		t.Errorf("cancelled job was run")
	}
}

func TestJob_CancelUnknownOrFinished(t *testing.T) {
	env := newJobEnv(t, exitWith(0, nil, ""))
	if _, err := env.jobs.Cancel(uuid.NewV7()); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown job: err = %v, want ErrNotFound", err)
	}

	job, err := env.jobs.Submit(env.request())
	if err != nil {
		t.Fatal(err)
	}
	waitFinished(t, env.jobs, job.ID)
	if got, err := env.jobs.Cancel(job.ID); err != nil || got.Status != JobPassed {
		t.Errorf("cancel finished job = %s, %v; want passed, unchanged", got.Status, err)
	}
}
