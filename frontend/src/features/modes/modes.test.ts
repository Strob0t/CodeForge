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

describe("Modes Feature", () => {
  it("should export ModesPage component", async () => {
    const mod = await import("./ModesPage");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export ModesContent component", async () => {
    const mod = await import("./ModesPage");
    expect(mod.ModesContent).toBeDefined();
    expect(typeof mod.ModesContent).toBe("function");
  });
});
