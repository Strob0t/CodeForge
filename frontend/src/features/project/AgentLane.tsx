import { For } from "solid-js";

import type { Agent } from "~/api/types";

import type { AgentWork, LaneOutputLine, LaneToolCall } from "./liveEvents";

/**
 * One agent's live lane: the output of the task and the tool calls of the run
 * the agent works on (WarRoom follows both from the project's events and
 * passes them in), and the run's progress.
 */
export default function AgentLane(props: {
  agent: Agent;
  work: AgentWork;
  outputs: readonly LaneOutputLine[];
  toolCalls: readonly LaneToolCall[];
}) {
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
        <For each={props.outputs}>
          {(o) => <div class={o.stream === "stderr" ? "text-cf-danger-fg" : ""}>{o.line}</div>}
        </For>
        <For each={props.toolCalls}>
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
