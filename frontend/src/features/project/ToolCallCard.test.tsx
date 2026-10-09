import { fireEvent, render, screen } from "@solidjs/testing-library";
import { describe, expect, it, vi } from "vitest";

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

vi.mock("~/api/client", () => ({ api: { runs: { revert: vi.fn() } } }));

import { I18nProvider } from "~/i18n";

import ToolCallCard from "./ToolCallCard";

describe("ToolCallCard", () => {
  // The Core cuts a large diff to a few KB for the browser (an overwritten
  // file travels whole otherwise) and says so.
  it("says when the Core shortened the diff", () => {
    render(() => (
      <I18nProvider>
        <ToolCallCard
          name="write_file"
          status="completed"
          diff={{
            path: ".env",
            truncated: true,
            hunks: [
              {
                old_start: 1,
                old_lines: 2,
                new_start: 1,
                new_lines: 1,
                old_content: "A=1\nB=2\n",
                new_content: "A=1\n",
              },
            ],
          }}
        />
      </I18nProvider>
    ));
    fireEvent.click(screen.getByRole("button", { name: /write_file/ }));
    expect(screen.getByText(/Only the start of this diff is shown/)).toBeTruthy();
  });

  it("shows no note for a whole diff", () => {
    render(() => (
      <I18nProvider>
        <ToolCallCard
          name="edit_file"
          status="completed"
          diff={{
            path: "a.go",
            hunks: [
              {
                old_start: 1,
                old_lines: 1,
                new_start: 1,
                new_lines: 1,
                old_content: "a",
                new_content: "b",
              },
            ],
          }}
        />
      </I18nProvider>
    ));
    fireEvent.click(screen.getByRole("button", { name: /edit_file/ }));
    expect(screen.queryByText(/Only the start of this diff is shown/)).toBeNull();
  });

  // A long argument preview is cut ("...") and no longer JSON: the card
  // shows its text instead of nothing.
  it("shows an argument preview that is no JSON as text", () => {
    const preview = '{"command": "grep -rn TODO src/ | head -50 && echo do...';
    render(() => (
      <I18nProvider>
        <ToolCallCard name="bash" status="running" argsText={preview} />
      </I18nProvider>
    ));
    fireEvent.click(screen.getByRole("button", { name: /bash/ }));
    expect(screen.getByText(preview)).toBeTruthy();
  });
});
