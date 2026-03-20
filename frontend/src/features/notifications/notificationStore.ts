import { createStore } from "solid-js/store";

import { loadSettings } from "./notificationSettings";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type NotificationType =
  | "permission_request"
  | "run_complete"
  | "run_failed"
  | "agent_message"
  | "info";

export interface Notification {
  id: string;
  type: NotificationType;
  title: string;
  message: string;
  timestamp: number;
  read: boolean;
  archived: boolean;
  actionUrl?: string;
  metadata?: Record<string, string>;
}

// ---------------------------------------------------------------------------
// Store (module-level singleton — intentionally long-lived)
//
// This store persists for the lifetime of the SPA. It is NOT tied to any
// component lifecycle and does not need disposal via onCleanup(). The SolidJS
// createStore does not allocate external resources (timers, sockets, etc.)
// that would leak. AudioContext instances created in playNotificationSound()
// are short-lived and self-dispose after the oscillator stops.
// ---------------------------------------------------------------------------

const MAX_NOTIFICATIONS = 50;

const [notifications, setNotifications] = createStore<Notification[]>([]);

let idCounter = 0;

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

export function addNotification(
  opts: Omit<Notification, "id" | "timestamp" | "read" | "archived">,
): void {
  const notification: Notification = {
    ...opts,
    id: `notif-${Date.now()}-${++idCounter}`,
    timestamp: Date.now(),
    read: false,
    archived: false,
  };

  setNotifications((prev) => {
    const next = [notification, ...prev];
    return next.length > MAX_NOTIFICATIONS ? next.slice(0, MAX_NOTIFICATIONS) : next;
  });

  const settings = loadSettings();

  // Browser push notification when tab is not visible
  if (settings.enablePush && document.hidden) {
    sendPushNotification(notification);
  }

  // Audible alert
  if (settings.enableSound && settings.soundType !== "none") {
    playNotificationSound(settings.soundType);
  }
}

export function markRead(id: string): void {
  setNotifications((n) => n.id === id, "read", true);
}

export function markAllRead(): void {
  setNotifications({}, "read", true);
}

export function archiveNotification(id: string): void {
  setNotifications((n) => n.id === id, "archived", true);
}

export function clearAll(): void {
  setNotifications([]);
}

export function getNotifications(): Notification[] {
  return notifications;
}

export function getUnreadCount(): number {
  return notifications.filter((n) => !n.read && !n.archived).length;
}

// ---------------------------------------------------------------------------
// Browser Push
// ---------------------------------------------------------------------------

function sendPushNotification(notification: Notification): void {
  if (!("Notification" in window)) return;

  if (Notification.permission === "granted") {
    new Notification(notification.title, {
      body: notification.message,
      tag: notification.id,
    });
  } else if (Notification.permission !== "denied") {
    void Notification.requestPermission();
  }
}

// ---------------------------------------------------------------------------
// Sound
// ---------------------------------------------------------------------------

function playNotificationSound(soundType: "default" | "subtle"): void {
  try {
    const ctx = new AudioContext();
    const osc = ctx.createOscillator();
    const gain = ctx.createGain();
    osc.connect(gain);
    gain.connect(ctx.destination);

    osc.frequency.value = soundType === "subtle" ? 440 : 800;
    gain.gain.value = soundType === "subtle" ? 0.05 : 0.1;

    osc.start();
    osc.stop(ctx.currentTime + 0.15);
  } catch {
    // AudioContext not available (e.g. SSR, unsupported browser)
  }
}
