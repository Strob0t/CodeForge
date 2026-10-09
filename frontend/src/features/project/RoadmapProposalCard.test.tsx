import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { AGUIRoadmapProposal } from "~/api/websocket";

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

// ~/ui links through the router, whose .jsx sources vitest cannot load.
vi.mock("@solidjs/router", () => ({
  A: (props: Record<string, unknown>) => props,
  useNavigate: () => () => undefined,
  useLocation: () => ({ pathname: "/" }),
}));

const apiMock = vi.hoisted(() => ({
  createMilestone: vi.fn<(projectId: string, data: { title: string }) => Promise<unknown>>(),
}));

vi.mock("~/api/client", () => ({
  api: { roadmap: { createMilestone: apiMock.createMilestone } },
}));

import { I18nProvider } from "~/i18n";

import RoadmapProposalCard from "./RoadmapProposalCard";

const proposal: AGUIRoadmapProposal = {
  run_id: "r-1",
  proposal_id: "p-1",
  action: "create_milestone",
  milestone_title: "Scaffold the package",
};

function renderCard(onApprove: (title: string) => void): void {
  render(() => (
    <I18nProvider>
      <RoadmapProposalCard
        proposal={proposal}
        projectId="proj-1"
        onApprove={onApprove}
        onReject={() => undefined}
      />
    </I18nProvider>
  ));
}

beforeEach(() => {
  vi.clearAllMocks();
});

// KI-157: approving a card failed silently when the project had no roadmap.
describe("RoadmapProposalCard", () => {
  it("creates the proposed milestone on approval", async () => {
    apiMock.createMilestone.mockResolvedValue({ id: "m-1" });
    const onApprove = vi.fn<(title: string) => void>();
    renderCard(onApprove);

    fireEvent.click(screen.getByText("Approve"));

    await waitFor(() => expect(onApprove).toHaveBeenCalledWith("Scaffold the package"));
    expect(apiMock.createMilestone).toHaveBeenCalledWith("proj-1", {
      title: "Scaffold the package",
      description: undefined,
      sort_order: undefined,
    });
  });

  it("says why an approval failed", async () => {
    apiMock.createMilestone.mockRejectedValue(new Error("project not found"));
    const onApprove = vi.fn<(title: string) => void>();
    renderCard(onApprove);

    fireEvent.click(screen.getByText("Approve"));

    expect((await screen.findByRole("alert")).textContent).toContain("project not found");
    expect(onApprove).not.toHaveBeenCalled();
  });
});
