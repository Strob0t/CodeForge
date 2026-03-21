import { beforeAll, describe, expect, it, vi } from "vitest";

vi.mock("@solidjs/router", () => ({
  A: (props: Record<string, unknown>) => props,
  useNavigate: () => () => undefined,
  useParams: () => ({}),
  useSearchParams: () => [{}, () => undefined],
  useLocation: () => ({ pathname: "/" }),
}));

vi.mock("@unovis/solid", () => ({
  VisArea: () => null,
  VisAxis: () => null,
  VisXYContainer: () => null,
  VisDonut: () => null,
  VisSingleContainer: () => null,
  VisGroupedBar: () => null,
  VisStackedBar: () => null,
}));
vi.mock("@unovis/ts", () => ({
  CurveType: { MonotoneX: "monotoneX" },
  Orientation: { Horizontal: "horizontal", Vertical: "vertical" },
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

describe("Costs Feature", () => {
  it("should export CostDashboardPage component", async () => {
    const mod = await import("./CostDashboardPage");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export ProjectCostSection component", async () => {
    const mod = await import("./CostDashboardPage");
    expect(mod.ProjectCostSection).toBeDefined();
    expect(typeof mod.ProjectCostSection).toBe("function");
  });
});
