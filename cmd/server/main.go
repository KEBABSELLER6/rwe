package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/KEBABSELLER6/rwe/internal/handler"
	"github.com/KEBABSELLER6/rwe/internal/runner"
	"github.com/KEBABSELLER6/rwe/internal/service"
)

const shutdownTimeout = 30 * time.Second

var version = "dev"

func main() {
	addr := flag.String("addr", envOr("RWE_ADDR", ":8999"), "listen address (env RWE_ADDR)")
	dataDir := flag.String("data", envOr("RWE_DATA", "local_data"), "directory for workspaces and job logs (env RWE_DATA)")
	jobMemory := flag.String("job-memory", envOr("RWE_JOB_MEMORY", ""), "memory cap per job, e.g. 2g; also the default when a job sets none (env RWE_JOB_MEMORY)")
	logRetention := flag.String("log-retention", envOr("RWE_LOG_RETENTION", service.DefaultJobLimits.LogRetention.String()), "delete job logs this long after the job finished, e.g. 72h; 0 keeps them (env RWE_LOG_RETENTION)")
	workspaceRetention := flag.String("workspace-retention", envOr("RWE_WORKSPACE_RETENTION", service.DefaultWorkspaceLimits.Retention.String()), "delete workspaces this long after their last sync or job, e.g. 168h; 0 keeps them (env RWE_WORKSPACE_RETENTION)")
	cacheRetention := flag.String("cache-retention", envOr("RWE_CACHE_RETENTION", service.DefaultJobLimits.CacheRetention.String()), "delete shared caches this long after the last job that used them, e.g. 168h; 0 keeps them (env RWE_CACHE_RETENTION)")
	artifactRetention := flag.String("artifact-retention", envOr("RWE_ARTIFACT_RETENTION", service.DefaultJobLimits.ArtifactRetention.String()), "delete job artifacts this long after the job finished, e.g. 72h; 0 keeps them (env RWE_ARTIFACT_RETENTION)")
	artifactMaxSize := flag.String("artifact-max-size", envOr("RWE_ARTIFACT_MAX_SIZE", "256m"), "drop a job's artifacts when together they are larger, e.g. 1g; 0 means no limit (env RWE_ARTIFACT_MAX_SIZE)")
	flag.Parse()

	limits := service.DefaultJobLimits
	var err error
	if limits.MaxMemory, err = parseBytes(*jobMemory); err != nil {
		slog.Error("invalid job memory", "value", *jobMemory, "err", err)
		os.Exit(2)
	}
	if limits.LogRetention, err = time.ParseDuration(*logRetention); err != nil {
		slog.Error("invalid log retention", "value", *logRetention, "err", err)
		os.Exit(2)
	}
	if limits.CacheRetention, err = time.ParseDuration(*cacheRetention); err != nil {
		slog.Error("invalid cache retention", "value", *cacheRetention, "err", err)
		os.Exit(2)
	}

	if limits.ArtifactRetention, err = time.ParseDuration(*artifactRetention); err != nil {
		slog.Error("invalid artifact retention", "value", *artifactRetention, "err", err)
		os.Exit(2)
	}
	if limits.ArtifactMaxSize, err = parseBytes(*artifactMaxSize); err != nil {
		slog.Error("invalid artifact max size", "value", *artifactMaxSize, "err", err)
		os.Exit(2)
	}

	retention, err := time.ParseDuration(*workspaceRetention)
	if err != nil {
		slog.Error("invalid workspace retention", "value", *workspaceRetention, "err", err)
		os.Exit(2)
	}

	if err := run(*addr, *dataDir, limits, retention); err != nil {
		slog.Error("application stopped", "err", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func run(addr, dataDir string, limits service.JobLimits, workspaceRetention time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := slog.Default()

	ws, err := service.NewWorkspaceService(filepath.Join(dataDir, "workspaces"), workspaceRetention)
	if err != nil {
		return fmt.Errorf("init workspace service: %w", err)
	}
	owner, err := filepath.Abs(dataDir)
	if err != nil {
		return fmt.Errorf("resolve data dir: %w", err)
	}
	docker := runner.NewDocker(owner)
	if removed, err := docker.RemoveStale(ctx); err != nil {
		logger.Warn("remove containers left by a previous run", "err", err)
	} else if removed > 0 {
		logger.Info("removed containers left by a previous run", "count", removed)
	}

	jobs, err := service.NewJobService(filepath.Join(dataDir, "jobs"), filepath.Join(dataDir, "caches"), ws, docker, limits)
	if err != nil {
		return fmt.Errorf("init job service: %w", err)
	}

	go ws.StartPruning(ctx)

	workerDone := make(chan struct{})
	go func() {
		jobs.Start(ctx)
		close(workerDone)
	}()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		stop()
		<-workerDone
		return err
	}

	srv := &http.Server{
		Handler:           handler.NewRouter(ws, jobs, logger),
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	logger.Info("server started", "version", version, "addr", ln.Addr().String(), "data", dataDir, "job_memory", limits.MaxMemory, "log_retention", limits.LogRetention, "workspace_retention", workspaceRetention, "cache_retention", limits.CacheRetention, "artifact_retention", limits.ArtifactRetention, "artifact_max_size", limits.ArtifactMaxSize)

	var runErr error
	select {
	case err := <-serveErr:
		runErr = fmt.Errorf("server: %w", err)
	case <-ctx.Done():
		logger.Info("shutting down")
	}
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	shutdownErr := srv.Shutdown(shutdownCtx)
	select {
	case <-workerDone:
	case <-shutdownCtx.Done():
		shutdownErr = errors.Join(shutdownErr, errors.New("job worker did not stop in time"))
	}

	logger.Info("server stopped")
	return errors.Join(runErr, shutdownErr)
}
