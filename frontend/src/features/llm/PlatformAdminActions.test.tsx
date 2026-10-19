import { render, screen, waitFor } from "@solidjs/testing-library";
import type { JSX } from "solid-js";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { LLMModel, SubscriptionProvider } from "~/api/types";

// All tenants share the LiteLLM models and the subscription provider
// credentials; the backend lets only platform admins change them (KI-75), so
// the UI shows those actions only to platform admins.

const state = vi.hoisted(() => ({ platformAdmin: false }));

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

const model: LLMModel = {
  model_name: "gpt-4o",
  model_id: "dep-1",
};

const provider: SubscriptionProvider = {
  name: "github_copilot",
  display_name: "GitHub Copilot",
  description: "Copilot models",
  env_var: "GITHUB_TOKEN",
  models: ["github_copilot/gpt-4o"],
  connected: false,
};

vi.mock("~/api/client", () => ({
  api: {
    llm: {
      models: () => Promise.resolve([model]),
      health: () => Promise.resolve({ status: "healthy" }),
      addModel: () => Promise.resolve(undefined),
      deleteModel: () => Promise.resolve(undefined),
      discover: () => Promise.resolve({ models: [] }),
    },
    subscriptionProviders: {
      list: () => Promise.resolve({ providers: [provider] }),
      connect: () => Promise.reject(new Error("not in this test")),
      status: () => Promise.reject(new Error("not in this test")),
      disconnect: () => Promise.resolve(undefined),
    },
  },
}));

vi.mock("~/components/AuthProvider", () => ({
  useAuth: () => ({ isPlatformAdmin: () => state.platformAdmin }),
}));

import { ConfirmProvider } from "~/components/ConfirmProvider";
import { ToastProvider } from "~/components/Toast";
import SubscriptionsSection from "~/features/settings/SubscriptionsSection";
import { I18nProvider } from "~/i18n";

import { ModelsContent } from "./ModelsPage";

function renderWithProviders(component: () => JSX.Element): void {
  render(() => (
    <I18nProvider>
      <ToastProvider>
        <ConfirmProvider>{component()}</ConfirmProvider>
      </ToastProvider>
    </I18nProvider>
  ));
}

describe("platform-wide LLM actions", () => {
  beforeEach(() => {
    state.platformAdmin = false;
  });

  it("shows add and delete model to platform admins", async () => {
    state.platformAdmin = true;
    renderWithProviders(() => <ModelsContent />);

    await waitFor(() => expect(screen.getByText("gpt-4o")).toBeDefined());
    expect(screen.queryByText("Add Model")).not.toBeNull();
    expect(screen.queryByLabelText("Delete model gpt-4o")).not.toBeNull();
    expect(screen.queryByText("Only platform admins can add or remove models.")).toBeNull();
  });

  it("hides add and delete model from everyone else", async () => {
    renderWithProviders(() => <ModelsContent />);

    await waitFor(() => expect(screen.getByText("gpt-4o")).toBeDefined());
    expect(screen.queryByText("Add Model")).toBeNull();
    expect(screen.queryByLabelText("Delete model gpt-4o")).toBeNull();
    expect(screen.queryByText("Only platform admins can add or remove models.")).not.toBeNull();
    expect(screen.queryByText("Discover Models")).not.toBeNull();
  });

  it("shows connect to platform admins", async () => {
    state.platformAdmin = true;
    renderWithProviders(() => <SubscriptionsSection />);

    await waitFor(() => expect(screen.getByText("GitHub Copilot")).toBeDefined());
    expect(screen.queryByText("Connect")).not.toBeNull();
  });

  it("hides connect from everyone else", async () => {
    renderWithProviders(() => <SubscriptionsSection />);

    await waitFor(() => expect(screen.getByText("GitHub Copilot")).toBeDefined());
    expect(screen.queryByText("Connect")).toBeNull();
    expect(
      screen.queryByText("Only platform admins can connect or disconnect providers."),
    ).not.toBeNull();
  });
});
