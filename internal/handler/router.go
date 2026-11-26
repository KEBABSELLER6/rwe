package handler

import (
	"context"
	"fmt"
	"kebabseller6/rwe/internal/runner"
	"kebabseller6/rwe/internal/service"
	"log/slog"
	"net/http"
	"path/filepath"
)

func NewRouter(ctx context.Context) (http.Handler, error) {
	eh := NewErrorHandler(slog.Default())
	workdir := "local_data"
	wspcService, err := service.NewWorkspaceService(filepath.Join(workdir, "workspaces"))
	if err != nil {
		return nil, fmt.Errorf("init workspace service: %w", err)
	}
	docker := runner.NewDocker()
	jobService, err := service.NewJobService(filepath.Join(workdir, "jobs"), wspcService, docker)
	if err != nil {
		return nil, fmt.Errorf("init job service: %w", err)
	}
	go jobService.Start(ctx)

	wspcHandler := NewWorkspaceHandler(wspcService, eh)
	jobHandler := NewJobHandler(jobService, eh)

	mux := http.NewServeMux()

	mux.HandleFunc("POST /workspaces", wspcHandler.HandleCreate)
	mux.HandleFunc("POST /workspaces/{id}/sync", wspcHandler.HandleSync)
	mux.HandleFunc("PUT /workspaces/{id}/blobs/{sha256}", wspcHandler.HandleUpload)
	mux.HandleFunc("POST /workspaces/{id}/jobs", jobHandler.HandleSubmit)
	mux.HandleFunc("GET /jobs/{id}", jobHandler.HandleGet)
	mux.HandleFunc("GET /jobs/{id}/logs", jobHandler.HandleLogs)

	return mux, nil
}
