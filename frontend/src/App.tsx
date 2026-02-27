import type { RouteSectionProps } from "@solidjs/router";
import { useLocation } from "@solidjs/router";
import { createResource, ErrorBoundary, type JSX, Show } from "solid-js";

import { api } from "~/api/client";
import { createCodeForgeWS } from "~/api/websocket";
import { AuthProvider, useAuth } from "~/components/AuthProvider";
import { CommandPalette } from "~/components/CommandPalette";
import { OfflineBanner } from "~/components/OfflineBanner";
import { RouteGuard } from "~/components/RouteGuard";
import { ThemeProvider, ThemeToggle } from "~/components/ThemeProvider";
import { ToastProvider } from "~/components/Toast";
import { I18nProvider, useI18n } from "~/i18n";
import { LocaleSwitcher } from "~/i18n/LocaleSwitcher";
import { Button, NavLink, Sidebar, StatusDot } from "~/ui";

// ---------------------------------------------------------------------------
// Error fallback (rendered when an uncaught error bubbles up)
// ---------------------------------------------------------------------------

function ErrorFallback(props: { error: unknown; reset: () => void }): JSX.Element {
  const { t } = useI18n();
  const message = () => (props.error instanceof Error ? props.error.message : String(props.error));

  return (
    <div class="flex h-screen items-center justify-center bg-cf-bg-primary" role="alert">
      <div class="max-w-md rounded-lg border border-red-200 bg-cf-bg-surface p-8 text-center shadow-md dark:border-red-800">
        <h1 class="mb-2 text-lg font-bold text-red-700 dark:text-red-400">
          {t("app.error.title")}
        </h1>
        <p class="mb-4 text-sm text-cf-text-secondary">{message()}</p>
        <Button variant="primary" onClick={() => props.reset()}>
          {t("app.error.retry")}
        </Button>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Inner shell (has access to I18n / Theme / Toast contexts)
// ---------------------------------------------------------------------------

function UserInfo(): JSX.Element {
  const { t } = useI18n();
  const { user, isAuthenticated, logout } = useAuth();

  return (
    <Show when={isAuthenticated()}>
      <div class="flex items-center justify-between border-b border-cf-border px-4 py-2 text-xs">
        <div class="truncate text-cf-text-secondary" title={user()?.email ?? ""}>
          {user()?.name ?? ""}{" "}
          <span class="rounded bg-cf-bg-surface-alt px-1 py-0.5 text-[10px] font-medium uppercase">
            {user()?.role ?? ""}
          </span>
        </div>
        <button
          type="button"
          onClick={() => void logout()}
          class="ml-2 text-cf-text-muted hover:text-red-500 dark:hover:text-red-400"
          title={t("auth.logout")}
        >
          {t("auth.logout")}
        </button>
      </div>
    </Show>
  );
}

function AppShell(props: {
  health: ReturnType<typeof createResource<{ status: string }>>[0];
  connected: () => boolean;
  children: JSX.Element;
}) {
  const { t } = useI18n();

  return (
    <>
      <CommandPalette />
      <a href="#main-content" class="skip-link">
        {t("app.skip")}
      </a>
      <div class="flex h-screen flex-col bg-cf-bg-primary text-cf-text-primary">
        <OfflineBanner wsConnected={props.connected} />

        <div class="flex flex-1 overflow-hidden">
          <Sidebar>
            <Sidebar.Header>
              <h1 class="text-xl font-bold">{t("app.title")}</h1>
              <p class="mt-1 text-xs text-cf-text-muted">{t("app.version")}</p>
            </Sidebar.Header>
            <UserInfo />

            <Sidebar.Nav>
              <NavLink href="/" end>
                {t("app.nav.dashboard")}
              </NavLink>
              <NavLink href="/costs">{t("app.nav.costs")}</NavLink>
              <NavLink href="/models">{t("app.nav.models")}</NavLink>
              <NavLink href="/modes">{t("app.nav.modes")}</NavLink>
              <NavLink href="/activity">{t("app.nav.activity")}</NavLink>
              <NavLink href="/knowledge-bases">{t("kb.title")}</NavLink>
              <NavLink href="/scopes">{t("app.nav.scopes")}</NavLink>
              <NavLink href="/teams">{t("app.nav.teams")}</NavLink>
              <NavLink href="/mcp">{t("app.nav.mcp")}</NavLink>
              <NavLink href="/prompts">{t("app.nav.prompts")}</NavLink>
              <NavLink href="/settings">{t("app.nav.settings")}</NavLink>
              <NavLink href="/benchmarks">{t("app.nav.benchmarks")}</NavLink>
            </Sidebar.Nav>

            <Sidebar.Footer>
              <div class="flex items-center gap-2">
                <ThemeToggle />
                <LocaleSwitcher />
              </div>
              <div
                class="mt-2 flex items-center gap-2 text-xs text-cf-text-muted"
                aria-live="polite"
              >
                <StatusDot
                  color={props.connected() ? "var(--cf-status-idle)" : "var(--cf-status-error)"}
                />
                <span>
                  {t("app.ws.label", {
                    status: props.connected() ? t("app.ws.connected") : t("app.ws.disconnected"),
                  })}
                </span>
              </div>
              <Show when={props.health()}>
                <div
                  class="mt-1 flex items-center gap-2 text-xs text-cf-text-muted"
                  aria-live="polite"
                >
                  <StatusDot color="var(--cf-status-idle)" />
                  <span>{t("app.api.label", { status: props.health()?.status ?? "" })}</span>
                </div>
              </Show>
            </Sidebar.Footer>
          </Sidebar>

          <main id="main-content" class="flex-1 overflow-auto p-6">
            {props.children}
          </main>
        </div>
      </div>
    </>
  );
}

// ---------------------------------------------------------------------------
// App shell
// ---------------------------------------------------------------------------

/** Inner component rendered inside AuthProvider so the WS has access to the auth token. */
function AuthenticatedApp(props: { children: JSX.Element }): JSX.Element {
  const [health] = createResource(() => api.health.check());
  const { connected } = createCodeForgeWS();
  const location = useLocation();

  const isLoginPage = (): boolean =>
    location.pathname === "/login" || location.pathname === "/change-password";

  return (
    <ToastProvider>
      <Show when={!isLoginPage()} fallback={props.children}>
        <RouteGuard>
          <AppShell health={health} connected={connected}>
            {props.children}
          </AppShell>
        </RouteGuard>
      </Show>
    </ToastProvider>
  );
}

export default function App(props: RouteSectionProps) {
  return (
    <I18nProvider>
      <ErrorBoundary fallback={(err, reset) => <ErrorFallback error={err} reset={reset} />}>
        <ThemeProvider>
          <AuthProvider>
            <AuthenticatedApp>{props.children}</AuthenticatedApp>
          </AuthProvider>
        </ThemeProvider>
      </ErrorBoundary>
    </I18nProvider>
  );
}
