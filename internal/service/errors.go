package service

import "errors"

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrTooLarge     = errors.New("too large")
)
