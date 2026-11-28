package handler

import (
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

func (h *ErrorHandler) HandleError(w http.ResponseWriter, r *http.Request, err error) {
	status := statusFor(err)

	msg := err.Error()
	if status >= http.StatusInternalServerError {
		msg = "internal error"
		h.logger.ErrorContext(r.Context(), "request failed", "status", status, "err", err)
	} else {
		h.logger.WarnContext(r.Context(), "request rejected", "status", status, "err", err)
	}

	writeJSON(w, status, ErrorResponse{Message: msg})
}

func statusFor(err error) int {
	var maxErr *http.MaxBytesError
	switch {
	case errors.As(err, &maxErr), errors.Is(err, service.ErrTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, service.ErrInvalidInput):
		return http.StatusBadRequest
	case errors.Is(err, service.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, service.ErrConflict):
		return http.StatusConflict
	case errors.Is(err, service.ErrGone):
		return http.StatusGone
	default:
		return http.StatusInternalServerError
	}
}
