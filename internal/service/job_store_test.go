package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
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
}

func TestJobStore_UnfinishedJobBecomesError(t *testing.T) {
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
