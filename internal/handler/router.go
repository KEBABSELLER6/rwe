package handler

import (
	"fmt"
	"kebabseller6/rwe/internal/service"
	"log/slog"
	"net/http"
)

func NewRouter() (http.Handler, error) {
	eh := NewErrorHandler(slog.Default())
	wspcService, err := service.NewWorkspaceService("local_data")
	if err != nil {
		return nil, fmt.Errorf("init workspace service: %w", err)
	}

	wspcHandler := NewWorkspaceHandler(wspcService, eh)

	mux := http.NewServeMux()

	mux.HandleFunc("POST /workspaces", wspcHandler.HandleCreate)
	mux.HandleFunc("POST /workspaces/{id}/sync", wspcHandler.HandleSync)

	return mux, nil
}
