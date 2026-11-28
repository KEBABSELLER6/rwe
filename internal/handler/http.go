package handler

import (
	"encoding/json/v2"
	"fmt"
	"mime"
	"net/http"
	"uuid"

	"github.com/KEBABSELLER6/rwe/internal/service"
)

func pathID(r *http.Request, kind string) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		return uuid.Nil(), fmt.Errorf("%w: invalid %s id %q", service.ErrInvalidInput, kind, r.PathValue("id"))
	}
	return id, nil
}

func requireContentType(r *http.Request, want string) error {
	got, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || got != want {
		return fmt.Errorf("%w: content-type must be %s", service.ErrInvalidInput, want)
	}
	return nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, limit int64, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)

	if err := requireContentType(r, "application/json"); err != nil {
		return err
	}

	if err := json.UnmarshalRead(r.Body, v, json.RejectUnknownMembers(true)); err != nil {
		return fmt.Errorf("%w: invalid json: %w", service.ErrInvalidInput, err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.MarshalWrite(w, v)
}
