import type { RouteSectionProps } from "@solidjs/router";
import { useLocation } from "@solidjs/router";
import { createEffect, createResource, ErrorBoundary, type JSX, onCleanup, Show } from "solid-js";

import { api } from "~/api/client";
import type { HealthStatus } from "~/api/types";
import { AuthProvider, useAuth } from "~/components/AuthProvider";
import { CommandPalette } from "~/components/CommandPalette";
import { ConfirmProvider } from "~/components/ConfirmProvider";
import { ConversationRunProvider, useConversationRuns } from "~/components/ConversationRunProvider";
import { OfflineBanner } from "~/components/OfflineBanner";
import { RouteGuard } from "~/components/RouteGuard";
import { SidebarProvider, useSidebar } from "~/components/SidebarProvider";
import { ThemeProvider, ThemeToggle } from "~/components/ThemeProvider";
import { ToastProvider } from "~/components/Toast";
import { useWebSocket, WebSocketProvider } from "~/components/WebSocketProvider";
import ChannelList from "~/features/channels/ChannelList";
import NotificationBell from "~/features/notifications/NotificationBell";
import { addNotification, getUnreadCount } from "~/features/notifications/notificationStore";
import { useBreakpoint } from "~/hooks/useBreakpoint";
import { I18nProvider, useI18n } from "~/i18n";
import { LocaleSwitcher } from "~/i18n/LocaleSwitcher";
import { ShortcutProvider } from "~/shortcuts";
import { Button, NavLink, Sidebar, StatusDot, Tooltip } from "~/ui";
import { NavSection } from "~/ui/layout";
import {
  ActivityIcon,
  BenchmarksIcon,
  CostsIcon,
  DashboardIcon,
  KnowledgeBaseIcon,
  McpIcon,
  ModelsIcon,
  PromptsIcon,
  SettingsIcon,
} from "~/ui/layout/NavIcons";
import { updateTabBadge } from "~/utils/tabBadge";

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

function AppShell(props: {
  health: ReturnType<typeof createResource<HealthStatus>>[0];
  connected: () => boolean;
  children: JSX.Element;
}) {
  const { t } = useI18n();
  const { user, isAuthenticated, logout } = useAuth();
  const { collapsed, openMobile } = useSidebar();
  const { isMobile } = useBreakpoint();
  const { activeRuns } = useConversationRuns();
  const { onAGUIEvent } = useWebSocket();

  // Subscribe to AG-UI events and feed the notification store
  const offPermission = onAGUIEvent("agui.permission_request", (ev) => {
    addNotification({
      type: "permission_request",
      title: "Approval Required",
      message: `Agent requests permission for: ${ev.tool || "action"}`,
      metadata: { run_id: ev.run_id, call_id: ev.call_id },
    });
  });

  const offRunFinished = onAGUIEvent("agui.run_finished", (ev) => {
    const failed = ev.status === "failed" || ev.status === "error";
    addNotification({
      type: failed ? "run_failed" : "run_complete",
      title: failed ? "Run Failed" : "Run Complete",
      message: ev.error ? `Run ${ev.run_id}: ${ev.error}` : `Run ${ev.run_id} ${ev.status}`,
    });
  });

  onCleanup(() => {
    offPermission();
    offRunFinished();
  });

  // Sync tab badge with unread notification count
  createEffect(() => {
    updateTabBadge(getUnreadCount());
  });

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
              <div class="flex items-center justify-between">
                <div>
                  <h1 class="text-xl font-bold">{t("app.title")}</h1>
                  <p class="mt-1 text-xs text-cf-text-muted">{t("app.version")}</p>
                </div>
                <NotificationBell />
              </div>
            </Sidebar.Header>

            <Sidebar.Nav>
              <NavSection>
                <NavLink href="/" end icon={<DashboardIcon />} label={t("app.nav.dashboard")}>
                  {t("app.nav.dashboard")}
                </NavLink>
                <NavLink href="/activity" icon={<ActivityIcon />} label={t("app.nav.activity")}>
                  {t("app.nav.activity")}
                </NavLink>
              </NavSection>
              <NavSection label={t("app.nav.section.ai")}>
                <NavLink href="/ai" icon={<ModelsIcon />} label={t("app.nav.aiConfig")}>
                  {t("app.nav.aiConfig")}
                </NavLink>
                <NavLink href="/prompts" icon={<PromptsIcon />} label={t("app.nav.prompts")}>
                  {t("app.nav.prompts")}
                </NavLink>
                <NavLink href="/mcp" icon={<McpIcon />} label={t("app.nav.mcp")}>
                  {t("app.nav.mcp")}
                </NavLink>
                <Show when={props.health()?.dev_mode}>
                  <NavLink
                    href="/benchmarks"
                    icon={<BenchmarksIcon />}
                    label={t("app.nav.benchmarks")}
                  >
                    {t("app.nav.benchmarks")}
                  </NavLink>
                </Show>
              </NavSection>
              <NavSection label={t("app.nav.section.knowledge")}>
                <NavLink
                  href="/knowledge"
                  icon={<KnowledgeBaseIcon />}
                  label={t("app.nav.knowledge")}
                >
                  {t("app.nav.knowledge")}
                </NavLink>
              </NavSection>
              <ChannelList />
              <NavSection label={t("app.nav.section.system")} bottom>
                <NavLink href="/costs" icon={<CostsIcon />} label={t("app.nav.costs")}>
                  {t("app.nav.costs")}
                </NavLink>
                <NavLink href="/settings" icon={<SettingsIcon />} label={t("app.nav.settings")}>
                  {t("app.nav.settings")}
                </NavLink>
              </NavSection>
            </Sidebar.Nav>

            <Sidebar.Footer>
              <Show
                when={!collapsed() || isMobile()}
                fallback={
                  <div class="flex flex-col items-center gap-1.5">
                    <Tooltip text={t("theme.toggle", { name: "" })}>
                      <ThemeToggle iconOnly />
                    </Tooltip>
                    <Tooltip text={`Language`}>
                      <LocaleSwitcher />
                    </Tooltip>
                    <Tooltip
                      text={t("app.ws.label", {
                        status: props.connected()
                          ? t("app.ws.connected")
                          : t("app.ws.disconnected"),
                      })}
                    >
                      <div class="flex items-center justify-center p-1.5">
                        <StatusDot
                          color={
                            props.connected() ? "var(--cf-status-idle)" : "var(--cf-status-error)"
                          }
                        />
                      </div>
                    </Tooltip>
                    <Show when={props.health()}>
                      <Tooltip text={t("app.api.label", { status: props.health()?.status ?? "" })}>
                        <div class="flex items-center justify-center p-1.5">
                          <StatusDot color="var(--cf-status-idle)" />
                        </div>
                      </Tooltip>
                    </Show>
                  </div>
                }
              >
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
                <Show when={activeRuns().size > 0}>
                  <div
                    class="mt-1 flex items-center gap-2 text-xs text-cf-accent"
                    aria-live="polite"
                  >
                    <span class="inline-block h-2 w-2 rounded-full bg-cf-accent animate-pulse" />
                    <span>
                      {activeRuns().size} run{activeRuns().size !== 1 ? "s" : ""} active
                    </span>
                  </div>
                </Show>
              </Show>
            </Sidebar.Footer>
          </Sidebar>

          <div class="flex flex-1 flex-col overflow-hidden">
            <header class="flex items-center justify-between border-b border-cf-border bg-cf-bg-surface px-3 py-2 sm:px-4">
              <div class="flex items-center gap-2">
                <Show when={isMobile()}>
                  <button
                    type="button"
                    onClick={openMobile}
                    class="rounded-cf-md p-2 min-h-[44px] min-w-[44px] flex items-center justify-center text-cf-text-secondary hover:bg-cf-bg-surface-alt transition-colors"
                    aria-label="Open menu"
                  >
                    <svg
                      class="h-5 w-5"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                      stroke-width="2"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        d="M3.75 6.75h16.5M3.75 12h16.5m-16.5 5.25h16.5"
                      />
                    </svg>
                  </button>
                </Show>
              </div>
              <Show when={isAuthenticated()}>
                <div class="flex items-center gap-2 text-sm">
                  <span class="text-cf-text-secondary">{user()?.name ?? ""}</span>
                  <span class="rounded bg-cf-bg-surface-alt px-1.5 py-0.5 text-xs font-medium uppercase text-cf-text-muted">
                    {user()?.role ?? ""}
                  </span>
                  <Button
                    variant="link"
                    size="xs"
                    onClick={() => void logout()}
                    title={t("auth.logout")}
                  >
                    {t("auth.logout")}
                  </Button>
                </div>
              </Show>
            </header>
            <main
              id="main-content"
              class="flex-1 min-h-0 overflow-hidden p-3 sm:p-4 lg:p-6"
              style={{ "padding-bottom": "max(0.75rem, var(--cf-safe-bottom))" }}
            >
              {props.children}
            </main>
          </div>
        </div>
      </div>
    </>
  );
}

