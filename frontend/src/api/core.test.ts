import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { getQueueLength } from "./cache";
import { createCoreClient, FetchError } from "./core";

// S7-G review: DELETE /me/data went through the client's generic retries
// (502/503/504 and network errors are sent again) and, when the browser was
// offline, into the offline queue, which answers only once the browser is
// back online, so the deletion dialog could wait forever. requestOnce sends
// a request exactly once and reports every failure.
describe("requestOnce", () => {
  const fetchMock = vi.fn<typeof fetch>();
  let online = true;

  beforeEach(() => {
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
    online = true;
    vi.spyOn(navigator, "onLine", "get").mockImplementation(() => online);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("sends the request once and answers its result", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));

    await expect(
      createCoreClient().requestOnce<undefined>("/me/data", { method: "DELETE" }),
    ).resolves.toBeUndefined();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/me/data");
    expect(fetchMock.mock.calls[0][1]?.method).toBe("DELETE");
  });

  it("does not repeat a request the server answered with 503", async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ error: "unavailable" }), { status: 503 }),
    );

    const err = await createCoreClient()
      .requestOnce("/me/data", { method: "DELETE" })
      .catch((e: unknown) => e);
    expect(err).toBeInstanceOf(FetchError);
    expect((err as FetchError).status).toBe(503);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("neither repeats nor queues a request that failed while offline", async () => {
    online = false;
    fetchMock.mockRejectedValue(new TypeError("Failed to fetch"));
    const queued = getQueueLength();

    await expect(
      createCoreClient().requestOnce("/me/data", { method: "DELETE" }),
    ).rejects.toBeInstanceOf(TypeError);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(getQueueLength()).toBe(queued);
  });
});
