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
import type { Conversation, ConversationMessage } from "~/api/types";
import { createCodeForgeWS } from "~/api/websocket";
import { useI18n } from "~/i18n";
import { Badge, Button } from "~/ui";

import Markdown from "./Markdown";
import ToolCallCard from "./ToolCallCard";

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

  // Tool call tracking from AG-UI events
  interface ToolCallState {
    callId: string;
    name: string;
    args?: Record<string, unknown>;
    result?: string;
    status: "pending" | "running" | "completed" | "failed";
  }
  const [toolCalls, setToolCalls] = createSignal<ToolCallState[]>([]);

  // Plan step tracking from AG-UI events
  interface PlanStepState {
    stepId: string;
    name: string;
    status: "running" | "completed" | "failed" | "cancelled" | "skipped";
  }
  const [planSteps, setPlanSteps] = createSignal<PlanStepState[]>([]);

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

  // Auto-create conversation on mount if none exists
  createEffect(() => {
    const convList = conversations();
    if (convList === undefined) return; // still loading
    if (activeConversation()) return; // already have one

    if (convList.length > 0) {
      // Select the first existing conversation
      setActiveConversation(convList[0].id);
    } else {
      // Create a new conversation automatically
      void (async () => {
        try {
          const conv: Conversation = await api.conversations.create(props.projectId, {
            title: t("chat.newConversation"),
          });
          await refetchConversations();
          setActiveConversation(conv.id);
        } catch {
          // toast handled by API layer
        }
      })();
    }
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
      setStreamingContent((prev) => prev + content);
      scrollToBottom();
    }
  });

  // When a tool call starts, add it to the tool calls list
  const cleanupToolCall = onAGUIEvent("agui.tool_call", (payload) => {
    const runId = payload.run_id as string;
    if (runId === activeConversation()) {
      const callId = payload.call_id as string;
      let args: Record<string, unknown> | undefined;
      try {
        args = JSON.parse(payload.args as string) as Record<string, unknown>;
      } catch {
        // args may not be valid JSON
      }
      setToolCalls((prev) => [
        ...prev,
        { callId, name: payload.name as string, args, status: "running" },
      ]);
      scrollToBottom();
    }
  });

  // When a tool result arrives, update the corresponding tool call
  const cleanupToolResult = onAGUIEvent("agui.tool_result", (payload) => {
    const runId = payload.run_id as string;
    if (runId === activeConversation()) {
      const callId = payload.call_id as string;
      const error = payload.error as string | undefined;
      setToolCalls((prev) =>
        prev.map((tc) =>
          tc.callId === callId
            ? { ...tc, result: payload.result as string, status: error ? "failed" : "completed" }
            : tc,
        ),
      );
    }
  });

  // When a run finishes, clear streaming state and refetch persisted messages
  const cleanupRunFinished = onAGUIEvent("agui.run_finished", (payload) => {
    const runId = payload.run_id as string;
    if (runId === activeConversation()) {
      setAgentRunning(false);
      setStreamingContent("");
      setToolCalls([]);
      setPlanSteps([]);
      void refetchMessages();
    }
  });

  // When a plan step starts, add it to the step tracker
  const cleanupStepStarted = onAGUIEvent("agui.step_started", (payload) => {
    const stepId = payload.step_id as string;
    const name = payload.name as string;
    setPlanSteps((prev) => [...prev, { stepId, name, status: "running" }]);
  });

  // When a plan step finishes, update its status
  const cleanupStepFinished = onAGUIEvent("agui.step_finished", (payload) => {
    const stepId = payload.step_id as string;
    const status = payload.status as PlanStepState["status"];
    setPlanSteps((prev) => prev.map((s) => (s.stepId === stepId ? { ...s, status } : s)));
  });

  onCleanup(() => {
    cleanupRunStarted();
    cleanupTextMessage();
    cleanupToolCall();
    cleanupToolResult();
    cleanupRunFinished();
    cleanupStepStarted();
    cleanupStepFinished();
  });

  // --- Handlers ---

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

  function stepBadgeVariant(status: string): "info" | "success" | "danger" {
    if (status === "running") return "info";
    if (status === "completed") return "success";
    return "danger";
  }

  return (
    <div class="flex flex-col h-full bg-cf-bg-surface">
      <Show
        when={activeConversation()}
        fallback={
          <div class="flex-1 flex items-center justify-center text-cf-text-muted">
            <p>{t("common.loading")}</p>
          </div>
        }
      >
        {/* Messages */}
        <div class="flex-1 overflow-y-auto p-4 space-y-4">
          <For each={messages() ?? []}>
            {(msg) => (
              <div class={`flex ${msg.role === "user" ? "justify-end" : "justify-start"}`}>
                <div
                  class={`max-w-[75%] rounded-cf-md px-4 py-2 text-sm ${
                    msg.role === "user"
                      ? "bg-cf-accent text-white whitespace-pre-wrap"
                      : "bg-cf-bg-surface-alt text-cf-text-primary"
                  }`}
                >
                  <Show when={msg.role === "assistant"} fallback={msg.content}>
                    <Markdown content={msg.content} />
                  </Show>
                  <Show when={msg.model}>
                    <div class="mt-1 text-xs opacity-60">{msg.model}</div>
                  </Show>
                </div>
              </div>
            )}
          </For>

          {/* Plan step status badges from AG-UI events */}
          <Show when={planSteps().length > 0}>
            <div class="flex flex-wrap gap-2 px-1">
              <For each={planSteps()}>
                {(step) => (
                  <Badge variant={stepBadgeVariant(step.status)} pill>
                    {step.name}
                  </Badge>
                )}
              </For>
            </div>
          </Show>

          {/* Active tool calls from AG-UI events */}
          <Show when={toolCalls().length > 0}>
            <div class="flex justify-start">
              <div class="max-w-[75%] w-full">
                <For each={toolCalls()}>
                  {(tc) => (
                    <ToolCallCard
                      name={tc.name}
                      args={tc.args}
                      result={tc.result}
                      status={tc.status}
                    />
                  )}
                </For>
              </div>
            </div>
          </Show>

          {/* Streaming assistant message from AG-UI text_message events */}
          <Show when={streamingContent()}>
            {(content) => (
              <div class="flex justify-start">
                <div class="max-w-[75%] rounded-cf-md px-4 py-2 text-sm bg-cf-bg-surface-alt text-cf-text-primary">
                  <Markdown content={content()} />
                  <div class="mt-1 text-xs opacity-60">{t("chat.streaming")}</div>
                </div>
              </div>
            )}
          </Show>

          {/* Thinking indicator: shown when agent run is active but no text has streamed yet */}
          <Show when={(sending() || agentRunning()) && !streamingContent()}>
            <div class="flex justify-start">
              <div class="bg-cf-bg-surface-alt rounded-cf-md px-4 py-2 text-sm text-cf-text-tertiary animate-pulse">
                {t("chat.thinking")}
              </div>
            </div>
          </Show>
          <div ref={messagesEndRef} />
        </div>

        {/* Input area */}
        <div class="border-t border-cf-border p-3 flex-shrink-0">
          <div class="flex gap-2">
            <textarea
              class="flex-1 rounded-cf-md border border-cf-border bg-cf-bg-surface px-3 py-2 text-sm text-cf-text-primary placeholder-cf-text-muted focus:border-cf-accent focus:ring-1 focus:ring-cf-accent resize-none"
              rows={2}
              placeholder={t("chat.placeholder")}
              value={input()}
              onInput={(e) => setInput(e.currentTarget.value)}
              onKeyDown={handleKeyDown}
              disabled={sending()}
            />
            <Button
              variant="primary"
              size="sm"
              class="self-end"
              onClick={handleSend}
              disabled={sending() || !input().trim()}
            >
              {t("chat.send")}
            </Button>
          </div>
        </div>
      </Show>
    </div>
  );
}
