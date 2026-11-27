package main

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const projectConfigFile = ".rwe.json"

type config struct {
	Server string   `json:"server"`
	Image  string   `json:"image"`
	Caches []string `json:"caches"`

	dir string
}

var defaultConfig = config{
	Server: "http://localhost:8999",
	Image:  "golang:1.27",
}

func loadConfig(dir string) (config, error) {
	cfg := defaultConfig
	cfg.dir = dir

	if userDir, err := os.UserConfigDir(); err == nil {
		if err := mergeFile(&cfg, filepath.Join(userDir, "rwe", "config.json")); err != nil {
			return config{}, err
		}
	}
	if err := mergeFile(&cfg, filepath.Join(dir, projectConfigFile)); err != nil {
		return config{}, err
	}

	mergeEnv(&cfg.Server, "RWE_URL")
	mergeEnv(&cfg.Image, "RWE_IMAGE")
	return cfg, nil
}

func mergeFile(cfg *config, path string) error {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	var file config
	if err := json.Unmarshal(raw, &file, json.RejectUnknownMembers(true)); err != nil {
		return fmt.Errorf("config %s: %w", path, err)
	}
	if file.Server != "" {
		cfg.Server = file.Server
	}
	if file.Image != "" {
		cfg.Image = file.Image
	}
	if file.Caches != nil {
		cfg.Caches = file.Caches
	}
	return nil
}

func mergeEnv(field *string, key string) {
	if v := os.Getenv(key); v != "" {
		*field = v
	}
}
