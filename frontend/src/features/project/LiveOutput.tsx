import { createEffect, createSignal, For, on, Show } from "solid-js";

import { useI18n } from "~/i18n";

interface OutputLine {
  line: string;
  stream: "stdout" | "stderr";
  timestamp: number;
}

interface LiveOutputProps {
  taskId: string | null;
  lines: OutputLine[];
}

export type { OutputLine };

export default function LiveOutput(props: LiveOutputProps) {
  const { t } = useI18n();
  const [autoScroll, setAutoScroll] = createSignal(true);
  let containerRef: HTMLDivElement | undefined;

  createEffect(
    on(
      () => props.lines.length,
      () => {
        if (autoScroll() && containerRef) {
          containerRef.scrollTop = containerRef.scrollHeight;
        }
      },
    ),
  );

  const handleScroll = () => {
    if (!containerRef) return;
    const atBottom =
      containerRef.scrollHeight - containerRef.scrollTop - containerRef.clientHeight < 30;
    setAutoScroll(atBottom);
  };

  return (
    <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <div class="mb-2 flex items-center justify-between">
        <h3 class="text-lg font-semibold">{t("output.title")}</h3>
        <Show when={props.taskId}>
          <span class="text-xs text-gray-400 dark:text-gray-500">
            {t("output.taskLabel")} {props.taskId?.slice(0, 8) ?? ""}
          </span>
        </Show>
      </div>

      <div
        ref={containerRef}
        onScroll={handleScroll}
        class="h-64 overflow-auto rounded bg-gray-900 p-3 font-mono text-xs leading-relaxed"
        role="log"
        aria-label={t("output.logAria")}
        aria-live="polite"
      >
        <Show
          when={props.lines.length > 0}
          fallback={<span class="text-gray-500">{t("output.waiting")}</span>}
        >
          <For each={props.lines}>
            {(entry) => (
              <div class={entry.stream === "stderr" ? "text-red-400" : "text-green-400"}>
                {entry.line}
              </div>
            )}
          </For>
        </Show>
      </div>

      <Show when={props.lines.length > 0}>
        <div class="mt-2 flex items-center justify-between text-xs text-gray-400 dark:text-gray-500">
          <span>{t("output.lines", { n: props.lines.length })}</span>
          <button
            type="button"
            class="hover:text-gray-600 dark:hover:text-gray-300"
            onClick={() => {
              setAutoScroll(true);
              if (containerRef) containerRef.scrollTop = containerRef.scrollHeight;
            }}
            aria-label={t("output.scrollAria")}
          >
            {t("output.scrollBottom")}
          </button>
        </div>
      </Show>
    </div>
  );
}
