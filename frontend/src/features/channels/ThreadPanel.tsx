import {
  createEffect,
  createResource,
  createSignal,
  For,
  type JSX,
  onCleanup,
  Show,
} from "solid-js";
import { Portal } from "solid-js/web";

import { api } from "~/api/client";
import { useAuth } from "~/components/AuthProvider";
import { useWebSocket } from "~/components/WebSocketProvider";
import { Backdrop, Button, Spinner } from "~/ui";

import { addMessage, parseChannelMessageEvent } from "./channelEvents";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Shape of the parent message passed in by the caller. */
export interface ThreadParentMessage {
  id: string;
  sender_type: string;
  sender_name: string;
  content: string;
  created_at: string;
}

export interface ThreadPanelProps {
  channelId: string;
  parentMessage: ThreadParentMessage;
  visible: boolean;
  onClose: () => void;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString(undefined, {
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
}

function senderInitial(name: string): string {
  return name.charAt(0).toUpperCase() || "?";
}

// ---------------------------------------------------------------------------
// ThreadPanel
// ---------------------------------------------------------------------------

export default function ThreadPanel(props: ThreadPanelProps): JSX.Element {
  const [replyText, setReplyText] = createSignal("");
  const [sending, setSending] = createSignal(false);
  const [sendError, setSendError] = createSignal("");
  // Replies need the editor or admin role; viewers read the thread only.
  const { hasRole } = useAuth();
  const canReply = () => hasRole("admin", "editor");

  // Fetch thread replies — re-fetches whenever the parent message id changes.
  // The API lists newest first; a thread reads oldest first.
  const [replies, { mutate }] = createResource(
    () => (props.visible ? { parentId: props.parentMessage.id, channelId: props.channelId } : null),
    async (source) => {
      const allMessages = await api.channels.messages(source.channelId);
      return allMessages.filter((m) => m.parent_id === source.parentId).reverse();
    },
  );

  // Replies from other users, agents and webhooks arrive as channel.message;
  // listen while the panel is open, for the thread it shows.
  const { onMessage } = useWebSocket();
  createEffect(() => {
    if (!props.visible) return;
    const channelId = props.channelId;
    const parentId = props.parentMessage.id;
    const unsubscribe = onMessage((msg) => {
      const incoming = parseChannelMessageEvent(msg);
      if (incoming?.channel_id !== channelId || incoming.parent_id !== parentId) return;
      mutate((prev) => addMessage(prev, incoming, "end"));
    });
    onCleanup(unsubscribe);
  });

  // Close on Escape key
  createEffect(() => {
    if (!props.visible) return;

    function handleKeyDown(e: KeyboardEvent): void {
      if (e.key === "Escape") {
        e.stopPropagation();
        props.onClose();
      }
    }

    document.addEventListener("keydown", handleKeyDown);
    onCleanup(() => document.removeEventListener("keydown", handleKeyDown));
  });

  async function handleSend(): Promise<void> {
    const content = replyText().trim();
    if (!content || sending()) return;

    setSending(true);
    setSendError("");
    try {
      const reply = await api.channels.sendThreadReply(props.channelId, props.parentMessage.id, {
        sender_name: "You",
        sender_type: "user",
        content,
      });
      setReplyText("");
      mutate((prev) => addMessage(prev, reply, "end"));
    } catch (err) {
      // The reply text stays in the input, so it can be sent again.
      setSendError(err instanceof Error ? err.message : String(err));
    } finally {
      setSending(false);
    }
  }

  function handleInputKeyDown(e: KeyboardEvent): void {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      void handleSend();
    }
  }

  return (
    <Show when={props.visible}>
      <Portal>
        {/* Backdrop */}
        <Backdrop onClick={() => props.onClose()} />

        {/* Slide-over panel */}
        <div class="fixed inset-y-0 right-0 z-50 flex w-96 max-w-full flex-col border-l border-cf-border bg-cf-bg-surface shadow-cf-lg">
          {/* Header */}
          <div class="flex items-center justify-between border-b border-cf-border px-4 py-3">
            <h2 class="text-sm font-semibold text-cf-text-primary">Thread</h2>
            <Button
              variant="icon"
              size="xs"
              onClick={() => props.onClose()}
              aria-label="Close thread panel"
            >
              {"\u2715"}
            </Button>
          </div>

          {/* Parent message (highlighted) */}
          <div class="border-b border-cf-border bg-cf-bg-surface-alt px-4 py-3">
            <div class="flex items-center gap-2">
              <span class="flex h-6 w-6 items-center justify-center rounded-full bg-cf-accent text-xs font-bold text-cf-accent-fg">
                {senderInitial(props.parentMessage.sender_name)}
              </span>
              <span class="text-sm font-medium text-cf-text-primary">
                {props.parentMessage.sender_name}
              </span>
              <span class="text-xs text-cf-text-muted">
                {formatTime(props.parentMessage.created_at)}
              </span>
            </div>
            <p class="mt-1 whitespace-pre-wrap text-sm text-cf-text-secondary">
              {props.parentMessage.content}
            </p>
          </div>

          {/* Thread replies list */}
          <div class="flex-1 overflow-y-auto px-4 py-2">
            <Show when={replies.loading}>
              <div class="flex justify-center py-4">
                <Spinner size="sm" />
              </div>
            </Show>

            <Show when={!replies.loading && replies()}>
              {(replyList) => (
                <Show
                  when={replyList().length > 0}
                  fallback={
                    <p class="py-4 text-center text-xs text-cf-text-muted">No replies yet</p>
                  }
                >
                  <div class="space-y-3">
                    <For each={replyList()}>
                      {(reply) => (
                        <div class="flex gap-2">
                          <span class="mt-0.5 flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-cf-bg-tertiary text-xs font-bold text-cf-text-secondary">
                            {senderInitial(reply.sender_name)}
                          </span>
                          <div class="min-w-0 flex-1">
                            <div class="flex items-baseline gap-2">
                              <span class="text-sm font-medium text-cf-text-primary">
                                {reply.sender_name}
                              </span>
                              <span class="text-xs text-cf-text-muted">
                                {formatTime(reply.created_at)}
                              </span>
                            </div>
                            <p class="mt-0.5 whitespace-pre-wrap text-sm text-cf-text-secondary">
                              {reply.content}
                            </p>
                          </div>
                        </div>
                      )}
                    </For>
                  </div>
                </Show>
              )}
            </Show>
          </div>

          {/* Reply input */}
          <div class="border-t border-cf-border px-4 py-3">
            <Show when={sendError()}>
              {(message) => (
                <p class="mb-2 text-sm text-cf-danger-fg" role="alert">
                  The reply was not sent: {message()}
                </p>
              )}
            </Show>
            <Show
              when={canReply()}
              fallback={
                <p class="text-xs text-cf-text-muted">Only editors and admins can reply.</p>
              }
            >
              <div class="flex gap-2">
                <input
                  type="text"
                  placeholder="Reply..."
                  value={replyText()}
                  onInput={(e) => setReplyText(e.currentTarget.value)}
                  onKeyDown={handleInputKeyDown}
                  disabled={sending()}
                  class="block flex-1 rounded-cf-md border border-cf-border-input bg-cf-bg-surface px-3 py-2 text-sm text-cf-text-primary placeholder:text-cf-text-muted transition-colors focus:border-cf-accent focus:outline-none focus:ring-2 focus:ring-cf-focus-ring"
                />
                <Button
                  variant="primary"
                  size="xs"
                  disabled={replyText().trim() === "" || sending()}
                  loading={sending()}
                  onClick={() => void handleSend()}
                >
                  Send
                </Button>
              </div>
            </Show>
          </div>
        </div>
      </Portal>
    </Show>
  );
}
