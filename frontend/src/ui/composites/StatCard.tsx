import type { JSX } from "solid-js";

import { cx } from "~/utils/cx";

import { Card } from "./Card";

interface StatCardProps {
  label: string;
  value: JSX.Element;
  class?: string;
}

export function StatCard(props: StatCardProps) {
  return (
    <Card class={cx("p-3", props.class)}>
      <div class="text-xs text-cf-text-muted">{props.label}</div>
      <div class="mt-1 text-lg font-semibold text-cf-text-primary">{props.value}</div>
    </Card>
  );
}
