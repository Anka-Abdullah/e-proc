// RBAC mirror of pkg/auth/auth.go. The UI hides what a role cannot do; the
// services still enforce it, so this is usability, not security.

import type { Role } from "./types";

export type Permission =
  | "vendor:read"
  | "vendor:write"
  | "tender:read"
  | "tender:write"
  | "tender:publish"
  | "bid:read"
  | "bid:submit"
  | "po:read"
  | "po:write"
  | "approval:read"
  | "approval:act"
  | "audit:read"
  | "report:read"
  | "user:write";

const rolePermissions: Record<Role, Permission[]> = {
  admin: [
    "vendor:read", "vendor:write",
    "tender:read", "tender:write", "tender:publish",
    "bid:read", "bid:submit",
    "po:read", "po:write",
    "approval:read", "approval:act",
    "audit:read", "report:read", "user:write",
  ],
  procurement_officer: [
    "vendor:read", "vendor:write",
    "tender:read", "tender:write", "tender:publish",
    "bid:read",
    "po:read", "po:write",
    "approval:read",
    "audit:read", "report:read",
  ],
  approver: [
    "vendor:read",
    "tender:read",
    "bid:read",
    "po:read",
    "approval:read", "approval:act",
    "audit:read", "report:read",
  ],
  vendor: ["tender:read", "bid:read", "bid:submit", "po:read"],
};

/** can reports whether a role holds a permission. Unknown roles get nothing. */
export function can(role: Role | undefined, permission: Permission): boolean {
  if (!role) return false;
  return (rolePermissions[role] ?? []).includes(permission);
}

/** roleLabel is the human label shown in the UI. */
export function roleLabel(role: Role | undefined): string {
  switch (role) {
    case "admin":
      return "Administrator";
    case "procurement_officer":
      return "Procurement Officer";
    case "approver":
      return "Approver";
    case "vendor":
      return "Vendor";
    default:
      return "Tidak dikenal";
  }
}
