import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  clearCache,
  getCached,
  getQueueLength,
  invalidateCache,
  processQueue,
  queueAction,
  setCached,
} from "./cache";

afterEach(() => {
  clearCache();
});

describe("Response Cache", () => {
  it("returns undefined for missing keys", () => {
    expect(getCached("/api/missing")).toBeUndefined();
  });

  it("stores and retrieves a value", () => {
    setCached("/api/projects", [{ id: "1", name: "test" }]);
    const result = getCached<{ id: string; name: string }[]>("/api/projects");
    expect(result).toEqual([{ id: "1", name: "test" }]);
  });

  it("returns undefined for expired entries", () => {
    vi.useFakeTimers();
    try {
      setCached("/api/old", { data: "stale" });
      // Advance time past the maxAge
      vi.advanceTimersByTime(100);
      expect(getCached("/api/old", 50)).toBeUndefined();
    } finally {
      vi.useRealTimers();
    }
  });

  it("respects custom maxAgeMs", () => {
    setCached("/api/recent", { data: "fresh" });
    // With a generous max age, value should still be available
    const result = getCached<{ data: string }>("/api/recent", 60_000);
    expect(result).toEqual({ data: "fresh" });
  });

  it("invalidates entries by prefix", () => {
    setCached("/api/projects/1", { id: "1" });
    setCached("/api/projects/2", { id: "2" });
    setCached("/api/models", { id: "m1" });

    invalidateCache("/api/projects");

    expect(getCached("/api/projects/1")).toBeUndefined();
    expect(getCached("/api/projects/2")).toBeUndefined();
    expect(getCached("/api/models")).toEqual({ id: "m1" });
  });

  it("clears all entries", () => {
    setCached("/api/a", 1);
    setCached("/api/b", 2);

    clearCache();

    expect(getCached("/api/a")).toBeUndefined();
    expect(getCached("/api/b")).toBeUndefined();
  });
});

describe("Offline Action Queue", () => {
  // Drain leftover queue items between tests to avoid cross-test leakage.
  beforeEach(async () => {
    if (getQueueLength() > 0) {
      await processQueue(() => Promise.resolve(null));
    }
  });

  it("starts with queue length 0", () => {
    expect(getQueueLength()).toBe(0);
  });

  it("processes queued actions successfully", async () => {
    const executor = vi.fn().mockResolvedValue({ ok: true });

    queueAction("/api/test", { method: "POST" });
    queueAction("/api/test2", { method: "PUT" });

    const result = await processQueue(executor);

    expect(result).toEqual({ succeeded: 2, failed: 0 });
    expect(executor).toHaveBeenCalledTimes(2);
    expect(getQueueLength()).toBe(0);
  });

  it("counts failures when executor rejects", async () => {
    const executor = vi.fn().mockRejectedValue(new Error("Network error"));

    // Attach a catch handler to avoid unhandled rejection
    const promise = queueAction("/api/fail", { method: "POST" });
    promise.catch(() => {
      /* expected rejection */
    });

    const result = await processQueue(executor);

    expect(result).toEqual({ succeeded: 0, failed: 1 });
    expect(getQueueLength()).toBe(0);
  });

  it("returns zero counts when queue is empty", async () => {
    const executor = vi.fn();
    const result = await processQueue(executor);
    expect(result).toEqual({ succeeded: 0, failed: 0 });
    expect(executor).not.toHaveBeenCalled();
  });
});
