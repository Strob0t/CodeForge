import { createEffect, createSignal, For, onCleanup } from "solid-js";

import type { Agent } from "~/api/types";
import { useWebSocket } from "~/components/WebSocketProvider";

import { type AgentWork, parseTaskOutput, parseToolCall } from "./liveEvents";

interface ToolCall {
  callId: string;
  tool: string;
  phase: string;
}

interface OutputLine {
  line: string;
  stream: string;
}

/**
 * One agent's live lane. `work` names the run and task the agent works on
 * (WarRoom follows it from the agent's events); the lane shows only that
 * task's output and that run's tool calls.
 */
export default function AgentLane(props: { agent: Agent; work: AgentWork }) {
  const { onMessage } = useWebSocket();
  const [toolCalls, setToolCalls] = createSignal<ToolCall[]>([]);
  const [outputs, setOutputs] = createSignal<OutputLine[]>([]);

  createEffect(() => {
    const { runId, taskId } = props.work;
    const unsub = onMessage((msg) => {
      const output = parseTaskOutput(msg);
      if (output) {
        if (taskId && output.taskId === taskId) {
          setOutputs((prev) => [...prev.slice(-49), { line: output.line, stream: output.stream }]);
        }
        return;
      }
      const call = parseToolCall(msg);
      if (call && runId && call.run_id === runId) {
        setToolCalls((prev) => [
          ...prev.slice(-19),
          { callId: call.call_id, tool: call.tool, phase: call.phase },
        ]);
      }
    });
    onCleanup(unsub);
  });

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
        <span>Steps: {props.work.steps}</span>
        <span>${props.work.costUsd.toFixed(4)}</span>
      </div>
    </div>
  );
}
