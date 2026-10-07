package device

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("already exists")
	ErrInvalidInput = errors.New("invalid input")
)
