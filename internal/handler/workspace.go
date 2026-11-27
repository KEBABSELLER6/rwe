package handler

import (
	"fmt"
	"kebabseller6/rwe/internal/manifest"
	"kebabseller6/rwe/internal/service"
	"net/http"
)

var maxUploadBytes = service.DefaultWorkspaceLimits.MaxTotalBytes

type SyncRequest struct {
	Files      manifest.Manifest `json:"files"`
	Sizes      map[string]int64  `json:"sizes"`
	Executable []string          `json:"executable"`
}

type SyncResponse struct {
	Missing []string `json:"missing"`
}

type UploadResponse struct {
	Remaining int `json:"remaining"`
}

type WorkspaceHandler struct {
	s  WorkspaceService
	eh *ErrorHandler
}

func NewWorkspaceHandler(s WorkspaceService, eh *ErrorHandler) *WorkspaceHandler {
	return &WorkspaceHandler{
		s:  s,
		eh: eh,
	}
}

func (h *WorkspaceHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	if err := requireContentType(r, "application/gzip"); err != nil {
		h.eh.HandleError(w, r, err)
		return
	}

	res, err := h.s.Create(r.Context(), r.Body)
	if err != nil {
		h.eh.HandleError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, res)
}

func (h *WorkspaceHandler) HandleSync(w http.ResponseWriter, r *http.Request) {
	var req SyncRequest
	err := decodeJSON(w, r, 4<<20, &req)
	if err != nil {
		h.eh.HandleError(w, r, err)
		return
	}

	id, err := pathID(r, "workspace")
	if err != nil {
		h.eh.HandleError(w, r, err)
		return
	}

	if req.Files == nil {
		h.eh.HandleError(w, r, fmt.Errorf(`%w: "files" is required`, service.ErrInvalidInput))
		return
	}

	missing, err := h.s.Sync(r.Context(), id, req.Files, req.Sizes, req.Executable)
	if err != nil {
		h.eh.HandleError(w, r, err)
		return
	}

	res := SyncResponse{Missing: missing}
	writeJSON(w, http.StatusOK, res)
}

func (h *WorkspaceHandler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	if err := requireContentType(r, "application/octet-stream"); err != nil {
		h.eh.HandleError(w, r, err)
		return
	}

	id, err := pathID(r, "workspace")
	if err != nil {
		h.eh.HandleError(w, r, err)
		return
	}

	remaining, err := h.s.PutBlob(r.Context(), id, r.PathValue("sha256"), r.Body)
	if err != nil {
		h.eh.HandleError(w, r, err)
		return
	}

	res := UploadResponse{Remaining: remaining}
	writeJSON(w, http.StatusOK, res)
}
