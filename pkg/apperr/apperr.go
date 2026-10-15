// Package apperr defines the domain error kinds every service returns, so HTTP
// and gRPC layers can map them to status codes in one place.
package apperr

import (
	"errors"
	"fmt"
)

// ErrNotFound is returned when an entity does not exist for the tenant.
var ErrNotFound = errors.New("not found")

// Conflict reports a uniqueness or state violation (HTTP 409).
type Conflict struct {
	Message string
}

func (e *Conflict) Error() string { return e.Message }

// NewConflict builds a Conflict error.
func NewConflict(format string, args ...any) *Conflict {
	return &Conflict{Message: fmt.Sprintf(format, args...)}
}

// Validation carries per-field messages (HTTP 422).
type Validation struct {
	Fields map[string]string
}

func (e *Validation) Error() string { return "validation failed" }

// NewValidation builds a Validation error from field messages.
func NewValidation(fields map[string]string) *Validation {
	return &Validation{Fields: fields}
}

// Forbidden reports an authorisation failure at the domain level (HTTP 403).
type Forbidden struct {
	Message string
}

func (e *Forbidden) Error() string { return e.Message }

// NewForbidden builds a Forbidden error.
func NewForbidden(format string, args ...any) *Forbidden {
	return &Forbidden{Message: fmt.Sprintf(format, args...)}
}

// IsNotFound reports whether err wraps ErrNotFound.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// AsValidation extracts a *Validation error, if any.
func AsValidation(err error) (*Validation, bool) {
	var v *Validation
	if errors.As(err, &v) {
		return v, true
	}
	return nil, false
}

// AsConflict extracts a *Conflict error, if any.
func AsConflict(err error) (*Conflict, bool) {
	var c *Conflict
	if errors.As(err, &c) {
		return c, true
	}
	return nil, false
}

// AsForbidden extracts a *Forbidden error, if any.
func AsForbidden(err error) (*Forbidden, bool) {
	var f *Forbidden
	if errors.As(err, &f) {
		return f, true
	}
	return nil, false
}
