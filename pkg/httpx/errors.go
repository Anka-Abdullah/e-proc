package httpx

import (
	"errors"
	"net/http"

	"github.com/ank/e-proc/pkg/apperr"
)

// FromError maps a domain error to the HTTP status code and error envelope
// used by every service. Unknown errors become a 500 without leaking details.
func FromError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}

	if errors.Is(err, apperr.ErrNotFound) {
		Fail(w, http.StatusNotFound, CodeNotFound, "resource not found")
		return
	}
	if v, ok := apperr.AsValidation(err); ok {
		FailValidation(w, v.Fields)
		return
	}
	if c, ok := apperr.AsConflict(err); ok {
		Fail(w, http.StatusConflict, CodeConflict, c.Message)
		return
	}
	if f, ok := apperr.AsForbidden(err); ok {
		Fail(w, http.StatusForbidden, CodeForbidden, f.Message)
		return
	}
	Fail(w, http.StatusInternalServerError, CodeInternal, "internal server error")
}
