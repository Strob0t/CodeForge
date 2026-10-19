import type { ChannelMessageRecord, ChannelReadState } from "~/api/types";
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
    // Go omits sender_id for agents, bots and webhooks.
    sender_id: stringField(m, "sender_id"),
    sender_type: stringField(m, "sender_type") ?? "",
    sender_name: stringField(m, "sender_name") ?? "",
    content,
    // Go omits an empty parent_id: the message is not a thread reply.
    parent_id: stringField(m, "parent_id") ?? "",
    created_at: stringField(m, "created_at") ?? "",
  };
}

/**
 * Returns the read position of a channel.read event (Go event.ChannelReadEvent),
 * or null for other events and incomplete payloads.
 */
export function parseChannelReadEvent(msg: WSMessage): ChannelReadState | null {
  if (msg.type !== "channel.read") return null;
  const channelId = stringField(msg.payload, "channel_id");
  const userId = stringField(msg.payload, "user_id");
  const messageId = stringField(msg.payload, "message_id");
  if (!channelId || !userId || !messageId) return null;
  return {
    channel_id: channelId,
    user_id: userId,
    last_read_message_id: messageId,
    last_read_at: stringField(msg.payload, "last_read_at") ?? "",
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

/** Counts the thread replies per parent message. */
export function replyCounts(messages: readonly { parent_id: string }[]): Map<string, number> {
  const counts = new Map<string, number>();
  for (const m of messages) {
    if (m.parent_id) counts.set(m.parent_id, (counts.get(m.parent_id) ?? 0) + 1);
  }
  return counts;
}

/**
 * Applies a live channel message to the unread counts shown for the channel
 * list, like the server counts them: top-level messages of others raise the
 * channel's count unless the reader follows that channel (open and scrolled
 * to the bottom). Read positions are not applied here: one may be older than
 * the newest message, so the list takes the counts from the server after a
 * read. Returns `counts` itself when the event changes nothing (most events
 * are not channel messages).
 */
export function applyUnreadEvent(
  counts: Readonly<Record<string, number>>,
  msg: WSMessage,
  me: string | undefined,
  followedChannelId: string | undefined,
): Readonly<Record<string, number>> {
  const message = parseChannelMessageEvent(msg);
  if (!message) return counts;
  const own = me !== undefined && message.sender_id === me;
  if (message.parent_id || own || message.channel_id === followedChannelId) return counts;
  return { ...counts, [message.channel_id]: (counts[message.channel_id] ?? 0) + 1 };
}
