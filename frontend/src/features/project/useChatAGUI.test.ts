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

function setup(first: string, onWorkspaceActivity?: () => void) {
  ws.handlers.clear();
  return createRoot((dispose) => {
    const [conv, setConv] = createSignal<string | null>(first);
    const agui = useChatAGUI({
      activeConversation: conv,
      scrollToBottom: () => undefined,
      refetchMessages: () => undefined,
      refetchSession: () => undefined,
      onWorkspaceActivity,
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

  // KI-129, KI-161: a conversation turn shows its tool calls while it runs.
  it("shows a tool card live and completes it from its result", async () => {
    apiMock.runState.mockReset().mockResolvedValue(idle);
    const { agui, dispose } = setup("conv-1");
    await settle();
    emit("agui.tool_call", {
      run_id: "conv-1",
      call_id: "c1",
      name: "read_file",
      args: '{"path": "a.go"}',
    });
    expect(agui.toolCalls()).toEqual([
      { callId: "c1", name: "read_file", args: { path: "a.go" }, status: "running" },
    ]);
    emit("agui.tool_result", { run_id: "conv-1", call_id: "c1", result: "package a" });
    expect(agui.toolCalls()[0]).toMatchObject({ status: "completed", result: "package a" });
    dispose();
  });

  // A denied call has no output; its card shows why it was denied.
  it("shows the error of a failed call without output", async () => {
    apiMock.runState.mockReset().mockResolvedValue(idle);
    const { agui, dispose } = setup("conv-1");
    await settle();
    emit("agui.tool_call", { run_id: "conv-1", call_id: "c1", name: "bash", args: "" });
    emit("agui.tool_result", {
      run_id: "conv-1",
      call_id: "c1",
      result: "",
      error: "Permission denied: not allowed",
    });
    expect(agui.toolCalls()[0]).toMatchObject({
      status: "failed",
      result: "Permission denied: not allowed",
    });
    dispose();
  });

  // A long argument preview is cut with "..." and is no JSON any more.
  it("keeps an argument preview that is no JSON as text", async () => {
    apiMock.runState.mockReset().mockResolvedValue(idle);
    const { agui, dispose } = setup("conv-1");
    await settle();
    const cut = '{"command": "grep -rn TODO src/ | he...';
    emit("agui.tool_call", { run_id: "conv-1", call_id: "c1", name: "bash", args: cut });
    expect(agui.toolCalls()[0]).toMatchObject({ args: undefined, argsText: cut });
    dispose();
  });

  it("ignores the tool calls of another conversation", async () => {
    apiMock.runState.mockReset().mockResolvedValue(idle);
    const { agui, dispose } = setup("conv-1");
    await settle();
    emit("agui.tool_call", { run_id: "conv-2", call_id: "c1", name: "bash", args: "" });
    emit("agui.tool_result", { run_id: "conv-2", call_id: "c1", result: "x" });
    expect(agui.toolCalls()).toEqual([]);
    dispose();
  });

  // The branch badge follows what the agent did in the workspace.
  it("reports workspace activity on tool results and the turn's end", async () => {
    apiMock.runState.mockReset().mockResolvedValue(idle);
    const activity = vi.fn();
    const { dispose } = setup("conv-1", activity);
    await settle();
    emit("agui.tool_call", { run_id: "conv-1", call_id: "c1", name: "bash", args: "" });
    expect(activity).not.toHaveBeenCalled();
    emit("agui.tool_result", { run_id: "conv-1", call_id: "c1", result: "ok" });
    expect(activity).toHaveBeenCalledTimes(1);
    emit("agui.tool_result", { run_id: "conv-2", call_id: "c9", result: "ok" });
    emit("agui.run_finished", { run_id: "conv-2", status: "completed" });
    expect(activity).toHaveBeenCalledTimes(1);
    emit("agui.run_finished", { run_id: "conv-1", status: "completed" });
    expect(activity).toHaveBeenCalledTimes(2);
    dispose();
  });
});
