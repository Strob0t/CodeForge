import { useParams } from "@solidjs/router";
import {
  createEffect,
  createResource,
  createSignal,
  For,
  onCleanup,
  onMount,
  Show,
} from "solid-js";

import { api } from "~/api/client";
import type { ChannelMessageRecord } from "~/api/types";
import { useAuth } from "~/components/AuthProvider";
import { useConfirm } from "~/components/ConfirmProvider";
import { useWebSocket } from "~/components/WebSocketProvider";
import { Alert, Badge, Button } from "~/ui";

import { addMessage, parseChannelMessageEvent, replyCounts } from "./channelEvents";
import ChannelInput from "./ChannelInput";
import type { ChannelMessageData } from "./ChannelMessage";
import ChannelMessage from "./ChannelMessage";
import { followedChannel, noteUntrackedRead, setFollowedChannel } from "./channelReading";
import ThreadPanel from "./ThreadPanel";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Map channel type to a Badge variant. */
function channelTypeBadgeVariant(type: string): "primary" | "info" | "default" {
  switch (type) {
    case "project":
      return "primary";
    case "bot":
      return "info";
    default:
      return "default";
  }
}

/** Delay before the newest shown message is marked read (bursts mark once). */
const MARK_READ_DELAY_MS = 1000;

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function ChannelView() {
  onMount(() => {
    document.title = "Channel - CodeForge";
  });
  const params = useParams<{ id: string }>();
  const { hasRole } = useAuth();
  const { confirm } = useConfirm();

  let messagesEndRef: HTMLDivElement | undefined;
  const [sending, setSending] = createSignal(false);
  const [threadParent, setThreadParent] = createSignal<ChannelMessageData | null>(null);
  const [webhookKey, setWebhookKey] = createSignal<string | null>(null);
  const [webhookError, setWebhookError] = createSignal<string | null>(null);

  // Fetch channel details
  const [channel, { refetch: refetchChannel }] = createResource(
    () => params.id,
    (id) => api.channels.get(id),
  );

  // Live messages that arrive while the list is (re)loading would be replaced
  // by the fetch result; they are kept here and merged into it.
  let arrivedWhileLoading: ChannelMessageRecord[] = [];

  // Fetch messages (newest first)
  const [messages, { mutate: mutateMessages }] = createResource(
    () => params.id,
    async (id) => {
      arrivedWhileLoading = [];
      const list = await api.channels.messages(id);
      const merged = arrivedWhileLoading
        .filter((m) => m.channel_id === id)
        .reduce((acc, m) => addMessage(acc, m, "start"), list);
      arrivedWhileLoading = [];
      scheduleMarkRead();
      return merged;
    },
  );

  // Mark the newest message read once the reader has seen it: after loading
  // and after live messages while the reader follows the conversation.
  let markTimer: ReturnType<typeof setTimeout> | undefined;
  let lastMarked = "";
  function scheduleMarkRead(): void {
    if (markTimer !== undefined) clearTimeout(markTimer);
    markTimer = setTimeout(() => {
      markTimer = undefined;
      const newest = messages()?.[0];
      if (!newest || newest.id === lastMarked) return;
      lastMarked = newest.id;
      // A failed mark only leaves the channel's unread count as it is.
      api.channels
        .markRead(params.id, newest.id)
        .then((state) => {
          if (state === undefined) noteUntrackedRead();
        })
        .catch(() => {
          lastMarked = "";
        });
    }, MARK_READ_DELAY_MS);
  }
  onCleanup(() => {
    if (markTimer !== undefined) clearTimeout(markTimer);
  });

  // The reader follows the channel while the list is scrolled to the bottom:
  // new messages are then seen and marked read, and the channel list does not
  // count them. Scrolled up, they stay unread until the reader reaches the
  // bottom again.
  createEffect(() => setFollowedChannel(params.id));
  onCleanup(() => {
    if (followedChannel() === params.id) setFollowedChannel(undefined);
  });
  function handleScroll(): void {
    const following = followedChannel() === params.id;
    if (isNearBottom() === following) return;
    setFollowedChannel(following ? undefined : params.id);
    if (!following) scheduleMarkRead();
  }

  // Messages posted by other users, agents and webhooks arrive as channel.message.
  const { onMessage } = useWebSocket();
  const unsubscribe = onMessage((msg) => {
    const incoming = parseChannelMessageEvent(msg);
    if (!incoming || incoming.channel_id !== params.id) return;
    if (messages.loading) {
      arrivedWhileLoading.push(incoming);
      return;
    }
    const follow = followedChannel() === params.id;
    mutateMessages((prev) => addMessage(prev, incoming, "start"));
    // Keep following the conversation, but do not pull a reader of older
    // history back to the bottom.
    if (follow) {
      setTimeout(scrollToBottom, 50);
      scheduleMarkRead();
    }
  });
  onCleanup(unsubscribe);

  let listRef: HTMLDivElement | undefined;

  function isNearBottom(): boolean {
    if (!listRef) return true;
    return listRef.scrollHeight - listRef.scrollTop - listRef.clientHeight < 120;
  }

  /** Scroll the message list to the bottom. */
  function scrollToBottom(): void {
    messagesEndRef?.scrollIntoView({ behavior: "smooth" });
  }

  // Scroll to bottom when messages load
  onMount(() => {
    // Defer to let DOM render
    setTimeout(scrollToBottom, 50);
  });

  /** Chronologically ordered top-level messages (API returns newest-first); replies open in the thread panel. */
  function orderedMessages(): ChannelMessageData[] {
    const raw = messages();
    if (!raw) return [];
    return raw.filter((m) => !m.parent_id).reverse();
  }

  const threadReplies = () => replyCounts(messages() ?? []);

  function openThread(messageId: string): void {
    const parent = messages()?.find((m) => m.id === messageId);
    if (parent) setThreadParent(parent);
  }

  async function handleSend(content: string): Promise<void> {
    if (sending()) return;
    setSending(true);
    try {
      const sent = await api.channels.send(params.id, content, "User");
      mutateMessages((prev) => addMessage(prev, sent, "start"));
      // Scroll after new message renders
      setTimeout(scrollToBottom, 50);
    } finally {
      setSending(false);
    }
  }

  async function handleWebhookKey(): Promise<void> {
    if (channel()?.has_webhook_key === true) {
      const ok = await confirm({
        title: "Replace webhook key",
        message: "The current key stops working immediately.",
        variant: "danger",
        confirmLabel: "Replace",
      });
      if (!ok) return;
    }
    setWebhookError(null);
    try {
      const { webhook_key: key } = await api.channels.regenerateWebhookKey(params.id);
      setWebhookKey(key);
      void refetchChannel();
    } catch {
      setWebhookError("The webhook key could not be generated.");
    }
  }

  return (
    <div class="flex h-full flex-col">
      {/* Header */}
      <div class="flex items-center gap-3 border-b border-cf-border bg-cf-bg-surface px-4 py-3">
        <Show
          when={channel()}
          fallback={<span class="text-sm text-cf-text-muted">Loading channel...</span>}
        >
          {(ch) => (
            <>
              <h2 class="text-lg font-semibold text-cf-text-primary"># {ch().name}</h2>
              <Badge variant={channelTypeBadgeVariant(ch().type)}>{ch().type}</Badge>
              <Show when={ch().description}>
                <span class="text-sm text-cf-text-muted">&mdash; {ch().description}</span>
              </Show>
              <Show when={hasRole("admin")}>
                <Button
                  variant="ghost"
                  size="xs"
                  class="ml-auto"
                  onClick={() => void handleWebhookKey()}
                >
                  {ch().has_webhook_key ? "Replace webhook key" : "Create webhook key"}
                </Button>
              </Show>
            </>
          )}
        </Show>
      </div>

      <Show when={webhookKey()}>
        {(key) => (
          <div class="border-b border-cf-border px-4 py-3">
            <Alert variant="warning">
              <p class="text-sm">
                Webhook key (shown only now; store it in the sending system). Post messages with the
                header <code>X-Webhook-Key</code> to{" "}
                <code>/api/v1/webhooks/channels/{params.id}</code>.
              </p>
              <p class="mt-2 select-all break-all font-mono text-xs" data-testid="webhook-key">
                {key()}
              </p>
              <Button variant="ghost" size="xs" class="mt-2" onClick={() => setWebhookKey(null)}>
                Done
              </Button>
            </Alert>
          </div>
        )}
      </Show>
      <Show when={webhookError()}>
        {(message) => (
          <div class="border-b border-cf-border px-4 py-3">
            <Alert variant="error">{message()}</Alert>
          </div>
        )}
      </Show>

      {/* Message list */}
      <div
        ref={listRef}
        class="flex-1 overflow-y-auto"
        data-testid="channel-messages"
        onScroll={handleScroll}
      >
        <Show
          when={!messages.loading}
          fallback={
            <div class="flex items-center justify-center py-12">
              <span class="text-sm text-cf-text-muted">Loading messages...</span>
            </div>
          }
        >
          <Show
            when={orderedMessages().length > 0}
            fallback={
              <div class="flex items-center justify-center py-12">
                <span class="text-sm text-cf-text-muted">
                  No messages yet. Start the conversation!
                </span>
              </div>
            }
          >
            <ul class="list-none m-0 p-0 py-2">
              <For each={orderedMessages()}>
                {(msg) => (
                  <li>
                    <ChannelMessage
                      message={msg}
                      replyCount={threadReplies().get(msg.id) ?? 0}
                      onThreadClick={openThread}
                    />
                  </li>
                )}
              </For>
            </ul>
          </Show>
        </Show>
        {/* Scroll anchor */}
        <div ref={messagesEndRef} />
      </div>

      {/* Input */}
      <ChannelInput
        onSend={(content) => void handleSend(content)}
        placeholder={channel() ? `Message #${channel()?.name ?? ""}` : "Type a message..."}
      />

      <Show when={threadParent()}>
        {(parent) => (
          <ThreadPanel
            channelId={params.id}
            parentMessage={parent()}
            visible={true}
            onClose={() => setThreadParent(null)}
          />
        )}
      </Show>
    </div>
  );
}
