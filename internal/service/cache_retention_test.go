package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"
	"uuid"
)

func TestCacheRetention_DeletesCachesOfDeletedWorkspaces(t *testing.T) {
	env := newJobEnv(t, nil)
	jobs := env.jobs
	live := jobs.cacheMounts(env.wsID, []string{"/workspace/node_modules"})[0].HostDir
	gone := jobs.cacheMounts(uuid.NewV7(), []string{"/workspace/node_modules"})[0].HostDir
	for _, dir := range []string{live, gone} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	jobs.pruneCaches(time.Now())

	if _, err := os.Stat(live); err != nil {
		t.Errorf("cache of existing workspace deleted: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(gone)); !os.IsNotExist(err) {
		t.Errorf("cache of deleted workspace kept: stat err %v", err)
	}
}

func TestCacheRetention_DeletesOnlyUnusedSharedCaches(t *testing.T) {
	for _, tc := range []struct {
		name      string
		retention time.Duration
		age       time.Duration
		wantKept  bool
	}{
		{"expired", time.Hour, 2 * time.Hour, false},
		{"recent", time.Hour, time.Minute, true},
		{"disabled", 0, 1000 * time.Hour, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jobs := &JobService{cachesDir: t.TempDir(), limits: JobLimits{CacheRetention: tc.retention}}
			mounts := jobs.cacheMounts(uuid.NewV7(), []string{"/tmp/.cache"})
			if err := prepareCaches(mounts, t.TempDir()); err != nil {
				t.Fatal(err)
			}

			jobs.pruneCaches(time.Now().Add(tc.age))

			_, err := os.Stat(mounts[0].HostDir)
			if kept := err == nil; kept != tc.wantKept {
				t.Errorf("cache kept = %v, want %v (stat err %v)", kept, tc.wantKept, err)
			}
		})
	}
}
