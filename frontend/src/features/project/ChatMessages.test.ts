import { beforeAll, describe, expect, it, vi } from "vitest";

// Mock @solidjs/router to avoid .jsx extension error in vitest
vi.mock("@solidjs/router", () => ({
  A: (props: Record<string, unknown>) => props,
  useNavigate: () => () => undefined,
  useParams: () => ({}),
  useSearchParams: () => [{}, () => undefined],
  useLocation: () => ({ pathname: "/" }),
}));

// Polyfill window.matchMedia for jsdom
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

describe("ChatMessages", () => {
  it("should export default component", async () => {
    const mod = await import("./ChatMessages");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should have the component name ChatMessages", async () => {
    const mod = await import("./ChatMessages");
    expect(mod.default.name).toContain("ChatMessages");
  });
});
