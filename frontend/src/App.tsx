import type { RouteSectionProps } from "@solidjs/router";
import { A, useLocation } from "@solidjs/router";
import { createResource, ErrorBoundary, type JSX, Show } from "solid-js";

import { api } from "~/api/client";
import { createCodeForgeWS } from "~/api/websocket";
import { AuthProvider, useAuth } from "~/components/AuthProvider";
import { CommandPalette } from "~/components/CommandPalette";
import { OfflineBanner } from "~/components/OfflineBanner";
import { ThemeProvider, ThemeToggle } from "~/components/ThemeProvider";
import { ToastProvider } from "~/components/Toast";
import { I18nProvider, useI18n } from "~/i18n";
import { LocaleSwitcher } from "~/i18n/LocaleSwitcher";

// ---------------------------------------------------------------------------
// Error fallback (rendered when an uncaught error bubbles up)
// ---------------------------------------------------------------------------

function ErrorFallback(props: { error: unknown; reset: () => void }): JSX.Element {
  const { t } = useI18n();
  const message = () => (props.error instanceof Error ? props.error.message : String(props.error));

  return (
    <div class="flex h-screen items-center justify-center bg-gray-50 dark:bg-gray-900" role="alert">
      <div class="max-w-md rounded-lg border border-red-200 bg-white p-8 text-center shadow-md dark:border-red-800 dark:bg-gray-800">
        <h1 class="mb-2 text-lg font-bold text-red-700 dark:text-red-400">
          {t("app.error.title")}
        </h1>
        <p class="mb-4 text-sm text-gray-600 dark:text-gray-400">{message()}</p>
        <button
          type="button"
          class="rounded bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
          onClick={() => props.reset()}
        >
          {t("app.error.retry")}
        </button>
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
      <div class="flex items-center justify-between border-b border-gray-200 px-4 py-2 text-xs dark:border-gray-700">
        <div class="truncate text-gray-600 dark:text-gray-400" title={user()?.email ?? ""}>
          {user()?.name ?? ""}{" "}
          <span class="rounded bg-gray-100 px-1 py-0.5 text-[10px] font-medium uppercase dark:bg-gray-700">
            {user()?.role ?? ""}
          </span>
        </div>
        <button
          type="button"
          onClick={() => void logout()}
          class="ml-2 text-gray-400 hover:text-red-500 dark:text-gray-500 dark:hover:text-red-400"
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
      <div class="flex h-screen flex-col bg-gray-50 text-gray-900 dark:bg-gray-900 dark:text-gray-100">
        <OfflineBanner wsConnected={props.connected} />

        <div class="flex flex-1 overflow-hidden">
          <aside
            class="flex w-64 flex-col border-r border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800"
            aria-label="Sidebar"
          >
            <div class="p-4">
              <h1 class="text-xl font-bold">{t("app.title")}</h1>
              <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">{t("app.version")}</p>
            </div>
            <UserInfo />

            <nav class="flex-1 px-3" aria-label="Main navigation">
              <A
                href="/"
                class="block rounded-md px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                activeClass="bg-gray-100 text-blue-600 dark:bg-gray-700 dark:text-blue-400"
                end
              >
                {t("app.nav.dashboard")}
              </A>
              <A
                href="/costs"
                class="block rounded-md px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                activeClass="bg-gray-100 text-blue-600 dark:bg-gray-700 dark:text-blue-400"
              >
                {t("app.nav.costs")}
              </A>
              <A
                href="/models"
                class="block rounded-md px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                activeClass="bg-gray-100 text-blue-600 dark:bg-gray-700 dark:text-blue-400"
              >
                {t("app.nav.models")}
              </A>
            </nav>

            <div class="border-t border-gray-200 p-4 dark:border-gray-700">
              <div class="flex items-center gap-2">
                <ThemeToggle />
                <LocaleSwitcher />
              </div>
              <div
                class="mt-2 flex items-center gap-2 text-xs text-gray-400 dark:text-gray-500"
                aria-live="polite"
              >
                <span
                  class={`inline-block h-2 w-2 rounded-full ${props.connected() ? "bg-green-400" : "bg-red-400"}`}
                  aria-hidden="true"
                />
                <span>
                  {t("app.ws.label", {
                    status: props.connected() ? t("app.ws.connected") : t("app.ws.disconnected"),
                  })}
                </span>
              </div>
              <Show when={props.health()}>
                <div
                  class="mt-1 flex items-center gap-2 text-xs text-gray-400 dark:text-gray-500"
                  aria-live="polite"
                >
                  <span class="inline-block h-2 w-2 rounded-full bg-green-400" aria-hidden="true" />
                  <span>{t("app.api.label", { status: props.health()?.status ?? "" })}</span>
                </div>
              </Show>
            </div>
          </aside>

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

export default function App(props: RouteSectionProps) {
  const [health] = createResource(() => api.health.check());
  const { connected } = createCodeForgeWS();
  const location = useLocation();

  // Skip AppShell for the login page.
  const isLoginPage = (): boolean => location.pathname === "/login";

  return (
    <ErrorBoundary fallback={(err, reset) => <ErrorFallback error={err} reset={reset} />}>
      <I18nProvider>
        <ThemeProvider>
          <AuthProvider>
            <ToastProvider>
              <Show when={!isLoginPage()} fallback={props.children}>
                <AppShell health={health} connected={connected}>
                  {props.children}
                </AppShell>
              </Show>
            </ToastProvider>
          </AuthProvider>
        </ThemeProvider>
      </I18nProvider>
    </ErrorBoundary>
  );
}
