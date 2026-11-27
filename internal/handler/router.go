package handler

import (
	"log/slog"
	"net/http"
)

func NewRouter(ws WorkspaceService, jobs JobService, logger *slog.Logger) http.Handler {
	eh := NewErrorHandler(logger)
	workspaceHandler := NewWorkspaceHandler(ws, eh)
	jobHandler := NewJobHandler(jobs, eh)

	mux := http.NewServeMux()

	mux.HandleFunc("POST /workspaces", workspaceHandler.HandleCreate)
	mux.HandleFunc("POST /workspaces/{id}/sync", workspaceHandler.HandleSync)
	mux.HandleFunc("PUT /workspaces/{id}/blobs/{sha256}", workspaceHandler.HandleUpload)
	mux.HandleFunc("POST /workspaces/{id}/jobs", jobHandler.HandleSubmit)
	mux.HandleFunc("GET /jobs/{id}", jobHandler.HandleGet)
	mux.HandleFunc("GET /jobs/{id}/logs", jobHandler.HandleLogs)
	mux.HandleFunc("DELETE /jobs/{id}", jobHandler.HandleCancel)

	return mux
}
