import type { ChannelMessageRecord } from "~/api/types";
import type { WSMessage } from "~/api/websocket";

function stringField(record: Record<string, unknown>, key: string): string | undefined {
  const value = record[key];
  return typeof value === "string" ? value : undefined;
}

/**
 * Returns the message of a channel.message event (Go event.ChannelMessageEvent),
 * or null for other events and for payloads that lack a required field.
 */
export function parseChannelMessageEvent(msg: WSMessage): ChannelMessageRecord | null {
  if (msg.type !== "channel.message") return null;
  const channelId = stringField(msg.payload, "channel_id");
  const raw = msg.payload.message;
  if (!channelId || typeof raw !== "object" || raw === null) return null;

  const m = raw as Record<string, unknown>;
  const id = stringField(m, "id");
  const content = stringField(m, "content");
  if (!id || content === undefined || stringField(m, "channel_id") !== channelId) return null;

  return {
    id,
    channel_id: channelId,
    sender_type: stringField(m, "sender_type") ?? "",
    sender_name: stringField(m, "sender_name") ?? "",
    content,
    // Go omits an empty parent_id: the message is not a thread reply.
    parent_id: stringField(m, "parent_id") ?? "",
    created_at: stringField(m, "created_at") ?? "",
  };
}

/**
 * Adds a message at the start (newest-first list) or end (chronological list)
 * unless a message with its ID is already there: the sender receives the event
 * of its own message as well.
 */
export function addMessage<T extends { id: string }>(
  list: T[] | undefined,
  message: T,
  at: "start" | "end",
): T[] {
  const current = list ?? [];
  if (current.some((m) => m.id === message.id)) return current;
  return at === "start" ? [message, ...current] : [...current, message];
}
