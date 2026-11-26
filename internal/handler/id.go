package handler

import (
	"fmt"
	"kebabseller6/rwe/internal/service"
	"net/http"
	"uuid"
)

func getId(r *http.Request) (uuid.UUID, error) {
	rawId := r.PathValue("id")
	id, err := uuid.Parse(rawId)
	if err != nil {
		return uuid.Nil(), fmt.Errorf("%w: invalid workspace id", service.ErrInvalidInput)
	}

	return id, nil
}
