import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMock = vi.hoisted(() => ({
  approve: vi.fn<(runId: string, callId: string, decision: string) => Promise<void>>(),
  allowAlways:
    vi.fn<(projectId: string, tool: string, command?: string, profile?: string) => Promise<void>>(),
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

// ~/ui (used by the toast) links through the router, whose .jsx sources
// vitest cannot load.
vi.mock("@solidjs/router", () => ({
  A: (props: Record<string, unknown>) => props,
  useNavigate: () => () => undefined,
  useLocation: () => ({ pathname: "/" }),
}));

vi.mock("~/api/client", () => ({
  api: {
    runs: { approve: apiMock.approve },
    policies: { allowAlways: apiMock.allowAlways },
  },
}));

import { ToastProvider } from "~/components/Toast";
import { I18nProvider } from "~/i18n";

import PermissionRequestCard, { type PermissionRequestCardProps } from "./PermissionRequestCard";

function renderCard(props: PermissionRequestCardProps): void {
  render(() => (
    <I18nProvider>
      <ToastProvider>
        <PermissionRequestCard {...props} />
      </ToastProvider>
    </I18nProvider>
  ));
}

describe("PermissionRequestCard", () => {
  beforeEach(() => {
    apiMock.approve.mockReset().mockResolvedValue(undefined);
    apiMock.allowAlways.mockReset().mockResolvedValue(undefined);
  });

  it("extends the policy profile that asked when allowing always", async () => {
    renderCard({
      projectId: "p1",
      runId: "r1",
      callId: "c1",
      tool: "bash",
      command: "make build",
      profile: "headless-safe-sandbox",
    });
    fireEvent.click(screen.getByText("Allow Always"));
    await waitFor(() =>
      expect(apiMock.allowAlways).toHaveBeenCalledWith(
        "p1",
        "bash",
        "make build",
        "headless-safe-sandbox",
      ),
    );
    expect(apiMock.approve).toHaveBeenCalledWith("r1", "c1", "allow");
  });

  // Review finding 12: a failed allow-always request was swallowed, so the
  // user believed the rule was saved.
  it("shows an error toast when the allow-always rule cannot be saved", async () => {
    apiMock.allowAlways.mockRejectedValue(new Error("allow-always rules cannot be persisted"));
    renderCard({ projectId: "p1", runId: "r1", callId: "c1", tool: "write_file", path: "a.txt" });
    fireEvent.click(screen.getByText("Allow Always"));
    expect(await screen.findByText(/allow-always rules cannot be persisted/)).toBeTruthy();
    expect(apiMock.approve).toHaveBeenCalledWith("r1", "c1", "allow");
  });
});
