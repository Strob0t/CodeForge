import { beforeAll, describe, expect, it, vi } from "vitest";

vi.mock("@solidjs/router", () => ({
  A: (props: Record<string, unknown>) => props,
  useNavigate: () => () => undefined,
  useParams: () => ({}),
  useSearchParams: () => [{}, () => undefined],
  useLocation: () => ({ pathname: "/" }),
}));

beforeAll(() => {
  if (typeof window !== "undefined" && !window.matchMedia) {
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    });
  }
});

describe("Settings Feature", () => {
  it("should export SettingsPage component", async () => {
    const mod = await import("./SettingsPage");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export ShortcutRecorder component", async () => {
    const mod = await import("./ShortcutRecorder");
    expect(mod.ShortcutRecorder).toBeDefined();
    expect(typeof mod.ShortcutRecorder).toBe("function");
  });

  it("should export ShortcutsSection component", async () => {
    const mod = await import("./ShortcutsSection");
    expect(mod.ShortcutsSection).toBeDefined();
    expect(typeof mod.ShortcutsSection).toBe("function");
  });
});