// ---------------------------------------------------------------------------
// App shell
// ---------------------------------------------------------------------------

// Known application routes (used to detect 404 pages for unauthenticated users)
const KNOWN_ROUTES = new Set([
  "/",
  "/projects",
  "/costs",
  "/ai",
  "/activity",
  "/knowledge",
  "/mcp",
  "/prompts",
  "/settings",
  "/benchmarks",
]);

/** Inner component rendered inside AuthProvider so the WS has access to the auth token. */
function AuthenticatedApp(props: { children: JSX.Element }): JSX.Element {
  const [health] = createResource(() => api.health.check());
  const { connected } = useWebSocket();
  const location = useLocation();

  const isPublicPage = (): boolean =>
    location.pathname === "/login" ||
    location.pathname === "/change-password" ||
    location.pathname === "/setup" ||
    location.pathname === "/forgot-password" ||
    location.pathname === "/reset-password";

  const isKnownRoute = (): boolean =>
    isPublicPage() ||
    KNOWN_ROUTES.has(location.pathname) ||
    location.pathname.startsWith("/projects/");

  return (
    <ToastProvider>
      <ConfirmProvider>
        <SidebarProvider>
          <ShortcutProvider>
            <Show when={!isPublicPage() && isKnownRoute()} fallback={props.children}>
              <RouteGuard>
                <AppShell health={health} connected={connected}>
                  {props.children}
                </AppShell>
              </RouteGuard>
            </Show>
          </ShortcutProvider>
        </SidebarProvider>
      </ConfirmProvider>
    </ToastProvider>
  );
}

export default function App(props: RouteSectionProps) {
  return (
    <I18nProvider>
      <ErrorBoundary fallback={(err, reset) => <ErrorFallback error={err} reset={reset} />}>
        <ThemeProvider>
          <AuthProvider>
            <WebSocketProvider>
              <ConversationRunProvider>
                <AuthenticatedApp>{props.children}</AuthenticatedApp>
              </ConversationRunProvider>
            </WebSocketProvider>
          </AuthProvider>
        </ThemeProvider>
      </ErrorBoundary>
    </I18nProvider>
  );
}
