import type { JSX } from "solid-js";
import { createSignal, Show } from "solid-js";

import { cx } from "~/utils/cx";

import NotificationCenter from "./NotificationCenter";
import { getUnreadCount } from "./notificationStore";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface NotificationBellProps {
  class?: string;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function NotificationBell(props: NotificationBellProps): JSX.Element {
  const [open, setOpen] = createSignal(false);

  return (
    <div class={cx("relative", props.class)}>
      <button
        class="relative rounded-cf-sm p-2 text-cf-text-muted transition-colors hover:bg-cf-bg-surface-alt hover:text-cf-text-primary"
        onClick={() => setOpen((prev) => !prev)}
        aria-label="Notifications"
        aria-expanded={open()}
      >
        {/* Bell SVG icon */}
        <svg
          xmlns="http://www.w3.org/2000/svg"
          class="h-5 w-5"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
          <path d="M13.73 21a2 2 0 0 1-3.46 0" />
        </svg>

        {/* Unread count badge */}
        <Show when={getUnreadCount() > 0}>
          <span class="absolute -right-0.5 -top-0.5 flex h-4 min-w-[16px] items-center justify-center rounded-full bg-cf-danger px-1 text-[10px] font-bold text-white">
            {getUnreadCount() > 99 ? "99+" : getUnreadCount()}
          </span>
        </Show>
      </button>

      <NotificationCenter visible={open()} onClose={() => setOpen(false)} />
    </div>
  );
}
