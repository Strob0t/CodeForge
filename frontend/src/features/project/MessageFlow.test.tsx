import { render } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { HandoffStatusEvent } from "~/api/types";
import type { WSMessage } from "~/api/websocket";

const ws = vi.hoisted(() => {
  const handlers = new Set<(msg: WSMessage) => void>();
  return {
    handlers,
    emit(payload: HandoffStatusEvent): void {
      for (const h of [...handlers]) h({ type: "handoff.status", payload: { ...payload } });
    },
  };
});

vi.mock("~/components/WebSocketProvider", () => ({
  useWebSocket: () => ({
    onMessage: (handler: (msg: WSMessage) => void) => {
      ws.handlers.add(handler);
      return () => ws.handlers.delete(handler);
    },
  }),
}));

import MessageFlow from "./MessageFlow";

// S2-G fix, f1: the Go Core announces a handoff as initiated (with the
// target's run), quarantined, rejected, failed or a2a_delegated. The War
// Room knew only initiated/accepted/completed/failed: a quarantined or
// rejected handoff showed a grey arrow that never went away.
describe("MessageFlow", () => {
  let container: HTMLDivElement;

  beforeEach(() => {
    vi.useFakeTimers();
    ws.handlers.clear();
    container = document.createElement("div");
    for (const id of ["agent-a", "agent-b"]) {
      const lane = document.createElement("div");
      lane.dataset.agentId = id;
      container.appendChild(lane);
    }
    document.body.appendChild(container);
  });

  afterEach(() => {
    vi.useRealTimers();
    container.remove();
  });

  const arrows = (root: HTMLElement): string[] =>
    [...root.querySelectorAll("path")].map((p) => p.getAttribute("stroke") ?? "");

  const handoff = (status: HandoffStatusEvent["status"]): HandoffStatusEvent => ({
    source_agent_id: "agent-a",
    target_agent_id: "agent-b",
    run_id: status === "initiated" ? "run-1" : undefined,
    status,
    context: "review the fix",
  });

  it.each([
    ["quarantined", "var(--cf-warning)"],
    ["rejected", "var(--cf-danger)"],
    ["failed", "var(--cf-danger)"],
    ["a2a_delegated", "var(--cf-accent)"],
  ] as const)("shows a %s handoff and removes its arrow after 10 s", (status, color) => {
    const { container: root } = render(() => <MessageFlow containerRef={container} />);

    ws.emit(handoff(status));
    expect(arrows(root)).toEqual([color]);

    vi.advanceTimersByTime(10_000);
    expect(arrows(root)).toEqual([]);
  });

  it("keeps an initiated handoff's arrow while its run works", () => {
    const { container: root } = render(() => <MessageFlow containerRef={container} />);

    ws.emit(handoff("initiated"));
    vi.advanceTimersByTime(10_000);

    expect(arrows(root)).toEqual(["var(--cf-accent)"]);
  });

  it("follows a held handoff that is approved later", () => {
    const { container: root } = render(() => <MessageFlow containerRef={container} />);

    ws.emit(handoff("quarantined"));
    vi.advanceTimersByTime(5_000);
    ws.emit(handoff("initiated"));
    vi.advanceTimersByTime(10_000);

    expect(arrows(root)).toEqual(["var(--cf-accent)"]);
  });
});
