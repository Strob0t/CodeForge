import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { QuarantineMessage, QuarantineReviewRequest, QuarantineStats } from "~/api/types";
import type { WSMessage } from "~/api/websocket";

const apiMock = vi.hoisted(() => ({
  list: vi.fn<(projectId: string, status?: string) => Promise<QuarantineMessage[]>>(),
  stats: vi.fn<(projectId: string) => Promise<QuarantineStats>>(),
  approve: vi.fn<(id: string, data: QuarantineReviewRequest) => Promise<{ status: string }>>(),
}));

const ws = vi.hoisted(() => {
  const handlers = new Set<(msg: WSMessage) => void>();
  return {
    handlers,
    emit(type: string, payload: Record<string, unknown>): void {
      for (const h of [...handlers]) h({ type, payload });
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
    projects: { list: () => Promise.resolve([{ id: "p-1", name: "Proj" }]) },
    quarantine: {
      list: apiMock.list,
      stats: apiMock.stats,
      approve: apiMock.approve,
      reject: apiMock.approve,
    },
  },
}));

import { ToastProvider } from "~/components/Toast";
import { I18nProvider } from "~/i18n";

import QuarantinePage from "./QuarantinePage";

const pending: QuarantineMessage = {
  id: "q-1",
  tenant_id: "t-1",
  project_id: "p-1",
  subject: "runs.start",
  payload: "",
  trust_origin: "a2a",
  trust_level: "untrusted",
  risk_score: 0.8,
  risk_factors: [],
  status: "pending",
  reviewed_by: "",
  review_note: "",
  created_at: "2026-10-01T00:00:00Z",
  expires_at: "2026-10-02T00:00:00Z",
};

beforeEach(() => {
  ws.handlers.clear();
  apiMock.list.mockReset().mockResolvedValue([pending]);
  apiMock.stats.mockReset().mockResolvedValue({ pending: 1, approved: 0, rejected: 0, expired: 0 });
  apiMock.approve.mockReset().mockResolvedValue({ status: "approved" });
});

function renderPage(): void {
  render(() => (
    <I18nProvider>
      <ToastProvider>
        <QuarantinePage />
      </ToastProvider>
    </I18nProvider>
  ));
}

/** Renders the page with project p-1 selected and its pending message shown. */
async function renderWithProject(): Promise<void> {
  renderPage();
  await waitFor(() => expect(screen.getAllByRole("option").length).toBeGreaterThan(1));
  fireEvent.change(screen.getByLabelText("Select a project"), { target: { value: "p-1" } });
  await screen.findByText("runs.start");
}

// KI-142: the page did not follow messages that were quarantined, approved,
// rejected, withdrawn or expired elsewhere until it reloaded.
describe("QuarantinePage live updates", () => {
  it("shows a message resolved elsewhere as resolved", async () => {
    await renderWithProject();
    expect(screen.getAllByRole("button", { name: "Approve" })).toHaveLength(1);
    const listCalls = apiMock.list.mock.calls.length;
    const statsCalls = apiMock.stats.mock.calls.length;

    apiMock.list.mockResolvedValue([{ ...pending, status: "approved", reviewed_by: "bob" }]);
    ws.emit("quarantine.resolved", { id: "q-1", project_id: "p-1", action: "approved" });

    await waitFor(() =>
      expect(screen.queryAllByRole("button", { name: "Approve" })).toHaveLength(0),
    );
    expect(apiMock.list.mock.calls.length).toBe(listCalls + 1);
    expect(apiMock.stats.mock.calls.length).toBe(statsCalls + 1);
  });

  it("shows a newly quarantined message", async () => {
    await renderWithProject();
    apiMock.list.mockResolvedValue([pending, { ...pending, id: "q-2", subject: "tasks.agent.x" }]);
    ws.emit("quarantine.alert", { id: "q-2", project_id: "p-1", subject: "tasks.agent.x" });
    expect(await screen.findByText("tasks.agent.x")).toBeTruthy();
  });

  // A withdrawal by its sender names no project.
  it("follows a withdrawal of a shown message", async () => {
    await renderWithProject();
    const listCalls = apiMock.list.mock.calls.length;
    apiMock.list.mockResolvedValue([{ ...pending, status: "rejected" }]);
    ws.emit("quarantine.resolved", { id: "q-1", action: "withdrawn", reviewed_by: "sender" });
    await waitFor(() => expect(apiMock.list.mock.calls.length).toBe(listCalls + 1));
  });

  it("ignores the messages of other projects", async () => {
    await renderWithProject();
    const listCalls = apiMock.list.mock.calls.length;
    ws.emit("quarantine.alert", { id: "q-9", project_id: "p-2", subject: "x" });
    ws.emit("quarantine.resolved", { id: "q-8", project_id: "p-2", action: "approved" });
    ws.emit("quarantine.resolved", { id: "q-7", action: "withdrawn" });
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(apiMock.list.mock.calls.length).toBe(listCalls);
  });

  it("stops offering approve in an open detail of a message resolved elsewhere", async () => {
    await renderWithProject();
    fireEvent.click(screen.getByRole("button", { name: "runs.start" }));
    await screen.findByText("Message Detail");
    expect(screen.getAllByRole("button", { name: "Approve" })).toHaveLength(2);

    ws.emit("quarantine.resolved", { id: "q-1", project_id: "p-1", action: "expired" });

    await waitFor(() =>
      expect(screen.queryAllByRole("button", { name: "Approve" })).toHaveLength(1),
    );
  });
});

// KI-79: the reviewer was a free-text name typed into the form; the server now
// records the logged-in user, so the form asks for the note only.
describe("QuarantinePage review", () => {
  it("sends only the note; there is no reviewer name field", async () => {
    render(() => (
      <I18nProvider>
        <ToastProvider>
          <QuarantinePage />
        </ToastProvider>
      </I18nProvider>
    ));
    await waitFor(() => expect(screen.getAllByRole("option").length).toBeGreaterThan(1));
    fireEvent.change(screen.getByLabelText("Select a project"), { target: { value: "p-1" } });

    fireEvent.click((await screen.findAllByRole("button", { name: "Approve" }))[0]);
    expect(document.getElementById("q-reviewer")).toBeNull();
    fireEvent.input(screen.getByLabelText("Review note"), { target: { value: "checked" } });
    const approveButtons = screen.getAllByRole("button", { name: "Approve" });
    fireEvent.click(approveButtons[approveButtons.length - 1]);

    await waitFor(() => expect(apiMock.approve).toHaveBeenCalledWith("q-1", { note: "checked" }));
  });
});
