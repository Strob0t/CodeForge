import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { InitialSetupRequest } from "~/api/types";

// KI-119: the first-admin setup needs the one-time setup token the Core logs
// and writes to data/setup_token on its first start; the page asks for it and
// says where to find it.

const mocks = vi.hoisted(() => ({
  setup: vi.fn<(data: InitialSetupRequest) => Promise<object>>(() => Promise.resolve({})),
  login: vi.fn<(email: string, password: string) => Promise<void>>(() => Promise.resolve()),
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
  useNavigate: () => () => undefined,
}));

vi.mock("~/api/client", () => ({
  api: {
    auth: {
      setupStatus: () => Promise.resolve({ needs_setup: true, setup_timeout_minutes: 5 }),
      setup: mocks.setup,
    },
  },
}));

vi.mock("~/components/AuthProvider", () => ({
  useAuth: () => ({ login: mocks.login }),
}));

import { I18nProvider } from "~/i18n";

import SetupPage from "./SetupPage";

function renderPage(): void {
  render(() => (
    <I18nProvider>
      <SetupPage />
    </I18nProvider>
  ));
}

describe("SetupPage", () => {
  beforeEach(() => {
    mocks.setup.mockClear();
    mocks.login.mockClear();
  });

  it("asks for the setup token and says where to find it", () => {
    renderPage();
    const field = screen.getByLabelText(/setup token/i) as HTMLInputElement;
    expect(field.required).toBe(true);
    const help = screen.getByText(/data\/setup_token/);
    expect(help.textContent).toContain("SETUP TOKEN");
  });

  it("sends the trimmed token with the setup request", async () => {
    renderPage();
    fireEvent.input(screen.getByLabelText(/setup token/i), {
      target: { value: "  abc123\n" },
    });
    fireEvent.input(screen.getByLabelText(/display name/i), { target: { value: "Admin" } });
    fireEvent.input(screen.getByLabelText(/^password/i), { target: { value: "Password123" } });
    fireEvent.input(screen.getByLabelText(/confirm password/i), {
      target: { value: "Password123" },
    });
    fireEvent.submit(screen.getByRole("button", { name: /create account/i }));

    await waitFor(() => expect(mocks.setup).toHaveBeenCalledTimes(1));
    expect(mocks.setup.mock.calls[0][0]).toEqual({
      email: "admin@localhost",
      name: "Admin",
      password: "Password123",
      setup_token: "abc123",
    });
  });
});
