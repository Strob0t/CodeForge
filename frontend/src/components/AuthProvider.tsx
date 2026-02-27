import { useNavigate } from "@solidjs/router";
import { createContext, createEffect, createSignal, type JSX, onMount, useContext } from "solid-js";

import { clearCache } from "~/api/cache";
import { api, setAccessTokenGetter } from "~/api/client";
import type { User, UserRole } from "~/api/types";

interface AuthContextValue {
  user: () => User | null;
  isAuthenticated: () => boolean;
  /** True while the initial session restore (refresh cookie) is in progress. */
  initializing: () => boolean;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  hasRole: (...roles: UserRole[]) => boolean;
}

const AuthContext = createContext<AuthContextValue>();

export function AuthProvider(props: { children: JSX.Element }): JSX.Element {
  const navigate = useNavigate();
  const [user, setUser] = createSignal<User | null>(null);
  const [accessToken, setAccessToken] = createSignal<string | null>(null);
  const [initializing, setInitializing] = createSignal(true);

  // Wire up token getter for API client — createEffect tracks the signal
  // and re-registers the getter whenever the token changes.
  createEffect(() => {
    const token = accessToken();
    setAccessTokenGetter(() => token);
  });

  const scheduleRefresh = (expiresIn: number): void => {
    // Refresh 60s before expiry, minimum 10s.
    const delay = Math.max((expiresIn - 60) * 1000, 10_000);
    setTimeout(() => {
      void refreshTokens();
    }, delay);
  };

  const refreshTokens = async (): Promise<boolean> => {
    try {
      const resp = await api.auth.refresh();
      setAccessToken(resp.access_token);
      setUser(resp.user);
      scheduleRefresh(resp.expires_in);
      return true;
    } catch {
      setAccessToken(null);
      setUser(null);
      return false;
    }
  };

  const login = async (email: string, password: string): Promise<void> => {
    const resp = await api.auth.login({ email, password });
    setAccessToken(resp.access_token);
    setUser(resp.user);
    scheduleRefresh(resp.expires_in);
  };

  const logout = async (): Promise<void> => {
    try {
      await api.auth.logout();
    } finally {
      setAccessToken(null);
      setUser(null);
      clearCache();
      navigate("/login", { replace: true });
    }
  };

  const hasRole = (...roles: UserRole[]): boolean => {
    const u = user();
    if (!u) return false;
    return roles.includes(u.role);
  };

  const isAuthenticated = (): boolean => user() !== null;

  // Try to restore session via refresh cookie on mount.
  onMount(async () => {
    await refreshTokens();
    setInitializing(false);
  });

  const value: AuthContextValue = {
    user,
    isAuthenticated,
    initializing,
    login,
    logout,
    hasRole,
  };

  return <AuthContext.Provider value={value}>{props.children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return ctx;
}
