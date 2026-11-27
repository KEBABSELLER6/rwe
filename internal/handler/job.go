package handler

import (
	"encoding/json/v2"
	"fmt"
	"kebabseller6/rwe/internal/service"
	"net/http"
	"time"
)

type JobHandler struct {
	s  JobService
	eh *ErrorHandler
}

func NewJobHandler(s JobService, eh *ErrorHandler) *JobHandler {
	return &JobHandler{
		eh: eh,
		s:  s,
	}
}

type JobRequest struct {
	ExpectedHash   string   `json:"expected_hash"`
	Image          string   `json:"image"`
	Command        []string `json:"command"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	MemoryBytes    int64    `json:"memory_bytes"`
	CPUs           float64  `json:"cpus"`
	Caches         []string `json:"caches"`
}

func (h *JobHandler) HandleSubmit(w http.ResponseWriter, r *http.Request) {
	var jr JobRequest
	if err := decodeJSON(w, r, 64<<10, &jr); err != nil {
		h.eh.HandleError(w, r, err)
		return
	}

	id, err := pathID(r, "workspace")
	if err != nil {
		h.eh.HandleError(w, r, err)
		return
	}

	req := service.JobRequest{
		WorkspaceID:  id,
		ExpectedHash: jr.ExpectedHash,
		Image:        jr.Image,
		Command:      jr.Command,
		Timeout:      time.Duration(jr.TimeoutSeconds) * time.Second,
		Memory:       jr.MemoryBytes,
		CPUs:         jr.CPUs,
		Caches:       jr.Caches,
	}

	j, err := h.s.Submit(req)
	if err != nil {
		h.eh.HandleError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, j)
}

func (h *JobHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "job")
	if err != nil {
		h.eh.HandleError(w, r, err)
		return
	}

	res, err := h.s.Get(id)
	if err != nil {
		h.eh.HandleError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, res)
}

func (h *JobHandler) HandleCancel(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "job")
	if err != nil {
		h.eh.HandleError(w, r, err)
		return
	}

	res, err := h.s.Cancel(id)
	if err != nil {
		h.eh.HandleError(w, r, err)
		return
	}

	writeJSON(w, http.StatusAccepted, res)
}

func (h *JobHandler) HandleLogs(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "job")
	if err != nil {
		h.eh.HandleError(w, r, err)
		return
	}
	if _, err := h.s.Get(id); err != nil {
		h.eh.HandleError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)

	err = h.s.FollowLogs(r.Context(), id, func(line []byte) error {
		fmt.Fprintf(w, "data: %s\n\n", line)
		return rc.Flush()
	})
	if err != nil {
		return
	}

	job, _ := h.s.Get(id)
	fmt.Fprint(w, "event: done\ndata: ")
	json.MarshalWrite(w, job)
	fmt.Fprint(w, "\n\n")
	rc.Flush()
}
