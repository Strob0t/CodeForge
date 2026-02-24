import { createSignal, Show } from "solid-js";

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
        return "text-gray-400";
      case "running":
        return "text-blue-500 animate-pulse";
      case "completed":
        return "text-green-500";
      case "failed":
        return "text-red-500";
    }
  };

  return (
    <div class="my-1 rounded border border-gray-200 bg-gray-50 text-sm dark:border-gray-700 dark:bg-gray-800/50">
      <button
        class="flex w-full items-center gap-2 px-3 py-1.5 text-left hover:bg-gray-100 dark:hover:bg-gray-700/50"
        onClick={() => setExpanded(!expanded())}
        aria-expanded={expanded()}
      >
        <span class={statusColor()}>{statusIcon()}</span>
        <span class="font-mono text-xs text-gray-700 dark:text-gray-300">{props.name}</span>
        <span class="ml-auto text-xs text-gray-400">{expanded() ? "\u25B2" : "\u25BC"}</span>
      </button>

      <Show when={expanded()}>
        <div class="border-t border-gray-200 px-3 py-2 dark:border-gray-700">
          <Show when={props.args && Object.keys(props.args).length > 0}>
            <div class="mb-1">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">Arguments:</span>
              <pre class="mt-0.5 max-h-32 overflow-auto rounded bg-gray-100 p-2 text-xs dark:bg-gray-900">
                {JSON.stringify(props.args, null, 2)}
              </pre>
            </div>
          </Show>

          <Show when={props.result}>
            <div>
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">Result:</span>
              <pre class="mt-0.5 max-h-32 overflow-auto rounded bg-gray-100 p-2 text-xs dark:bg-gray-900">
                {props.result}
              </pre>
            </div>
          </Show>
        </div>
      </Show>
    </div>
  );
}
