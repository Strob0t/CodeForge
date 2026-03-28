import type { JSX } from "solid-js";
import { createMemo, createSignal, For, Show } from "solid-js";

import { useFocusTrap } from "~/hooks/useFocusTrap";
import { Backdrop } from "~/ui";
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
  let panelRef: HTMLDivElement | undefined;

  const { onKeyDown: trapKeyDown } = useFocusTrap(
    () => panelRef,
    () => props.visible,
  );

  function handleKeyDown(e: KeyboardEvent) {
    if (e.key === "Escape") {
      e.stopPropagation();
      props.onClose();
      return;
    }
    trapKeyDown(e);
  }

  const filtered = createMemo(() => filterNotifications(getNotifications(), activeTab()));

  return (
    <Show when={props.visible}>
      {/* Backdrop — click to close */}
      <Backdrop onClick={() => props.onClose()} class="bg-transparent" />

      {/* Dropdown panel */}
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-label="Notifications"
        tabIndex={-1}
        onKeyDown={handleKeyDown}
        class="absolute right-0 top-full z-50 mt-2 w-80 rounded-cf-md border border-cf-border bg-cf-bg-surface shadow-cf-lg"
      >
        {/* Header */}
        <div class="flex items-center justify-between border-b border-cf-border px-3 py-2">
          <h3 class="text-sm font-semibold text-cf-text-primary">Notifications</h3>
          <button
            class="text-xs text-cf-accent hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cf-focus-ring focus-visible:ring-offset-2"
            onClick={() => markAllRead()}
          >
            Mark all read
          </button>
        </div>

        {/* Filter tabs */}
        <div class="flex border-b border-cf-border">
          <For each={TABS}>
            {(tab) => (
              <button
                class={cx(
                  "flex-1 py-1.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cf-focus-ring focus-visible:ring-offset-2",
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
                  onClose={props.onClose}
                />
              )}
            </For>
          </Show>
        </div>
      </div>
    </Show>
  );
}
