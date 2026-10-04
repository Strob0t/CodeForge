import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { ProviderInfo, Roadmap, RoadmapSyncRequest, RoadmapSyncResult } from "~/api/types";

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

const apiMock = vi.hoisted(() => ({
  get: vi.fn<(projectId: string) => Promise<Roadmap>>(),
  sync: vi.fn<(projectId: string, data: RoadmapSyncRequest) => Promise<RoadmapSyncResult>>(),
  pm: vi.fn<() => Promise<ProviderInfo[]>>(),
}));

vi.mock("~/api/client", () => ({
  api: {
    roadmap: { get: apiMock.get, sync: apiMock.sync },
    providers: { pm: apiMock.pm },
  },
}));

import { ToastProvider } from "~/components/Toast";
import { I18nProvider } from "~/i18n";

import RoadmapPanel from "./RoadmapPanel";

const ROADMAP: Roadmap = {
  id: "r-1",
  project_id: "p-1",
  title: "Shop roadmap",
  description: "",
  status: "active",
  milestones: [],
  version: 1,
  created_at: "",
  updated_at: "",
};

function renderPanel(): void {
  render(() => (
    <I18nProvider>
      <ToastProvider>
        <RoadmapPanel projectId="p-1" onError={() => undefined} />
      </ToastProvider>
    </I18nProvider>
  ));
}

// KI-121: the roadmap UI never called POST /projects/{id}/roadmap/sync.
describe("RoadmapPanel sync", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    apiMock.get.mockResolvedValue(ROADMAP);
    apiMock.pm.mockResolvedValue([
      { name: "github-issues", capabilities: { create_item: true, update_item: true } },
    ]);
    apiMock.sync.mockResolvedValue({
      direction: "pull",
      created: 1,
      updated: 0,
      skipped: 0,
      dry_run: false,
    });
  });

  it("offers the sync next to Import from PM and reloads the roadmap after a sync", async () => {
    renderPanel();
    await screen.findByText("Shop roadmap");
    expect(screen.getByRole("button", { name: "Import from PM" })).toBeDefined();

    fireEvent.click(screen.getByRole("button", { name: "Sync with PM" }));
    fireEvent.change(await screen.findByLabelText("Select PM provider"), {
      target: { value: "github-issues" },
    });
    fireEvent.input(screen.getByLabelText("PM project reference"), {
      target: { value: "owner/repo" },
    });
    fireEvent.click(screen.getByRole("checkbox", { name: /^Preview only/ }));
    fireEvent.click(screen.getByRole("button", { name: "Sync now" }));

    await waitFor(() => expect(apiMock.sync).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(apiMock.get).toHaveBeenCalledTimes(2));
  });
});
