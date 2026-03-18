import { useParams } from "@solidjs/router";
import { createResource, createSignal, For, onMount, Show } from "solid-js";

import { api } from "~/api/client";
import { Badge } from "~/ui";

import ChannelInput from "./ChannelInput";
import type { ChannelMessageData } from "./ChannelMessage";
import ChannelMessage from "./ChannelMessage";

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

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function ChannelView() {
  onMount(() => {
    document.title = "Channel - CodeForge";
  });
  const params = useParams<{ id: string }>();

  let messagesEndRef: HTMLDivElement | undefined;
  const [sending, setSending] = createSignal(false);

  // Fetch channel details
  const [channel] = createResource(
    () => params.id,
    (id) => api.channels.get(id),
  );

  // Fetch messages
  const [messages, { refetch: refetchMessages }] = createResource(
    () => params.id,
    (id) => api.channels.messages(id),
  );

  /** Scroll the message list to the bottom. */
  function scrollToBottom(): void {
    messagesEndRef?.scrollIntoView({ behavior: "smooth" });
  }

  // Scroll to bottom when messages load
  onMount(() => {
    // Defer to let DOM render
    setTimeout(scrollToBottom, 50);
  });

  /** Chronologically ordered messages (API returns newest-first). */
  function orderedMessages(): ChannelMessageData[] {
    const raw = messages();
    if (!raw) return [];
    return [...raw].reverse();
  }

  async function handleSend(content: string): Promise<void> {
    if (sending()) return;
    setSending(true);
    try {
      await api.channels.send(params.id, content, "User");
      await refetchMessages();
      // Scroll after new message renders
      setTimeout(scrollToBottom, 50);
    } finally {
      setSending(false);
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
            </>
          )}
        </Show>
      </div>

      {/* Message list */}
      <div class="flex-1 overflow-y-auto">
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
                    <ChannelMessage message={msg} />
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
    </div>
  );
}
