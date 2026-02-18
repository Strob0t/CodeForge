import { createResource, Show } from "solid-js";
import { A } from "@solidjs/router";
import type { RouteSectionProps } from "@solidjs/router";
import { api } from "~/api/client";
import { createCodeForgeWS } from "~/api/websocket";

export default function App(props: RouteSectionProps) {
  const [health] = createResource(() => api.health.check());
  const { connected } = createCodeForgeWS();

  return (
    <div class="flex h-screen bg-gray-50 text-gray-900">
      <aside class="flex w-64 flex-col border-r border-gray-200 bg-white">
        <div class="p-4">
          <h1 class="text-xl font-bold">CodeForge</h1>
          <p class="mt-1 text-xs text-gray-400">v0.1.0</p>
        </div>

        <nav class="flex-1 px-3">
          <A
            href="/"
            class="block rounded-md px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100"
            activeClass="bg-gray-100 text-blue-600"
            end
          >
            Dashboard
          </A>
          <A
            href="/costs"
            class="block rounded-md px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100"
            activeClass="bg-gray-100 text-blue-600"
          >
            Costs
          </A>
          <A
            href="/models"
            class="block rounded-md px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100"
            activeClass="bg-gray-100 text-blue-600"
          >
            Models
          </A>
        </nav>

        <div class="border-t border-gray-200 p-4">
          <div class="flex items-center gap-2 text-xs text-gray-400">
            <span
              class={`inline-block h-2 w-2 rounded-full ${connected() ? "bg-green-400" : "bg-red-400"}`}
            />
            <span>WS {connected() ? "connected" : "disconnected"}</span>
          </div>
          <Show when={health()}>
            <div class="mt-1 flex items-center gap-2 text-xs text-gray-400">
              <span class="inline-block h-2 w-2 rounded-full bg-green-400" />
              <span>API {health()?.status}</span>
            </div>
          </Show>
        </div>
      </aside>

      <main class="flex-1 overflow-auto p-6">{props.children}</main>
    </div>
  );
}
