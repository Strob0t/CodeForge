import { createSignal, Show } from "solid-js";

import { Card } from "~/ui";

interface ToolCallCardProps {
  name: string;
  args?: Record<string, unknown>;
  result?: string;
  status: "pending" | "running" | "completed" | "failed";
}

export default function ToolCallCard(props: ToolCallCardProps) {
  const [expanded, setExpanded] = createSignal(false);

  const statusIcon = () => {
    switch (props.status) {
      case "pending":
        return "\u25CB"; // empty circle
      case "running":
        return "\u25D4"; // half circle
      case "completed":
        return "\u2713"; // check mark
      case "failed":
        return "\u2717"; // x mark
    }
  };

  const statusColor = () => {
    switch (props.status) {
      case "pending":
        return "text-cf-text-muted";
      case "running":
        return "text-cf-accent animate-pulse";
      case "completed":
        return "text-green-500";
      case "failed":
        return "text-red-500";
    }
  };

  return (
    <Card class="my-1 text-sm">
      <button
        class="flex w-full items-center gap-2 px-3 py-1.5 text-left hover:bg-cf-bg-surface-alt"
        onClick={() => setExpanded(!expanded())}
        aria-expanded={expanded()}
      >
        <span class={statusColor()}>{statusIcon()}</span>
        <span class="font-mono text-xs text-cf-text-primary">{props.name}</span>
        <span class="ml-auto text-xs text-cf-text-muted">{expanded() ? "\u25B2" : "\u25BC"}</span>
      </button>

      <Show when={expanded()}>
        <div class="border-t border-cf-border px-3 py-2">
          <Show when={props.args && Object.keys(props.args).length > 0}>
            <div class="mb-1">
              <span class="text-xs font-medium text-cf-text-tertiary">Arguments:</span>
              <pre class="mt-0.5 max-h-32 overflow-auto rounded-cf-sm bg-cf-bg-inset p-2 text-xs">
                {JSON.stringify(props.args, null, 2)}
              </pre>
            </div>
          </Show>

          <Show when={props.result}>
            <div>
              <span class="text-xs font-medium text-cf-text-tertiary">Result:</span>
              <pre class="mt-0.5 max-h-32 overflow-auto rounded-cf-sm bg-cf-bg-inset p-2 text-xs">
                {props.result}
              </pre>
            </div>
          </Show>
        </div>
      </Show>
    </Card>
  );
}
