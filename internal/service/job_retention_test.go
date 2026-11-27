package service

import (
	"os"
	"testing"
	"time"
)

func TestJobRetention_PrunesOnlyExpiredLogs(t *testing.T) {
	for _, tc := range []struct {
		name      string
		retention time.Duration
		age       time.Duration
		wantLog   bool
	}{
		{"expired", time.Hour, 2 * time.Hour, false},
		{"recent", time.Hour, time.Minute, true},
		{"disabled", 0, 1000 * time.Hour, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newJobEnv(t, exitWith(0, nil, "hello\n"))
			limits := DefaultJobLimits
			limits.LogRetention = tc.retention
			jobs, err := NewJobService(t.TempDir(), t.TempDir(), env.ws, env.runner, limits)
			if err != nil {
				t.Fatal(err)
			}
			job, err := jobs.Submit(env.request())
			if err != nil {
				t.Fatal(err)
			}
			jobs.run(t.Context(), job.ID)
			finished, _ := jobs.Get(job.ID)

			jobs.pruneLogs(finished.FinishedAt.Add(tc.age))

			_, err = os.Stat(jobs.logPath(job.ID))
			if gotLog := err == nil; gotLog != tc.wantLog {
				t.Errorf("log exists = %v, want %v (stat err %v)", gotLog, tc.wantLog, err)
			}
			if _, err := os.Stat(jobs.statePath(job.ID)); err != nil {
				t.Errorf("job state removed: %v", err)
			}
			if got, err := jobs.Get(job.ID); err != nil || got.Status != JobPassed {
				t.Errorf("get after prune = %s, %v; want passed", got.Status, err)
			}
		})
	}
}

func TestJobRetention_KeepsUnfinishedJobLogs(t *testing.T) {
	env := newJobEnv(t, exitWith(0, nil, ""))
	jobs, err := NewJobService(t.TempDir(), t.TempDir(), env.ws, env.runner, DefaultJobLimits)
	if err != nil {
		t.Fatal(err)
	}
	job, err := jobs.Submit(env.request())
	if err != nil {
		t.Fatal(err)
	}

	jobs.pruneLogs(time.Now().Add(365 * 24 * time.Hour))

	if _, err := os.Stat(jobs.logPath(job.ID)); err != nil {
		t.Errorf("queued job log removed: %v", err)
	}
}

func TestJobRetention_PrunedLogsFollowAsEmpty(t *testing.T) {
	env := newJobEnv(t, exitWith(0, nil, "hello\n"))
	job, err := env.jobs.Submit(env.request())
	if err != nil {
		t.Fatal(err)
	}
	waitFinished(t, env.jobs, job.ID)
	if err := os.Remove(env.jobs.logPath(job.ID)); err != nil {
		t.Fatal(err)
	}

	var lines int
	err = env.jobs.FollowLogs(t.Context(), job.ID, func([]byte) error { lines++; return nil })
	if err != nil || lines != 0 {
		t.Errorf("follow pruned log = %d lines, %v; want 0, nil", lines, err)
	}
}
