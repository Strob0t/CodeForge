import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { createSignal } from "solid-js";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { PendingReviewDecision, ReviewImpactEvent } from "~/api/types";
import type { WSMessage } from "~/api/websocket";

const ws = vi.hoisted(() => {
  const handlers = new Set<(msg: WSMessage) => void>();
  return {
    handlers,
    connected: { get: (): boolean => true, set: (() => undefined) as (v: boolean) => void },
    emit(type: string, payload: Record<string, unknown>): void {
      for (const h of [...handlers]) h({ type, payload });
    },
  };
});

const apiMock = vi.hoisted(() => ({
  approveRefactor:
    vi.fn<(runId: string, step: { plan_id: string; step_id: string }) => Promise<unknown>>(),
  rejectRefactor:
    vi.fn<(runId: string, step: { plan_id: string; step_id: string }) => Promise<unknown>>(),
  pending: vi.fn<(projectId: string) => Promise<PendingReviewDecision[]>>(),
  toast: vi.fn<(level: string, message: string) => number>(),
}));

// jsdom has no matchMedia; the UI modules read it when they are loaded.
vi.hoisted(() => {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => undefined,
      removeListener: () => undefined,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      dispatchEvent: () => false,
    }),
  });
});

vi.mock("@solidjs/router", () => ({
  A: (props: Record<string, unknown>) => props,
  useNavigate: () => () => undefined,
  useLocation: () => ({ pathname: "/" }),
}));

vi.mock("~/api/client", () => ({
  api: {
    runs: { approveRefactor: apiMock.approveRefactor, rejectRefactor: apiMock.rejectRefactor },
    projects: { pendingReviewDecisions: apiMock.pending },
  },
}));

vi.mock("~/components/WebSocketProvider", () => ({
  useWebSocket: () => ({
    connected: () => ws.connected.get(),
    onMessage: (handler: (msg: WSMessage) => void) => {
      ws.handlers.add(handler);
      return () => ws.handlers.delete(handler);
    },
  }),
}));

vi.mock("~/components/Toast", () => ({
  useToast: () => ({ show: apiMock.toast, dismiss: () => undefined }),
}));

import RefactorApproval from "./RefactorApproval";

function impact(overrides: Partial<ReviewImpactEvent> = {}): Record<string, unknown> {
  return {
    run_id: "run-4",
    plan_id: "plan-1",
    step_id: "step-3",
    project_id: "p-1",
    impact_level: "high",
    files_changed: 3,
    lines_added: 250,
    lines_removed: 40,
    cross_layer: true,
    structural: false,
    ...overrides,
  };
}

function pendingDecision(overrides: Partial<PendingReviewDecision> = {}): PendingReviewDecision {
  return {
    ...(impact() as unknown as ReviewImpactEvent),
    step_status: "waiting_approval",
    plan_status: "running",
    since: "2026-10-01T10:00:00Z",
    ...overrides,
  };
}

beforeEach(() => {
  ws.handlers.clear();
  const [connected, setConnected] = createSignal(true);
  ws.connected.get = connected;
  ws.connected.set = setConnected;
  apiMock.pending.mockReset().mockResolvedValue([]);
  apiMock.approveRefactor.mockReset().mockResolvedValue({ status: "approved" });
  apiMock.rejectRefactor.mockReset().mockResolvedValue({ status: "rejected" });
  apiMock.toast.mockReset();
});

