package handler

import (
	"encoding/json"
	"errors"
	"kebabseller6/rwe/internal/service"
	"log/slog"
	"net/http"
)

type ErrorHandler struct {
	logger *slog.Logger
}

func NewErrorHandler(logger *slog.Logger) *ErrorHandler {
	return &ErrorHandler{logger: logger}
}

type ErrorResponse struct {
	Message string `json:"message"`
}

func buildError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	resp := &ErrorResponse{Message: msg}
	out, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"internal error"}`))
		return
	}
	w.WriteHeader(status)
	w.Write(out)
}

func (h *ErrorHandler) HandleError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.ErrorContext(r.Context(), "request failed", "err", err)

	var maxErr *http.MaxBytesError
	switch {
	case errors.As(err, &maxErr), errors.Is(err, service.ErrTooLarge):
		buildError(w, "request too large", http.StatusRequestEntityTooLarge)
	case errors.Is(err, service.ErrInvalidInput):
		buildError(w, "invalid input", http.StatusBadRequest)
	case errors.Is(err, service.ErrNotFound):
		buildError(w, "not found", http.StatusNotFound)
	default:
		buildError(w, "internal error", http.StatusInternalServerError)
	}
}
