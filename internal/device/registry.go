package device

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("already exists")
	ErrInvalidInput = errors.New("invalid input")
	// ErrUnauthenticated means the caller proved neither enrollment nor device identity.
	ErrUnauthenticated = errors.New("unauthenticated")
	// ErrRefreshTooEarly means the token is not yet close enough to expiry.
	ErrRefreshTooEarly = errors.New("token refresh too early")
)
