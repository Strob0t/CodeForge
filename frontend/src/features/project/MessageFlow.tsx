import { createEffect, createSignal, For, on, onCleanup } from "solid-js";

import type { HandoffStatusEvent } from "~/api/types";
import { useWebSocket } from "~/components/WebSocketProvider";

import { payloadString } from "./liveEvents";

function isHandoffStatusEvent(p: unknown): p is HandoffStatusEvent {
  return (
    typeof p === "object" &&
    p !== null &&
    "source_agent_id" in p &&
    "target_agent_id" in p &&
    "status" in p
  );
}

interface Arrow {
  id: string;
  sourceId: string;
  targetId: string;
  status: string;
  /** The target's run of an initiated handoff; the arrow stays until it ends. */
  runId?: string;
  /** Changes with every status, so a removal timer removes only the arrow it was set for. */
  version: number;
}

/** Statuses after which the War Room no longer follows a handoff: their arrow goes after 10 s. */
const SETTLED_STATUSES: ReadonlySet<string> = new Set([
  "quarantined",
  "rejected",
  "failed",
  "a2a_delegated",
]);

/** run.status statuses that end a run; an initiated handoff settles with its run. */
const RUN_END_STATUSES: ReadonlySet<string> = new Set([
  "completed",
  "failed",
  "cancelled",
  "timeout",
]);

const SETTLE_MS = 10_000;

export default function MessageFlow(props: { containerRef?: HTMLDivElement }) {
  const { onMessage, connected } = useWebSocket();
  const [arrows, setArrows] = createSignal<Arrow[]>([]);
  const pendingTimers = new Set<ReturnType<typeof setTimeout>>();
  let version = 0;

  /**
   * The arrow goes after 10 s, unless a later status changed it meanwhile (a
   * held handoff that was approved, a new handoff between the same agents).
   */
  function settle(arrowVersion: number): void {
    const timerId = setTimeout(() => {
      pendingTimers.delete(timerId);
      setArrows((prev) => prev.filter((a) => a.version !== arrowVersion));
    }, SETTLE_MS);
    pendingTimers.add(timerId);
  }

  /** Settles the arrows of the initiated handoffs whose run matches. */
  function settleFollowed(matches: (runId: string) => boolean): void {
    for (const a of arrows()) {
      if (a.status === "initiated" && a.runId && matches(a.runId)) settle(a.version);
    }
  }

  function followHandoff(p: HandoffStatusEvent): void {
    const v = ++version;
    const runId = p.status === "initiated" && p.run_id ? p.run_id : undefined;

    setArrows((prev) => {
      const existing = prev.find(
        (a) => a.sourceId === p.source_agent_id && a.targetId === p.target_agent_id,
      );
      if (existing) {
        return prev.map((a) =>
          a.id === existing.id ? { ...a, status: p.status, runId, version: v } : a,
        );
      }
      return [
        ...prev,
        {
          id: `${p.source_agent_id}-${p.target_agent_id}-${v}`,
          sourceId: p.source_agent_id,
          targetId: p.target_agent_id,
          status: p.status,
          runId,
          version: v,
        },
      ];
    });

    // An initiated handoff is followed while the target's run works (KI-92);
    // one that names no run has nothing to follow and settles like the others.
    if (SETTLED_STATUSES.has(p.status) || (p.status === "initiated" && !runId)) settle(v);
  }

  // eslint-disable-next-line solid/reactivity -- subscription callback, not a reactive computation
  const cleanup = onMessage((msg) => {
    if (msg.type === "run.status") {
      const runId = payloadString(msg.payload, "run_id");
      const status = payloadString(msg.payload, "status");
      if (runId && status && RUN_END_STATUSES.has(status)) settleFollowed((id) => id === runId);
      return;
    }
    if (msg.type !== "handoff.status") return;
    if (!isHandoffStatusEvent(msg.payload)) return;
    followHandoff(msg.payload);
  });

  // While the socket was down a followed run may have ended unseen: after a
  // reconnect its arrow settles instead of staying for the rest of the session.
  createEffect(
    on(connected, (isConnected, wasConnected) => {
      if (isConnected && wasConnected === false) settleFollowed(() => true);
    }),
  );

  onCleanup(() => {
    cleanup();
    for (const id of pendingTimers) clearTimeout(id);
    pendingTimers.clear();
  });

  const arrowColor = (status: string) => {
    switch (status) {
      case "initiated":
      case "a2a_delegated":
        return "var(--cf-accent)";
      case "quarantined":
        return "var(--cf-warning)";
      case "rejected":
      case "failed":
        return "var(--cf-danger)";
      default:
        return "var(--cf-text-muted)";
    }
  };

  return (
    <svg
      class="absolute inset-0 pointer-events-none"
      style={{ width: "100%", height: "100%", "z-index": 10 }}
    >
      <defs>
        <marker id="arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
          <polygon points="0 0, 10 3.5, 0 7" fill="currentColor" />
        </marker>
      </defs>
      <For each={arrows()}>
        {(arrow) => {
          if (!props.containerRef) return null;
          const sourceLane = props.containerRef.querySelector(
            `[data-agent-id="${arrow.sourceId}"]`,
          );
          const targetLane = props.containerRef.querySelector(
            `[data-agent-id="${arrow.targetId}"]`,
          );
          if (!sourceLane || !targetLane) return null;

          const containerRect = props.containerRef.getBoundingClientRect();
          const sourceRect = sourceLane.getBoundingClientRect();
          const targetRect = targetLane.getBoundingClientRect();

          const x1 = sourceRect.right - containerRect.left;
          const y1 = sourceRect.top + sourceRect.height / 2 - containerRect.top;
          const x2 = targetRect.left - containerRect.left;
          const y2 = targetRect.top + targetRect.height / 2 - containerRect.top;
          const cx1 = x1 + (x2 - x1) / 3;
          const cx2 = x2 - (x2 - x1) / 3;

          return (
            <path
              d={`M ${x1} ${y1} C ${cx1} ${y1}, ${cx2} ${y2}, ${x2} ${y2}`}
              fill="none"
              stroke={arrowColor(arrow.status)}
              stroke-width="2"
              marker-end="url(#arrowhead)"
              style={{ color: arrowColor(arrow.status) }}
            />
          );
        }}
      </For>
    </svg>
  );
}
