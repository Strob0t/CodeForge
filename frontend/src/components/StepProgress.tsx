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
    if (p >= 90) return "bg-red-500 dark:bg-red-400";
    if (p >= 70) return "bg-yellow-500 dark:bg-yellow-400";
    return "bg-blue-500 dark:bg-blue-400";
  };

  return (
    <div class="flex items-center gap-2">
      <span class="whitespace-nowrap text-xs text-gray-600 dark:text-gray-400">
        {props.label ?? t("progress.steps")}
      </span>
      <div
        class="relative h-2 flex-1 overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700"
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
              class="absolute inset-0 animate-pulse rounded-full bg-blue-400/50 dark:bg-blue-500/40"
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
      <span class="whitespace-nowrap text-xs font-medium tabular-nums text-gray-700 dark:text-gray-300">
        <Show when={hasMax()} fallback={String(props.current)}>
          {props.current} / {props.max}
        </Show>
      </span>
    </div>
  );
}
