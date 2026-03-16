import { createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { PromptAnalysisReport, TacticalFix } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { Badge, Button, Card } from "~/ui";

interface PromptOptimizationPanelProps {
  runId: string;
  suiteId: string;
}

export function PromptOptimizationPanel(props: PromptOptimizationPanelProps) {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [report, setReport] = createSignal<PromptAnalysisReport | null>(null);
  const [loading, setLoading] = createSignal(false);
  const [accepted, setAccepted] = createSignal<Set<string>>(new Set());
  const [rejected, setRejected] = createSignal<Set<string>>(new Set());

  const handleAnalyze = async () => {
    setLoading(true);
    try {
      const result = await api.benchmarks.analyzeRun(props.runId);
      setReport(result);
    } catch {
      toast("error", "Analysis failed");
    } finally {
      setLoading(false);
    }
  };

  const handleAccept = (taskId: string) => {
    setAccepted((prev) => {
      const next = new Set(prev);
      next.add(taskId);
      return next;
    });
    setRejected((prev) => {
      const next = new Set(prev);
      next.delete(taskId);
      return next;
    });
  };

  const handleReject = (taskId: string) => {
    setRejected((prev) => {
      const next = new Set(prev);
      next.add(taskId);
      return next;
    });
    setAccepted((prev) => {
      const next = new Set(prev);
      next.delete(taskId);
      return next;
    });
  };

  return (
    <Card class="mt-4 p-4">
      <div class="flex items-center justify-between">
        <h3 class="text-sm font-semibold">{t("benchmark.promptOptimization")}</h3>
        <Button size="sm" onClick={handleAnalyze} disabled={loading()}>
          {loading() ? "Analyzing..." : t("benchmark.analyzePrompts")}
        </Button>
      </div>

      <Show when={report()}>
        {(r) => (
          <div class="mt-4 space-y-4">
            {/* Summary */}
            <div class="flex gap-4 text-sm">
              <span>
                <span class="text-cf-text-muted">Model Family:</span>{" "}
                <Badge variant="default">{r().model_family}</Badge>
              </span>
              <span>
                <span class="text-cf-text-muted">Failed:</span>{" "}
                <span class="font-mono">
                  {r().failed_tasks}/{r().total_tasks}
                </span>
              </span>
              <span>
                <span class="text-cf-text-muted">Failure Rate:</span>{" "}
                <span class="font-mono">{(r().failure_rate * 100).toFixed(1)}%</span>
              </span>
            </div>

            {/* Strategic Principles */}
            <Show when={r().strategic_principles.length > 0}>
              <div>
                <div class="mb-1 text-xs font-semibold text-cf-text-muted">
                  Strategic Principles
                </div>
                <ul class="list-inside list-disc space-y-1 text-sm">
                  <For each={r().strategic_principles}>{(principle) => <li>{principle}</li>}</For>
                </ul>
              </div>
            </Show>

            {/* Tactical Fixes */}
            <Show when={r().tactical_fixes.length > 0}>
              <div>
                <div class="mb-2 text-xs font-semibold text-cf-text-muted">
                  {t("benchmark.promptDiff")} ({r().tactical_fixes.length})
                </div>
                <div class="space-y-2">
                  <For each={r().tactical_fixes}>
                    {(fix: TacticalFix) => {
                      const isAccepted = () => accepted().has(fix.task_id);
                      const isRejected = () => rejected().has(fix.task_id);

                      return (
                        <div
                          class={`rounded border border-cf-border p-3 text-sm ${
                            isAccepted()
                              ? "border-cf-success-border bg-cf-success-bg"
                              : isRejected()
                                ? "border-cf-danger-border bg-cf-danger-bg"
                                : ""
                          }`}
                        >
                          <div class="flex items-start justify-between gap-2">
                            <div class="flex-1">
                              <div class="font-medium">Task: {fix.task_id}</div>
                              <div class="mt-1 text-cf-text-secondary">
                                {fix.failure_description}
                              </div>
                              <Show when={fix.root_cause}>
                                <div class="mt-1 text-xs text-cf-text-muted">
                                  Root cause: {fix.root_cause}
                                </div>
                              </Show>
                              <Show when={fix.proposed_addition}>
                                <div class="mt-2 rounded bg-cf-bg-surface-alt p-2 text-xs">
                                  {fix.proposed_addition}
                                </div>
                              </Show>
                              <Show when={fix.confidence > 0}>
                                <div class="mt-1 text-xs text-cf-text-muted">
                                  Confidence: {(fix.confidence * 100).toFixed(0)}%
                                </div>
                              </Show>
                            </div>
                            <div class="flex gap-1">
                              <Button
                                size="xs"
                                variant={isAccepted() ? "primary" : "secondary"}
                                onClick={() => handleAccept(fix.task_id)}
                              >
                                {t("benchmark.acceptPatch")}
                              </Button>
                              <Button
                                size="xs"
                                variant={isRejected() ? "danger" : "secondary"}
                                onClick={() => handleReject(fix.task_id)}
                              >
                                {t("benchmark.rejectPatch")}
                              </Button>
                            </div>
                          </div>
                        </div>
                      );
                    }}
                  </For>
                </div>
              </div>
            </Show>

            {/* Re-run placeholder */}
            <div class="flex justify-end pt-2">
              <Button size="sm" variant="secondary" disabled>
                {t("benchmark.rerunBenchmark")}
              </Button>
            </div>
          </div>
        )}
      </Show>
    </Card>
  );
}
