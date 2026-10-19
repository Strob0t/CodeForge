import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { ProviderInfo, RoadmapSyncRequest, RoadmapSyncResult } from "~/api/types";

// KI-121: the roadmap UI never called the bidirectional sync
// (POST /projects/{id}/roadmap/sync). It sits next to Import from PM now, with
// an explicit direction.

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

const roadmap = vi.hoisted(() => ({
  sync: vi.fn<(projectId: string, data: RoadmapSyncRequest) => Promise<RoadmapSyncResult>>(),
}));

vi.mock("~/api/client", () => ({ api: { roadmap } }));

import { ToastProvider } from "~/components/Toast";
import { I18nProvider } from "~/i18n";

import RoadmapSyncForm from "./RoadmapSyncForm";

const PROVIDERS: ProviderInfo[] = [
  {
    name: "github-issues",
    capabilities: { list_items: true, create_item: true, update_item: true },
  },
  { name: "plane", capabilities: { list_items: true, create_item: true, update_item: true } },
  { name: "readonly-pm", capabilities: { list_items: true } },
];

const onSynced = vi.fn<(result: RoadmapSyncResult) => void>();

function renderForm(): void {
  render(() => (
    <I18nProvider>
      <ToastProvider>
        <RoadmapSyncForm
          projectId="p-1"
          providers={PROVIDERS}
          onSynced={onSynced}
          onCancel={() => undefined}
        />
      </ToastProvider>
    </I18nProvider>
  ));
}

function choose(provider: string, ref = "owner/repo"): void {
  fireEvent.change(screen.getByLabelText("Select PM provider"), { target: { value: provider } });
  fireEvent.input(screen.getByLabelText("PM project reference"), { target: { value: ref } });
}

const radio = (name: RegExp): HTMLInputElement =>
  screen.getByRole("radio", { name }) as HTMLInputElement;
const checkbox = (name: RegExp): HTMLInputElement =>
  screen.getByRole("checkbox", { name }) as HTMLInputElement;

beforeEach(() => {
  vi.clearAllMocks();
  roadmap.sync.mockResolvedValue({
    direction: "pull",
    created: 2,
    updated: 1,
    skipped: 3,
    dry_run: true,
  });
});

describe("RoadmapSyncForm", () => {
  it("previews a pull by default, creating and updating, without credentials", async () => {
    renderForm();
    choose("github-issues");

    expect(radio(/^Pull/).checked).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Preview" }));

    await waitFor(() => expect(roadmap.sync).toHaveBeenCalled());
    expect(roadmap.sync).toHaveBeenCalledWith("p-1", {
      provider: "github-issues",
      project_ref: "owner/repo",
      direction: "pull",
      dry_run: true,
      create_new: true,
      update_exist: true,
    });
    expect(await screen.findByText("pull: 2 created, 1 updated, 3 skipped")).toBeDefined();
    expect(screen.getByText("Preview only: nothing was changed.")).toBeDefined();
    // A preview changes nothing the panel would have to reload.
    expect(onSynced).not.toHaveBeenCalled();
  });

  it.each([
    [/^Push/, "push"],
    [/^Both/, "bidi"],
  ] as const)(
    "runs the chosen direction %s with the options and the token",
    async (label, direction) => {
      roadmap.sync.mockResolvedValue({
        direction,
        created: 1,
        updated: 0,
        skipped: 0,
        errors: ["create f-9: rate limited"],
        dry_run: false,
      });
      renderForm();
      choose("github-issues");
      fireEvent.click(radio(label));
      fireEvent.click(checkbox(/^Update items/));
      fireEvent.click(checkbox(/^Preview only/));
      fireEvent.input(screen.getByLabelText(/^Access token/), { target: { value: "ghp_x" } });
      fireEvent.click(screen.getByRole("button", { name: "Sync now" }));

      await waitFor(() => expect(roadmap.sync).toHaveBeenCalled());
      expect(roadmap.sync.mock.calls[0][1]).toEqual({
        provider: "github-issues",
        project_ref: "owner/repo",
        direction,
        dry_run: false,
        create_new: true,
        update_exist: false,
        provider_config: { token: "ghp_x" },
      });
      expect(await screen.findByText("create f-9: rate limited")).toBeDefined();
      expect(onSynced).toHaveBeenCalledTimes(1);
    },
  );

  it("sends a Plane token as api_token", async () => {
    renderForm();
    choose("plane", "ws/proj");
    fireEvent.input(screen.getByLabelText(/^Access token/), { target: { value: "pl-1" } });
    fireEvent.click(screen.getByRole("button", { name: "Preview" }));

    await waitFor(() => expect(roadmap.sync).toHaveBeenCalled());
    expect(roadmap.sync.mock.calls[0][1].provider_config).toEqual({ api_token: "pl-1" });
  });

  it("offers only pull for a provider that cannot write", () => {
    renderForm();
    choose("github-issues");
    fireEvent.click(radio(/^Push/));
    choose("readonly-pm");

    expect(radio(/^Push/).disabled).toBe(true);
    expect(radio(/^Both/).disabled).toBe(true);
    expect(radio(/^Pull/).checked).toBe(true);
    expect(
      screen.getByText("readonly-pm cannot create or update items, so only pull is possible."),
    ).toBeDefined();
  });

  it.each([
    ["", "owner/repo"],
    ["github-issues", "   "],
  ])("needs a provider and a project reference (%j, %j)", (provider, ref) => {
    renderForm();
    choose(provider, ref);

    expect((screen.getByRole("button", { name: "Preview" }) as HTMLButtonElement).disabled).toBe(
      true,
    );
  });

  // S7-G review: the token typed for one provider was sent to the next one
  // chosen, and it stayed in the form after the sync it was for.
  it("forgets the token when the provider changes", async () => {
    renderForm();
    choose("github-issues");
    const token = screen.getByLabelText(/^Access token/) as HTMLInputElement;
    fireEvent.input(token, { target: { value: "ghp_x" } });

    choose("plane", "ws/proj");
    expect(token.value).toBe("");
    fireEvent.click(screen.getByRole("button", { name: "Preview" }));

    await waitFor(() => expect(roadmap.sync).toHaveBeenCalled());
    expect(roadmap.sync.mock.calls[0][1]).not.toHaveProperty("provider_config", expect.anything());
  });

  it("forgets the token after a real sync, keeps it after a preview", async () => {
    renderForm();
    choose("github-issues");
    const token = screen.getByLabelText(/^Access token/) as HTMLInputElement;
    fireEvent.input(token, { target: { value: "ghp_x" } });

    fireEvent.click(screen.getByRole("button", { name: "Preview" }));
    await screen.findByText("Preview only: nothing was changed.");
    expect(token.value).toBe("ghp_x");

    roadmap.sync.mockResolvedValue({
      direction: "pull",
      created: 1,
      updated: 0,
      skipped: 0,
      dry_run: false,
    });
    fireEvent.click(checkbox(/^Preview only/));
    fireEvent.click(screen.getByRole("button", { name: "Sync now" }));
    await waitFor(() => expect(onSynced).toHaveBeenCalled());
    expect(token.value).toBe("");
  });

  it("reports a failed sync", async () => {
    roadmap.sync.mockRejectedValue(new Error("project_ref is required"));
    renderForm();
    choose("github-issues");
    fireEvent.click(screen.getByRole("button", { name: "Preview" }));

    expect(await screen.findByText("project_ref is required")).toBeDefined();
    expect(onSynced).not.toHaveBeenCalled();
  });
});
