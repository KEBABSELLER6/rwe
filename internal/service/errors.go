package service

import "errors"

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrTooLarge     = errors.New("too large")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
)
