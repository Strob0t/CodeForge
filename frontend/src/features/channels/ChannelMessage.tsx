import type { Component } from "solid-js";
import { Show } from "solid-js";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface ChannelMessageData {
  id: string;
  sender_type: string;
  sender_name: string;
  content: string;
  parent_id: string;
  created_at: string;
}

interface ChannelMessageProps {
  message: ChannelMessageData;
  onThreadClick?: (messageId: string) => void;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Sender type to icon mapping (Unicode symbols, no icon library). */
function senderIcon(senderType: string): string {
  switch (senderType) {
    case "user":
      return "\u{1F464}"; // bust in silhouette
    case "agent":
      return "\u{1F916}"; // robot face
    case "bot":
      return "\u2699"; // gear
    case "webhook":
      return "\u{1F517}"; // link
    default:
      return "\u25CF"; // filled circle
  }
}

/** Sender type to badge variant class. */
function senderBadgeClass(senderType: string): string {
  switch (senderType) {
    case "user":
      return "bg-cf-accent/10 text-cf-accent";
    case "agent":
      return "bg-cf-success-bg text-cf-success-fg";
    case "bot":
      return "bg-cf-warning-bg text-cf-warning-fg";
    case "webhook":
      return "bg-cf-info-bg text-cf-info-fg";
    default:
      return "bg-cf-bg-surface-alt text-cf-text-secondary";
  }
}

/** Format an ISO timestamp into a short, readable time string. */
function formatTimestamp(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;

  const now = new Date();
  const isToday =
    d.getFullYear() === now.getFullYear() &&
    d.getMonth() === now.getMonth() &&
    d.getDate() === now.getDate();

  if (isToday) {
    return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  }
  return d.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

const ChannelMessage: Component<ChannelMessageProps> = (props) => {
  return (
    <div class="group flex gap-3 px-4 py-2 hover:bg-cf-bg-surface-alt transition-colors">
      {/* Sender icon */}
      <div
        class={
          "flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-sm " +
          senderBadgeClass(props.message.sender_type)
        }
      >
        {senderIcon(props.message.sender_type)}
      </div>

      {/* Message body */}
      <div class="min-w-0 flex-1">
        <div class="flex items-baseline gap-2">
          <span class="text-sm font-semibold text-cf-text-primary">
            {props.message.sender_name}
          </span>
          <span class="text-xs text-cf-text-muted">
            {formatTimestamp(props.message.created_at)}
          </span>
        </div>
        <p class="mt-0.5 text-sm text-cf-text-secondary whitespace-pre-wrap break-words">
          {props.message.content}
        </p>

        {/* Thread reply link — only for top-level messages when handler is provided */}
        <Show when={!props.message.parent_id && props.onThreadClick}>
          <button
            type="button"
            class="mt-1 text-xs text-cf-accent hover:text-cf-accent-hover transition-colors opacity-0 group-hover:opacity-100 focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cf-focus-ring focus-visible:ring-offset-2"
            onClick={() => props.onThreadClick?.(props.message.id)}
          >
            Reply in thread
          </button>
        </Show>
      </div>
    </div>
  );
};

export default ChannelMessage;
