import { type Component, For, Show } from "solid-js";

import type { DashboardStats } from "~/api/types";

interface KpiCardProps {
  label: string;
  shortLabel: string;
  value: string;
  delta: number;
  invertDelta?: boolean;
}

const KpiCard: Component<KpiCardProps> = (props) => {
  const isPositive = () => (props.invertDelta ? props.delta < 0 : props.delta > 0);
  const isNegative = () => (props.invertDelta ? props.delta > 0 : props.delta < 0);
  const arrow = () => (props.delta > 0 ? "\u2191" : props.delta < 0 ? "\u2193" : "");

  return (
    <div class="min-w-0 sm:min-w-[130px] rounded-lg border border-[var(--cf-border)] bg-[var(--cf-bg-surface)] p-3 text-center">
      <Show when={props.delta !== 0}>
        <p
          class="text-xs font-medium"
          classList={{
            "text-[var(--cf-success)]": isPositive(),
            "text-[var(--cf-danger)]": isNegative(),
            "text-[var(--cf-text-muted)]": !isPositive() && !isNegative(),
          }}
        >
          {arrow()} {Math.abs(props.delta).toFixed(1)}%
        </p>
      </Show>
      <p class="text-xl font-bold text-[var(--cf-text-primary)]">{props.value}</p>
      <p class="text-xs text-[var(--cf-text-muted)]">
        <span class="hidden sm:inline">{props.label}</span>
        <span class="sm:hidden">{props.shortLabel}</span>
      </p>
    </div>
  );
};

interface KpiStripProps {
  stats: DashboardStats | undefined;
}

const KpiStrip: Component<KpiStripProps> = (props) => {
  const cards = () => {
    const s = props.stats;
    if (!s) return [];
    return [
      {
        label: "Cost Today",
        shortLabel: "Cost",
        value: `$${s.cost_today_usd.toFixed(2)}`,
        delta: s.cost_today_delta_pct,
        invertDelta: true,
      },
      { label: "Active Runs", shortLabel: "Runs", value: String(s.active_runs), delta: 0 },
      {
        label: "Success Rate (7d)",
        shortLabel: "Success 7d",
        value: `${s.success_rate_7d_pct.toFixed(1)}%`,
        delta: s.success_rate_delta_pct,
      },
      { label: "Active Agents", shortLabel: "Agents", value: String(s.active_agents), delta: 0 },
      {
        label: "Avg Cost/Run",
        shortLabel: "Avg Cost",
        value: `$${s.avg_cost_per_run_usd.toFixed(2)}`,
        delta: s.avg_cost_delta_pct,
        invertDelta: true,
      },
      {
        label: "Tokens Today",
        shortLabel: "Tokens",
        value: formatTokens(s.token_usage_today),
        delta: s.token_usage_delta_pct,
        invertDelta: true,
      },
      {
        label: "Error Rate (24h)",
        shortLabel: "Err 24h",
        value: `${s.error_rate_24h_pct.toFixed(1)}%`,
        delta: s.error_rate_delta_pct,
        invertDelta: true,
      },
    ];
  };

  return (
    <div class="flex gap-3 overflow-x-auto pb-2">
      <For each={cards()}>{(card) => <KpiCard {...card} />}</For>
    </div>
  );
};

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}

export default KpiStrip;
