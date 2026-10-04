import { createSignal, For, onCleanup } from "solid-js";

import type { HandoffStatusEvent } from "~/api/types";
import { useWebSocket } from "~/components/WebSocketProvider";

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

export default function MessageFlow(props: { containerRef?: HTMLDivElement }) {
  const { onMessage } = useWebSocket();
  const [arrows, setArrows] = createSignal<Arrow[]>([]);
  const pendingTimers = new Set<ReturnType<typeof setTimeout>>();
  let version = 0;

  const cleanup = onMessage((msg) => {
    if (msg.type !== "handoff.status") return;
    if (!isHandoffStatusEvent(msg.payload)) return;
    const p = msg.payload;
    const v = ++version;

    setArrows((prev) => {
      const existing = prev.find(
        (a) => a.sourceId === p.source_agent_id && a.targetId === p.target_agent_id,
      );
      if (existing) {
        return prev.map((a) => (a.id === existing.id ? { ...a, status: p.status, version: v } : a));
      }
      return [
        ...prev,
        {
          id: `${p.source_agent_id}-${p.target_agent_id}-${v}`,
          sourceId: p.source_agent_id,
          targetId: p.target_agent_id,
          status: p.status,
          version: v,
        },
      ];
    });

    // A settled handoff's arrow goes after 10 s, unless a later status (a
    // held handoff that was approved) changed it meanwhile.
    if (SETTLED_STATUSES.has(p.status)) {
      const timerId = setTimeout(() => {
        pendingTimers.delete(timerId);
        setArrows((prev) => prev.filter((a) => a.version !== v));
      }, 10000);
      pendingTimers.add(timerId);
    }
  });
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
