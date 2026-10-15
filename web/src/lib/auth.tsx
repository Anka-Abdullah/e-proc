"use client";

// Auth context: holds the signed-in user, exposes login/logout, and restores the
// session from the stored token on first render.

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";

import { ApiError, api, getToken, setToken } from "./api";
import type { Role, User } from "./types";

interface AuthState {
  user: User | null;
  loading: boolean;
  error: string | null;
  login: (input: { tenantId: string; email: string; password: string }) => Promise<User>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
  role: Role | undefined;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    if (!getToken()) {
      setUser(null);
      setLoading(false);
      return;
    }
    try {
      const current = await api.me();
      setUser(current);
      setError(null);
    } catch (err) {
      // A dead token must not keep the UI in a signed-in-looking state.
      if (err instanceof ApiError && err.isAuth) {
        setToken(null);
        setUser(null);
      } else if (err instanceof Error) {
        setError(err.message);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    let active = true;

    // Yield before touching state: the effect body must only start the restore,
    // not update state synchronously (cascading renders otherwise).
    const restore = async () => {
      await Promise.resolve();
      if (!active) return;
      await refresh();
    };
    void restore();

    return () => {
      active = false;
    };
  }, [refresh]);

  const login = useCallback(
    async (input: { tenantId: string; email: string; password: string }) => {
      setError(null);
      const result = await api.login({
        tenant_id: input.tenantId,
        email: input.email,
        password: input.password,
      });
      setToken(result.token);
      setUser(result.user);
      return result.user;
    },
    [],
  );

  const logout = useCallback(async () => {
    try {
      await api.logout();
    } catch {
      // Revocation is best-effort: clearing the local token is what matters.
    }
    setToken(null);
    setUser(null);
  }, []);

  const value = useMemo<AuthState>(
    () => ({ user, loading, error, login, logout, refresh, role: user?.role }),
    [user, loading, error, login, logout, refresh],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

/** useAuth returns the auth context, failing loudly outside the provider. */
export function useAuth(): AuthState {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used inside <AuthProvider>");
  }
  return context;
}
