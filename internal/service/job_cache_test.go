package service

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"uuid"
)

func TestValidateCaches(t *testing.T) {
	for _, tt := range []struct {
		name  string
		paths []string
		ok    bool
	}{
		{"none", nil, true},
		{"shared and workspace", []string{"/tmp/.cache/go-build", "/workspace/node_modules"}, true},
		{"relative", []string{"tmp/cache"}, false},
		{"unclean", []string{"/workspace/../etc"}, false},
		{"root", []string{"/"}, false},
		{"workspace itself", []string{"/workspace"}, false},
		{"colon", []string{"/tmp/a:b"}, false},
		{"duplicate", []string{"/tmp/a", "/tmp/a"}, false},
		{"too many", strings.Fields("/a /b /c /d /e /f /g /h /i"), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCaches(tt.paths)
			if tt.ok && err != nil {
				t.Errorf("err = %v, want nil", err)
			}
			if !tt.ok && !errors.Is(err, ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestCacheMounts_Scopes(t *testing.T) {
	s := &JobService{cachesDir: "/data/caches"}
	a, b := uuid.NewV7(), uuid.NewV7()
	paths := []string{"/tmp/.bun/install/cache", "/workspace/node_modules"}

	ma, mb := s.cacheMounts(a, paths), s.cacheMounts(b, paths)

	if ma[0].HostDir != mb[0].HostDir || !strings.HasPrefix(ma[0].HostDir, "/data/caches/shared/") {
		t.Errorf("shared cache dirs = %s, %s; want one dir under shared/", ma[0].HostDir, mb[0].HostDir)
	}
	if ma[1].HostDir == mb[1].HostDir || !strings.HasPrefix(ma[1].HostDir, "/data/caches/workspaces/"+a.String()+"/") {
		t.Errorf("workspace cache dirs = %s, %s; want one per workspace", ma[1].HostDir, mb[1].HostDir)
	}
	if ma[0].HostDir == ma[1].HostDir || ma[1].Path != "/workspace/node_modules" {
		t.Errorf("mounts = %+v", ma)
	}
}

func TestJob_PreparesCacheDirs(t *testing.T) {
	var missing []string
	env := newJobEnv(t, nil)
	env.runner.run = func(_ context.Context, spec RunSpec, _ io.Writer) (int, error) {
		for _, p := range []string{spec.Caches[0].HostDir, spec.Caches[1].HostDir, filepath.Join(spec.FilesDir, "deps", "node_modules")} {
			if _, err := os.Stat(p); err != nil {
				missing = append(missing, p)
			}
		}
		return 0, nil
	}
	req := env.request()
	req.Caches = []string{"/tmp/.cache", "/workspace/deps/node_modules"}

	job, err := env.jobs.Submit(req)
	if err != nil {
		t.Fatal(err)
	}
	if got := waitFinished(t, env.jobs, job.ID); got.Status != JobPassed || len(got.Caches) != 2 {
		t.Fatalf("job = %s %q, caches %v", got.Status, got.Error, got.Caches)
	}
	if len(missing) > 0 {
		t.Errorf("dirs missing when the job started: %v", missing)
	}
}
