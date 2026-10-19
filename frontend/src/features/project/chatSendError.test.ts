import { describe, expect, it } from "vitest";

import { FetchError } from "~/api/core";

import { sendErrorKey } from "./chatSendError";

describe("sendErrorKey", () => {
  it("explains a refused message while a run is active (409)", () => {
    expect(sendErrorKey(new FetchError(409, { error: "conversation run in progress" }))).toBe(
      "chat.runInProgress",
    );
  });

  it("reports other API errors as a failed send", () => {
    expect(sendErrorKey(new FetchError(500, { error: "boom" }))).toBe("chat.sendFailed");
    expect(sendErrorKey(new FetchError(400, { error: "bad" }))).toBe("chat.sendFailed");
  });

  it("reports network errors as a failed send", () => {
    expect(sendErrorKey(new TypeError("Failed to fetch"))).toBe("chat.sendFailed");
    expect(sendErrorKey(undefined)).toBe("chat.sendFailed");
  });
});
