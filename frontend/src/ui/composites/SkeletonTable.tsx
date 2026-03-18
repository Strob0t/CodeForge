import type { JSX } from "solid-js";
import { For } from "solid-js";

import { cx } from "~/utils/cx";

interface SkeletonTableProps {
  columns?: number;
  rows?: number;
  class?: string;
}

export function SkeletonTable(props: SkeletonTableProps): JSX.Element {
  const cols = () => props.columns ?? 4;
  const rows = () => props.rows ?? 3;

  return (
    <div
      class={cx("animate-pulse rounded-cf-lg border border-cf-border overflow-hidden", props.class)}
    >
      <div class="flex gap-4 bg-cf-bg-muted px-4 py-3 border-b border-cf-border">
        <For each={Array.from({ length: cols() })}>
          {() => <div class="h-3 flex-1 rounded bg-cf-border" />}
        </For>
      </div>
      <For each={Array.from({ length: rows() })}>
        {() => (
          <div class="flex gap-4 px-4 py-3 border-b border-cf-border last:border-b-0">
            <For each={Array.from({ length: cols() })}>
              {() => <div class="h-2 flex-1 rounded bg-cf-border/60" />}
            </For>
          </div>
        )}
      </For>
    </div>
  );
}
