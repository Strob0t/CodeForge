import { describe, expect, it, vi } from "vitest";

import { coalesce } from "./coalesce";

/** A promise the test settles itself. */
function deferred(): { promise: Promise<void>; resolve: () => void; reject: (e: Error) => void } {
  let resolve: () => void = () => undefined;
  let reject: (e: Error) => void = () => undefined;
  const promise = new Promise<void>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const settle = () => new Promise((r) => setTimeout(r, 0));

describe("coalesce", () => {
  it("runs at once when idle", () => {
    const run = vi.fn(() => undefined);
    coalesce(run)();
    expect(run).toHaveBeenCalledTimes(1);
  });

  it("runs once more after a burst of triggers while it runs", async () => {
    const first = deferred();
    const run = vi.fn().mockReturnValueOnce(first.promise).mockReturnValue(undefined);
    const trigger = coalesce(run);
    trigger();
    trigger();
    trigger();
    trigger();
    expect(run).toHaveBeenCalledTimes(1);
    first.resolve();
    await settle();
    expect(run).toHaveBeenCalledTimes(2);
  });

  it("goes on after a run fails, also after one that throws", async () => {
    const failed = deferred();
    const run = vi
      .fn()
      .mockReturnValueOnce(failed.promise)
      .mockImplementationOnce(() => {
        throw new Error("sync failure");
      })
      .mockReturnValue(undefined);
    const trigger = coalesce(run);
    trigger();
    trigger();
    failed.reject(new Error("refresh failed"));
    await settle();
    expect(run).toHaveBeenCalledTimes(2);
    trigger();
    await settle();
    expect(run).toHaveBeenCalledTimes(3);
  });
});
