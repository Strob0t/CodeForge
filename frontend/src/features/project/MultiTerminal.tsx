import { createEffect, createSignal, For, on, Show } from "solid-js";

import { useI18n } from "~/i18n";

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
      class={`flex flex-col rounded-lg border bg-white dark:bg-gray-800 ${
        props.expanded
          ? "col-span-full border-indigo-300 dark:border-indigo-600"
          : "border-gray-200 dark:border-gray-700"
      }`}
    >
      {/* Header */}
      <div class="flex items-center justify-between border-b border-gray-200 px-3 py-1.5 dark:border-gray-700">
        <div class="flex items-center gap-2">
          <span class="h-2 w-2 rounded-full bg-green-500" aria-hidden="true" />
          <span class="text-xs font-medium text-gray-700 dark:text-gray-300">
            {props.terminal.agentName}
          </span>
          <span class="text-xs text-gray-400">
            {t("multiTerminal.lines", { n: props.terminal.lines.length })}
          </span>
        </div>
        <button
          type="button"
          class="text-xs text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
          onClick={() => props.onToggle()}
          aria-label={
            props.expanded
              ? t("multiTerminal.collapse")
              : t("multiTerminal.expand", { name: props.terminal.agentName })
          }
        >
          {props.expanded ? "\u25BC" : "\u25A0"}
        </button>
      </div>

      {/* Output */}
      <div
        ref={containerRef}
        onScroll={handleScroll}
        class={`overflow-auto bg-gray-900 p-2 font-mono text-xs leading-relaxed ${
          props.expanded ? "h-64" : "h-40"
        }`}
        role="log"
        aria-label={t("multiTerminal.logAria", { name: props.terminal.agentName })}
        aria-live="polite"
      >
        <Show
          when={visibleLines().length > 0}
          fallback={<span class="text-gray-500">{t("output.waiting")}</span>}
        >
          <For each={visibleLines()}>
            {(entry) => (
              <div class={entry.stream === "stderr" ? "text-red-400" : "text-green-400"}>
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
    <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <div class="mb-3 flex items-center justify-between">
        <h3 class="text-lg font-semibold">{t("multiTerminal.title")}</h3>
        <span class="text-xs text-gray-400">
          {t("multiTerminal.agentCount", { n: props.terminals.length })}
        </span>
      </div>

      <Show
        when={props.terminals.length > 0}
        fallback={
          <p class="text-sm text-gray-400 dark:text-gray-500">{t("multiTerminal.empty")}</p>
        }
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
    </div>
  );
}

export type { AgentTerminal, TerminalLine };
