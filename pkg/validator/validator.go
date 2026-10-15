// Package validator gives handlers the small, dependency-free validation
// helpers the API needs, with per-field error reporting.
package validator

import (
	"fmt"
	"net/mail"
	"strings"
)

// Errors collects per-field validation failures.
type Errors map[string]string

// Any reports whether anything failed.
func (e Errors) Any() bool { return len(e) > 0 }

// Add records a field error once; the first message for a field wins.
func (e Errors) Add(field, message string) {
	if _, exists := e[field]; !exists {
		e[field] = message
	}
}

// Required records an error when value is blank.
func (e Errors) Required(field, value string) {
	if strings.TrimSpace(value) == "" {
		e.Add(field, field+" is required")
	}
}

// Email records an error when value is not a valid address.
func (e Errors) Email(field, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	if _, err := mail.ParseAddress(value); err != nil {
		e.Add(field, field+" must be a valid email address")
	}
}

// MinLen records an error when value is shorter than n runes.
func (e Errors) MinLen(field, value string, n int) {
	if len([]rune(strings.TrimSpace(value))) < n {
		e.Add(field, fmt.Sprintf("%s must be at least %d characters", field, n))
	}
}

// MaxLen records an error when value is longer than n runes.
func (e Errors) MaxLen(field, value string, n int) {
	if len([]rune(value)) > n {
		e.Add(field, fmt.Sprintf("%s must be at most %d characters", field, n))
	}
}

// OneOf records an error when value is not in allowed (case-insensitive).
func (e Errors) OneOf(field, value string, allowed ...string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	for _, candidate := range allowed {
		if strings.EqualFold(strings.TrimSpace(value), candidate) {
			return
		}
	}
	e.Add(field, fmt.Sprintf("%s must be one of: %s", field, strings.Join(allowed, ", ")))
}

// Positive records an error when value is not greater than zero.
func (e Errors) Positive(field string, value float64) {
	if value <= 0 {
		e.Add(field, field+" must be greater than zero")
	}
}

// NonNegative records an error when value is below zero.
func (e Errors) NonNegative(field string, value float64) {
	if value < 0 {
		e.Add(field, field+" must not be negative")
	}
}
