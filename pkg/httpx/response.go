// Package httpx standardises REST request decoding and response envelopes so
// every service returns the same JSON shape.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// maxBodyBytes caps request bodies to keep a single client from exhausting memory.
const maxBodyBytes = 1 << 20 // 1 MiB

// Meta carries pagination information for list endpoints.
type Meta struct {
	Page    int   `json:"page"`
	PerPage int   `json:"per_page"`
	Total   int64 `json:"total"`
}

// APIError is the machine-readable error body.
type APIError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

// Envelope is the response wrapper: exactly one of data or error is present.
type Envelope struct {
	Data  any       `json:"data,omitempty"`
	Meta  *Meta     `json:"meta,omitempty"`
	Error *APIError `json:"error,omitempty"`
}

// Error codes shared by the services.
const (
	CodeBadRequest   = "bad_request"
	CodeUnauthorized = "unauthorized"
	CodeForbidden    = "forbidden"
	CodeNotFound     = "not_found"
	CodeConflict     = "conflict"
	CodeValidation   = "validation_failed"
	CodeInternal     = "internal_error"
	CodeRateLimited  = "rate_limited"
)

// JSON writes any payload with the given status code.
func JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// The status is already written; nothing left but to stop.
		return
	}
}

// OK writes a 200 response.
func OK(w http.ResponseWriter, data any) { JSON(w, http.StatusOK, Envelope{Data: data}) }

// Created writes a 201 response.
func Created(w http.ResponseWriter, data any) { JSON(w, http.StatusCreated, Envelope{Data: data}) }

// List writes a page of results together with its pagination meta.
func List(w http.ResponseWriter, data any, meta Meta) {
	JSON(w, http.StatusOK, Envelope{Data: data, Meta: &meta})
}

// NoContent writes a 204 response.
func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// Fail writes an error envelope.
func Fail(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, Envelope{Error: &APIError{Code: code, Message: message}})
}

// FailValidation writes a 422 with per-field messages.
func FailValidation(w http.ResponseWriter, details map[string]string) {
	JSON(w, http.StatusUnprocessableEntity, Envelope{
		Error: &APIError{Code: CodeValidation, Message: "validation failed", Details: details},
	})
}

// Decode reads a JSON body into dest, rejecting unknown fields and oversized
// or malformed payloads before any business logic sees them.
func Decode(r *http.Request, dest any) error {
	if r.Body == nil {
		return errors.New("request body is empty")
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	if decoder.More() {
		return errors.New("request body must contain exactly one JSON object")
	}
	return nil
}

// PageParams reads page/per_page with sane bounds (default 1/20, max 100).
func PageParams(r *http.Request) (page, perPage int) {
	query := r.URL.Query()
	page = atoiDefault(query.Get("page"), 1)
	if page < 1 {
		page = 1
	}
	perPage = atoiDefault(query.Get("per_page"), 20)
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	return page, perPage
}

// Skip computes the Mongo skip value for a page.
func Skip(page, perPage int) int64 { return int64((page - 1) * perPage) }

// BearerToken extracts the token from an Authorization header.
func BearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", false
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	token := strings.TrimSpace(parts[1])
	return token, token != ""
}

// IdempotencyKey returns the caller-supplied idempotency key, if any.
func IdempotencyKey(r *http.Request) (string, bool) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	return key, key != ""
}

func atoiDefault(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}
