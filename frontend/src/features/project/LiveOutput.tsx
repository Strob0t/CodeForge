import { createEffect, createSignal, For, on, Show } from "solid-js";

import { useI18n } from "~/i18n";
import { Button, Card } from "~/ui";

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
    <Card>
      <Card.Header>
        <div class="flex items-center justify-between">
          <h3 class="text-lg font-semibold">{t("output.title")}</h3>
          <Show when={props.taskId}>
            <span class="text-xs text-cf-text-muted">
              {t("output.taskLabel")} {props.taskId?.slice(0, 8) ?? ""}
            </span>
          </Show>
        </div>
      </Card.Header>

      <Card.Body>
        <div
          ref={containerRef}
          onScroll={handleScroll}
          class="h-64 overflow-auto rounded-cf-sm bg-cf-bg-primary p-3 font-mono text-xs leading-relaxed"
          role="log"
          aria-label={t("output.logAria")}
          aria-live="polite"
        >
          <Show
            when={props.lines.length > 0}
            fallback={<span class="text-cf-text-tertiary">{t("output.waiting")}</span>}
          >
            <For each={props.lines}>
              {(entry) => (
                <div class={entry.stream === "stderr" ? "text-cf-danger-fg" : "text-cf-success-fg"}>
                  {entry.line}
                </div>
              )}
            </For>
          </Show>
        </div>

        <Show when={props.lines.length > 0}>
          <div class="mt-2 flex items-center justify-between text-xs text-cf-text-muted">
            <span>{t("output.lines", { n: props.lines.length })}</span>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setAutoScroll(true);
                if (containerRef) containerRef.scrollTop = containerRef.scrollHeight;
              }}
              aria-label={t("output.scrollAria")}
            >
              {t("output.scrollBottom")}
            </Button>
          </div>
        </Show>
      </Card.Body>
    </Card>
  );
}
