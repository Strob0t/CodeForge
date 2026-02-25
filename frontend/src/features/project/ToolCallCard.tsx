import { createSignal, Show } from "solid-js";

import { Card } from "~/ui";

interface ToolCallCardProps {
  name: string;
  args?: Record<string, unknown>;
  result?: string;
  status: "pending" | "running" | "completed" | "failed";
}

/** Map tool names to Unicode icon categories */
function toolIcon(name: string): string {
  const lower = name.toLowerCase();
  // File operations
  if (lower.includes("read") || lower.includes("write") || lower.includes("edit")) {
    return "\u25A1"; // white square - file
  }
  // Terminal / command execution
  if (
    lower.includes("bash") ||
    lower.includes("exec") ||
    lower.includes("shell") ||
    lower.includes("terminal")
  ) {
    return "\u25B8"; // right-pointing small triangle - terminal
  }
  // Search operations
  if (
    lower.includes("search") ||
    lower.includes("glob") ||
    lower.includes("grep") ||
    lower.includes("find")
  ) {
    return "\u25C7"; // white diamond - search
  }
  // Directory listing
  if (lower.includes("list") || lower.includes("dir") || lower.includes("ls")) {
    return "\u25A3"; // white square containing black small square - folder
  }
  // Default
  return "\u25CB"; // white circle
}

/** Check if result contains a permission denied error */
function hasPermissionDenied(result: string | undefined): boolean {
  if (!result) return false;
  return result.toLowerCase().includes("permission denied");
}

/** Content length thresholds */
const COLLAPSE_THRESHOLD = 200;
const TRUNCATE_THRESHOLD = 500;

export default function ToolCallCard(props: ToolCallCardProps) {
  const [expanded, setExpanded] = createSignal(false);
  const [argsExpanded, setArgsExpanded] = createSignal(false);
  const [resultExpanded, setResultExpanded] = createSignal(false);
  const [resultFullyShown, setResultFullyShown] = createSignal(false);

  const argsText = () => (props.args ? JSON.stringify(props.args, null, 2) : "");
  const isLongArgs = () => argsText().length > COLLAPSE_THRESHOLD;
  const isLongResult = () => (props.result?.length ?? 0) > TRUNCATE_THRESHOLD;

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

  /** Truncate text if needed and full view not toggled */
  const displayResult = () => {
    const raw = props.result ?? "";
    if (resultFullyShown() || raw.length <= TRUNCATE_THRESHOLD) return raw;
    return raw.slice(0, TRUNCATE_THRESHOLD);
  };

  return (
    <Card class="my-1 text-sm">
      <button
        class="flex w-full items-center gap-2 px-3 py-1.5 text-left hover:bg-cf-bg-surface-alt"
        onClick={() => setExpanded(!expanded())}
        aria-expanded={expanded()}
      >
        <span class="text-cf-text-muted" title={props.name}>
          {toolIcon(props.name)}
        </span>
        <span class={statusColor()}>{statusIcon()}</span>
        <span class="font-mono text-xs text-cf-text-primary">{props.name}</span>

        {/* Permission Denied badge */}
        <Show when={hasPermissionDenied(props.result)}>
          <span class="ml-1 rounded-full bg-red-600 px-1.5 py-0.5 text-[10px] font-semibold text-white leading-none">
            Permission Denied
          </span>
        </Show>

        <span class="ml-auto text-xs text-cf-text-muted">{expanded() ? "\u25B2" : "\u25BC"}</span>
      </button>

      <Show when={expanded()}>
        <div class="border-t border-cf-border px-3 py-2">
          {/* Arguments section */}
          <Show when={props.args && Object.keys(props.args).length > 0}>
            <div class="mb-1">
              <button
                class="flex items-center gap-1 text-xs font-medium text-cf-text-tertiary hover:text-cf-text-secondary"
                onClick={() => setArgsExpanded(!argsExpanded())}
              >
                <span>{argsExpanded() ? "\u25BE" : "\u25B8"}</span>
                Arguments
              </button>
              <Show when={!isLongArgs() || argsExpanded()}>
                <pre class="mt-0.5 max-h-48 overflow-auto rounded-cf-sm bg-cf-bg-inset p-2 text-xs">
                  {argsText()}
                </pre>
              </Show>
              <Show when={isLongArgs() && !argsExpanded()}>
                <pre class="mt-0.5 max-h-12 overflow-hidden rounded-cf-sm bg-cf-bg-inset p-2 text-xs opacity-60">
                  {argsText().slice(0, 80)}...
                </pre>
              </Show>
            </div>
          </Show>

          {/* Result section */}
          <Show when={props.result}>
            <div>
              <button
                class="flex items-center gap-1 text-xs font-medium text-cf-text-tertiary hover:text-cf-text-secondary"
                onClick={() => setResultExpanded(!resultExpanded())}
              >
                <span>{resultExpanded() ? "\u25BE" : "\u25B8"}</span>
                Result
              </button>
              <Show when={!((props.result?.length ?? 0) > COLLAPSE_THRESHOLD) || resultExpanded()}>
                <pre class="mt-0.5 max-h-48 overflow-auto rounded-cf-sm bg-cf-bg-inset p-2 text-xs whitespace-pre-wrap break-all">
                  {displayResult()}
                </pre>
                <Show when={isLongResult() && !resultFullyShown()}>
                  <button
                    class="mt-1 text-xs text-cf-accent hover:underline"
                    onClick={() => setResultFullyShown(true)}
                  >
                    Show more ({((props.result?.length ?? 0) - TRUNCATE_THRESHOLD).toLocaleString()}{" "}
                    more chars)
                  </button>
                </Show>
                <Show when={isLongResult() && resultFullyShown()}>
                  <button
                    class="mt-1 text-xs text-cf-accent hover:underline"
                    onClick={() => setResultFullyShown(false)}
                  >
                    Show less
                  </button>
                </Show>
              </Show>
              <Show when={(props.result?.length ?? 0) > COLLAPSE_THRESHOLD && !resultExpanded()}>
                <pre class="mt-0.5 max-h-12 overflow-hidden rounded-cf-sm bg-cf-bg-inset p-2 text-xs opacity-60">
                  {(props.result ?? "").slice(0, 80)}...
                </pre>
              </Show>
            </div>
          </Show>
        </div>
      </Show>
    </Card>
  );
}
