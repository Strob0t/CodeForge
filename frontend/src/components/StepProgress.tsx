import type { JSX } from "solid-js";
import { Show } from "solid-js";

import { useI18n } from "~/i18n";

interface StepProgressProps {
  /** Current step count */
  current: number;
  /** Maximum steps (0 or undefined = no limit known) */
  max?: number;
  /** Optional label override (default: i18n "progress.steps") */
  label?: string;
}

/**
 * Visual step-progress indicator with fraction label and bar.
 * Shows "3 / 50 steps" with a proportional fill bar.
 * When max is unknown, shows only the current count with an indeterminate style.
 */
export function StepProgress(props: StepProgressProps): JSX.Element {
  const { t } = useI18n();

  const percent = () => {
    const m = props.max;
    if (!m || m <= 0) return 0;
    return Math.min(100, Math.round((props.current / m) * 100));
  };

  const hasMax = () => (props.max ?? 0) > 0;

  const barColor = () => {
    const p = percent();
    if (p >= 90) return "bg-cf-danger";
    if (p >= 70) return "bg-cf-warning";
    return "bg-cf-accent";
  };

  return (
    <div class="flex items-center gap-2">
      <span class="whitespace-nowrap text-xs text-cf-text-secondary">
        {props.label ?? t("progress.steps")}
      </span>
      <div
        class="relative h-2 flex-1 overflow-hidden rounded-full bg-cf-bg-inset"
        role="progressbar"
        aria-valuenow={props.current}
        aria-valuemin={0}
        aria-valuemax={props.max ?? undefined}
        aria-label={
          hasMax()
            ? t("progress.stepsOf", {
                current: String(props.current),
                max: String(props.max),
              })
            : t("progress.stepsCount", { current: String(props.current) })
        }
      >
        <Show
          when={hasMax()}
          fallback={
            <div
              class="absolute inset-0 animate-pulse rounded-full bg-cf-accent/40"
              style={{ width: "100%" }}
            />
          }
        >
          <div
            class={`h-full rounded-full transition-all duration-300 ${barColor()}`}
            style={{ width: `${percent()}%` }}
          />
        </Show>
      </div>
      <span class="whitespace-nowrap text-xs font-medium tabular-nums text-cf-text-secondary">
        <Show when={hasMax()} fallback={String(props.current)}>
          {props.current} / {props.max}
        </Show>
      </span>
    </div>
  );
}
