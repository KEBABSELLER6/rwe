package handler

import (
	"encoding/json/v2"
	"fmt"
	"kebabseller6/rwe/internal/manifest"
	"kebabseller6/rwe/internal/service"
	"net/http"
)

type SyncRequest struct {
	Files manifest.Manifest `json:"files"`
}

type SyncResponse struct {
	Missing []string `json:"missing"`
}

type UploadResponse struct {
	Remaning int `json:"remaining"`
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

	id, err := getId(r)
	if err != nil {
		h.eh.HandleError(w, r, err)
		return
	}

	var req SyncRequest
	err = json.UnmarshalRead(r.Body, &req, json.RejectUnknownMembers(true))
	if err != nil {
		h.eh.HandleError(w, r, fmt.Errorf("%w: invalid json", service.ErrInvalidInput))
		return
	}

	if req.Files == nil {
		h.eh.HandleError(w, r, fmt.Errorf("%w: empty target", service.ErrInvalidInput))
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

func (h *WorkSpaceHandler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 20<<20)

	if r.Header.Get("Content-Type") != "application/octet-stream" {
		h.eh.HandleError(w, r, fmt.Errorf("%w: invalid content-type", service.ErrInvalidInput))
		return
	}

	id, err := getId(r)
	if err != nil {
		h.eh.HandleError(w, r, err)
		return
	}

	hash := r.PathValue("sha256")
	if !manifest.Sha256Hex.MatchString(hash) {
		h.eh.HandleError(w, r, fmt.Errorf("%w: invalid hash", service.ErrInvalidInput))
		return
	}

	remaining, err := h.s.PutBlob(r.Context(), id, hash, r.Body)
	if err != nil {
		h.eh.HandleError(w, r, err)
		return
	}
	res := UploadResponse{Remaning: remaining}
	if remaining > 0 {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusCreated)
	}

	w.Header().Set("Content-Type", "application/json")
	json.MarshalWrite(w, res)
}
