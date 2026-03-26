import { createResource, createSignal, Show } from "solid-js";

import { api, FetchError } from "~/api/client";
import type { BenchmarkResult } from "~/api/types";
import { useI18n } from "~/i18n";
import { extractErrorMessage } from "~/lib/errorUtils";
import { Alert, Button, Card, FormField, Input, ModelCombobox, Section, Textarea } from "~/ui";

export default function DevToolsSection() {
  const { t } = useI18n();

  const [devMode] = createResource(() => api.health.check().then((h) => h.dev_mode === true));

  const [benchModel, setBenchModel] = createSignal("");
  const [benchSystemPrompt, setBenchSystemPrompt] = createSignal("");
  const [benchPrompt, setBenchPrompt] = createSignal("");
  const [benchTemp, setBenchTemp] = createSignal(0.7);
  const [benchMaxTokens, setBenchMaxTokens] = createSignal(1000);
  const [benchRunning, setBenchRunning] = createSignal(false);
  const [benchResult, setBenchResult] = createSignal<BenchmarkResult | null>(null);
  const [benchError, setBenchError] = createSignal<string | null>(null);

  const handleRunBenchmark = async () => {
    setBenchRunning(true);
    setBenchResult(null);
    setBenchError(null);
    try {
      const result = await api.dev.benchmark({
        model: benchModel(),
        prompt: benchPrompt(),
        system_prompt: benchSystemPrompt() || undefined,
        temperature: benchTemp(),
        max_tokens: benchMaxTokens(),
      });
      setBenchResult(result);
    } catch (err) {
      if (err instanceof FetchError && err.status === 403) {
        setBenchError(t("settings.benchmark.devModeRequired"));
      } else {
        setBenchError(extractErrorMessage(err, "Benchmark failed"));
      }
    } finally {
      setBenchRunning(false);
    }
  };

  return (
    <Show when={devMode()}>
      <Section id="settings-devtools" title={t("settings.devTools")} class="mb-8">
        <h4 class="mb-3 text-sm font-medium text-cf-text-secondary">
          {t("settings.benchmark.title")}
        </h4>
        <div class="space-y-3">
          <FormField label={t("settings.benchmark.model")} id="bench-model">
            <ModelCombobox
              id="bench-model"
              value={benchModel()}
              onInput={setBenchModel}
              class="max-w-md"
            />
          </FormField>

          <FormField label={t("settings.benchmark.systemPrompt")} id="bench-system">
            <Textarea
              id="bench-system"
              value={benchSystemPrompt()}
              onInput={(e) => setBenchSystemPrompt(e.currentTarget.value)}
              placeholder="Optional system instructions..."
              rows={2}
            />
          </FormField>

          <FormField label={t("settings.benchmark.prompt")} id="bench-prompt">
            <Textarea
              id="bench-prompt"
              value={benchPrompt()}
              onInput={(e) => setBenchPrompt(e.currentTarget.value)}
              placeholder="Enter your prompt..."
              rows={4}
            />
          </FormField>

          {/* Temperature + Max Tokens */}
          <div class="flex gap-4">
            <div class="flex-1">
              <label for="bench-temp" class="mb-1 block text-sm font-medium text-cf-text-secondary">
                {t("settings.benchmark.temperature")}: {benchTemp().toFixed(1)}
              </label>
              <input
                id="bench-temp"
                type="range"
                min="0"
                max="2"
                step="0.1"
                value={benchTemp()}
                onInput={(e) => setBenchTemp(parseFloat(e.currentTarget.value))}
                class="w-full max-w-md"
              />
            </div>
            <div class="w-40">
              <FormField label={t("settings.benchmark.maxTokens")} id="bench-tokens">
                <Input
                  id="bench-tokens"
                  type="number"
                  min="1"
                  max="128000"
                  value={benchMaxTokens()}
                  onInput={(e) => setBenchMaxTokens(parseInt(e.currentTarget.value, 10) || 1000)}
                />
              </FormField>
            </div>
          </div>

          {/* Run button */}
          <div class="pt-2">
            <Button
              onClick={handleRunBenchmark}
              loading={benchRunning()}
              disabled={!benchModel().trim() || !benchPrompt().trim()}
            >
              {benchRunning() ? t("settings.benchmark.running") : t("settings.benchmark.run")}
            </Button>
          </div>

          {/* Error */}
          <Show when={benchError()}>{(err) => <Alert variant="error">{err()}</Alert>}</Show>

          {/* Results */}
          <Show when={benchResult()}>
            {(result) => (
              <Card>
                <Card.Body>
                  <div class="flex flex-wrap gap-4 text-sm">
                    <div>
                      <span class="font-medium text-cf-text-tertiary">
                        {t("settings.benchmark.model")}:
                      </span>{" "}
                      <span class="font-mono">{result().model}</span>
                    </div>
                    <div>
                      <span class="font-medium text-cf-text-tertiary">
                        {t("settings.benchmark.latency")}:
                      </span>{" "}
                      {result().latency_ms} ms
                    </div>
                    <div>
                      <span class="font-medium text-cf-text-tertiary">
                        {t("settings.benchmark.tokensIn")}:
                      </span>{" "}
                      {result().tokens_in}
                    </div>
                    <div>
                      <span class="font-medium text-cf-text-tertiary">
                        {t("settings.benchmark.tokensOut")}:
                      </span>{" "}
                      {result().tokens_out}
                    </div>
                  </div>
                  <div class="mt-3">
                    <p class="mb-1 text-sm font-medium text-cf-text-tertiary">
                      {t("settings.benchmark.response")}:
                    </p>
                    <pre class="max-h-64 overflow-auto whitespace-pre-wrap rounded bg-cf-bg-surface-alt p-3 text-sm">
                      {result().content}
                    </pre>
                  </div>
                </Card.Body>
              </Card>
            )}
          </Show>
        </div>
      </Section>
    </Show>
  );
}
