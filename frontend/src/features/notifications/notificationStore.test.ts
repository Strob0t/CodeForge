import { describe, expect, it } from "vitest";

import {
  addNotification,
  archiveNotification,
  clearAll,
  getNotifications,
  getUnreadCount,
  markAllRead,
  markRead,
} from "./notificationStore";

describe("notificationStore", () => {
  // FIX-034: Verify notificationStore exports and basic API shape.

  it("should export addNotification function", () => {
    expect(typeof addNotification).toBe("function");
  });

  it("should export markRead function", () => {
    expect(typeof markRead).toBe("function");
  });

  it("should export markAllRead function", () => {
    expect(typeof markAllRead).toBe("function");
  });

  it("should export archiveNotification function", () => {
    expect(typeof archiveNotification).toBe("function");
  });

  it("should export clearAll function", () => {
    expect(typeof clearAll).toBe("function");
  });

  it("should export getNotifications function", () => {
    expect(typeof getNotifications).toBe("function");
  });

  it("should export getUnreadCount function", () => {
    expect(typeof getUnreadCount).toBe("function");
  });
});
