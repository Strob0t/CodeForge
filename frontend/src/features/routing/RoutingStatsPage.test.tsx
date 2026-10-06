import { render, screen, waitFor } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { AvailableModelsResponse, LLMModel, ModelPerformanceStats } from "~/api/types";

// KI-129: the Routing page kept the stats of removed models; it marks them.

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

function statsRow(id: string, model: string): ModelPerformanceStats {
  return {
    id,
    model_name: model,
    task_type: "code",
    complexity_tier: "simple",
    trial_count: 3,
    total_reward: 1.5,
    avg_reward: 0.5,
    avg_cost_usd: 0.01,
    avg_latency_ms: 900,
    avg_quality: 0.7,
    input_cost_per: 0,
    output_cost_per: 0,
    supports_tools: true,
    supports_vision: false,
    max_context: 128000,
    created_at: "",
    updated_at: "",
  };
}

const mocks = vi.hoisted(() => ({
  models: vi.fn<() => Promise<LLMModel[]>>(),
  available: vi.fn<() => Promise<AvailableModelsResponse>>(),
}));

vi.mock("~/api/client", () => ({
  api: {
    routing: {
      stats: () =>
        Promise.resolve([
          statsRow("s1", "openai/gpt-4o"),
          statsRow("s2", "openai/gpt-3.5-turbo"),
          statsRow("s3", "llama3.2:3b"),
        ]),
      outcomes: () => Promise.resolve([]),
    },
    llm: { models: mocks.models, available: mocks.available },
  },
}));

import { ToastProvider } from "~/components/Toast";
import { I18nProvider } from "~/i18n";

import RoutingStatsPage from "./RoutingStatsPage";

function renderPage(): void {
  render(() => (
    <I18nProvider>
      <ToastProvider>
        <RoutingStatsPage />
      </ToastProvider>
    </I18nProvider>
  ));
}

describe("RoutingStatsPage", () => {
  beforeEach(() => {
    mocks.models.mockReset();
    // The worker routes from /llm/available, which also holds the Ollama
    // models the registry discovered, under their bare names.
    mocks.available.mockReset().mockResolvedValue({
      models: [
        {
          model_name: "llama3.2:3b",
          model_id: "ollama/llama3.2:3b",
          status: "reachable",
          source: "ollama",
        },
      ],
      best_model: "openai/gpt-4o",
    });
  });

  it("marks the stats of a model that is no longer configured", async () => {
    mocks.models.mockResolvedValue([{ model_name: "openai/gpt-4o" }]);
    renderPage();
    await waitFor(() =>
      expect(screen.getByText("openai/gpt-3.5-turbo").className).toContain("line-through"),
    );
    expect(screen.getByText("openai/gpt-4o").className).not.toContain("line-through");
    expect(screen.getByText("llama3.2:3b").className).not.toContain("line-through");
    expect(screen.getAllByText("removed")).toHaveLength(1);
  });

  it.each([
    ["the configured models", "models"],
    ["the available models", "available"],
  ] as const)("marks nothing when %s cannot be loaded", async (_name, failing) => {
    mocks.models.mockResolvedValue([{ model_name: "openai/gpt-4o" }]);
    mocks[failing].mockRejectedValue(new Error("LLM service unavailable"));
    renderPage();
    await screen.findByText("openai/gpt-3.5-turbo");
    await waitFor(() => expect(mocks[failing]).toHaveBeenCalled());
    await new Promise((r) => setTimeout(r, 0));
    expect(screen.queryByText("removed")).toBeNull();
  });
});
