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

describe("Activity Feature", () => {
  it("should export ActivityPage component", async () => {
    const mod = await import("./ActivityPage");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export ActivityContent component", async () => {
    const mod = await import("./ActivityPage");
    expect(mod.ActivityContent).toBeDefined();
    expect(typeof mod.ActivityContent).toBe("function");
  });

  // A partial delivery (branch pushed, pull request not opened) needs the
  // user's attention: a warning, not an info line.
  it.each([
    ["completed", "success"],
    ["partial", "warning"],
    ["failed", "error"],
    ["started", "info"],
  ] as const)("shows a %s delivery as %s", async (status, severity) => {
    const mod = await import("./ActivityPage");
    expect(mod.deliverySeverity(status)).toBe(severity);
  });
});
