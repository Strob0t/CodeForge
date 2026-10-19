import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { BoundaryConfig, ReviewTriggerResponse } from "~/api/types";
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
  getBoundaries: vi.fn<(id: string) => Promise<BoundaryConfig>>(),
  trigger: vi.fn<(id: string) => Promise<ReviewTriggerResponse>>(),
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
    projects: { getBoundaries: apiMock.getBoundaries, triggerBoundaryAnalysis: apiMock.trigger },
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

import BoundariesPanel from "./BoundariesPanel";

const config: BoundaryConfig = {
  project_id: "p-1",
  boundaries: [{ path: "api/user.proto", type: "api", counterpart: "", auto_detected: true }],
  last_analyzed: "2026-09-30T00:00:00Z",
  version: 1,
};

beforeEach(() => {
  ws.handlers.clear();
  apiMock.getBoundaries.mockReset().mockResolvedValue(config);
  apiMock.trigger.mockReset().mockResolvedValue({ triggered: true, plan_id: "plan-1" });
});

// KI-17: the analysis runs as a plan; the panel reloads when it ends and says
// why nothing started.
describe("BoundariesPanel analysis", () => {
  it("reloads the boundaries when the analysis plan ends", async () => {
    render(() => <BoundariesPanel projectId="p-1" />);
    await screen.findByText("api/user.proto");

    fireEvent.click(screen.getByRole("button", { name: "Re-analyze" }));
    await screen.findByRole("button", { name: /Analyzing/ });

    ws.emit("plan.status", { plan_id: "plan-9", project_id: "p-1", status: "completed" });
    ws.emit("plan.status", { plan_id: "plan-1", project_id: "p-1", status: "running" });
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(apiMock.getBoundaries).toHaveBeenCalledTimes(1);

    ws.emit("plan.status", { plan_id: "plan-1", project_id: "p-1", status: "completed" });
    await waitFor(() => expect(apiMock.getBoundaries).toHaveBeenCalledTimes(2));
    await screen.findByRole("button", { name: "Re-analyze" });
  });

  it("shows why the analysis did not start", async () => {
    apiMock.trigger.mockRejectedValue(new Error("project has no agents"));
    render(() => <BoundariesPanel projectId="p-1" />);
    await screen.findByText("api/user.proto");

    fireEvent.click(screen.getByRole("button", { name: "Re-analyze" }));

    await screen.findByText(/project has no agents/);
    expect(screen.getByRole("button", { name: "Re-analyze" })).toBeTruthy();
  });
});
