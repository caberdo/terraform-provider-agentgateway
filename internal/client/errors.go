package client

import (
	"errors"
	"fmt"
)

var (
	// ErrNotFound is returned when the requested config resource does not exist.
	ErrNotFound = errors.New("resource not found")
	// ErrConflict is returned when the server reports a generation or ownership
	// conflict (HTTP 409).
	ErrConflict = errors.New("conflict")
	// ErrInvalid is returned for HTTP 400/422 and malformed client requests.
	ErrInvalid = errors.New("invalid request")
	// ErrReadOnly is returned when the gateway storage is configured as read-only.
	ErrReadOnly = errors.New("gateway storage is read-only")
	// ErrTransient covers HTTP 429/502/503/504 and transport-level failures.
	ErrTransient = errors.New("transient failure")
	// ErrServer covers unexpected 5xx responses and malformed successful bodies.
	ErrServer = errors.New("gateway error")
)

// APIError carries the status code and the server's message for diagnostics.
type APIError struct {
	Status  int
	Message string
	Kind    error
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Status == 0 {
		return e.Message
	}
	return fmt.Sprintf("%d: %s", e.Status, e.Message)
}

// Is lets errors.Is(err, ErrConflict) succeed for an APIError whose Kind is
// ErrConflict (and likewise for the other sentinels).
func (e *APIError) Is(target error) bool {
	if e.Kind == nil {
		return false
	}
	return e.Kind == target
}
