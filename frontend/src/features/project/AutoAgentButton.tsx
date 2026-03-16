import { createResource, Show } from "solid-js";

import { api } from "~/api/client";
import type { AutoAgentStatus } from "~/api/types";
import { useAsyncAction } from "~/hooks";
import { Button } from "~/ui";

interface Props {
  projectId: string;
  /** Externally pushed status from WebSocket events. */
  wsStatus: () => AutoAgentStatus | undefined;
}

export default function AutoAgentButton(props: Props) {
  const [polledStatus, { refetch }] = createResource(
    () => props.projectId,
    async (id) => {
      try {
        return await api.autoAgent.status(id);
      } catch {
        return undefined;
      }
    },
  );

  /** Prefer WS-pushed status, fall back to polled. */
  const status = (): AutoAgentStatus | undefined => props.wsStatus() ?? polledStatus();

  const isRunning = () => {
    const s = status()?.status;
    return s === "running" || s === "stopping";
  };

  const isStopping = () => status()?.status === "stopping";

  const {
    run: handleToggle,
    loading,
    error,
  } = useAsyncAction(async () => {
    if (isRunning()) {
      await api.autoAgent.stop(props.projectId);
    } else {
      await api.autoAgent.start(props.projectId);
    }
    refetch();
  });

  const progressText = () => {
    const s = status();
    if (!s || s.status === "idle") return "";
    const done = (s.features_complete ?? 0) + (s.features_failed ?? 0);
    const total = s.features_total ?? 0;
    if (total === 0) return "";
    return `${done}/${total}`;
  };

  return (
    <div class="flex items-center gap-2">
      <Button
        variant={isRunning() ? "danger" : "primary"}
        size="sm"
        onClick={() => void handleToggle()}
        disabled={loading() || isStopping()}
        loading={loading() || isStopping()}
        title={isRunning() ? "Stop auto-agent" : "Start auto-agent on pending features"}
      >
        {isStopping() ? "Stopping..." : isRunning() ? "Stop Agent" : "Auto-Agent"}
      </Button>

      <Show when={isRunning() && progressText()}>
        <span class="text-xs text-cf-text-tertiary font-mono">{progressText()}</span>
      </Show>

      <Show when={status()?.status === "failed"}>
        <span class="text-xs text-cf-danger-fg" title={status()?.error ?? ""}>
          Failed
        </span>
      </Show>

      <Show when={error()}>
        <span class="text-xs text-cf-danger-fg">{error()}</span>
      </Show>
    </div>
  );
}
