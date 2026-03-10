import type { JSX } from "solid-js";
import { createMemo, createSignal, For, Show } from "solid-js";

import { cx } from "~/utils/cx";

import NotificationItem from "./NotificationItem";
import {
  archiveNotification,
  getNotifications,
  markAllRead,
  markRead,
  type Notification,
} from "./notificationStore";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface NotificationCenterProps {
  visible: boolean;
  onClose: () => void;
}

// ---------------------------------------------------------------------------
// Filter tabs
// ---------------------------------------------------------------------------

type FilterTab = "all" | "unread" | "archived";

const TABS: { key: FilterTab; label: string }[] = [
  { key: "all", label: "All" },
  { key: "unread", label: "Unread" },
  { key: "archived", label: "Archived" },
];

function filterNotifications(items: Notification[], tab: FilterTab): Notification[] {
  switch (tab) {
    case "unread":
      return items.filter((n) => !n.read && !n.archived);
    case "archived":
      return items.filter((n) => n.archived);
    case "all":
    default:
      return items.filter((n) => !n.archived);
  }
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function NotificationCenter(props: NotificationCenterProps): JSX.Element {
  const [activeTab, setActiveTab] = createSignal<FilterTab>("all");

  const filtered = createMemo(() => filterNotifications(getNotifications(), activeTab()));

  return (
    <Show when={props.visible}>
      {/* Backdrop — click to close */}
      <div class="fixed inset-0 z-40" onClick={() => props.onClose()} />

      {/* Dropdown panel */}
      <div class="fixed right-4 top-14 z-50 w-80 rounded-cf-md border border-cf-border bg-cf-bg-surface shadow-cf-lg">
        {/* Header */}
        <div class="flex items-center justify-between border-b border-cf-border px-3 py-2">
          <h3 class="text-sm font-semibold text-cf-text-primary">Notifications</h3>
          <button class="text-xs text-cf-accent hover:underline" onClick={() => markAllRead()}>
            Mark all read
          </button>
        </div>

        {/* Filter tabs */}
        <div class="flex border-b border-cf-border">
          <For each={TABS}>
            {(tab) => (
              <button
                class={cx(
                  "flex-1 py-1.5 text-xs font-medium transition-colors",
                  activeTab() === tab.key
                    ? "border-b-2 border-cf-accent text-cf-accent"
                    : "text-cf-text-muted hover:text-cf-text-primary",
                )}
                onClick={() => setActiveTab(tab.key)}
              >
                {tab.label}
              </button>
            )}
          </For>
        </div>

        {/* Notification list */}
        <div class="max-h-80 overflow-y-auto">
          <Show
            when={filtered().length > 0}
            fallback={
              <div class="px-4 py-8 text-center text-sm text-cf-text-muted">No notifications</div>
            }
          >
            <For each={filtered()}>
              {(notification) => (
                <NotificationItem
                  notification={notification}
                  onMarkRead={markRead}
                  onArchive={archiveNotification}
                />
              )}
            </For>
          </Show>
        </div>
      </div>
    </Show>
  );
}
