import { createEffect, createSignal, For, on, Show } from "solid-js";

import { useI18n } from "~/i18n";
import { Button, Card, StatusDot } from "~/ui";

interface TerminalLine {
  line: string;
  stream: "stdout" | "stderr";
  timestamp: number;
}

interface AgentTerminal {
  agentId: string;
  agentName: string;
  lines: TerminalLine[];
}

interface MultiTerminalProps {
  /** Map of agent_id → { name, lines } */
  terminals: AgentTerminal[];
  /** Maximum lines per agent terminal before truncating old ones */
  maxLines?: number;
}

const MAX_LINES_DEFAULT = 500;

function TerminalTile(props: {
  terminal: AgentTerminal;
  expanded: boolean;
  onToggle: () => void;
  maxLines: number;
}) {
  const { t } = useI18n();
  const [autoScroll, setAutoScroll] = createSignal(true);
  let containerRef: HTMLDivElement | undefined;

  createEffect(
    on(
      () => props.terminal.lines.length,
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

  const visibleLines = () => {
    const all = props.terminal.lines;
    return all.length > props.maxLines ? all.slice(all.length - props.maxLines) : all;
  };

  return (
    <div
      class={`flex flex-col rounded-cf-md border bg-cf-bg-surface ${
        props.expanded
          ? "col-span-full border-indigo-300 dark:border-indigo-600"
          : "border-cf-border"
      }`}
    >
      {/* Header */}
      <div class="flex items-center justify-between border-b border-cf-border px-3 py-1.5">
        <div class="flex items-center gap-2">
          <StatusDot color="#22c55e" />
          <span class="text-xs font-medium text-cf-text-secondary">{props.terminal.agentName}</span>
          <span class="text-xs text-cf-text-muted">
            {t("multiTerminal.lines", { n: props.terminal.lines.length })}
          </span>
        </div>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => props.onToggle()}
          aria-label={
            props.expanded
              ? t("multiTerminal.collapse")
              : t("multiTerminal.expand", { name: props.terminal.agentName })
          }
        >
          {props.expanded ? "\u25BC" : "\u25A0"}
        </Button>
      </div>

      {/* Output */}
      <div
        ref={containerRef}
        onScroll={handleScroll}
        class={`overflow-auto bg-cf-bg-primary p-2 font-mono text-xs leading-relaxed ${
          props.expanded ? "h-64" : "h-40"
        }`}
        role="log"
        aria-label={t("multiTerminal.logAria", { name: props.terminal.agentName })}
        aria-live="polite"
      >
        <Show
          when={visibleLines().length > 0}
          fallback={<span class="text-cf-text-tertiary">{t("output.waiting")}</span>}
        >
          <For each={visibleLines()}>
            {(entry) => (
              <div class={entry.stream === "stderr" ? "text-cf-danger-fg" : "text-cf-success-fg"}>
                {entry.line}
              </div>
            )}
          </For>
        </Show>
      </div>
    </div>
  );
}

export default function MultiTerminal(props: MultiTerminalProps) {
  const { t } = useI18n();
  const [expandedId, setExpandedId] = createSignal<string | null>(null);
  const maxLines = () => props.maxLines ?? MAX_LINES_DEFAULT;

  const toggleExpand = (agentId: string) => {
    setExpandedId((prev) => (prev === agentId ? null : agentId));
  };

  return (
    <Card>
      <Card.Header>
        <div class="flex items-center justify-between">
          <h3 class="text-lg font-semibold">{t("multiTerminal.title")}</h3>
          <span class="text-xs text-cf-text-muted">
            {t("multiTerminal.agentCount", { n: props.terminals.length })}
          </span>
        </div>
      </Card.Header>

      <Card.Body>
        <Show
          when={props.terminals.length > 0}
          fallback={<p class="text-sm text-cf-text-muted">{t("multiTerminal.empty")}</p>}
        >
          <div
            class={`grid gap-3 ${
              expandedId()
                ? "grid-cols-1"
                : props.terminals.length === 1
                  ? "grid-cols-1"
                  : props.terminals.length <= 4
                    ? "grid-cols-1 lg:grid-cols-2"
                    : "grid-cols-1 lg:grid-cols-2 xl:grid-cols-3"
            }`}
          >
            <For each={props.terminals}>
              {(terminal) => (
                <Show when={!expandedId() || expandedId() === terminal.agentId}>
                  <TerminalTile
                    terminal={terminal}
                    expanded={expandedId() === terminal.agentId}
                    onToggle={() => toggleExpand(terminal.agentId)}
                    maxLines={maxLines()}
                  />
                </Show>
              )}
            </For>
          </div>
        </Show>
      </Card.Body>
    </Card>
  );
}

export type { AgentTerminal, TerminalLine };
