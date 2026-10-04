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
    runStatus(runId: string, status: string): void {
      const payload = { run_id: runId, project_id: "p-1", agent_id: "agent-b", status };
      for (const h of [...handlers]) h({ type: "run.status", payload });
    },
  };
});

/** Sets whether the socket is connected (the mock's signal). */
const socket = vi.hoisted(() => ({ set: null as ((connected: boolean) => void) | null }));

vi.mock("~/components/WebSocketProvider", async () => {
  const { createSignal } = await import("solid-js");
  const [connected, setConnected] = createSignal(true);
  socket.set = setConnected;
  return {
    useWebSocket: () => ({
      connected,
      onMessage: (handler: (msg: WSMessage) => void) => {
        ws.handlers.add(handler);
        return () => ws.handlers.delete(handler);
      },
    }),
  };
});

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
    socket.set?.(true);
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

  const handoff = (
    status: HandoffStatusEvent["status"],
    runId: string | undefined = status === "initiated" ? "run-1" : undefined,
  ): HandoffStatusEvent => ({
    source_agent_id: "agent-a",
    target_agent_id: "agent-b",
    run_id: runId,
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

  // KI-92: the arrow of an initiated handoff was never removed, so arrows
  // piled up in a long session. It now settles like the others once the
  // target's run ends.
  it.each(["completed", "failed", "cancelled", "timeout"])(
    "removes an initiated handoff's arrow 10 s after its run ends (%s)",
    (status) => {
      const { container: root } = render(() => <MessageFlow containerRef={container} />);

      ws.emit(handoff("initiated"));
      ws.runStatus("run-1", "running");
      ws.runStatus("run-1", "quality_gate");
      vi.advanceTimersByTime(60_000);
      expect(arrows(root)).toEqual(["var(--cf-accent)"]);

      ws.runStatus("run-1", status);
      vi.advanceTimersByTime(9_999);
      expect(arrows(root)).toEqual(["var(--cf-accent)"]);
      vi.advanceTimersByTime(1);
      expect(arrows(root)).toEqual([]);
    },
  );

  it("keeps the arrow when another run ends", () => {
    const { container: root } = render(() => <MessageFlow containerRef={container} />);

    ws.emit(handoff("initiated"));
    ws.runStatus("run-2", "completed");
    ws.runStatus("", "completed");
    vi.advanceTimersByTime(10_000);

    expect(arrows(root)).toEqual(["var(--cf-accent)"]);
  });

  it("settles an initiated handoff that names no run", () => {
    const { container: root } = render(() => <MessageFlow containerRef={container} />);

    ws.emit(handoff("initiated", ""));
    vi.advanceTimersByTime(10_000);

    expect(arrows(root)).toEqual([]);
  });

  it("follows a new handoff between the same agents after the first run ended", () => {
    const { container: root } = render(() => <MessageFlow containerRef={container} />);

    ws.emit(handoff("initiated"));
    ws.runStatus("run-1", "completed");
    vi.advanceTimersByTime(5_000);
    ws.emit(handoff("initiated", "run-2"));
    vi.advanceTimersByTime(10_000);
    expect(arrows(root)).toEqual(["var(--cf-accent)"]);

    ws.runStatus("run-2", "completed");
    vi.advanceTimersByTime(10_000);
    expect(arrows(root)).toEqual([]);
  });

  it("settles the followed handoffs after a reconnect, whose run ends it may have missed", () => {
    const { container: root } = render(() => <MessageFlow containerRef={container} />);

    ws.emit(handoff("initiated"));
    socket.set?.(false);
    vi.advanceTimersByTime(30_000);
    expect(arrows(root)).toEqual(["var(--cf-accent)"]);

    socket.set?.(true);
    vi.advanceTimersByTime(10_000);
    expect(arrows(root)).toEqual([]);
  });

  // S7-G review: a run that ended before its handoff's initiated status
  // arrived left an arrow that nothing removed.
  it("settles a handoff whose run ended before the handoff was announced", () => {
    const { container: root } = render(() => <MessageFlow containerRef={container} />);

    ws.runStatus("run-1", "failed");
    ws.emit(handoff("initiated"));
    expect(arrows(root)).toEqual(["var(--cf-accent)"]);

    vi.advanceTimersByTime(10_000);
    expect(arrows(root)).toEqual([]);
  });

  it("remembers the last 200 ended runs only", () => {
    const { container: root } = render(() => <MessageFlow containerRef={container} />);

    ws.runStatus("run-1", "completed");
    for (let i = 0; i < 200; i++) ws.runStatus(`other-${i}`, "completed");
    ws.emit(handoff("initiated"));
    vi.advanceTimersByTime(10_000);

    // run-1 is forgotten: its arrow is followed until a run end or reconnect.
    expect(arrows(root)).toEqual(["var(--cf-accent)"]);
  });

  it("removes no arrow after it is unmounted", () => {
    const { container: root, unmount } = render(() => <MessageFlow containerRef={container} />);

    ws.emit(handoff("initiated"));
    ws.runStatus("run-1", "completed");
    unmount();

    expect(vi.getTimerCount()).toBe(0);
    expect(arrows(root)).toEqual([]);
  });
});
