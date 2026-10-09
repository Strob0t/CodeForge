import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { describe, expect, it, vi } from "vitest";

import type { DiscoveredModel, LLMModel } from "~/api/types";

// KI-129: long model IDs overflowed their card. A model's name is cut to one
// line with the full name on hover; its ID wraps.

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

const longName = "openrouter/meta-llama/llama-3.3-70b-instruct-turbo-free-preview-2026-09";
const longId = "deployment-0f9c2e4a-7b1d-4c53-9e8a-3f2b6d1c0a97-us-east-1-production";

const model: LLMModel = { model_name: longName, model_id: longId };
const discovered: DiscoveredModel = {
  model_name: longName,
  model_id: longId,
  provider: "openrouter",
  status: "reachable",
  source: "litellm",
};

vi.mock("~/api/client", () => ({
  api: {
    llm: {
      models: () => Promise.resolve([model]),
      health: () => Promise.resolve({ status: "healthy" }),
      discover: () => Promise.resolve({ models: [discovered], count: 1, ollama_url: "" }),
    },
  },
}));

vi.mock("~/components/AuthProvider", () => ({
  useAuth: () => ({ isPlatformAdmin: () => false }),
}));

import { ConfirmProvider } from "~/components/ConfirmProvider";
import { ToastProvider } from "~/components/Toast";
import { I18nProvider } from "~/i18n";

import { ModelsContent } from "./ModelsPage";

function renderModels(): void {
  render(() => (
    <I18nProvider>
      <ToastProvider>
        <ConfirmProvider>
          <ModelsContent />
        </ConfirmProvider>
      </ToastProvider>
    </I18nProvider>
  ));
}

describe("ModelsPage", () => {
  it("cuts a long model name to its card and wraps its ID", async () => {
    renderModels();
    const name = await screen.findByText(longName);
    expect(name.getAttribute("title")).toBe(longName);
    expect(name.className).toContain("truncate");

    fireEvent.click(name);
    const id = await screen.findByText(longId);
    expect(id.getAttribute("title")).toBe(longId);
    expect(id.className).toContain("break-all");
  });

  it("cuts a long discovered model name to its card and wraps its ID", async () => {
    renderModels();
    fireEvent.click(await screen.findByText("Discover Models"));
    await waitFor(() => expect(screen.getAllByText(longName)).toHaveLength(2));
    for (const name of screen.getAllByText(longName)) {
      expect(name.getAttribute("title")).toBe(longName);
    }
    for (const id of screen.getAllByText(longId)) {
      expect(id.className).toContain("break-all");
    }
  });
});