// KI-17: the dialog follows the Go event (review.approval_required with
// event.ReviewImpactEvent) and decides through the authenticated API client.
describe("RefactorApproval", () => {
  it("opens for its project's approval request with the impact", async () => {
    render(() => <RefactorApproval projectId="p-1" />);

    ws.emit("review.approval_required", impact());

    await screen.findByRole("dialog", { name: "Refactor approval" });
    expect(screen.getByText("3")).toBeTruthy();
    expect(screen.getByText("+250")).toBeTruthy();
    expect(screen.getByText("-40")).toBeTruthy();
    expect(screen.getByText(/Cross-layer changes/)).toBeTruthy();
  });

  it("ignores other projects and the old event name", async () => {
    render(() => <RefactorApproval projectId="p-1" />);

    ws.emit("review.approval_required", impact({ project_id: "p-2" }));
    ws.emit("refactor.approval_required", impact());
    await new Promise((resolve) => setTimeout(resolve, 20));

    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("approves through the API client and closes", async () => {
    render(() => <RefactorApproval projectId="p-1" />);
    ws.emit("review.approval_required", impact());

    fireEvent.click(await screen.findByRole("button", { name: "Approve" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(apiMock.approveRefactor).toHaveBeenCalledWith("run-4", {
      plan_id: "plan-1",
      step_id: "step-3",
    });
    expect(apiMock.rejectRefactor).not.toHaveBeenCalled();
  });

  it("keeps the dialog open with the error when the rejection fails", async () => {
    apiMock.rejectRefactor.mockRejectedValue(new Error("no workspace baseline to undo it"));
    render(() => <RefactorApproval projectId="p-1" />);
    ws.emit("review.approval_required", impact());

    fireEvent.click(await screen.findByRole("button", { name: "Reject" }));

    await screen.findByText(/no workspace baseline to undo it/);
    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(apiMock.rejectRefactor).toHaveBeenCalledWith("run-4", {
      plan_id: "plan-1",
      step_id: "step-3",
    });
  });

  it("shows why an unscored refactoring needs approval", async () => {
    render(() => <RefactorApproval projectId="p-1" />);
    ws.emit(
      "review.approval_required",
      impact({ reason: "the change could not be measured", impact_level: "high" }),
    );

    await screen.findByText(/the change could not be measured/);
  });

  it("notifies about an applied medium-impact refactoring", async () => {
    render(() => <RefactorApproval projectId="p-1" />);

    ws.emit("review.refactor_applied", impact({ impact_level: "medium", lines_added: 80 }));
    ws.emit("review.refactor_applied", impact({ project_id: "p-2" }));

    await waitFor(() => expect(apiMock.toast).toHaveBeenCalledTimes(1));
    expect(apiMock.toast.mock.calls[0][0]).toBe("info");
    expect(apiMock.toast.mock.calls[0][1]).toMatch(/applied/i);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  // S6-F 6: pending decisions come from the server too, so a missed event
  // loses nothing; several wait in a queue.
  it("loads the pending decisions on mount and decides them one after the other", async () => {
    apiMock.pending.mockResolvedValue([
      pendingDecision(),
      pendingDecision({
        run_id: "run-9",
        step_id: "step-9",
        step_status: "failed",
        reason: "the refactoring step ended failed",
      }),
    ]);
    render(() => <RefactorApproval projectId="p-1" />);

    await screen.findByText(/1 of 2 refactorings/);
    expect(apiMock.pending).toHaveBeenCalledWith("p-1");
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));

    await screen.findByText(/the refactoring step ended failed/);
    expect(screen.getByText(/Refactoring failed: keep or undo/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(apiMock.approveRefactor).toHaveBeenCalledWith("run-4", {
      plan_id: "plan-1",
      step_id: "step-3",
    });
    expect(apiMock.rejectRefactor).toHaveBeenCalledWith("run-9", {
      plan_id: "plan-1",
      step_id: "step-9",
    });
  });

  it("queues a second request instead of replacing the first", async () => {
    render(() => <RefactorApproval projectId="p-1" />);
    ws.emit("review.approval_required", impact());
    ws.emit("review.approval_required", impact({ run_id: "run-5", step_id: "step-5" }));
    ws.emit("review.approval_required", impact()); // the same request again

    await screen.findByText(/1 of 2 refactorings/);
  });

  it("reloads the pending decisions when the WebSocket reconnects", async () => {
    render(() => <RefactorApproval projectId="p-1" />);
    await waitFor(() => expect(apiMock.pending).toHaveBeenCalledTimes(1));
    apiMock.pending.mockResolvedValue([pendingDecision()]);

    ws.connected.set(false);
    ws.connected.set(true);

    await screen.findByRole("dialog", { name: "Refactor approval" });
    expect(apiMock.pending).toHaveBeenCalledTimes(2);
  });

  it("says when an undo left HEAD where it is", async () => {
    apiMock.rejectRefactor.mockResolvedValue({
      status: "rejected",
      head_restored: false,
      message: "HEAD was left at main: only the files were restored",
    });
    render(() => <RefactorApproval projectId="p-1" />);
    ws.emit("review.approval_required", impact());

    fireEvent.click(await screen.findByRole("button", { name: "Reject" }));

    await waitFor(() =>
      expect(apiMock.toast).toHaveBeenCalledWith("warning", expect.stringMatching(/HEAD was left/)),
    );
  });
});
