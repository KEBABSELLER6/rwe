package handler

import (
	"log/slog"
	"net/http"
)

func NewRouter(ws WorkspaceService, jobs JobService, logger *slog.Logger) http.Handler {
	eh := NewErrorHandler(logger)
	wspcHandler := NewWorkspaceHandler(ws, eh)
	jobHandler := NewJobHandler(jobs, eh)

	mux := http.NewServeMux()

	mux.HandleFunc("POST /workspaces", wspcHandler.HandleCreate)
	mux.HandleFunc("POST /workspaces/{id}/sync", wspcHandler.HandleSync)
	mux.HandleFunc("PUT /workspaces/{id}/blobs/{sha256}", wspcHandler.HandleUpload)
	mux.HandleFunc("POST /workspaces/{id}/jobs", jobHandler.HandleSubmit)
	mux.HandleFunc("GET /jobs/{id}", jobHandler.HandleGet)
	mux.HandleFunc("GET /jobs/{id}/logs", jobHandler.HandleLogs)

	return mux
}
