import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  addNotification,
  archiveNotification,
  clearAll,
  getNotifications,
  getUnreadCount,
  markAllRead,
  markRead,
} from "./notificationStore";

beforeEach(() => {
  clearAll();
  vi.stubGlobal(
    "AudioContext",
    vi.fn(() => ({
      createOscillator: () => ({
        connect: vi.fn(),
        frequency: { value: 0 },
        start: vi.fn(),
        stop: vi.fn(),
      }),
      createGain: () => ({ connect: vi.fn(), gain: { value: 0 } }),
      destination: {},
      currentTime: 0,
    })),
  );
  vi.stubGlobal("Notification", Object.assign(vi.fn(), { permission: "denied" }));
});

afterEach(() => {
  vi.unstubAllGlobals();
  clearAll();
});

describe("notificationStore behavioral", () => {
  it("addNotification adds to the front", () => {
    addNotification({ type: "info", title: "First", message: "m" });
    addNotification({ type: "info", title: "Second", message: "m" });
    const ns = getNotifications();
    expect(ns).toHaveLength(2);
    expect(ns[0].title).toBe("Second");
  });

  it("sets read=false and archived=false", () => {
    addNotification({ type: "info", title: "T", message: "m" });
    expect(getNotifications()[0].read).toBe(false);
    expect(getNotifications()[0].archived).toBe(false);
  });

  it("caps at 50 notifications", () => {
    for (let i = 0; i < 55; i++) addNotification({ type: "info", title: `n${i}`, message: "m" });
    expect(getNotifications()).toHaveLength(50);
  });

  it("markRead marks only the target", () => {
    addNotification({ type: "info", title: "A", message: "m" });
    addNotification({ type: "info", title: "B", message: "m" });
    const [b, a] = getNotifications();
    markRead(a.id);
    expect(getNotifications().find((n) => n.id === a.id)?.read).toBe(true);
    expect(getNotifications().find((n) => n.id === b.id)?.read).toBe(false);
  });

  it("markAllRead marks all", () => {
    addNotification({ type: "info", title: "A", message: "m" });
    addNotification({ type: "info", title: "B", message: "m" });
    markAllRead();
    expect(getNotifications().every((n) => n.read)).toBe(true);
  });

  it("archiveNotification sets archived on target only", () => {
    addNotification({ type: "info", title: "A", message: "m" });
    addNotification({ type: "info", title: "B", message: "m" });
    const [, a] = getNotifications();
    archiveNotification(a.id);
    expect(getNotifications().find((n) => n.id === a.id)?.archived).toBe(true);
  });

  it("clearAll empties the store", () => {
    addNotification({ type: "info", title: "A", message: "m" });
    clearAll();
    expect(getNotifications()).toHaveLength(0);
  });

  it("getUnreadCount excludes read and archived", () => {
    addNotification({ type: "info", title: "A", message: "m" });
    addNotification({ type: "info", title: "B", message: "m" });
    addNotification({ type: "info", title: "C", message: "m" });
    const [c, b] = getNotifications();
    markRead(c.id);
    archiveNotification(b.id);
    expect(getUnreadCount()).toBe(1);
  });
});
