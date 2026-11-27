package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setup(t *testing.T, user, project string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("RWE_URL", "")
	t.Setenv("RWE_IMAGE", "")

	if user != "" {
		write(t, filepath.Join(home, "rwe", "config.json"), user)
	}
	dir := t.TempDir()
	if project != "" {
		write(t, filepath.Join(dir, projectConfigFile), project)
	}
	return dir
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg, err := loadConfig(setup(t, "", ""))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server != defaultConfig.Server || cfg.Image != defaultConfig.Image {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestConfigLayering(t *testing.T) {
	dir := setup(t,
		`{"server": "http://box:8999", "image": "user-image"}`,
		`{"image": "project-image"}`,
	)

	cfg, err := loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server != "http://box:8999" || cfg.Image != "project-image" || cfg.dir != dir {
		t.Fatalf("cfg = %+v, want user server, project image", cfg)
	}

	t.Setenv("RWE_URL", "http://env:1")
	t.Setenv("RWE_IMAGE", "env-image")
	cfg, err = loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server != "http://env:1" || cfg.Image != "env-image" {
		t.Fatalf("cfg = %+v, want env to win", cfg)
	}
}

func TestConfigRejectsTypos(t *testing.T) {
	_, err := loadConfig(setup(t, "", `{"imgae": "x"}`))
	if err == nil || !strings.Contains(err.Error(), projectConfigFile) {
		t.Fatalf("err = %v, want an error naming %s", err, projectConfigFile)
	}
}
