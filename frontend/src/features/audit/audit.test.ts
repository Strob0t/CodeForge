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

describe("Audit Feature", () => {
  it("should export AuditTrailPage component", async () => {
    const mod = await import("./AuditTrailPage");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export AuditContent component", async () => {
    const mod = await import("./AuditTrailPage");
    expect(mod.AuditContent).toBeDefined();
    expect(typeof mod.AuditContent).toBe("function");
  });

  it("should export AuditTable component", async () => {
    const mod = await import("./AuditTable");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });
});
