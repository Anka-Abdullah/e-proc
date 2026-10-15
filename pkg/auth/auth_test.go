package auth

import (
	"testing"
	"time"
)

const testSecret = "test-secret"

func TestIssueAndParse(t *testing.T) {
	token, err := Issue(testSecret, time.Hour, Claims{
		UserID:   "u-1",
		Email:    "u-1@example.com",
		Role:     RoleProcurement,
		TenantID: "tenant-1",
	})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	claims, err := Parse(testSecret, token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if claims.UserID != "u-1" || claims.Role != RoleProcurement || claims.TenantID != "tenant-1" {
		t.Fatalf("claims round-trip mismatch: %+v", claims)
	}
}

func TestParseRejectsWrongSecret(t *testing.T) {
	token, err := Issue(testSecret, time.Hour, Claims{UserID: "u-1", Role: RoleAdmin})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	if _, err := Parse("different-secret", token); err == nil {
		t.Fatal("expected signature verification to fail")
	}
}

func TestParseRejectsExpiredToken(t *testing.T) {
	token, err := Issue(testSecret, -time.Minute, Claims{UserID: "u-1", Role: RoleAdmin})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	if _, err := Parse(testSecret, token); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestCan(t *testing.T) {
	tests := []struct {
		role Role
		perm Permission
		want bool
	}{
		{RoleAdmin, PermUserWrite, true},
		{RoleProcurement, PermTenderPublish, true},
		{RoleProcurement, PermApprovalAct, false},
		{RoleApprover, PermApprovalAct, true},
		{RoleApprover, PermVendorWrite, false},
		{RoleVendor, PermBidSubmit, true},
		{RoleVendor, PermPOWrite, false},
		{RoleVendor, PermAuditRead, false},
	}

	for _, tc := range tests {
		claims := &Claims{Role: tc.role}
		if got := claims.Can(tc.perm); got != tc.want {
			t.Errorf("role %s + %s: got %v, want %v", tc.role, tc.perm, got, tc.want)
		}
	}
}

func TestCanOnNilClaims(t *testing.T) {
	var claims *Claims
	if claims.Can(PermVendorRead) {
		t.Fatal("nil claims must not hold any permission")
	}
}

func TestUnknownRoleHasNoPermissions(t *testing.T) {
	claims := &Claims{Role: Role("intern")}
	if claims.Can(PermVendorRead) {
		t.Fatal("unknown roles must be denied")
	}
}
