import type { JSX } from "solid-js";

import { cx } from "~/utils/cx";

interface SkeletonCardProps {
  variant?: "default" | "stat";
  class?: string;
}

export function SkeletonCard(props: SkeletonCardProps): JSX.Element {
  const isStat = () => props.variant === "stat";

  return (
    <div
      class={cx(
        "animate-pulse rounded-cf-lg border border-cf-border bg-cf-bg-surface p-4",
        isStat() ? "h-20" : "h-32",
        props.class,
      )}
    >
      <div class="h-3 w-2/3 rounded bg-cf-border mb-3" />
      <div class="h-2 w-full rounded bg-cf-border/60 mb-2" />
      {!isStat() && <div class="h-2 w-4/5 rounded bg-cf-border/60" />}
    </div>
  );
}
