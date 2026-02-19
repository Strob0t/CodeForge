import type { RouteSectionProps } from "@solidjs/router";
import { A } from "@solidjs/router";
import { createResource, ErrorBoundary, type JSX, Show } from "solid-js";

import { api } from "~/api/client";
import { createCodeForgeWS } from "~/api/websocket";
import { OfflineBanner } from "~/components/OfflineBanner";
import { ThemeProvider, ThemeToggle } from "~/components/ThemeProvider";
import { ToastProvider } from "~/components/Toast";

// ---------------------------------------------------------------------------
// Error fallback (rendered when an uncaught error bubbles up)
// ---------------------------------------------------------------------------

function ErrorFallback(props: { error: unknown; reset: () => void }): JSX.Element {
  const message = () => (props.error instanceof Error ? props.error.message : String(props.error));

  return (
    <div class="flex h-screen items-center justify-center bg-gray-50 dark:bg-gray-900" role="alert">
      <div class="max-w-md rounded-lg border border-red-200 bg-white p-8 text-center shadow-md dark:border-red-800 dark:bg-gray-800">
        <h1 class="mb-2 text-lg font-bold text-red-700 dark:text-red-400">Something went wrong</h1>
        <p class="mb-4 text-sm text-gray-600 dark:text-gray-400">{message()}</p>
        <button
          type="button"
          class="rounded bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
          onClick={() => props.reset()}
        >
          Try again
        </button>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// App shell
// ---------------------------------------------------------------------------

export default function App(props: RouteSectionProps) {
  const [health] = createResource(() => api.health.check());
  const { connected } = createCodeForgeWS();

  return (
    <ErrorBoundary fallback={(err, reset) => <ErrorFallback error={err} reset={reset} />}>
      <ThemeProvider>
        <ToastProvider>
          <div class="flex h-screen flex-col bg-gray-50 text-gray-900 dark:bg-gray-900 dark:text-gray-100">
            <OfflineBanner wsConnected={connected} />

            <div class="flex flex-1 overflow-hidden">
              <aside class="flex w-64 flex-col border-r border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
                <div class="p-4">
                  <h1 class="text-xl font-bold">CodeForge</h1>
                  <p class="mt-1 text-xs text-gray-400 dark:text-gray-500">v0.1.0</p>
                </div>

                <nav class="flex-1 px-3">
                  <A
                    href="/"
                    class="block rounded-md px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                    activeClass="bg-gray-100 text-blue-600 dark:bg-gray-700 dark:text-blue-400"
                    end
                  >
                    Dashboard
                  </A>
                  <A
                    href="/costs"
                    class="block rounded-md px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                    activeClass="bg-gray-100 text-blue-600 dark:bg-gray-700 dark:text-blue-400"
                  >
                    Costs
                  </A>
                  <A
                    href="/models"
                    class="block rounded-md px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                    activeClass="bg-gray-100 text-blue-600 dark:bg-gray-700 dark:text-blue-400"
                  >
                    Models
                  </A>
                </nav>

                <div class="border-t border-gray-200 p-4 dark:border-gray-700">
                  <ThemeToggle />
                  <div class="mt-2 flex items-center gap-2 text-xs text-gray-400 dark:text-gray-500">
                    <span
                      class={`inline-block h-2 w-2 rounded-full ${connected() ? "bg-green-400" : "bg-red-400"}`}
                    />
                    <span>WS {connected() ? "connected" : "disconnected"}</span>
                  </div>
                  <Show when={health()}>
                    <div class="mt-1 flex items-center gap-2 text-xs text-gray-400 dark:text-gray-500">
                      <span class="inline-block h-2 w-2 rounded-full bg-green-400" />
                      <span>API {health()?.status}</span>
                    </div>
                  </Show>
                </div>
              </aside>

              <main class="flex-1 overflow-auto p-6">{props.children}</main>
            </div>
          </div>
        </ToastProvider>
      </ThemeProvider>
    </ErrorBoundary>
  );
}
