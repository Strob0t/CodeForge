import { render, screen } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

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

import { MAX_VISIBLE_TOASTS } from "~/config/constants";
import { I18nProvider } from "~/i18n";

import { ToastProvider, useToast } from "./Toast";

type Show = ReturnType<typeof useToast>["show"];

/** Renders the provider and hands out its show function. */
function renderToasts(): Show {
  let show: Show | undefined;
  function Grab(): null {
    show = useToast().show;
    return null;
  }
  render(() => (
    <I18nProvider>
      <ToastProvider>
        <Grab />
      </ToastProvider>
    </I18nProvider>
  ));
  if (!show) throw new Error("ToastProvider did not render");
  return show;
}

const shown = (): string[] =>
  screen.queryAllByRole("status").map((el) => el.querySelector("p")?.textContent ?? "");

describe("ToastProvider", () => {
  beforeEach(() => {
    expect(MAX_VISIBLE_TOASTS).toBe(3);
  });

  it("evicts the oldest toast above the maximum", () => {
    const show = renderToasts();
    for (const message of ["one", "two", "three", "four"]) show("info", message);
    expect(shown()).toEqual(["two", "three", "four"]);
  });

  // S9-D review: a persistent toast (dismissMs 0) carries what the user must
  // act on (a webhook secret that could not be shown); only the user
  // dismisses it.
  it("keeps a persistent toast and evicts the oldest timed one instead", () => {
    const show = renderToasts();
    show("warning", "persistent", 0);
    for (const message of ["one", "two", "three"]) show("info", message);
    expect(shown()).toEqual(["persistent", "two", "three"]);
  });

  it("shows a new toast even when only persistent ones are left to evict", () => {
    const show = renderToasts();
    for (const message of ["p1", "p2", "p3"]) show("warning", message, 0);
    show("info", "new");
    expect(shown()).toEqual(["p1", "p2", "p3", "new"]);
  });
});
