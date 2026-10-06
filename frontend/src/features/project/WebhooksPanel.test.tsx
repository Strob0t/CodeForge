import { fireEvent, render, screen, waitFor, within } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { WebhookEndpoint, WebhookRegistered } from "~/api/types";

// KI-109: the per-project inbound webhooks of KI-85 had no screen. Admins
// register, rotate and delete them (the secret is shown once), editors see
// them, viewers get 403 from the Go Core and see nothing.

const webhooks = vi.hoisted(() => ({
  list: vi.fn<(projectId: string) => Promise<WebhookEndpoint[]>>(),
  create: vi.fn(),
  rotate: vi.fn(),
  setAPIToken: vi.fn(),
  delete: vi.fn(),
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
  useLocation: () => ({ pathname: "/" }),
}));

vi.mock("~/api/client", () => ({ api: { webhooks } }));

/** The signed-in user's role; the Go Core stays the authority. */
const auth = vi.hoisted(() => ({ role: "admin" as "admin" | "editor" | "viewer" }));

vi.mock("~/components/AuthProvider", () => ({
  useAuth: () => ({
    hasRole: (...roles: string[]) => roles.includes(auth.role),
  }),
}));

import { ToastProvider } from "~/components/Toast";
import { I18nProvider } from "~/i18n";

import WebhooksPanel from "./WebhooksPanel";

const vcsGitHub: WebhookEndpoint = {
  id: "w-vcs",
  project_id: "p1",
  kind: "vcs",
  provider: "github",
  url: "/api/v1/webhooks/vcs/github/w-vcs",
  has_api_token: false,
  created_at: "2026-10-01T10:00:00Z",
  secret_rotated_at: "2026-10-01T10:00:00Z",
};

const pmPlane: WebhookEndpoint = {
  id: "w-plane",
  project_id: "p1",
  kind: "pm",
  provider: "plane",
  url: "/api/v1/webhooks/pm/plane/w-plane",
  has_api_token: true,
  created_at: "2026-10-02T10:00:00Z",
  secret_rotated_at: "2026-10-03T10:00:00Z",
};

const SECRET = "a1b2c3d4".repeat(8);

function registered(endpoint: WebhookEndpoint, secret = SECRET): WebhookRegistered {
  return { ...endpoint, secret };
}

const fullURL = (path: string): string => new URL(path, window.location.origin).href;

function renderPanel(): void {
  render(() => (
    <I18nProvider>
      <ToastProvider>
        <WebhooksPanel projectId="p1" />
      </ToastProvider>
    </I18nProvider>
  ));
}

/** Everything the page keeps in browser storage, to check no secret lands there. */
function browserStorage(): string {
  const dump = (s: Storage): string =>
    Array.from({ length: s.length }, (_, i) => {
      const key = s.key(i) ?? "";
      return `${key}=${s.getItem(key) ?? ""}`;
    }).join("\n");
  return dump(localStorage) + dump(sessionStorage);
}

async function openForm(): Promise<HTMLFormElement> {
  fireEvent.click(await screen.findByRole("button", { name: "Add webhook" }));
  return screen.findByRole("form", { name: "Register a webhook" });
}

