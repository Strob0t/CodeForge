import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { LSPServerInfo } from "~/api/types";

interface LSPPanelProps {
  projectId: string;
}

export default function LSPPanel(props: LSPPanelProps) {
  const [servers, { refetch }] = createResource(
    () => props.projectId || undefined,
    async (id: string): Promise<LSPServerInfo[] | null> => {
      try {
        return await api.lsp.status(id);
      } catch {
        return null;
      }
    },
  );

  const [starting, setStarting] = createSignal(false);
  const [stopping, setStopping] = createSignal(false);
  const [error, setError] = createSignal("");

  const handleStart = async () => {
    setStarting(true);
    setError("");
    try {
      await api.lsp.start(props.projectId);
      setTimeout(() => refetch(), 2000);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to start LSP servers");
    } finally {
      setStarting(false);
    }
  };

  const handleStop = async () => {
    setStopping(true);
    setError("");
    try {
      await api.lsp.stop(props.projectId);
      setTimeout(() => refetch(), 1000);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to stop LSP servers");
    } finally {
      setStopping(false);
    }
  };

  const statusColor = (status: string): string => {
    switch (status) {
      case "ready":
        return "bg-green-500";
      case "starting":
        return "bg-yellow-500";
      case "failed":
        return "bg-red-500";
      default:
        return "bg-gray-400";
    }
  };

  return (
    <div class="space-y-4">
      <div class="flex items-center justify-between">
        <h3 class="text-sm font-medium text-zinc-300">Language Servers</h3>
        <div class="flex gap-2">
          <button
            class="rounded bg-blue-600 px-3 py-1 text-xs text-white hover:bg-blue-700 disabled:opacity-50"
            disabled={starting()}
            onClick={handleStart}
          >
            {starting() ? "Starting..." : "Start"}
          </button>
          <button
            class="rounded bg-zinc-600 px-3 py-1 text-xs text-white hover:bg-zinc-700 disabled:opacity-50"
            disabled={stopping()}
            onClick={handleStop}
          >
            {stopping() ? "Stopping..." : "Stop"}
          </button>
        </div>
      </div>

      <Show when={error()}>
        <div class="rounded bg-red-900/30 px-3 py-2 text-xs text-red-400">{error()}</div>
      </Show>

      <Show
        when={servers() && (servers() ?? []).length > 0}
        fallback={
          <p class="text-xs text-zinc-500">
            No language servers running. Click Start to auto-detect and launch servers.
          </p>
        }
      >
        <div class="space-y-2">
          <For each={servers() ?? []}>
            {(server) => (
              <div class="flex items-center justify-between rounded bg-zinc-800 px-3 py-2">
                <div class="flex items-center gap-2">
                  <span class={`inline-block h-2 w-2 rounded-full ${statusColor(server.status)}`} />
                  <span class="text-sm font-medium text-zinc-200">{server.language}</span>
                  <span class="text-xs text-zinc-500">{server.command}</span>
                </div>
                <div class="flex items-center gap-3">
                  <Show when={server.diagnostics > 0}>
                    <span class="rounded-full bg-yellow-600/20 px-2 py-0.5 text-xs text-yellow-400">
                      {server.diagnostics} diag
                    </span>
                  </Show>
                  <span class="text-xs text-zinc-500">{server.status}</span>
                  <Show when={server.error}>
                    <span class="text-xs text-red-400" title={server.error}>
                      error
                    </span>
                  </Show>
                </div>
              </div>
            )}
          </For>
        </div>
      </Show>
    </div>
  );
}
