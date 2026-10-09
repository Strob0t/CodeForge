import { render, screen } from "@solidjs/testing-library";
import { describe, expect, it, vi } from "vitest";

// KI-129: the panel missed the supervised-ask-all preset and offered to
// delete it. The server names the built-in presets; none of them gets a
// delete button.

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

const policies = vi.hoisted(() => ({
  list: vi.fn(() =>
    Promise.resolve({
      profiles: ["my-custom", "plan-readonly", "supervised-ask-all"],
      presets: ["plan-readonly", "supervised-ask-all"],
    }),
  ),
  get: vi.fn(),
}));

vi.mock("~/api/client", () => ({ api: { policies } }));

vi.mock("@solidjs/router", () => ({
  A: (props: Record<string, unknown>) => props,
  useNavigate: () => () => undefined,
  useLocation: () => ({ pathname: "/" }),
}));

import { ConfirmProvider } from "~/components/ConfirmProvider";
import { I18nProvider } from "~/i18n";

import PolicyPanel from "./PolicyPanel";

describe("PolicyPanel", () => {
  it("offers delete for custom profiles only, never for a built-in preset", async () => {
    render(() => (
      <I18nProvider>
        <ConfirmProvider>
          <PolicyPanel projectId="p1" onError={() => undefined} />
        </ConfirmProvider>
      </I18nProvider>
    ));
    expect(await screen.findByLabelText("Delete policy my-custom")).toBeTruthy();
    expect(screen.queryByLabelText("Delete policy supervised-ask-all")).toBeNull();
    expect(screen.queryByLabelText("Delete policy plan-readonly")).toBeNull();
    expect(screen.getAllByText("preset")).toHaveLength(2);
  });
});
