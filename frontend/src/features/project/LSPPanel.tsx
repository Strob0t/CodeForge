import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { LSPServerInfo } from "~/api/types";
import { Alert, Badge, Button, StatusDot } from "~/ui";

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

  const statusDotColor = (status: string): string => {
    switch (status) {
      case "ready":
        return "#22c55e";
      case "starting":
        return "#eab308";
      case "failed":
        return "#ef4444";
      default:
        return "#9ca3af";
    }
  };

  return (
    <div class="space-y-4">
      <div class="flex items-center justify-between">
        <h3 class="text-sm font-medium text-cf-text-secondary">Language Servers</h3>
        <div class="flex gap-2">
          <Button
            variant="primary"
            size="sm"
            disabled={starting()}
            onClick={handleStart}
            loading={starting()}
          >
            {starting() ? "Starting..." : "Start"}
          </Button>
          <Button
            variant="secondary"
            size="sm"
            disabled={stopping()}
            onClick={handleStop}
            loading={stopping()}
          >
            {stopping() ? "Stopping..." : "Stop"}
          </Button>
        </div>
      </div>

      <Show when={error()}>
        <Alert variant="error">{error()}</Alert>
      </Show>

      <Show
        when={servers() && (servers() ?? []).length > 0}
        fallback={
          <p class="text-xs text-cf-text-tertiary">
            No language servers running. Click Start to auto-detect and launch servers.
          </p>
        }
      >
        <div class="space-y-2">
          <For each={servers() ?? []}>
            {(server) => (
              <div class="flex items-center justify-between rounded-cf-sm bg-cf-bg-surface-alt px-3 py-2">
                <div class="flex items-center gap-2">
                  <StatusDot color={statusDotColor(server.status)} />
                  <span class="text-sm font-medium text-cf-text-primary">{server.language}</span>
                  <span class="text-xs text-cf-text-tertiary">{server.command}</span>
                </div>
                <div class="flex items-center gap-3">
                  <Show when={server.diagnostics > 0}>
                    <Badge variant="warning" pill>
                      {server.diagnostics} diag
                    </Badge>
                  </Show>
                  <span class="text-xs text-cf-text-tertiary">{server.status}</span>
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
