import { createResource, createSignal, For, onCleanup, onMount, Show } from "solid-js";

import { api } from "~/api/client";
import type { Conversation, ConversationMessage } from "~/api/types";
import { createCodeForgeWS } from "~/api/websocket";
import { useI18n } from "~/i18n";

interface ChatPanelProps {
  projectId: string;
}

export default function ChatPanel(props: ChatPanelProps) {
  const { t } = useI18n();
  const { onAGUIEvent } = createCodeForgeWS();

  const [activeConversation, setActiveConversation] = createSignal<string | null>(null);
  const [conversations, { refetch: refetchConversations }] = createResource(
    () => props.projectId,
    (pid) => api.conversations.list(pid),
  );
  const [messages, { refetch: refetchMessages }] = createResource(activeConversation, (cid) =>
    cid ? api.conversations.messages(cid) : Promise.resolve([] as ConversationMessage[]),
  );
  const [input, setInput] = createSignal("");
  const [sending, setSending] = createSignal(false);

  // Streaming text from AG-UI text_message events, appended to the bottom of the chat
  const [streamingContent, setStreamingContent] = createSignal("");
  // Track whether the assistant is actively processing via run_started / run_finished
  const [agentRunning, setAgentRunning] = createSignal(false);

  let messagesEndRef: HTMLDivElement | undefined;

  const scrollToBottom = () => {
    messagesEndRef?.scrollIntoView({ behavior: "smooth" });
  };

  // Auto-scroll when messages change
  const trackMessages = () => {
    messages();
    scrollToBottom();
  };

  onMount(() => {
    trackMessages();
  });

  // --- AG-UI event subscriptions ---

  // When a run starts for the active conversation, show the thinking indicator
  const cleanupRunStarted = onAGUIEvent("agui.run_started", (payload) => {
    const runId = payload.run_id as string;
    if (runId === activeConversation()) {
      setAgentRunning(true);
      setStreamingContent("");
    }
  });

  // When a text_message arrives for the active conversation, update streaming content
  const cleanupTextMessage = onAGUIEvent("agui.text_message", (payload) => {
    const runId = payload.run_id as string;
    if (runId === activeConversation()) {
      const content = payload.content as string;
      setStreamingContent(content);
      scrollToBottom();
    }
  });

  // When a run finishes, clear streaming state and refetch persisted messages
  const cleanupRunFinished = onAGUIEvent("agui.run_finished", (payload) => {
    const runId = payload.run_id as string;
    if (runId === activeConversation()) {
      setAgentRunning(false);
      setStreamingContent("");
      void refetchMessages();
    }
  });

  onCleanup(() => {
    cleanupRunStarted();
    cleanupTextMessage();
    cleanupRunFinished();
  });

  // --- Handlers ---

  const handleNewConversation = async () => {
    try {
      const conv: Conversation = await api.conversations.create(props.projectId, {
        title: t("chat.newConversation"),
      });
      await refetchConversations();
      setActiveConversation(conv.id);
    } catch {
      // toast handled by API layer
    }
  };

  const handleDeleteConversation = async (id: string) => {
    try {
      await api.conversations.delete(id);
      if (activeConversation() === id) {
        setActiveConversation(null);
      }
      await refetchConversations();
    } catch {
      // toast handled by API layer
    }
  };

  const handleSend = async () => {
    const content = input().trim();
    if (!content || !activeConversation() || sending()) return;

    setInput("");
    setSending(true);
    try {
      const convId = activeConversation();
      if (!convId) return;
      await api.conversations.send(convId, { content });
      await refetchMessages();
      scrollToBottom();
    } catch {
      // toast handled by API layer
    } finally {
      setSending(false);
    }
  };

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  return (
    <div class="flex h-[600px] border border-gray-200 dark:border-gray-700 rounded-lg overflow-hidden">
      {/* Sidebar - Conversation list */}
      <div class="w-64 border-r border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 flex flex-col">
        <div class="p-3 border-b border-gray-200 dark:border-gray-700">
          <button
            type="button"
            class="w-full rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
            onClick={handleNewConversation}
          >
            {t("chat.new")}
          </button>
        </div>
        <div class="flex-1 overflow-y-auto">
          <Show
            when={!conversations.loading}
            fallback={<p class="p-3 text-sm text-gray-500">{t("common.loading")}</p>}
          >
            <For
              each={conversations() ?? []}
              fallback={
                <p class="p-3 text-sm text-gray-400 dark:text-gray-500">
                  {t("chat.noConversations")}
                </p>
              }
            >
              {(conv) => (
                <div
                  class={`flex items-center justify-between px-3 py-2 cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-800 ${
                    activeConversation() === conv.id
                      ? "bg-blue-50 dark:bg-blue-900/30 border-l-2 border-blue-600"
                      : ""
                  }`}
                  onClick={() => setActiveConversation(conv.id)}
                >
                  <span class="text-sm truncate text-gray-700 dark:text-gray-300">
                    {conv.title}
                  </span>
                  <button
                    type="button"
                    class="ml-1 text-gray-400 hover:text-red-500 text-xs flex-shrink-0"
                    onClick={(e) => {
                      e.stopPropagation();
                      handleDeleteConversation(conv.id);
                    }}
                    aria-label={t("chat.deleteAria")}
                  >
                    ×
                  </button>
                </div>
              )}
            </For>
          </Show>
        </div>
      </div>

      {/* Main chat area */}
      <div class="flex-1 flex flex-col bg-white dark:bg-gray-800">
        <Show
          when={activeConversation()}
          fallback={
            <div class="flex-1 flex items-center justify-center text-gray-400 dark:text-gray-500">
              <p>{t("chat.selectOrNew")}</p>
            </div>
          }
        >
          {/* Messages */}
          <div class="flex-1 overflow-y-auto p-4 space-y-4">
            <For each={messages() ?? []}>
              {(msg) => (
                <div class={`flex ${msg.role === "user" ? "justify-end" : "justify-start"}`}>
                  <div
                    class={`max-w-[75%] rounded-lg px-4 py-2 text-sm whitespace-pre-wrap ${
                      msg.role === "user"
                        ? "bg-blue-600 text-white"
                        : "bg-gray-100 dark:bg-gray-700 text-gray-900 dark:text-gray-100"
                    }`}
                  >
                    {msg.content}
                    <Show when={msg.model}>
                      <div class="mt-1 text-xs opacity-60">{msg.model}</div>
                    </Show>
                  </div>
                </div>
              )}
            </For>

            {/* Streaming assistant message from AG-UI text_message events */}
            <Show when={streamingContent()}>
              <div class="flex justify-start">
                <div class="max-w-[75%] rounded-lg px-4 py-2 text-sm whitespace-pre-wrap bg-gray-100 dark:bg-gray-700 text-gray-900 dark:text-gray-100">
                  {streamingContent()}
                  <div class="mt-1 text-xs opacity-60">{t("chat.streaming")}</div>
                </div>
              </div>
            </Show>

            {/* Thinking indicator: shown when agent run is active but no text has streamed yet */}
            <Show when={(sending() || agentRunning()) && !streamingContent()}>
              <div class="flex justify-start">
                <div class="bg-gray-100 dark:bg-gray-700 rounded-lg px-4 py-2 text-sm text-gray-500 dark:text-gray-400 animate-pulse">
                  {t("chat.thinking")}
                </div>
              </div>
            </Show>
            <div ref={messagesEndRef} />
          </div>

          {/* Input area */}
          <div class="border-t border-gray-200 dark:border-gray-700 p-3">
            <div class="flex gap-2">
              <textarea
                class="flex-1 rounded-md border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 px-3 py-2 text-sm text-gray-900 dark:text-gray-100 placeholder-gray-400 dark:placeholder-gray-500 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 resize-none"
                rows={2}
                placeholder={t("chat.placeholder")}
                value={input()}
                onInput={(e) => setInput(e.currentTarget.value)}
                onKeyDown={handleKeyDown}
                disabled={sending()}
              />
              <button
                type="button"
                class="self-end rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600 disabled:opacity-50 disabled:cursor-not-allowed"
                onClick={handleSend}
                disabled={sending() || !input().trim()}
              >
                {t("chat.send")}
              </button>
            </div>
          </div>
        </Show>
      </div>
    </div>
  );
}
