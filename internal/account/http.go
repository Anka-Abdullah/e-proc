package account

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ank/e-proc/pkg/auth"
	"github.com/ank/e-proc/pkg/httpx"
	"github.com/ank/e-proc/pkg/middleware"
)

// MountPublic registers routes that must work without a token: login is the
// only way to obtain one.
func (s *Service) MountPublic(r chi.Router) {
	r.Post("/auth/login", s.handleLogin)
}

// MountProtected registers the authenticated account routes.
func (s *Service) MountProtected(r chi.Router) {
	r.Get("/auth/me", s.handleMe)
	r.Post("/auth/logout", s.handleLogout)
	r.With(middleware.RequirePermission(auth.PermUserWrite)).Post("/auth/register", s.handleRegister)
}

type loginBody struct {
	TenantID string `json:"tenant_id"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body loginBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
		return
	}

	result, err := s.Login(r.Context(), LoginInput(body), clientIP(r))
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.OK(w, result)
}

func (s *Service) handleMe(w http.ResponseWriter, r *http.Request) {
	user, err := s.Me(r.Context(), middleware.ClaimsFrom(r.Context()))
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.OK(w, user)
}

func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request) {
	token, _ := httpx.BearerToken(r)
	if err := s.Logout(r.Context(), token, middleware.ClaimsFrom(r.Context())); err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.NoContent(w)
}

func (s *Service) handleRegister(w http.ResponseWriter, r *http.Request) {
	var in RegisterInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
		return
	}
	claims := middleware.ClaimsFrom(r.Context())
	if in.TenantID == "" && claims != nil {
		in.TenantID = claims.TenantID
	}

	user, err := s.Register(r.Context(), in, claims.UserID)
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.Created(w, user)
}

func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return forwarded
	}
	return r.RemoteAddr
}
