package service

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestJob_CollectsArtifacts(t *testing.T) {
	outside := t.TempDir()
	env := newJobEnv(t, func(_ context.Context, spec RunSpec, _ io.Writer) (int, error) {
		write := func(name, content string) {
			p := filepath.Join(spec.FilesDir, name)
			os.MkdirAll(filepath.Dir(p), 0o755)
			os.WriteFile(p, []byte(content), 0o644)
		}
		write("coverage.out", "12345")
		write("dist/a.bin", "aa")
		write("dist/sub/b.bin", "bbb")
		os.Symlink(outside, filepath.Join(spec.FilesDir, "escape"))
		return 1, nil
	})
	os.WriteFile(filepath.Join(outside, "secret"), []byte("x"), 0o644)

	req := env.request()
	req.Artifacts = []string{"coverage.out", "dist/", "missing.txt", "escape/secret"}
	job, err := env.jobs.Submit(req)
	if err != nil {
		t.Fatal(err)
	}
	done := waitFinished(t, env.jobs, job.ID)
	if done.Status != JobFailed {
		t.Fatalf("status = %s, want failed jobs to keep their artifacts too", done.Status)
	}

	want := []Artifact{{Path: "coverage.out", Size: 5}, {Path: "dist", Size: 5}}
	if !slices.Equal(done.Artifacts, want) {
		t.Errorf("artifacts = %v, want %v", done.Artifacts, want)
	}
	artifacts := filepath.Join(filepath.Dir(env.jobs.logPath(job.ID)), artifactsDirName)
	if got, _ := os.ReadFile(filepath.Join(artifacts, "dist", "sub", "b.bin")); string(got) != "bbb" {
		t.Errorf("dist/sub/b.bin = %q, want it kept after the job copy is removed", got)
	}
	if _, err := os.Stat(filepath.Join(outside, "secret")); err != nil {
		t.Errorf("file outside the job dir was moved: %v", err)
	}

	log, _ := os.ReadFile(env.jobs.logPath(job.ID))
	for _, line := range []string{"artifact missing.txt not found", "artifact escape/secret:"} {
		if !strings.Contains(string(log), line) {
			t.Errorf("log %q does not mention %q", log, line)
		}
	}
}

func TestValidateArtifacts(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
	}{
		{"parent", []string{"../x"}},
		{"absolute", []string{"/etc/passwd"}},
		{"whole workspace", []string{"."}},
		{"duplicate", []string{"dist", "dist/"}},
		{"nested", []string{"dist/a", "dist"}},
		{"too many", slices.Repeat([]string{"x"}, maxArtifacts+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := validateArtifacts(tt.paths); !errors.Is(err, ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}

	got, err := validateArtifacts([]string{"dist/", "./coverage.out", "distx"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"dist", "coverage.out", "distx"}; !slices.Equal(got, want) {
		t.Errorf("clean = %v, want %v", got, want)
	}
}
