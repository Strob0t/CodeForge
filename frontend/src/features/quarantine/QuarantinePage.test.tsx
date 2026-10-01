import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { QuarantineMessage, QuarantineReviewRequest } from "~/api/types";

const apiMock = vi.hoisted(() => ({
  list: vi.fn<(projectId: string, status?: string) => Promise<QuarantineMessage[]>>(),
  approve: vi.fn<(id: string, data: QuarantineReviewRequest) => Promise<{ status: string }>>(),
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
      stats: () => Promise.resolve({ pending: 1, approved: 0, rejected: 0, expired: 0 }),
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
  apiMock.list.mockReset().mockResolvedValue([pending]);
  apiMock.approve.mockReset().mockResolvedValue({ status: "approved" });
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
