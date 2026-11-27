package service

import (
	"context"
	"io"
	"time"
)

type RunSpec struct {
	Name     string
	Image    string
	Command  []string
	FilesDir string
	Timeout  time.Duration
	Memory   int64
	CPUs     float64
	Caches   []CacheMount
}

type Runner interface {
	Run(ctx context.Context, spec RunSpec, logs io.Writer) (exitCode int, err error)
}
