package service

import (
	"encoding/json/v2"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"
)

func TestJobStore_FinishedJobSurvivesRestart(t *testing.T) {
	env := newJobEnv(t, exitWith(3, nil, "boom\n"))
	dir := t.TempDir()
	before, err := NewJobService(dir, t.TempDir(), env.ws, env.runner, DefaultJobLimits)
	if err != nil {
		t.Fatal(err)
	}
	job, err := before.Submit(env.request())
	if err != nil {
		t.Fatal(err)
	}
	before.run(t.Context(), job.ID)
	want, _ := before.Get(job.ID)

	after, err := NewJobService(dir, t.TempDir(), nil, nil, DefaultJobLimits)
	if err != nil {
		t.Fatal(err)
	}
	got, err := after.Get(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != JobFailed || got.ExitCode == nil || *got.ExitCode != 3 {
		t.Errorf("status = %s, exit = %v; want failed, 3", got.Status, got.ExitCode)
	}
	if !got.FinishedAt.Equal(want.FinishedAt) || got.DurationMS != want.DurationMS || got.Image != want.Image {
		t.Errorf("reloaded job = %+v, want %+v", got, want)
	}
	phases := slices.Sorted(maps.Keys(got.PhasesMS))
	if want := []string{"artifacts", "cleanup", "copy", "run"}; !slices.Equal(phases, want) {
		t.Errorf("phases = %v, want %v", phases, want)
	}
}

func restartWorkspaces(t *testing.T, env *jobEnv) *WorkspaceService {
	t.Helper()
	ws, err := NewWorkspaceService(filepath.Dir(env.wsDir), DefaultWorkspaceLimits.Retention)
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func TestJobStore_QueuedJobIsRequeued(t *testing.T) {
	env := newJobEnv(t, exitWith(0, nil, "ok\n"))
	dir := t.TempDir()
	caches := t.TempDir()
	before, err := NewJobService(dir, caches, env.ws, env.runner, DefaultJobLimits)
	if err != nil {
		t.Fatal(err)
	}
	req := env.request()
	req.Timeout = 90 * time.Second
	req.Memory = 1 << 30
	req.CPUs = 1.5
	req.Caches = []string{"/tmp/.cache"}
	req.Artifacts = []string{"out.txt"}
	job, err := before.Submit(req)
	if err != nil {
		t.Fatal(err)
	}
	wantSpec, wantArtifacts := before.jobs[job.ID].spec, before.jobs[job.ID].artifactPaths

	ws := restartWorkspaces(t, env)
	after, err := NewJobService(dir, caches, ws, env.runner, DefaultJobLimits)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := after.Get(job.ID); got.Status != JobQueued {
		t.Fatalf("status after restart = %s %q, want queued", got.Status, got.Error)
	}
	if _, err := ws.Acquire(env.wsID, env.hash); !errors.Is(err, ErrConflict) {
		t.Errorf("workspace not locked for the requeued job: %v", err)
	}
	if got := after.jobs[job.ID]; !reflect.DeepEqual(got.spec, wantSpec) || !slices.Equal(got.artifactPaths, wantArtifacts) {
		t.Errorf("requeued spec = %+v %v, want %+v %v", got.spec, got.artifactPaths, wantSpec, wantArtifacts)
	}

	stopped := make(chan struct{})
	go func() {
		after.Start(t.Context())
		close(stopped)
	}()
	t.Cleanup(func() { <-stopped })
	if got := waitFinished(t, after, job.ID); got.Status != JobPassed {
		t.Errorf("requeued job = %s %q, want passed", got.Status, got.Error)
	}
	raw, err := os.ReadFile(after.statePath(job.ID))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "requeue") {
		t.Errorf("finished job state still has requeue data: %s", raw)
	}
}

func TestJobStore_RunningJobBecomesError(t *testing.T) {
	env := newJobEnv(t, exitWith(0, nil, ""))
	dir := t.TempDir()
	before, err := NewJobService(dir, t.TempDir(), env.ws, env.runner, DefaultJobLimits)
	if err != nil {
		t.Fatal(err)
	}
	job, err := before.Submit(env.request())
	if err != nil {
		t.Fatal(err)
	}
	before.jobs[job.ID].Status = JobRunning
	if err := before.save(before.jobs[job.ID]); err != nil {
		t.Fatal(err)
	}
	leftover := before.jobFilesDir(job.ID)
	if err := os.MkdirAll(leftover, 0o755); err != nil {
		t.Fatal(err)
	}

	after, err := NewJobService(dir, t.TempDir(), nil, nil, DefaultJobLimits)
	if err != nil {
		t.Fatal(err)
	}
	got, err := after.Get(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != JobError || got.Error != errServerRestarted.Error() || got.FinishedAt.IsZero() {
		t.Errorf("job = %s %q at %v; want error after restart", got.Status, got.Error, got.FinishedAt)
	}
	if _, err := os.Stat(leftover); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("leftover job files not removed: %v", err)
	}

	again, err := NewJobService(dir, t.TempDir(), nil, nil, DefaultJobLimits)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded, _ := again.Get(job.ID); !reloaded.FinishedAt.Equal(got.FinishedAt) {
		t.Errorf("error state was not saved: finished_at %v, want %v", reloaded.FinishedAt, got.FinishedAt)
	}
}

func TestJobStore_QueuedJobNotRequeued(t *testing.T) {
	tests := []struct {
		name   string
		limits JobLimits
		change func(t *testing.T, env *jobEnv, ws *WorkspaceService, statePath string)
		reason string
	}{
		{
			name:   "workspace changed",
			limits: DefaultJobLimits,
			change: func(t *testing.T, env *jobEnv, ws *WorkspaceService, _ string) {
				want, sizes := manifestOf(map[string]string{"change.txt": "old", "gone/deep/a.txt": "a", "half/b.txt": "b", "half/c.txt": "c"})
				if _, err := ws.Sync(t.Context(), env.wsID, want, sizes, nil); err != nil {
					t.Fatal(err)
				}
			},
			reason: "expected_hash",
		},
		{
			name:   "workspace deleted",
			limits: DefaultJobLimits,
			change: func(t *testing.T, env *jobEnv, _ *WorkspaceService, _ string) {
				if err := os.RemoveAll(env.wsDir); err != nil {
					t.Fatal(err)
				}
			},
			reason: "requeue:",
		},
		{
			name:   "state from an older server",
			limits: DefaultJobLimits,
			change: func(t *testing.T, _ *jobEnv, _ *WorkspaceService, statePath string) {
				raw, err := os.ReadFile(statePath)
				if err != nil {
					t.Fatal(err)
				}
				var state map[string]any
				if err := json.Unmarshal(raw, &state); err != nil {
					t.Fatal(err)
				}
				delete(state, "requeue")
				if err := writeFileJSON(statePath, state); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:   "queue full",
			limits: JobLimits{QueueSize: 0},
			change: func(*testing.T, *jobEnv, *WorkspaceService, string) {},
			reason: "queue is full",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newJobEnv(t, exitWith(0, nil, ""))
			dir := t.TempDir()
			before, err := NewJobService(dir, t.TempDir(), env.ws, env.runner, DefaultJobLimits)
			if err != nil {
				t.Fatal(err)
			}
			job, err := before.Submit(env.request())
			if err != nil {
				t.Fatal(err)
			}
			ws := restartWorkspaces(t, env)
			tt.change(t, env, ws, before.statePath(job.ID))

			after, err := NewJobService(dir, t.TempDir(), ws, env.runner, tt.limits)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := after.Get(job.ID)
			if got.Status != JobError || !strings.HasPrefix(got.Error, errServerRestarted.Error()) || !strings.Contains(got.Error, tt.reason) {
				t.Errorf("job = %s %q, want error containing %q", got.Status, got.Error, tt.reason)
			}
			if ws.Exists(env.wsID) {
				if _, err := ws.Acquire(env.wsID, env.hash); errors.Is(err, ErrConflict) && strings.Contains(err.Error(), "queued or running") {
					t.Errorf("workspace still locked after failed requeue")
				}
			}
		})
	}
}

func TestJobStore_SkipsDirsWithoutState(t *testing.T) {
	dir := t.TempDir()
	noState := uuid.NewV7()
	broken := uuid.NewV7()
	for _, p := range []string{noState.String(), broken.String(), "not-a-uuid"} {
		if err := os.MkdirAll(filepath.Join(dir, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, broken.String(), jobStateFile), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	jobs, err := NewJobService(dir, t.TempDir(), nil, nil, DefaultJobLimits)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{noState, broken} {
		if _, err := jobs.Get(id); !errors.Is(err, ErrNotFound) {
			t.Errorf("job %s: err = %v, want ErrNotFound", id, err)
		}
	}
}
