package account

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/ank/e-proc/pkg/apperr"
	"github.com/ank/e-proc/pkg/auth"
	"github.com/ank/e-proc/pkg/cache"
)

// dummyHash is compared when the email is unknown, so login latency does not
// reveal whether an account exists.
var dummyHash = []byte("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy")

// Service owns authentication rules: password hashing, login, token issue and
// session revocation.
type Service struct {
	repo     *Repository
	cache    *cache.Client
	secret   string
	tokenTTL time.Duration
	log      *slog.Logger
}

// NewService builds the account service.
func NewService(repo *Repository, cacheClient *cache.Client, secret string, tokenTTL time.Duration, log *slog.Logger) *Service {
	return &Service{repo: repo, cache: cacheClient, secret: secret, tokenTTL: tokenTTL, log: log}
}

// EnsureIndexes is called once at boot.
func (s *Service) EnsureIndexes(ctx context.Context) error { return s.repo.EnsureIndexes(ctx) }

// Register creates an account with a bcrypt password hash.
func (s *Service) Register(ctx context.Context, in RegisterInput, actorID string) (*User, error) {
	in.Normalize()
	if errs := in.Validate(); errs.Any() {
		return nil, apperr.NewValidation(errs)
	}
	if len(in.Password) > 72 {
		// bcrypt silently truncates beyond 72 bytes; reject instead of accepting
		// a password whose tail is not checked.
		return nil, apperr.NewValidation(map[string]string{"password": "password must be at most 72 bytes"})
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	u := &User{
		TenantID:     in.TenantID,
		Email:        in.Email,
		Name:         in.Name,
		PasswordHash: string(hash),
		Role:         auth.Role(in.Role),
		Status:       StatusActive,
		CreatedBy:    actorID,
	}
	if err := s.repo.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// Login verifies credentials and issues a token backed by a session record.
func (s *Service) Login(ctx context.Context, in LoginInput, ip string) (*LoginResult, error) {
	in.Normalize()
	if errs := in.Validate(); errs.Any() {
		// Credential problems are deliberately vague: a validation error here
		// would tell an attacker which half was wrong.
		return nil, apperr.NewForbidden("invalid email or password")
	}

	user, err := s.repo.GetByEmail(ctx, in.TenantID, in.Email)
	if err != nil {
		if errors.Is(err, apperr.ErrNotFound) {
			// Constant-time comparison against a dummy hash to level the timing.
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(in.Password))
			return nil, apperr.NewForbidden("invalid email or password")
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)); err != nil {
		return nil, apperr.NewForbidden("invalid email or password")
	}
	if !user.IsActive() {
		return nil, apperr.NewForbidden("account is not active")
	}

	token, err := auth.Issue(s.secret, s.tokenTTL, auth.Claims{
		UserID:   user.ID,
		Email:    user.Email,
		Role:     user.Role,
		TenantID: user.TenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("issue token: %w", err)
	}

	if err := s.cache.StoreSession(ctx, token, user.ID, s.tokenTTL); err != nil {
		s.log.WarnContext(ctx, "session store failed", slog.String("error", err.Error()))
	}
	if err := s.repo.TouchLastLogin(ctx, user.TenantID, user.ID); err != nil {
		s.log.WarnContext(ctx, "last login update failed", slog.String("error", err.Error()))
	}
	s.log.InfoContext(ctx, "login succeeded",
		slog.String("user_id", user.ID), slog.String("tenant", user.TenantID), slog.String("ip", ip))

	return &LoginResult{Token: token, ExpiresAt: time.Now().UTC().Add(s.tokenTTL), User: *user}, nil
}

// Logout revokes the presented token so it stops working immediately.
func (s *Service) Logout(ctx context.Context, token string, claims *auth.Claims) error {
	if err := s.cache.RevokeToken(ctx, token, s.tokenTTL); err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	if err := s.cache.RevokeSession(ctx, token); err != nil {
		s.log.WarnContext(ctx, "session delete failed", slog.String("error", err.Error()))
	}
	if claims != nil {
		s.log.InfoContext(ctx, "logout", slog.String("user_id", claims.UserID))
	}
	return nil
}

// Me returns the account behind the current claims.
func (s *Service) Me(ctx context.Context, claims *auth.Claims) (*User, error) {
	return s.repo.GetByID(ctx, claims.TenantID, claims.UserID)
}
