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

describe("Routing Feature", () => {
  it("should export RoutingStatsPage component", async () => {
    const mod = await import("./RoutingStatsPage");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });
});

describe("Routing API resource", () => {
  it("should export createRoutingResource", async () => {
    const mod = await import("~/api/resources/routing");
    expect(typeof mod.createRoutingResource).toBe("function");
  });

  it("should create resource with expected methods", async () => {
    const mod = await import("~/api/resources/routing");
    const fakeCoreClient = {
      get: vi.fn(),
      post: vi.fn(),
      put: vi.fn(),
      patch: vi.fn(),
      del: vi.fn(),
      request: vi.fn(),
      BASE: "/api/v1",
      invalidateCache: vi.fn(),
    };
    const resource = mod.createRoutingResource(fakeCoreClient);
    expect(typeof resource.stats).toBe("function");
    expect(typeof resource.refreshStats).toBe("function");
    expect(typeof resource.outcomes).toBe("function");
    expect(typeof resource.recordOutcome).toBe("function");
    expect(typeof resource.seedFromBenchmarks).toBe("function");
  });

  it("stats should build query params correctly", async () => {
    const mod = await import("~/api/resources/routing");
    const getStub = vi.fn().mockResolvedValue([]);
    const fakeCoreClient = {
      get: getStub,
      post: vi.fn(),
      put: vi.fn(),
      patch: vi.fn(),
      del: vi.fn(),
      request: vi.fn(),
      BASE: "/api/v1",
      invalidateCache: vi.fn(),
    };
    const resource = mod.createRoutingResource(fakeCoreClient);

    await resource.stats();
    expect(getStub).toHaveBeenCalledWith("/routing/stats");

    await resource.stats("code");
    expect(getStub).toHaveBeenCalledWith("/routing/stats?task_type=code");

    await resource.stats("code", "simple");
    expect(getStub).toHaveBeenCalledWith("/routing/stats?task_type=code&tier=simple");

    await resource.stats(undefined, "complex");
    expect(getStub).toHaveBeenCalledWith("/routing/stats?tier=complex");
  });

  it("outcomes should append limit param", async () => {
    const mod = await import("~/api/resources/routing");
    const getStub = vi.fn().mockResolvedValue([]);
    const fakeCoreClient = {
      get: getStub,
      post: vi.fn(),
      put: vi.fn(),
      patch: vi.fn(),
      del: vi.fn(),
      request: vi.fn(),
      BASE: "/api/v1",
      invalidateCache: vi.fn(),
    };
    const resource = mod.createRoutingResource(fakeCoreClient);

    await resource.outcomes();
    expect(getStub).toHaveBeenCalledWith("/routing/outcomes");

    await resource.outcomes(50);
    expect(getStub).toHaveBeenCalledWith("/routing/outcomes?limit=50");
  });
});

describe("Routing resource registered on api client", () => {
  it("should have routing resource group with all methods", async () => {
    const { api } = await import("~/api/client");
    expect(api.routing).toBeDefined();
    expect(typeof api.routing.stats).toBe("function");
    expect(typeof api.routing.refreshStats).toBe("function");
    expect(typeof api.routing.outcomes).toBe("function");
    expect(typeof api.routing.recordOutcome).toBe("function");
    expect(typeof api.routing.seedFromBenchmarks).toBe("function");
  });
});

describe("Routing i18n keys", () => {
  it("should have all routing keys in English", async () => {
    const en = (await import("~/i18n/en")).default;
    expect(en["app.nav.routing"]).toBe("Routing");
    expect(en["routing.title"]).toBe("Model Routing");
    expect(en["routing.stats"]).toBe("Performance Stats");
    expect(en["routing.outcomes"]).toBe("Recent Outcomes");
    expect(en["routing.outcomes.record"]).toBe("Record Outcome");
    expect(en["routing.field.modelName"]).toBe("Model");
    expect(en["routing.stats.refresh"]).toBe("Refresh Stats");
    expect(en["routing.stats.seed"]).toBe("Seed from Benchmarks");
    expect(en["routing.recorded"]).toBe("Outcome recorded.");
    expect(en["routing.error.fetchFailed"]).toBeDefined();
    expect(en["routing.error.refreshFailed"]).toBeDefined();
    expect(en["routing.error.seedFailed"]).toBeDefined();
    expect(en["routing.error.recordFailed"]).toBeDefined();
  });

  it("should have all routing keys in German", async () => {
    const de = (await import("~/i18n/locales/de")).default;
    expect(de["app.nav.routing"]).toBe("Routing");
    expect(de["routing.title"]).toBe("Modell-Routing");
    expect(de["routing.stats"]).toBeDefined();
    expect(de["routing.outcomes"]).toBeDefined();
    expect(de["routing.field.modelName"]).toBeDefined();
    expect(de["routing.recorded"]).toBeDefined();
  });
});
