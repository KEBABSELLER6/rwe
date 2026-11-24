package handler

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"kebabseller6/rwe/internal/service"
	"net/http"
)

type FileService interface {
	Create(ctx context.Context, body io.Reader) (service.Workspace, error)
}

type WorkSpaceHandler struct {
	s  FileService
	eh *ErrorHandler
}

func NewWorkspaceHandler(s FileService, eh *ErrorHandler) *WorkSpaceHandler {
	return &WorkSpaceHandler{
		s:  s,
		eh: eh,
	}
}

func (h *WorkSpaceHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 20<<20)

	if r.Header.Get("Content-Type") != "application/gzip" {
		h.eh.HandleError(w, r, fmt.Errorf("%w: invalid content-type", service.ErrInvalidInput))
		return
	}

	res, err := h.s.Create(r.Context(), r.Body)
	if err != nil {
		h.eh.HandleError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.MarshalWrite(w, res)
}
