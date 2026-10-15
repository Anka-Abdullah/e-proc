// Package auth issues and verifies JWT access tokens and answers RBAC
// questions for the platform's four roles.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Role is one of the platform's actors.
type Role string

// The four MVP roles.
const (
	RoleAdmin       Role = "admin"
	RoleProcurement Role = "procurement_officer"
	RoleApprover    Role = "approver"
	RoleVendor      Role = "vendor"
)

// Permission is a named capability checked by the RBAC middleware.
type Permission string

// Permissions used across the API.
const (
	PermVendorRead    Permission = "vendor:read"
	PermVendorWrite   Permission = "vendor:write"
	PermTenderRead    Permission = "tender:read"
	PermTenderWrite   Permission = "tender:write"
	PermTenderPublish Permission = "tender:publish"
	PermBidRead       Permission = "bid:read"
	PermBidSubmit     Permission = "bid:submit"
	PermPORead        Permission = "po:read"
	PermPOWrite       Permission = "po:write"
	PermApprovalRead  Permission = "approval:read"
	PermApprovalAct   Permission = "approval:act"
	PermAuditRead     Permission = "audit:read"
	PermReportRead    Permission = "report:read"
	PermUserWrite     Permission = "user:write"
)

// rolePermissions is the authorisation matrix. Anything not listed is denied,
// so a new permission is opt-in rather than accidentally public.
var rolePermissions = map[Role][]Permission{
	RoleAdmin: {
		PermVendorRead, PermVendorWrite,
		PermTenderRead, PermTenderWrite, PermTenderPublish,
		PermBidRead, PermBidSubmit,
		PermPORead, PermPOWrite,
		PermApprovalRead, PermApprovalAct,
		PermAuditRead, PermReportRead, PermUserWrite,
	},
	RoleProcurement: {
		PermVendorRead, PermVendorWrite,
		PermTenderRead, PermTenderWrite, PermTenderPublish,
		PermBidRead,
		PermPORead, PermPOWrite,
		PermApprovalRead,
		PermAuditRead, PermReportRead,
	},
	RoleApprover: {
		PermVendorRead,
		PermTenderRead,
		PermBidRead,
		PermPORead,
		PermApprovalRead, PermApprovalAct,
		PermAuditRead, PermReportRead,
	},
	RoleVendor: {
		PermTenderRead,
		PermBidRead, PermBidSubmit,
		PermPORead,
	},
}

// Claims is the JWT payload carried by every authenticated request.
type Claims struct {
	UserID   string `json:"uid"`
	Email    string `json:"email"`
	Role     Role   `json:"role"`
	TenantID string `json:"tenant,omitempty"`
	jwt.RegisteredClaims
}

// Can reports whether the claims' role holds perm.
func (c *Claims) Can(perm Permission) bool {
	if c == nil {
		return false
	}
	for _, granted := range rolePermissions[c.Role] {
		if granted == perm {
			return true
		}
	}
	return false
}

// IsAdmin reports whether the caller is an administrator.
func (c *Claims) IsAdmin() bool { return c != nil && c.Role == RoleAdmin }

// Issue signs a token for the given subject with the configured TTL.
func Issue(secret string, ttl time.Duration, claims Claims) (string, error) {
	if secret == "" {
		return "", errors.New("jwt secret is empty")
	}
	now := time.Now()
	claims.RegisteredClaims = jwt.RegisteredClaims{
		Subject:   claims.UserID,
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

// Parse verifies a token's signature and expiry and returns its claims.
func Parse(secret, token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// HashPassword and CheckPassword live in user_service; this package only
// handles tokens.
