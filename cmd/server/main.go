package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"kebabseller6/rwe/internal/handler"
	"kebabseller6/rwe/internal/runner"
	"kebabseller6/rwe/internal/service"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const shutdownTimeout = 30 * time.Second

var version = "dev"

func main() {
	addr := flag.String("addr", envOr("RWE_ADDR", ":8999"), "listen address (env RWE_ADDR)")
	dataDir := flag.String("data", envOr("RWE_DATA", "local_data"), "directory for workspaces and job logs (env RWE_DATA)")
	jobMemory := flag.String("job-memory", envOr("RWE_JOB_MEMORY", ""), "memory cap per job, e.g. 2g; also the default when a job sets none (env RWE_JOB_MEMORY)")
	logRetention := flag.String("log-retention", envOr("RWE_LOG_RETENTION", service.DefaultJobLimits.LogRetention.String()), "delete job logs this long after the job finished, e.g. 72h; 0 keeps them (env RWE_LOG_RETENTION)")
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

	if err := run(*addr, *dataDir, limits); err != nil {
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

func run(addr, dataDir string, limits service.JobLimits) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := slog.Default()

	ws, err := service.NewWorkspaceService(filepath.Join(dataDir, "workspaces"))
	if err != nil {
		return fmt.Errorf("init workspace service: %w", err)
	}
	jobs, err := service.NewJobService(filepath.Join(dataDir, "jobs"), filepath.Join(dataDir, "caches"), ws, runner.NewDocker(), limits)
	if err != nil {
		return fmt.Errorf("init job service: %w", err)
	}

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
	logger.Info("server started", "version", version, "addr", ln.Addr().String(), "data", dataDir, "job_memory", limits.MaxMemory, "log_retention", limits.LogRetention)

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
