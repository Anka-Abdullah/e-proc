"use client";

// Application shell: sidebar navigation (filtered by the signed-in role) and a
// topbar showing who is signed in. Pages render inside <Shell>.

import { clsx } from "clsx";
import {
  BadgeCheck,
  ClipboardList,
  FileSpreadsheet,
  Gavel,
  LayoutDashboard,
  LogOut,
  ScrollText,
  ShoppingCart,
  Users,
} from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import type { ReactNode } from "react";
import { useEffect } from "react";

import { useAuth } from "@/lib/auth";
import { can, roleLabel, type Permission } from "@/lib/rbac";
import { Button } from "@/components/ui";

interface NavItem {
  href: string;
  label: string;
  icon: ReactNode;
  permission: Permission;
}

// The nav is data, so hiding an entry is a permission check rather than a
// hand-written condition inside the JSX.
const NAV: NavItem[] = [
  { href: "/dashboard", label: "Dashboard", icon: <LayoutDashboard size={16} />, permission: "report:read" },
  { href: "/vendors", label: "Vendor", icon: <Users size={16} />, permission: "vendor:read" },
  { href: "/tenders", label: "Tender", icon: <Gavel size={16} />, permission: "tender:read" },
  { href: "/bids", label: "Penawaran", icon: <ClipboardList size={16} />, permission: "bid:read" },
  { href: "/purchase-orders", label: "Purchase Order", icon: <ShoppingCart size={16} />, permission: "po:read" },
  { href: "/approvals", label: "Approval", icon: <BadgeCheck size={16} />, permission: "approval:read" },
  { href: "/audit-logs", label: "Audit Log", icon: <ScrollText size={16} />, permission: "audit:read" },
  { href: "/reports", label: "Laporan", icon: <FileSpreadsheet size={16} />, permission: "report:read" },
];

export function Shell({ children }: { children: ReactNode }) {
  const { user, loading, logout } = useAuth();
  const pathname = usePathname();
  const router = useRouter();

  // Protected pages must not render content for an anonymous visitor, even for
  // a moment: redirect as soon as the auth state settles.
  useEffect(() => {
    if (!loading && !user) {
      router.replace("/login");
    }
  }, [loading, user, router]);

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center text-sm text-slate-500">
        Memeriksa sesi…
      </div>
    );
  }
  if (!user) {
    return null;
  }

  const items = NAV.filter((item) => can(user.role, item.permission));

  return (
    <div className="flex min-h-screen bg-slate-50">
      <aside className="hidden w-56 shrink-0 border-r border-slate-200 bg-white md:block">
        <div className="border-b border-slate-100 px-4 py-4">
          <p className="text-sm font-semibold text-slate-900">E-Procurement</p>
          <p className="text-[11px] text-slate-500">{user.tenant_id}</p>
        </div>
        <nav className="space-y-0.5 p-2">
          {items.map((item) => {
            const active = pathname === item.href || pathname.startsWith(item.href + "/");
            return (
              <Link
                key={item.href}
                href={item.href}
                className={clsx(
                  "flex items-center gap-2 rounded-md px-3 py-2 text-sm transition",
                  active ? "bg-slate-900 text-white" : "text-slate-600 hover:bg-slate-100",
                )}
              >
                {item.icon}
                {item.label}
              </Link>
            );
          })}
        </nav>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex items-center justify-between border-b border-slate-200 bg-white px-4 py-3">
          <div className="min-w-0">
            <p className="truncate text-sm font-medium text-slate-900">{user.name}</p>
            <p className="text-[11px] text-slate-500">
              {roleLabel(user.role)} · {user.email}
            </p>
          </div>
          <Button
            variant="secondary"
            size="sm"
            onClick={async () => {
              await logout();
              router.replace("/login");
            }}
          >
            <LogOut size={14} />
            Keluar
          </Button>
        </header>

        {/* Mobile nav: the sidebar is hidden on small screens. */}
        <nav className="flex gap-1 overflow-x-auto border-b border-slate-200 bg-white px-2 py-2 md:hidden">
          {items.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className={clsx(
                "whitespace-nowrap rounded-md px-2.5 py-1.5 text-xs",
                pathname === item.href ? "bg-slate-900 text-white" : "text-slate-600 hover:bg-slate-100",
              )}
            >
              {item.label}
            </Link>
          ))}
        </nav>

        <main className="mx-auto w-full max-w-6xl flex-1 space-y-4 p-4">{children}</main>
      </div>
    </div>
  );
}
