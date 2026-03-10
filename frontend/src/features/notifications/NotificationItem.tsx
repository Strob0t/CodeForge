import type { JSX } from "solid-js";
import { Show } from "solid-js";

import { cx } from "~/utils/cx";

import type { Notification, NotificationType } from "./notificationStore";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface NotificationItemProps {
  notification: Notification;
  onMarkRead: (id: string) => void;
  onArchive: (id: string) => void;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const borderColors: Record<NotificationType, string> = {
  run_failed: "border-l-red-500",
  permission_request: "border-l-blue-500",
  run_complete: "border-l-green-500",
  agent_message: "border-l-cf-accent",
  info: "border-l-cf-border",
};

function formatRelativeTime(timestamp: number): string {
  const diff = Date.now() - timestamp;
  const minutes = Math.floor(diff / 60000);
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function NotificationItem(props: NotificationItemProps): JSX.Element {
  function handleClick() {
    if (!props.notification.read) {
      props.onMarkRead(props.notification.id);
    }
  }

  function handleArchive(e: MouseEvent) {
    e.stopPropagation();
    props.onArchive(props.notification.id);
  }

  return (
    <div
      class={cx(
        "group relative flex cursor-pointer gap-2 border-l-4 px-3 py-2 transition-colors hover:bg-cf-bg-surface-alt",
        borderColors[props.notification.type],
        props.notification.read ? "bg-cf-bg-surface" : "bg-cf-bg-secondary",
      )}
      onClick={handleClick}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") handleClick();
      }}
    >
      {/* Unread indicator */}
      <Show when={!props.notification.read}>
        <span class="mt-1.5 h-2 w-2 flex-shrink-0 rounded-full bg-blue-500" aria-label="Unread" />
      </Show>

      {/* Content */}
      <div class="min-w-0 flex-1">
        <div class="flex items-start justify-between gap-2">
          <p class="truncate text-sm font-semibold text-cf-text-primary">
            {props.notification.title}
          </p>
          <span class="flex-shrink-0 text-xs text-cf-text-muted">
            {formatRelativeTime(props.notification.timestamp)}
          </span>
        </div>
        <p class="mt-0.5 text-xs text-cf-text-muted line-clamp-2">{props.notification.message}</p>
        <Show
          when={props.notification.type === "permission_request" && props.notification.actionUrl}
        >
          <a
            href={props.notification.actionUrl}
            class="mt-1 inline-block text-xs font-medium text-cf-accent hover:underline"
            onClick={(e) => e.stopPropagation()}
          >
            View
          </a>
        </Show>
      </div>

      {/* Archive button (visible on hover) */}
      <button
        class="absolute right-1 top-1 hidden rounded p-0.5 text-cf-text-muted hover:bg-cf-bg-inset hover:text-cf-text-primary group-hover:block"
        onClick={handleArchive}
        title="Archive"
        aria-label="Archive notification"
      >
        {"\u00D7"}
      </button>
    </div>
  );
}
