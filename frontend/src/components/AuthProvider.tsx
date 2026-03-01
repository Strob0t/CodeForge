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
  /** True when the backend requires a password change before any other action. */
  mustChangePassword: () => boolean;
  /** True when the backend has no users yet and needs initial setup. */
  needsSetup: () => boolean;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  /** Change password and re-login to get a fresh token without the mcp flag. */
  changePassword: (oldPassword: string, newPassword: string) => Promise<void>;
  hasRole: (...roles: UserRole[]) => boolean;
}

const AuthContext = createContext<AuthContextValue>();

export function AuthProvider(props: { children: JSX.Element }): JSX.Element {
  const navigate = useNavigate();
  const [user, setUser] = createSignal<User | null>(null);
  const [accessToken, setAccessToken] = createSignal<string | null>(null);
  const [initializing, setInitializing] = createSignal(true);
  const [needsSetup, setNeedsSetup] = createSignal(false);

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

  const mustChangePassword = (): boolean => user()?.must_change_password === true;

  const login = async (email: string, password: string): Promise<void> => {
    const resp = await api.auth.login({ email, password });
    setAccessToken(resp.access_token);
    setUser(resp.user);
    scheduleRefresh(resp.expires_in);
  };

  const changePassword = async (oldPassword: string, newPassword: string): Promise<void> => {
    await api.auth.changePassword({ old_password: oldPassword, new_password: newPassword });
    // Re-login to get a fresh token without must_change_password flag.
    const u = user();
    if (u) {
      const resp = await api.auth.login({ email: u.email, password: newPassword });
      setAccessToken(resp.access_token);
      setUser(resp.user);
      scheduleRefresh(resp.expires_in);
    }
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
    try {
      const setupStatus = await api.auth.setupStatus();
      if (setupStatus.needs_setup) {
        setNeedsSetup(true);
        navigate("/setup", { replace: true });
        setInitializing(false);
        return;
      }
    } catch {
      // Setup status endpoint unavailable — proceed with normal auth flow.
    }

    await refreshTokens();
    setInitializing(false);
  });

  const value: AuthContextValue = {
    user,
    isAuthenticated,
    initializing,
    mustChangePassword,
    needsSetup,
    login,
    logout,
    changePassword,
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
