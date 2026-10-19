import { render, screen } from "@solidjs/testing-library";
import { afterEach, describe, expect, it, vi } from "vitest";

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

import { I18nProvider } from "~/i18n";

import PrivacyPolicy from "./PrivacyPolicy";

function renderPage(locale: "en" | "de"): void {
  localStorage.setItem("codeforge-locale", locale);
  render(() => (
    <I18nProvider>
      <PrivacyPolicy />
    </I18nProvider>
  ));
}

afterEach(() => localStorage.removeItem("codeforge-locale"));

// KI-79: the page said account data is kept "lifetime + 30 days after
// deletion", while the erasure is immediate and backups roll off after about
// five weeks (docs/disaster-recovery.md).
describe("PrivacyPolicy account data retention", () => {
  it("states immediate erasure and the backup period in English", async () => {
    renderPage("en");
    const line = await screen.findByText(/^Account data/);
    expect(line.textContent).toContain("erased immediately");
    expect(line.textContent).toContain("about five weeks");
    expect(screen.queryByText(/30 days after deletion/)).toBeNull();
  });

  it("states immediate erasure and the backup period in German", async () => {
    renderPage("de");
    const line = await screen.findByText(/^Kontodaten/);
    expect(line.textContent).toContain("sofort gelöscht");
    expect(line.textContent).toContain("etwa fünf Wochen");
    expect(await screen.findByText("Datenschutzerklärung")).toBeTruthy();
  });
});

// KI-93: the rights of access, erasure and objection pointed to a Settings >
// Privacy screen that did not exist; they now link to it.
describe("PrivacyPolicy rights", () => {
  it.each([
    ["en", "Settings > Privacy", [/^Right of access/, /^Right to erasure/, /^Right to object/]],
    [
      "de",
      "Einstellungen > Datenschutz",
      [/^Auskunftsrecht/, /^Recht auf Löschung/, /^Widerspruchsrecht/],
    ],
  ] as const)(
    "links the self-service rights to the privacy settings (%s)",
    async (locale, link, rights) => {
      renderPage(locale);
      for (const right of rights) {
        const item = await screen.findByText(right);
        const anchor = item.closest("li")?.querySelector("a");
        expect(anchor?.getAttribute("href")).toBe("/settings?section=privacy");
        expect(anchor?.textContent).toBe(link);
      }
      expect(screen.getAllByRole("link", { name: link })).toHaveLength(3);
    },
  );
});
