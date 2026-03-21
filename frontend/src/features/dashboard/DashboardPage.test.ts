import { beforeAll, describe, expect, it, vi } from "vitest";

// Mock @solidjs/router to avoid .jsx extension error in vitest
vi.mock("@solidjs/router", () => ({
  A: (props: Record<string, unknown>) => props,
  useNavigate: () => () => undefined,
  useParams: () => ({}),
  useSearchParams: () => [{}, () => undefined],
  useLocation: () => ({ pathname: "/" }),
}));

// Mock @unovis/solid and @unovis/ts to avoid jsdom hangs
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

describe("Dashboard Feature", () => {
  it("should export DashboardPage component", async () => {
    const mod = await import("./DashboardPage");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export KpiStrip component", async () => {
    const mod = await import("./KpiStrip");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export HealthDot component", async () => {
    const mod = await import("./HealthDot");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export ProjectCard component", async () => {
    const mod = await import("./ProjectCard");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export CreateProjectModal component", async () => {
    const mod = await import("./CreateProjectModal");
    expect(mod.CreateProjectModal).toBeDefined();
    expect(typeof mod.CreateProjectModal).toBe("function");
  });

  it("should export EditProjectModal component", async () => {
    const mod = await import("./EditProjectModal");
    expect(mod.EditProjectModal).toBeDefined();
    expect(typeof mod.EditProjectModal).toBe("function");
  });

  it("should export ActivityTimeline component", async () => {
    const mod = await import("./ActivityTimeline");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export ChartsPanel component", async () => {
    const mod = await import("./ChartsPanel");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  // Chart sub-components
  it("should export CostTrendChart component", async () => {
    const mod = await import("./charts/CostTrendChart");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export RunOutcomesDonut component", async () => {
    const mod = await import("./charts/RunOutcomesDonut");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export AgentPerformanceBars component", async () => {
    const mod = await import("./charts/AgentPerformanceBars");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export ModelUsagePie component", async () => {
    const mod = await import("./charts/ModelUsagePie");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export CostByProjectBars component", async () => {
    const mod = await import("./charts/CostByProjectBars");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });
});
