package client

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"kebabseller6/rwe/internal/service"
)

type buildRunner struct{}

func (buildRunner) Run(_ context.Context, spec service.RunSpec, _ io.Writer) (int, error) {
	os.MkdirAll(filepath.Join(spec.FilesDir, "dist"), 0o755)
	os.WriteFile(filepath.Join(spec.FilesDir, "dist", "app"), []byte("binary"), 0o755)
	return 0, nil
}

func TestFetchArtifactsKeepsOnlyLatestJob(t *testing.T) {
	c := newTestClientWith(t, buildRunner{})
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"main.go": "package main"})
	spec := JobSpec{Image: "img", Command: []string{"build"}, Artifacts: []string{"dist"}}

	var dsts []string
	for range 2 {
		job, _, err := c.Run(t.Context(), dir, spec, func(string) {})
		if err != nil {
			t.Fatal(err)
		}
		dst, err := c.FetchArtifacts(t.Context(), dir, job)
		if err != nil {
			t.Fatal(err)
		}
		if dst != ArtifactsDir(dir, job.ID) {
			t.Fatalf("dst = %s, want %s", dst, ArtifactsDir(dir, job.ID))
		}
		dsts = append(dsts, dst)
	}

	info, err := os.Stat(filepath.Join(dsts[1], "dist", "app"))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dsts[1], "dist", "app")); string(got) != "binary" || info.Mode().Perm()&0o100 == 0 {
		t.Errorf("dist/app = %q mode %s, want executable \"binary\"", got, info.Mode())
	}
	if _, err := os.Stat(dsts[0]); !os.IsNotExist(err) {
		t.Errorf("previous job's artifacts kept: %v", err)
	}
}

func TestFetchArtifactsWithoutArtifacts(t *testing.T) {
	c := newTestClient(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.txt": "a"})
	job, _, err := c.Run(t.Context(), dir, JobSpec{Image: "img", Command: []string{"ls"}}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if dst, err := c.FetchArtifacts(t.Context(), dir, job); dst != "" || err != nil {
		t.Errorf("fetch = %q, %v; want nothing", dst, err)
	}
	if _, err := os.Stat(filepath.Join(dir, stateDir, "artifacts")); !os.IsNotExist(err) {
		t.Errorf("artifacts dir created for a job without artifacts: %v", err)
	}
}
