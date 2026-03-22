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

  it("should export GeneralSection component", async () => {
    const mod = await import("./GeneralSection");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export VCSSection component", async () => {
    const mod = await import("./VCSSection");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export ProvidersSection component", async () => {
    const mod = await import("./ProvidersSection");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export ProxySection component", async () => {
    const mod = await import("./ProxySection");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export SubscriptionsSection component", async () => {
    const mod = await import("./SubscriptionsSection");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export APIKeysSection component", async () => {
    const mod = await import("./APIKeysSection");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export UsersSection component", async () => {
    const mod = await import("./UsersSection");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export DevToolsSection component", async () => {
    const mod = await import("./DevToolsSection");
    expect(mod.default).toBeDefined();
    expect(typeof mod.default).toBe("function");
  });

  it("should export SETTINGS_SECTIONS with 9 entries", async () => {
    const mod = await import("./settingsTypes");
    expect(mod.SETTINGS_SECTIONS).toBeDefined();
    expect(mod.SETTINGS_SECTIONS).toHaveLength(9);
  });

  it("should have matching section IDs in SETTINGS_SECTIONS", async () => {
    const mod = await import("./settingsTypes");
    const ids = mod.SETTINGS_SECTIONS.map((s) => s.id);
    expect(ids).toContain("settings-general");
    expect(ids).toContain("settings-shortcuts");
    expect(ids).toContain("settings-vcs");
    expect(ids).toContain("settings-providers");
    expect(ids).toContain("settings-proxy");
    expect(ids).toContain("settings-subscriptions");
    expect(ids).toContain("settings-apikeys");
    expect(ids).toContain("settings-users");
    expect(ids).toContain("settings-devtools");
  });

  it("should export SettingsSection type", async () => {
    const mod = await import("./settingsTypes");
    // Type exists if SETTINGS_SECTIONS is typed correctly
    const section = mod.SETTINGS_SECTIONS[0];
    expect(section).toHaveProperty("id");
    expect(section).toHaveProperty("label");
    expect(typeof section.id).toBe("string");
    expect(typeof section.label).toBe("string");
  });
});
