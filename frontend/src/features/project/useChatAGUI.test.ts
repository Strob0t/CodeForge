import { createRoot, createSignal } from "solid-js";
import { describe, expect, it, vi } from "vitest";

import type { ConversationRunState } from "~/api/types";
import type { AGUIPermissionRequest } from "~/api/websocket";

type Handler = (payload: Record<string, unknown>) => void;

const ws = vi.hoisted(() => ({ handlers: new Map<string, Handler[]>() }));
const apiMock = vi.hoisted(() => ({
  runState: vi.fn<(id: string) => Promise<ConversationRunState>>(),
}));

vi.mock("~/components/WebSocketProvider", () => ({
  useWebSocket: () => ({
    onAGUIEvent: (type: string, handler: Handler) => {
      ws.handlers.set(type, [...(ws.handlers.get(type) ?? []), handler]);
      return () => undefined;
    },
  }),
}));

vi.mock("~/api/client", () => ({
  api: { conversations: { runState: apiMock.runState } },
}));

import { useChatAGUI } from "./useChatAGUI";

function emit(type: string, payload: Record<string, unknown>): void {
  for (const h of ws.handlers.get(type) ?? []) h(payload);
}

function approval(callId: string, runId = "conv-1"): AGUIPermissionRequest {
  return { run_id: runId, call_id: callId, tool: "bash" };
}

function running(text: string, approvals: AGUIPermissionRequest[]): ConversationRunState {
  return { active: true, turn_id: "t1", streamed_text: text, pending_approvals: approvals };
}

const idle: ConversationRunState = { active: false, pending_approvals: [] };

/** A promise the test settles itself. */
function deferred<T>(): { promise: Promise<T>; resolve: (v: T) => void } {
  let resolve: (v: T) => void = () => undefined;
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

function setup(first: string) {
  ws.handlers.clear();
  return createRoot((dispose) => {
    const [conv, setConv] = createSignal<string | null>(first);
    const agui = useChatAGUI({
      activeConversation: conv,
      scrollToBottom: () => undefined,
      refetchMessages: () => undefined,
      refetchSession: () => undefined,
    });
    return { agui, setConv, dispose };
  });
}

const settle = () => new Promise((r) => setTimeout(r, 0));

describe("useChatAGUI", () => {
  it("should be the only named export", async () => {
    const mod = await import("./useChatAGUI");
    expect(Object.keys(mod)).toEqual(["useChatAGUI"]);
  });

  // KI-148: a reload restores the running turn the Core reports.
  it("restores the running turn on load", async () => {
    apiMock.runState.mockReset().mockResolvedValue(running("Hello", [approval("c1")]));
    const { agui, dispose } = setup("conv-1");
    await settle();
    expect(agui.agentRunning()).toBe(true);
    expect(agui.streamingContent()).toBe("Hello");
    expect(agui.permissionRequests().map((pr) => pr.call_id)).toEqual(["c1"]);
    dispose();
  });

  // KI-148 review: switching conversations must not carry the previous
  // conversation's approval cards or streamed text over.
  it("clears the previous conversation's cards and text on a switch", async () => {
    apiMock.runState
      .mockReset()
      .mockImplementation((id) =>
        Promise.resolve(id === "conv-1" ? running("Old", [approval("c1")]) : idle),
      );
    const { agui, setConv, dispose } = setup("conv-1");
    await settle();
    expect(agui.permissionRequests()).toHaveLength(1);
    setConv("conv-2");
    await settle();
    expect(agui.permissionRequests()).toEqual([]);
    expect(agui.streamingContent()).toBe("");
    dispose();
  });

  // KI-148 review: a run that starts while the request is out is shown from
  // its own live events; the answer, which may describe the run before, is
  // not mixed in.
  it("keeps a run that started during the request as its live events show it", async () => {
    const answer = deferred<ConversationRunState>();
    apiMock.runState.mockReset().mockReturnValue(answer.promise);
    const { agui, dispose } = setup("conv-1");
    emit("agui.run_started", { run_id: "conv-1" });
    emit("agui.text_message", { run_id: "conv-1", content: "New" });
    answer.resolve(running("Old turn text", [approval("old-call")]));
    await settle();
    expect(agui.streamingContent()).toBe("New");
    expect(agui.permissionRequests()).toEqual([]);
    dispose();
  });

  // KI-148 review: the live event of an approval the restore already shows
  // does not add a second card.
  it("shows a restored approval once when its live event arrives", async () => {
    apiMock.runState.mockReset().mockResolvedValue(running("", [approval("c1")]));
    const { agui, dispose } = setup("conv-1");
    await settle();
    emit("agui.permission_request", { ...approval("c1") });
    expect(agui.permissionRequests()).toHaveLength(1);
    dispose();
  });
});
