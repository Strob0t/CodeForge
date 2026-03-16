import { createSignal, For, onCleanup } from "solid-js";

import type { Agent } from "~/api/types";
import { useWebSocket } from "~/components/WebSocketProvider";

interface ToolCall {
  callId: string;
  tool: string;
  phase: string;
}

interface OutputLine {
  line: string;
  stream: string;
}

export default function AgentLane(props: { agent: Agent }) {
  const { onMessage } = useWebSocket();
  const [toolCalls, setToolCalls] = createSignal<ToolCall[]>([]);
  const [outputs, setOutputs] = createSignal<OutputLine[]>([]);
  const [stepCount, setStepCount] = createSignal(0);
  const [costUsd, setCostUsd] = createSignal(0);

  const cleanup = onMessage((msg) => {
    const p = msg.payload;

    switch (msg.type) {
      case "run.toolcall": {
        if ((p.agent_id as string) !== props.agent.id) {
          // Tool calls don't have agent_id directly — match via run association
          break;
        }
        setToolCalls((prev) => [
          ...prev.slice(-19),
          { callId: p.call_id as string, tool: p.tool as string, phase: p.phase as string },
        ]);
        break;
      }
      case "run.status": {
        if ((p.agent_id as string) === props.agent.id) {
          setStepCount(p.step_count as number);
          setCostUsd((p.cost_usd as number) ?? 0);
        }
        break;
      }
      case "task.output": {
        // Match by current task association
        setOutputs((prev) => [
          ...prev.slice(-49),
          { line: p.line as string, stream: p.stream as string },
        ]);
        break;
      }
    }
  });
  onCleanup(cleanup);

  const statusColor = () => {
    switch (props.agent.status) {
      case "running":
        return "bg-cf-success";
      case "error":
        return "bg-cf-danger";
      case "idle":
        return "bg-cf-warning";
      default:
        return "bg-cf-text-muted";
    }
  };

  return (
    <div class="border border-cf-border rounded-lg bg-cf-bg-secondary flex flex-col h-80">
      {/* Header */}
      <div class="flex items-center gap-2 px-3 py-2 border-b border-cf-border flex-shrink-0">
        <span
          class={`w-2.5 h-2.5 rounded-full ${statusColor()} ${props.agent.status === "running" ? "animate-pulse" : ""}`}
        />
        <span class="font-medium text-sm text-cf-text-primary truncate">{props.agent.name}</span>
        <span class="ml-auto text-xs px-1.5 py-0.5 rounded bg-cf-bg-tertiary text-cf-text-tertiary">
          {props.agent.backend}
        </span>
      </div>

      {/* Output stream */}
      <div class="flex-1 overflow-y-auto px-3 py-1 font-mono text-xs text-cf-text-secondary">
        <For each={outputs()}>
          {(o) => <div class={o.stream === "stderr" ? "text-cf-danger-fg" : ""}>{o.line}</div>}
        </For>
        <For each={toolCalls()}>
          {(tc) => (
            <div class="my-1 px-2 py-1 bg-cf-bg-tertiary rounded text-xs">
              <span class="text-cf-accent font-medium">{tc.tool}</span>
              <span class="ml-2 text-cf-text-muted">{tc.phase}</span>
            </div>
          )}
        </For>
      </div>

      {/* Footer */}
      <div class="flex items-center justify-between px-3 py-1.5 border-t border-cf-border text-xs text-cf-text-muted flex-shrink-0">
        <span>Steps: {stepCount()}</span>
        <span>${costUsd().toFixed(4)}</span>
      </div>
    </div>
  );
}
