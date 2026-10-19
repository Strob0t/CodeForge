import { describe, expect, it } from "vitest";

import { isConfiguredModel } from "./configuredModels";

// KI-129: the Routing page kept the stats of removed models. A stats row
// whose model is no longer configured is marked as removed.
describe("isConfiguredModel", () => {
  const cases: { name: string; model: string; configured: string[]; want: boolean }[] = [
    { name: "exact name", model: "openai/gpt-4o", configured: ["openai/gpt-4o"], want: true },
    { name: "removed model", model: "openai/gpt-4o", configured: ["anthropic/x"], want: false },
    { name: "nothing configured", model: "openai/gpt-4o", configured: [], want: false },
    { name: "no partial name match", model: "gpt-4o-mini", configured: ["gpt-4o"], want: false },
    { name: "case differs", model: "OpenAI/GPT-4o", configured: ["openai/gpt-4o"], want: false },
    // The worker expands a provider wildcard to that provider's models.
    { name: "provider wildcard", model: "groq/llama-3.1-8b", configured: ["groq/*"], want: true },
    { name: "other provider's wildcard", model: "groq/x", configured: ["openai/*"], want: false },
    { name: "wildcard without provider", model: "groq/x", configured: ["*"], want: false },
    { name: "provider prefix only", model: "groqx/y", configured: ["groq/*"], want: false },
    // Claude Code is offered by the worker, not configured in LiteLLM.
    { name: "claude code", model: "claudecode/default", configured: [], want: true },
    { name: "empty model name", model: "", configured: ["openai/gpt-4o"], want: false },
  ];
  for (const c of cases) {
    it(c.name, () => {
      expect(isConfiguredModel(c.model, c.configured)).toBe(c.want);
    });
  }
});
