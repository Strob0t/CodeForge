import { createSignal, onCleanup, Show } from "solid-js";

import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { extractErrorMessage } from "~/lib/errorUtils";

import { api } from "../../api/client";

export interface PermissionRequestCardProps {
  projectId: string;
  runId: string;
  callId: string;
  tool: string;
  command?: string;
  path?: string;
  profile?: string;
  /** Truncated JSON of the tool arguments (display only). */
  argumentsPreview?: string;
  timeoutSeconds?: number;
  /** When the Core denies the call unanswered (RFC 3339): the countdown's end (KI-148). */
  expiresAt?: string;
  onResolved?: (decision: "allow" | "deny") => void;
}

export default function PermissionRequestCard(props: PermissionRequestCardProps) {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const timeout = () => props.timeoutSeconds ?? 60;
  // Seconds to the Core's deadline; without one, the timeout from mount.
  const secondsToDeadline = (): number | undefined => {
    const deadline = props.expiresAt ? Date.parse(props.expiresAt) : Number.NaN;
    return Number.isNaN(deadline)
      ? undefined
      : Math.max(0, Math.ceil((deadline - Date.now()) / 1000));
  };
  const [remaining, setRemaining] = createSignal(secondsToDeadline() ?? timeout());
  const [resolved, setResolved] = createSignal<"allow" | "deny" | null>(null);
  const [loading, setLoading] = createSignal(false);

  const timer = setInterval(() => {
    setRemaining((r) => {
      const next = secondsToDeadline() ?? r - 1;
      if (next <= 0) {
        void handleDecision("deny");
        return 0;
      }
      return next;
    });
  }, 1000);

  onCleanup(() => clearInterval(timer));

  async function handleDecision(decision: "allow" | "deny") {
    if (resolved()) return;
    setLoading(true);
    clearInterval(timer);
    try {
      await api.runs.approve(props.runId, props.callId, decision);
      setResolved(decision);
      props.onResolved?.(decision);
    } catch {
      setResolved("deny");
    } finally {
      setLoading(false);
    }
  }

  async function handleAllowAlways() {
    await handleDecision("allow");
    try {
      await api.policies.allowAlways(props.projectId, props.tool, props.command, props.profile);
    } catch (err) {
      // The current call is approved; tell the user the rule was not saved.
      toast("error", t("policy.allowAlwaysFailed", { error: extractErrorMessage(err) }));
    }
  }

  const progressPercent = () => (remaining() / timeout()) * 100;

  const toolIcon = () => {
    switch (props.tool) {
      case "bash":
      case "exec":
      case "shell":
        return "\u25B8";
      case "read":
      case "read_file":
        return "\u25A1";
      case "edit":
      case "edit_file":
      case "write":
      case "write_file":
        return "\u25A1";
      case "search":
      case "glob":
      case "grep":
        return "\u25C7";
      default:
        return "\u25CB";
    }
  };

  return (
    <div
      class={`rounded-cf-md border-2 p-4 my-2 ${
        resolved() === "allow"
          ? "border-green-500 bg-green-500/5"
          : resolved() === "deny"
            ? "border-red-500 bg-red-500/5"
            : "border-amber-500 bg-amber-500/5"
      }`}
    >
      <div class="flex items-center gap-2 mb-3">
        <span class="text-amber-500 font-bold text-lg">{"\u26A0"}</span>
        <span class="font-semibold text-cf-text-primary text-sm">Permission Request</span>
      </div>

      <div class="space-y-1 mb-3 text-sm">
        <div class="flex gap-2">
          <span class="text-cf-text-muted w-20">Tool:</span>
          <span class="font-mono text-cf-text-primary">
            {toolIcon()} {props.tool}
          </span>
        </div>
        <Show when={props.command}>
          <div class="flex gap-2">
            <span class="text-cf-text-muted w-20">Command:</span>
            <span class="font-mono text-cf-text-primary break-all">{props.command}</span>
          </div>
        </Show>
        <Show when={props.path}>
          <div class="flex gap-2">
            <span class="text-cf-text-muted w-20">Path:</span>
            <span class="font-mono text-cf-text-primary break-all">{props.path}</span>
          </div>
        </Show>
        <Show when={props.argumentsPreview}>
          <div class="flex gap-2">
            <span class="text-cf-text-muted w-20 shrink-0">Arguments:</span>
            <pre class="font-mono text-xs text-cf-text-primary whitespace-pre-wrap break-all max-h-40 overflow-y-auto min-w-0">
              {props.argumentsPreview}
            </pre>
          </div>
        </Show>
      </div>

      <Show when={!resolved()}>
        <div class="mb-3">
          <div class="w-full bg-cf-bg-inset rounded-full h-1.5">
            <div
              class={`h-1.5 rounded-full transition-all duration-1000 ${
                remaining() > 30
                  ? "bg-amber-500"
                  : remaining() > 10
                    ? "bg-orange-500"
                    : "bg-red-500"
              }`}
              style={{ width: `${progressPercent()}%` }}
            />
          </div>
          <span class="text-xs text-cf-text-muted mt-1">{remaining()}s remaining</span>
        </div>

        <div class="flex gap-2">
          <button
            class="px-3 py-1.5 rounded-cf-sm bg-green-600 text-white text-sm font-medium hover:bg-green-700 disabled:opacity-50"
            onClick={() => handleDecision("allow")}
            disabled={loading()}
          >
            Allow
          </button>
          <button
            class="px-3 py-1.5 rounded-cf-sm bg-cf-bg-surface border border-cf-border text-cf-text-primary text-sm font-medium hover:bg-cf-bg-inset disabled:opacity-50"
            onClick={handleAllowAlways}
            disabled={loading()}
          >
            Allow Always
          </button>
          <button
            class="px-3 py-1.5 rounded-cf-sm bg-red-600 text-white text-sm font-medium hover:bg-red-700 disabled:opacity-50"
            onClick={() => handleDecision("deny")}
            disabled={loading()}
          >
            Deny
          </button>
        </div>
      </Show>

      <Show when={resolved()}>
        <div
          class={`text-sm font-medium ${
            resolved() === "allow" ? "text-green-500" : "text-red-500"
          }`}
        >
          {resolved() === "allow" ? "\u2713 Allowed" : "\u2717 Denied"}
        </div>
      </Show>
    </div>
  );
}
