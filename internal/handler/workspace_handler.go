package handler

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"kebabseller6/rwe/internal/manifest"
	"kebabseller6/rwe/internal/service"
	"net/http"
	"uuid"
)

type SyncRequest struct {
	Files manifest.Manifest `json:"files"`
}

type SyncResponse struct {
	Missing []string `json:"missing"`
}

type FileService interface {
	Create(ctx context.Context, body io.Reader) (service.Workspace, error)
	Sync(ctx context.Context, id uuid.UUID, body manifest.Manifest) ([]string, error)
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

func (h *WorkSpaceHandler) HandleSync(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)

	if r.Header.Get("Content-Type") != "application/json" {
		h.eh.HandleError(w, r, fmt.Errorf("%w: invalid content-type", service.ErrInvalidInput))
		return
	}

	rawId := r.PathValue("id")
	id, err := uuid.Parse(rawId)
	if err != nil {
		h.eh.HandleError(w, r, fmt.Errorf("%w: invalid workspace id", service.ErrInvalidInput))
		return
	}

	var req SyncRequest
	err = json.UnmarshalRead(r.Body, &req)
	if err != nil {
		h.eh.HandleError(w, r, fmt.Errorf("%w: invalid json", service.ErrInvalidInput))
		return
	}

	missing, err := h.s.Sync(r.Context(), id, req.Files)
	if err != nil {
		h.eh.HandleError(w, r, err)
		return
	}
	res := SyncResponse{Missing: missing}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.MarshalWrite(w, res)
}
