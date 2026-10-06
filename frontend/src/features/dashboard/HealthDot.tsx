import { type Component, createSignal, For, Show } from "solid-js";

import type { HealthFactors, HealthLevel } from "~/api/types";
import { useI18n } from "~/i18n";

interface HealthDotProps {
  score: number;
  level: HealthLevel;
  factors: HealthFactors;
}

const HealthDot: Component<HealthDotProps> = (props) => {
  const { t } = useI18n();
  const [showTooltip, setShowTooltip] = createSignal(false);
  // A project without runs has no evidence for a score (KI-129).
  const unknown = () => props.level === "unknown";

  const color = () => {
    switch (props.level) {
      case "healthy":
        return "bg-[var(--cf-success)]";
      case "warning":
        return "bg-[var(--cf-warning)]";
      case "critical":
        return "bg-[var(--cf-danger)]";
      case "unknown":
        return "bg-[var(--cf-text-muted)]";
    }
  };

  const factorRows = () => [
    { label: "Success rate (7d)", value: props.factors.success_rate },
    { label: "Error rate (24h)", value: props.factors.error_rate_inv },
    { label: "Recent activity", value: props.factors.activity_freshness },
    { label: "Task velocity", value: props.factors.task_velocity },
    { label: "Cost stability", value: props.factors.cost_stability },
  ];

  return (
    <div
      class="relative inline-flex"
      onMouseEnter={() => setShowTooltip(true)}
      onMouseLeave={() => setShowTooltip(false)}
    >
      <span
        class={`inline-block h-3 w-3 rounded-full ${color()}`}
        title={unknown() ? t("dashboard.health.noRuns") : `Health: ${props.score}`}
      />
      <Show when={showTooltip()}>
        <div class="absolute left-5 top-0 z-50 w-56 rounded-lg border border-[var(--cf-border)] bg-[var(--cf-bg-surface)] p-3 shadow-lg">
          <p class="mb-2 text-sm font-bold text-[var(--cf-text-primary)]">
            {unknown() ? t("dashboard.health.noRuns") : `Health Score: ${props.score}`}
          </p>
          <div class="space-y-1.5">
            <For each={factorRows()}>
              {(f) => (
                <div class="flex items-center gap-2 text-xs">
                  <span class="w-28 text-[var(--cf-text-muted)]">{f.label}</span>
                  <div class="h-1.5 flex-1 rounded-full bg-[var(--cf-bg-surface-alt)]">
                    <div
                      class={`h-1.5 rounded-full ${barColor(f.value)}`}
                      style={{ width: `${Math.min(f.value, 100)}%` }}
                    />
                  </div>
                  <span class="w-8 text-right font-mono text-[var(--cf-text-secondary)]">
                    {Math.round(f.value)}%
                  </span>
                </div>
              )}
            </For>
          </div>
        </div>
      </Show>
    </div>
  );
};

function barColor(value: number): string {
  if (value >= 75) return "bg-[var(--cf-success)]";
  if (value >= 40) return "bg-[var(--cf-warning)]";
  return "bg-[var(--cf-danger)]";
}

export default HealthDot;
