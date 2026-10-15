// Package account owns users and authentication: credentials, login, token
// issuance and session lifecycle. It is the only service that sees password
// hashes.
package account

import (
	"strings"
	"time"

	"github.com/ank/e-proc/pkg/auth"
	"github.com/ank/e-proc/pkg/validator"
)

// Status is the lifecycle state of a user account.
type Status string

// Account states.
const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
)

// User is a platform account. PasswordHash is never serialised to JSON.
type User struct {
	ID           string     `bson:"_id" json:"id"`
	TenantID     string     `bson:"tenant_id" json:"tenant_id"`
	Email        string     `bson:"email" json:"email"`
	Name         string     `bson:"name" json:"name"`
	PasswordHash string     `bson:"password_hash" json:"-"`
	Role         auth.Role  `bson:"role" json:"role"`
	Status       Status     `bson:"status" json:"status"`
	LastLoginAt  *time.Time `bson:"last_login_at,omitempty" json:"last_login_at,omitempty"`
	CreatedAt    time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt    time.Time  `bson:"updated_at" json:"updated_at"`
	CreatedBy    string     `bson:"created_by" json:"created_by"`
}

// RegisterInput is the payload for creating an account.
type RegisterInput struct {
	TenantID string `json:"tenant_id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// LoginInput is the payload for POST /auth/login.
type LoginInput struct {
	TenantID string `json:"tenant_id"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResult is what a successful login returns.
type LoginResult struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      User      `json:"user"`
}

// MinPasswordLength is the smallest accepted password.
const MinPasswordLength = 10

// Validate checks a registration payload.
func (in RegisterInput) Validate() validator.Errors {
	errs := validator.Errors{}
	errs.Required("tenant_id", in.TenantID)
	errs.Required("email", in.Email)
	errs.Email("email", in.Email)
	errs.Required("name", in.Name)
	errs.MaxLen("name", in.Name, 120)
	errs.MinLen("password", in.Password, MinPasswordLength)
	errs.OneOf("role", in.Role,
		string(auth.RoleAdmin), string(auth.RoleProcurement), string(auth.RoleApprover), string(auth.RoleVendor))
	return errs
}

// Validate checks a login payload.
func (in LoginInput) Validate() validator.Errors {
	errs := validator.Errors{}
	errs.Required("tenant_id", in.TenantID)
	errs.Required("email", in.Email)
	errs.Email("email", in.Email)
	errs.Required("password", in.Password)
	return errs
}

// Validate checks a login payload.
func (in *LoginInput) Normalize() {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.TenantID = strings.TrimSpace(in.TenantID)
}

// Normalize trims and lower-cases the registration fields.
func (in *RegisterInput) Normalize() {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	in.TenantID = strings.TrimSpace(in.TenantID)
	in.Role = strings.ToLower(strings.TrimSpace(in.Role))
}

// IsValidRole reports whether the role string is one of the platform roles.
func IsValidRole(role string) bool {
	switch auth.Role(strings.ToLower(role)) {
	case auth.RoleAdmin, auth.RoleProcurement, auth.RoleApprover, auth.RoleVendor:
		return true
	default:
		return false
	}
}

// IsActive reports whether the account may log in.
func (u *User) IsActive() bool { return u != nil && u.Status == StatusActive }
