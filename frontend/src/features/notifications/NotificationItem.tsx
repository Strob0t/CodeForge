import type { JSX } from "solid-js";
import { Show } from "solid-js";
import { useNavigate } from "@solidjs/router";

import { cx } from "~/utils/cx";

import type { Notification, NotificationType } from "./notificationStore";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface NotificationItemProps {
  notification: Notification;
  onMarkRead: (id: string) => void;
  onArchive: (id: string) => void;
  onClose?: () => void;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const borderColors: Record<NotificationType, string> = {
  run_failed: "border-l-cf-danger",
  permission_request: "border-l-cf-accent",
  run_complete: "border-l-cf-success",
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
  const navigate = useNavigate();

  function handleClick() {
    if (!props.notification.read) {
      props.onMarkRead(props.notification.id);
    }
    if (props.notification.actionUrl) {
      props.onClose?.();
      navigate(props.notification.actionUrl);
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
        <span class="mt-1.5 h-2 w-2 flex-shrink-0 rounded-full bg-cf-accent" aria-label="Unread" />
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
