import { FetchError } from "~/api/core";

/**
 * The message shown when sending a chat message fails. The backend refuses a
 * second message while a run of the conversation is active (409).
 */
export function sendErrorKey(err: unknown): "chat.runInProgress" | "chat.sendFailed" {
  return err instanceof FetchError && err.status === 409 ? "chat.runInProgress" : "chat.sendFailed";
}
