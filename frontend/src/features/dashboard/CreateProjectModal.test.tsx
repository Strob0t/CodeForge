import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { CreateProjectRequest, Project } from "~/api/types";

const apiMock = vi.hoisted(() => ({
  create: vi.fn<(data: CreateProjectRequest) => Promise<Project>>(),
  initWorkspace: vi.fn<(id: string) => Promise<unknown>>(),
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
    providers: { git: () => Promise.resolve({ providers: [] }) },
    projects: { create: apiMock.create, initWorkspace: apiMock.initWorkspace },
  },
}));

import { ToastProvider } from "~/components/Toast";
import { I18nProvider } from "~/i18n";

import { CreateProjectModal } from "./CreateProjectModal";

beforeEach(() => {
  apiMock.create.mockReset().mockResolvedValue({ id: "p-1" } as Project);
  apiMock.initWorkspace.mockReset().mockResolvedValue({});
});

// KI-74 (KI-41 follow-up): the modal wrote an autonomy_level config key that
// no backend code reads; autonomy comes from the selected mode.
describe("CreateProjectModal", () => {
  it("creates a project without an autonomy level", async () => {
    const onCreated = vi.fn();
    render(() => (
      <I18nProvider>
        <ToastProvider>
          <CreateProjectModal open onClose={() => undefined} onCreated={onCreated} />
        </ToastProvider>
      </I18nProvider>
    ));

    expect(screen.queryByText("Autonomy Level")).toBeNull();
    expect(screen.queryByText("Advanced Settings")).toBeNull();

    fireEvent.click(screen.getByRole("tab", { name: "Empty Project" }));
    fireEvent.input(screen.getByLabelText(/^Name/), { target: { value: "Demo" } });
    fireEvent.submit(
      screen.getByRole("button", { name: "Create Project" }).closest("form") as HTMLFormElement,
    );

    await waitFor(() => expect(onCreated).toHaveBeenCalledTimes(1));
    expect(apiMock.create).toHaveBeenCalledWith(
      expect.objectContaining({ name: "Demo", config: {} }),
    );
  });
});
