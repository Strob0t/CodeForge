import { createSignal, For, onCleanup } from "solid-js";

import type { HandoffStatusEvent } from "~/api/types";
import { createCodeForgeWS } from "~/api/websocket";

interface Arrow {
  id: string;
  sourceId: string;
  targetId: string;
  status: string;
  timestamp: number;
}

export default function MessageFlow(props: { containerRef?: HTMLDivElement }) {
  const { onMessage } = createCodeForgeWS();
  const [arrows, setArrows] = createSignal<Arrow[]>([]);

  const cleanup = onMessage((msg) => {
    if (msg.type !== "handoff.status") return;
    const p = msg.payload as unknown as HandoffStatusEvent;

    setArrows((prev) => {
      const existing = prev.find(
        (a) => a.sourceId === p.source_agent_id && a.targetId === p.target_agent_id,
      );
      if (existing) {
        return prev.map((a) =>
          a.id === existing.id ? { ...a, status: p.status, timestamp: Date.now() } : a,
        );
      }
      return [
        ...prev,
        {
          id: `${p.source_agent_id}-${p.target_agent_id}-${Date.now()}`,
          sourceId: p.source_agent_id,
          targetId: p.target_agent_id,
          status: p.status,
          timestamp: Date.now(),
        },
      ];
    });

    // Auto-remove completed/failed arrows after 10s
    if (p.status === "completed" || p.status === "failed") {
      setTimeout(() => {
        setArrows((prev) =>
          prev.filter(
            (a) => !(a.sourceId === p.source_agent_id && a.targetId === p.target_agent_id),
          ),
        );
      }, 10000);
    }
  });
  onCleanup(cleanup);

  const arrowColor = (status: string) => {
    switch (status) {
      case "initiated":
        return "#3b82f6";
      case "completed":
        return "#22c55e";
      case "failed":
        return "#ef4444";
      default:
        return "#6b7280";
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