describe("WebhooksPanel", () => {
  let clipboard: string[];

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    sessionStorage.clear();
    auth.role = "admin";
    webhooks.list.mockResolvedValue([vcsGitHub, pmPlane]);
    webhooks.create.mockResolvedValue(registered(vcsGitHub));
    webhooks.rotate.mockResolvedValue(registered(vcsGitHub));
    webhooks.setAPIToken.mockResolvedValue(undefined);
    webhooks.delete.mockResolvedValue(undefined);
    clipboard = [];
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: (text: string) => Promise.resolve(void clipboard.push(text)) },
    });
  });

  it("lists kind, provider, the full inbound URL and the creation time", async () => {
    renderPanel();

    expect(await screen.findByDisplayValue(fullURL(vcsGitHub.url))).toBeTruthy();
    expect(screen.getByDisplayValue(fullURL(pmPlane.url))).toBeTruthy();
    expect(webhooks.list).toHaveBeenCalledWith("p1");
    const row = screen.getByTestId("webhook-w-vcs");
    expect(within(row).getByText("VCS events")).toBeTruthy();
    expect(within(row).getByText("GitHub")).toBeTruthy();
    expect(within(row).getByText("Created")).toBeTruthy();
    const plane = screen.getByTestId("webhook-w-plane");
    expect(within(plane).getByText("Roadmap sync")).toBeTruthy();
    expect(within(plane).getByText("Plane")).toBeTruthy();
    expect(within(plane).getByText("Own token")).toBeTruthy();
  });

  it("copies the full inbound URL", async () => {
    renderPanel();
    fireEvent.click(await screen.findByLabelText("Copy the URL of the GitHub VCS events webhook"));
    await waitFor(() => expect(clipboard).toEqual([fullURL(vcsGitHub.url)]));
  });

  it("shows an empty state without webhooks", async () => {
    webhooks.list.mockResolvedValue([]);
    renderPanel();
    expect(await screen.findByText("No webhooks yet")).toBeTruthy();
  });

  it("shows an error with a retry when the list cannot be loaded", async () => {
    webhooks.list.mockRejectedValueOnce(new Error("boom"));
    renderPanel();
    expect(await screen.findByText("The webhooks could not be loaded.")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByDisplayValue(fullURL(vcsGitHub.url))).toBeTruthy();
    expect(webhooks.list).toHaveBeenCalledTimes(2);
  });

  it("registers a webhook and shows its secret once", async () => {
    renderPanel();
    const form = await openForm();
    fireEvent.change(within(form).getByLabelText(/Kind/), { target: { value: "vcs" } });
    fireEvent.change(within(form).getByLabelText(/Provider/), { target: { value: "github" } });
    fireEvent.submit(form);

    await waitFor(() =>
      expect(webhooks.create).toHaveBeenCalledWith("p1", { kind: "vcs", provider: "github" }),
    );
    const shown = await screen.findByTestId("webhook-secret");
    expect((shown as HTMLInputElement).value).toBe(SECRET);
    expect(screen.getByText(/You will not see this secret again/)).toBeTruthy();
    expect(webhooks.list).toHaveBeenCalledTimes(2);
    expect(browserStorage()).not.toContain(SECRET);

    fireEvent.click(screen.getByLabelText("Copy the secret"));
    await waitFor(() => expect(clipboard).toEqual([SECRET]));

    fireEvent.click(screen.getByRole("button", { name: "I have saved it" }));
    await waitFor(() => expect(screen.queryByTestId("webhook-secret")).toBeNull());
    expect(screen.queryByDisplayValue(SECRET)).toBeNull();
  });

  it("sends a PM webhook's API token and offers only the providers of the kind", async () => {
    webhooks.create.mockResolvedValue(registered({ ...vcsGitHub, kind: "pm", provider: "gitlab" }));
    renderPanel();
    const form = await openForm();
    const provider = within(form).getByLabelText(/Provider/);
    const options = (): string[] =>
      Array.from((provider as HTMLSelectElement).options, (o) => o.value);
    expect(options()).toEqual(["github", "gitlab"]);
    expect(within(form).queryByLabelText(/API token/)).toBeNull();

    fireEvent.change(within(form).getByLabelText(/Kind/), { target: { value: "pm" } });
    expect(options()).toEqual(["github", "gitlab", "plane"]);
    fireEvent.change(provider, { target: { value: "gitlab" } });
    fireEvent.input(within(form).getByLabelText(/API token/), { target: { value: "glpat-x" } });
    fireEvent.submit(form);

    await waitFor(() =>
      expect(webhooks.create).toHaveBeenCalledWith("p1", {
        kind: "pm",
        provider: "gitlab",
        api_token: "glpat-x",
      }),
    );
    expect(await screen.findByTestId("webhook-secret")).toBeTruthy();
  });

  it("registers a Plane webhook with Plane's secret and does not echo it", async () => {
    const planeSecret = "plane_wh_0123456789abcdef";
    webhooks.create.mockResolvedValue(registered(pmPlane, planeSecret));
    renderPanel();
    const form = await openForm();
    fireEvent.change(within(form).getByLabelText(/Kind/), { target: { value: "pm" } });
    fireEvent.change(within(form).getByLabelText(/Provider/), { target: { value: "plane" } });

    // Plane's secret is required and has at least 16 characters.
    fireEvent.input(within(form).getByLabelText(/Plane's signing secret/), {
      target: { value: "too-short" },
    });
    fireEvent.submit(form);
    expect(await within(form).findByText(/at least 16 characters/)).toBeTruthy();
    expect(webhooks.create).not.toHaveBeenCalled();

    fireEvent.input(within(form).getByLabelText(/Plane's signing secret/), {
      target: { value: planeSecret },
    });
    fireEvent.submit(form);
    await waitFor(() =>
      expect(webhooks.create).toHaveBeenCalledWith("p1", {
        kind: "pm",
        provider: "plane",
        secret: planeSecret,
      }),
    );
    await waitFor(() => expect(webhooks.list).toHaveBeenCalledTimes(2));
    expect(screen.queryByTestId("webhook-secret")).toBeNull();
    expect(screen.queryByDisplayValue(planeSecret)).toBeNull();
  });

  it("shows the server's error when the registration fails", async () => {
    webhooks.create.mockRejectedValue(new Error("project p1 has a vcs webhook for github already"));
    renderPanel();
    fireEvent.submit(await openForm());
    expect(await screen.findByText(/has a vcs webhook for github already/)).toBeTruthy();
    expect(screen.queryByTestId("webhook-secret")).toBeNull();
  });

  it("rotates a secret after confirmation and shows the new one once", async () => {
    const rotated = "f".repeat(64);
    webhooks.rotate.mockResolvedValue(registered(vcsGitHub, rotated));
    renderPanel();

    fireEvent.click(
      await screen.findByLabelText("Rotate the secret of the GitHub VCS events webhook"),
    );
    fireEvent.click(await screen.findByRole("button", { name: "Cancel" }));
    expect(webhooks.rotate).not.toHaveBeenCalled();

    fireEvent.click(screen.getByLabelText("Rotate the secret of the GitHub VCS events webhook"));
    fireEvent.click(await screen.findByRole("button", { name: "Rotate" }));

    await waitFor(() => expect(webhooks.rotate).toHaveBeenCalledWith("p1", "w-vcs", undefined));
    expect(((await screen.findByTestId("webhook-secret")) as HTMLInputElement).value).toBe(rotated);
    expect(browserStorage()).not.toContain(rotated);
  });

  it("rotates a Plane webhook with the secret Plane regenerated", async () => {
    const planeSecret = "plane_wh_new_0123456789";
    webhooks.rotate.mockResolvedValue(registered(pmPlane, planeSecret));
    renderPanel();

    fireEvent.click(
      await screen.findByLabelText("Rotate the secret of the Plane Roadmap sync webhook"),
    );
    const confirm = await screen.findByRole("button", { name: "Rotate" });
    fireEvent.click(confirm);
    expect(await screen.findByText(/at least 16 characters/)).toBeTruthy();
    expect(webhooks.rotate).not.toHaveBeenCalled();

    fireEvent.input(screen.getByLabelText(/Plane's new signing secret/), {
      target: { value: planeSecret },
    });
    fireEvent.click(confirm);
    await waitFor(() => expect(webhooks.rotate).toHaveBeenCalledWith("p1", "w-plane", planeSecret));
    expect(screen.queryByTestId("webhook-secret")).toBeNull();
  });

  it("deletes a webhook after confirmation", async () => {
    renderPanel();

    fireEvent.click(await screen.findByLabelText("Delete the GitHub VCS events webhook"));
    fireEvent.click(await screen.findByRole("button", { name: "Cancel" }));
    expect(webhooks.delete).not.toHaveBeenCalled();

    fireEvent.click(screen.getByLabelText("Delete the GitHub VCS events webhook"));
    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));
    await waitFor(() => expect(webhooks.delete).toHaveBeenCalledWith("p1", "w-vcs"));
    await waitFor(() => expect(webhooks.list).toHaveBeenCalledTimes(2));
  });

  it("sets and removes a PM webhook's API token", async () => {
    renderPanel();

    fireEvent.click(
      await screen.findByLabelText("Set the API token of the Plane Roadmap sync webhook"),
    );
    fireEvent.input(await screen.findByLabelText(/New API token/), {
      target: { value: "plane_api_x" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save token" }));
    await waitFor(() =>
      expect(webhooks.setAPIToken).toHaveBeenCalledWith("p1", "w-plane", "plane_api_x"),
    );

    fireEvent.click(
      await screen.findByLabelText("Set the API token of the Plane Roadmap sync webhook"),
    );
    fireEvent.click(await screen.findByRole("button", { name: "Remove token" }));
    await waitFor(() => expect(webhooks.setAPIToken).toHaveBeenCalledWith("p1", "w-plane", ""));
    // A VCS webhook has no API token.
    expect(
      screen.queryByLabelText("Set the API token of the GitHub VCS events webhook"),
    ).toBeNull();
  });

  it("shows editors the webhooks without any action", async () => {
    auth.role = "editor";
    renderPanel();

    expect(await screen.findByDisplayValue(fullURL(vcsGitHub.url))).toBeTruthy();
    expect(screen.getByText("Only admins register, rotate and delete webhooks.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Add webhook" })).toBeNull();
    expect(screen.queryByLabelText(/^Rotate the secret/)).toBeNull();
    expect(screen.queryByLabelText(/^Delete the/)).toBeNull();
    expect(screen.queryByLabelText(/^Set the API token/)).toBeNull();
    // Copying the URL needs no admin.
    expect(screen.getByLabelText("Copy the URL of the GitHub VCS events webhook")).toBeTruthy();
  });

  it("loads nothing for viewers and says why", async () => {
    auth.role = "viewer";
    renderPanel();

    expect(
      await screen.findByText("Only admins and editors see a project's webhooks."),
    ).toBeTruthy();
    expect(webhooks.list).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: "Add webhook" })).toBeNull();
  });
});
