import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

// KI-57: approval emails link to this page; it shows the pending tool call
// and decides only on a click (authenticated POST).

const apiMock = vi.hoisted(() => ({
  pendingApproval: vi.fn(),
  decideApproval: vi.fn(),
}));

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
  useLocation: () => ({ pathname: "/approvals/run-1/call-1" }),
  useParams: () => ({ runId: "run-1", callId: "call-1" }),
}));

vi.mock("~/api/client", () => ({
  api: {
    runs: { pendingApproval: apiMock.pendingApproval, decideApproval: apiMock.decideApproval },
  },
}));

import { I18nProvider } from "~/i18n";

import ApprovalPage from "./ApprovalPage";

function renderPage(): void {
  render(() => (
    <I18nProvider>
      <ApprovalPage />
    </I18nProvider>
  ));
}

describe("ApprovalPage", () => {
  beforeEach(() => {
    apiMock.pendingApproval.mockReset();
    apiMock.decideApproval.mockReset().mockResolvedValue({ status: "resolved", decision: "allow" });
  });

  it("shows the pending call and decides nothing until a button is clicked", async () => {
    apiMock.pendingApproval.mockResolvedValue({
      run_id: "run-1",
      call_id: "call-1",
      tool: "Bash",
      command: "make deploy",
    });
    renderPage();

    expect(await screen.findByText("make deploy")).toBeTruthy();
    expect(apiMock.pendingApproval).toHaveBeenCalledWith("run-1", "call-1");
    expect(apiMock.decideApproval).not.toHaveBeenCalled();

    fireEvent.click(screen.getByText("Approve"));
    await waitFor(() =>
      expect(apiMock.decideApproval).toHaveBeenCalledWith("run-1", "call-1", "allow"),
    );
    expect(await screen.findByText("Approved.")).toBeTruthy();
  });

  it("says so when the call is no longer pending", async () => {
    apiMock.pendingApproval.mockRejectedValue(new Error("404"));
    renderPage();

    expect(await screen.findByText(/no longer waiting for a decision/)).toBeTruthy();
    expect(screen.queryByText("Approve")).toBeNull();
  });
});
