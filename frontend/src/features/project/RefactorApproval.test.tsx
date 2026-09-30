import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { ReviewImpactEvent } from "~/api/types";
import type { WSMessage } from "~/api/websocket";

const ws = vi.hoisted(() => {
  const handlers = new Set<(msg: WSMessage) => void>();
  return {
    handlers,
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
  },
}));

vi.mock("~/components/WebSocketProvider", () => ({
  useWebSocket: () => ({
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

beforeEach(() => {
  ws.handlers.clear();
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
});
