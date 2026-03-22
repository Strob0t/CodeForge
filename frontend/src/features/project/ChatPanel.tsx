// TODO: FIX-106: Inline SVG icons are duplicated across ChatPanel and other
// components. Extract shared SVG icons into a reusable icon component library
// (e.g., frontend/src/ui/icons/).

import {
  createEffect,
  createMemo,
  createResource,
  createSignal,
  For,
  onMount,
  Show,
} from "solid-js";

import { api } from "~/api/client";
import type {
  AgentConfig,
  Conversation,
  ConversationMessage,
  MessageImage,
  Session,
} from "~/api/types";
import { useConversationRuns } from "~/components/ConversationRunProvider";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import type { TranslationKey } from "~/i18n/en";
import { Badge, Button, CostDisplay, StreamingCursor, TypingIndicator } from "~/ui";

import { buildCanvasPrompt, modelSupportsVision } from "../canvas/buildCanvasPrompt";
import { CanvasModal } from "../canvas/CanvasModal";
import type { CanvasExports } from "../canvas/canvasTypes";
import ChatInput from "../chat/ChatInput";
import { type CommandContext, executeCommand } from "../chat/commandExecutor";
import TokenBadge from "../chat/TokenBadge";
import ActionBar from "./ActionBar";
import ChatSuggestions from "./ChatSuggestions";
import { clearContextFiles, contextFiles, removeContextFile } from "./contextFilesStore";
import GoalProposalCard from "./GoalProposalCard";
import Markdown from "./Markdown";
import MessageBadge from "./MessageBadge";
import PermissionRequestCard from "./PermissionRequestCard";
import SessionFooter from "./SessionFooter";
import SessionPanel from "./SessionPanel";
import ToolCallCard from "./ToolCallCard";
import { useChatAGUI } from "./useChatAGUI";

interface ChatPanelProps {
  projectId: string;
  activeTab?: string;
}

export default function ChatPanel(props: ChatPanelProps) {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const { isRunActive } = useConversationRuns();

  const [forkLoading, setForkLoading] = createSignal(false);
  const [rewindLoading, setRewindLoading] = createSignal(false);
  const [resumeLoading, setResumeLoading] = createSignal(false);
  const [showSessionHistory, setShowSessionHistory] = createSignal(false);
  const [canvasOpen, setCanvasOpen] = createSignal(false);

  const [activeConversation, setActiveConversation] = createSignal<string | null>(null);
  const [conversations, { refetch: refetchConversations }] = createResource(
    () => props.projectId,
    (pid) => api.conversations.list(pid),
  );
  const [messages, { refetch: refetchMessages }] = createResource(activeConversation, (cid) =>
    cid ? api.conversations.messages(cid) : Promise.resolve([] as ConversationMessage[]),
  );
  // All sessions for this project — used to show status dots on conversation selector
  const [projectSessions] = createResource(
    () => props.projectId,
    (pid) => api.sessions.list(pid).catch(() => [] as Session[]),
  );
  const sessionByConv = () => {
    const map = new Map<string, Session>();
    for (const s of projectSessions() ?? []) {
      if (s.conversation_id) map.set(s.conversation_id, s);
    }
    return map;
  };
  const [session, { refetch: refetchSession }] = createResource(activeConversation, (cid) =>
    cid
      ? api.conversations.session(cid).catch(() => null as Session | null)
      : Promise.resolve(null as Session | null),
  );
  // Agent config (max_context_tokens etc.) — fetched once from backend.
  const DEFAULT_MAX_CONTEXT_TOKENS = 128_000;
  const [agentConfig] = createResource(
    () => true,
    () =>
      api.agentConfig
        .get()
        .catch(() => ({ max_context_tokens: DEFAULT_MAX_CONTEXT_TOKENS }) as AgentConfig),
  );
  const maxContextTokens = () => agentConfig()?.max_context_tokens ?? DEFAULT_MAX_CONTEXT_TOKENS;

  const [input, setInput] = createSignal("");
  const [sending, setSending] = createSignal(false);
  const [attaching, setAttaching] = createSignal(false);
  let chatFileInputRef: HTMLInputElement | undefined;

  function handleAttachChange(e: Event) {
    const fileInput = e.target as HTMLInputElement;
    const file = fileInput.files?.[0];
    if (!file) return;
    fileInput.value = "";
    const reader = new FileReader();
    reader.onload = async () => {
      const content = reader.result as string;
      setAttaching(true);
      try {
        await api.files.write(props.projectId, file.name, content);
        toast("success", t("chat.attachSuccess"));
        const ref = `[Attached: ${file.name}]\n`;
        setInput((prev) => ref + prev);
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        toast("error", t("chat.attachFailed") + ": " + msg);
      } finally {
        setAttaching(false);
      }
    };
    reader.onerror = () => {
      toast("error", t("chat.attachFailed"));
    };
    reader.readAsText(file);
  }

  let messagesContainerRef: HTMLDivElement | undefined;

  const scrollToBottom = () => {
    const el = messagesContainerRef;
    if (el) el.scrollTop = el.scrollHeight;
  };

  // AG-UI event subscriptions and streaming state
  const agui = useChatAGUI({
    activeConversation,
    scrollToBottom,
    refetchMessages: () => void refetchMessages(),
    refetchSession: () => void refetchSession(),
  });

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

  // Restore agentRunning state from global tracker when navigating back.
  createEffect(() => {
    const convId = activeConversation();
    if (convId && isRunActive(convId)) {
      agui.setAgentRunning(true);
    }
  });

  // Auto-onboarding greeting disabled — it dispatches an agentic conversation
  // that makes tool calls, blocking the NATS pipeline for other conversations.
  // Users can manually send a greeting or use the "AI Discover" button instead.

  // --- Handlers ---

  /** Send a chat message with explicit content (used by goal proposal callbacks). */
  const sendChatMessage = (content: string): void => {
    const convId = activeConversation();
    if (!convId || !content) return;
    void api.conversations
      .send(convId, { content })
      .then(() => refetchMessages())
      .then(() => scrollToBottom())
      .catch(() => {
        // toast handled by API layer
      });
  };

  const handleSend = async () => {
    const content = input().trim();
    if (!content || !activeConversation() || sending()) return;

    // Slash command interception: /command or //command
    if (content.startsWith("/")) {
      const stripped = content.startsWith("//") ? content.slice(2) : content.slice(1);
      const spaceIdx = stripped.indexOf(" ");
      const commandId = spaceIdx > 0 ? stripped.slice(0, spaceIdx) : stripped;
      const args = spaceIdx > 0 ? stripped.slice(spaceIdx + 1) : "";

      const convId = activeConversation();
      if (!convId) return;

      const ctx: CommandContext = {
        conversationId: convId,
        messages: (messages() ?? []).map((m) => ({ role: m.role, content: m.content })),
        sessionCostUsd: agui.sessionCostUsd(),
        sessionTokensIn: agui.sessionTokensIn(),
        sessionTokensOut: agui.sessionTokensOut(),
        sessionSteps: agui.sessionSteps(),
        sessionModel: agui.sessionModel(),
      };

      setInput("");
      try {
        const result = await executeCommand(commandId, args, ctx);
        switch (result.type) {
          case "display":
            agui.setCommandOutput(result.content ?? null);
            break;
          case "api_call":
            toast("success", result.content ?? "Done");
            await refetchMessages();
            scrollToBottom();
            break;
          case "modal":
            toast("info", `Action: ${result.action ?? "modal"}`);
            break;
        }
      } catch {
        toast("error", "Command failed");
      }
      return;
    }

    // Clear any previous command output
    agui.setCommandOutput(null);

    // Prepend context files as file references.
    const ctxPaths = contextFiles();
    const prefix = ctxPaths.length > 0 ? ctxPaths.map((p) => `@${p}`).join(" ") + "\n" : "";
    const fullContent = prefix + content;

    setInput("");
    setSending(true);
    agui.setRunError(null);
    try {
      const convId = activeConversation();
      if (!convId) return;
      // All paths now dispatch via NATS (202 Accepted).
      // Results stream via AG-UI WebSocket events; messages are
      // refetched when run_finished arrives.
      await api.conversations.send(convId, { content: fullContent });
      // User message is now persisted in DB — show it immediately.
      await refetchMessages();
      scrollToBottom();
      // Clear context files after sending.
      if (ctxPaths.length > 0) clearContextFiles();
    } catch {
      // toast handled by API layer
    } finally {
      setSending(false);
    }
  };

  const handleStop = () => {
    const convId = activeConversation();
    if (!convId) return;
    void api.conversations.stop(convId).catch(() => {
      // error handled by API layer
    });
  };

  function stepBadgeVariant(status: string): "info" | "success" | "danger" {
    if (status === "running") return "info";
    if (status === "completed") return "success";
    return "danger";
  }

  // Build tool result lookup from persisted tool messages for ToolCallCard rendering
  const toolResultMap = createMemo(() => {
    const map = new Map<string, string>();
    for (const msg of messages() ?? []) {
      if (msg.role === "tool" && msg.tool_call_id) {
        map.set(msg.tool_call_id, msg.content);
      }
    }
    return map;
  });

  return (
    <div class="flex flex-col flex-1 min-h-0 overflow-hidden bg-cf-bg-surface">
      <Show
        when={activeConversation()}
        fallback={
          <div class="flex-1 flex items-center justify-center text-cf-text-muted">
            <p>{t("common.loading")}</p>
          </div>
        }
      >
        {/* Chat header with agentic mode indicator */}
        <div class="flex items-center justify-between border-b border-cf-border px-4 py-2">
          <div class="flex items-center gap-2">
            <span class="text-sm font-medium text-cf-text-primary">{t("chat.tab")}</span>
            <button
              type="button"
              class="rounded p-0.5 text-cf-text-muted hover:text-cf-text-primary hover:bg-cf-bg-hover transition-colors"
              title="New Conversation"
              data-testid="new-conversation-btn"
              onClick={() => {
                void (async () => {
                  try {
                    const conv = await api.conversations.create(props.projectId, {
                      title: "New Chat",
                    });
                    await refetchConversations();
                    setActiveConversation(conv.id);
                  } catch {
                    toast("error", "Failed to create conversation");
                  }
                })();
              }}
            >
              <svg viewBox="0 0 20 20" fill="currentColor" class="w-4 h-4">
                <path d="M10.75 4.75a.75.75 0 0 0-1.5 0v4.5h-4.5a.75.75 0 0 0 0 1.5h4.5v4.5a.75.75 0 0 0 1.5 0v-4.5h4.5a.75.75 0 0 0 0-1.5h-4.5v-4.5Z" />
              </svg>
            </button>
            <Show when={agui.agentRunning()}>
              <span class="inline-flex items-center gap-1 rounded-full bg-cf-accent/10 px-2 py-0.5 text-xs font-medium text-cf-accent">
                <span class="inline-block h-1.5 w-1.5 rounded-full bg-cf-accent animate-pulse" />
                Agentic
              </span>
            </Show>
          </div>
          <div class="flex flex-wrap items-center gap-2 sm:gap-3">
            {/* Session status badge */}
            <Show when={session()}>
              {(sess) => (
                <Badge
                  variant={
                    sess().status === "active"
                      ? "success"
                      : sess().status === "forked"
                        ? "info"
                        : "default"
                  }
                  pill
                >
                  {t(("session.status." + sess().status) as TranslationKey)}
                </Badge>
              )}
            </Show>
            {/* Fork / Rewind buttons (only when not running) */}
            <Show when={!agui.agentRunning() && session()}>
              <Button
                variant="secondary"
                size="sm"
                class="text-xs px-2 py-0.5"
                disabled={forkLoading()}
                onClick={() => {
                  const convId = activeConversation();
                  if (!convId) return;
                  setForkLoading(true);
                  void api.conversations
                    .fork(convId)
                    .then(
                      () => {
                        void refetchSession();
                        toast("success", t("session.forkSuccess"));
                      },
                      () => {
                        toast("error", t("session.forkFailed"));
                      },
                    )
                    .finally(() => {
                      setForkLoading(false);
                    });
                }}
              >
                {forkLoading() ? "..." : t("session.fork")}
              </Button>
              <Button
                variant="secondary"
                size="sm"
                class="text-xs px-2 py-0.5"
                disabled={rewindLoading()}
                onClick={() => {
                  const convId = activeConversation();
                  if (!convId) return;
                  setRewindLoading(true);
                  void api.conversations
                    .rewind(convId)
                    .then(
                      () => {
                        void refetchSession();
                        void refetchMessages();
                        toast("success", t("session.rewindSuccess"));
                      },
                      () => {
                        toast("error", t("session.rewindFailed"));
                      },
                    )
                    .finally(() => {
                      setRewindLoading(false);
                    });
                }}
              >
                {rewindLoading() ? "..." : t("session.rewind")}
              </Button>
            </Show>
            {/* Resume button (paused/completed session with a run to resume) */}
            <Show
              when={
                !agui.agentRunning() &&
                session()?.current_run_id &&
                (session()?.status === "paused" || session()?.status === "completed")
              }
            >
              <Button
                variant="secondary"
                size="sm"
                class="text-xs px-2 py-0.5"
                disabled={resumeLoading()}
                onClick={() => {
                  const runId = session()?.current_run_id;
                  if (!runId) return;
                  setResumeLoading(true);
                  void api.runs
                    .resume(runId)
                    .then(
                      () => {
                        void refetchSession();
                        toast("success", t("session.resumeSuccess"));
                      },
                      () => {
                        toast("error", t("session.resumeFailed"));
                      },
                    )
                    .finally(() => {
                      setResumeLoading(false);
                    });
                }}
              >
                {resumeLoading() ? "..." : t("session.resume")}
              </Button>
            </Show>
            {/* Session History toggle */}
            <Show when={session()}>
              <Button
                variant="secondary"
                size="sm"
                class="text-xs px-2 py-0.5"
                onClick={() => setShowSessionHistory((v) => !v)}
              >
                {showSessionHistory() ? "\u25B2" : "\u25BC"} {t("session.title")}
              </Button>
            </Show>
            {/* Step counter during agentic turns */}
            <Show when={agui.agentRunning() && agui.stepCount() > 0}>
              <span class="text-xs text-cf-text-muted">Step {agui.stepCount()}</span>
            </Show>
            {/* Running cost during agentic turn */}
            <Show when={agui.agentRunning() && agui.runningCost() > 0}>
              <CostDisplay usd={agui.runningCost()} class="text-xs text-cf-text-muted" />
            </Show>
            {/* Stop button during active agentic runs */}
            <Show when={agui.agentRunning()}>
              <Button
                variant="primary"
                size="sm"
                class="bg-red-600 hover:bg-red-700 text-white text-xs px-2 py-0.5"
                onClick={handleStop}
              >
                {"\u25A0"} Stop
              </Button>
            </Show>
          </div>
        </div>

        {/* Conversation selector with session status dots */}
        <Show when={(conversations() ?? []).length > 1}>
          <div class="flex items-center gap-1 border-b border-cf-border px-3 py-1.5 overflow-x-auto scrollbar-none">
            <For each={conversations() ?? []}>
              {(conv) => {
                const convSession = () => sessionByConv().get(conv.id);
                const dotColor = () => {
                  const s = convSession();
                  if (!s) return "";
                  if (s.status === "active") return "bg-green-500";
                  if (s.status === "paused") return "bg-yellow-500";
                  return "bg-gray-400";
                };
                return (
                  <button
                    type="button"
                    class={`flex items-center gap-1.5 whitespace-nowrap rounded px-2 py-1 text-xs transition-colors ${
                      activeConversation() === conv.id
                        ? "bg-cf-accent/15 text-cf-accent font-medium"
                        : "text-cf-text-muted hover:text-cf-text-primary hover:bg-cf-bg-hover"
                    }`}
                    onClick={() => setActiveConversation(conv.id)}
                    title={convSession() ? `Session: ${convSession()?.status}` : undefined}
                  >
                    <Show when={convSession()}>
                      <span
                        class={`inline-block h-2 w-2 rounded-full flex-shrink-0 ${dotColor()}`}
                      />
                    </Show>
                    {conv.title || conv.id.slice(0, 8)}
                  </button>
                );
              }}
            </For>
          </div>
        </Show>

        {/* Inline Session History (collapsible) */}
        <Show when={showSessionHistory()}>
          <div class="border-b border-cf-border">
            <SessionPanel projectId={props.projectId} />
          </div>
        </Show>

        {/* Messages */}
        <div ref={messagesContainerRef} class="flex-1 overflow-y-auto p-4">
          <ul class="space-y-4 list-none m-0 p-0">
            <For
              each={(messages() ?? []).filter((msg) => {
                // Hide system and tool messages (tool results shown via ToolCallCards)
                if (msg.role === "system" || msg.role === "tool") return false;
                return true;
              })}
            >
              {(msg) => (
                <li class={`flex ${msg.role === "user" ? "justify-end" : "justify-start"}`}>
                  <div
                    class={`rounded-cf-md px-4 py-2 text-sm ${
                      msg.role === "user"
                        ? "max-w-[90%] sm:max-w-[75%] bg-cf-accent text-white whitespace-pre-wrap"
                        : msg.tool_calls && msg.tool_calls.length > 0
                          ? "max-w-[95%] sm:max-w-[90%] bg-cf-bg-surface-alt text-cf-text-primary"
                          : "max-w-[90%] sm:max-w-[75%] bg-cf-bg-surface-alt text-cf-text-primary"
                    }`}
                  >
                    <Show when={msg.role === "assistant"} fallback={msg.content}>
                      <Show when={msg.content?.trim()}>
                        <Markdown content={msg.content} />
                      </Show>
                    </Show>
                    {/* Persisted tool calls from this message */}
                    <Show when={msg.tool_calls && msg.tool_calls.length > 0}>
                      <div class="border-l-2 border-cf-accent/40 pl-3 mt-1">
                        <For each={msg.tool_calls}>
                          {(tc) => {
                            let args: Record<string, unknown> | undefined;
                            try {
                              args = JSON.parse(tc.function.arguments) as Record<string, unknown>;
                            } catch {
                              // args may not be valid JSON
                            }
                            const result = toolResultMap().get(tc.id);
                            return (
                              <ToolCallCard
                                name={tc.function.name}
                                args={args}
                                result={result}
                                status={result !== undefined ? "completed" : "pending"}
                              />
                            );
                          }}
                        </For>
                      </div>
                    </Show>
                    {/* Inline image thumbnails for multimodal messages */}
                    <Show when={msg.images && msg.images.length > 0}>
                      <div class="mt-2 flex flex-wrap gap-2">
                        <For each={msg.images}>
                          {(img: MessageImage) => (
                            <button
                              type="button"
                              class="block cursor-pointer rounded border border-cf-border overflow-hidden hover:opacity-80 transition-opacity"
                              onClick={() =>
                                window.open(`data:${img.media_type};base64,${img.data}`, "_blank")
                              }
                              title="Click to open full size"
                            >
                              <img
                                src={`data:${img.media_type};base64,${img.data}`}
                                alt={img.alt_text ?? "Canvas sketch"}
                                class="max-w-[200px] max-h-[150px] object-contain"
                              />
                            </button>
                          )}
                        </For>
                      </div>
                    </Show>
                    <MessageBadge
                      model={msg.model || undefined}
                      tokensIn={msg.tokens_in || undefined}
                      tokensOut={msg.tokens_out || undefined}
                    />
                  </div>
                </li>
              )}
            </For>
          </ul>

          {/* Plan step status badges from AG-UI events */}
          <Show when={agui.planSteps().length > 0}>
            <div class="flex flex-wrap gap-2 px-1">
              <For each={agui.planSteps()}>
                {(step) => (
                  <Badge variant={stepBadgeVariant(step.status)} pill>
                    {step.name}
                  </Badge>
                )}
              </For>
            </div>
          </Show>

          {/* Slash command output (e.g. /help, /cost) */}
          <Show when={agui.commandOutput()}>
            {(output) => (
              <div class="flex justify-start">
                <div class="max-w-[90%] sm:max-w-[75%] rounded-cf-md px-4 py-2 text-sm bg-cf-bg-surface-alt text-cf-text-secondary border border-cf-border">
                  <pre class="whitespace-pre-wrap font-mono text-xs">{output()}</pre>
                </div>
              </div>
            )}
          </Show>

          {/* Active tool calls from AG-UI events — grouped with vertical line */}
          <Show when={agui.toolCalls().length > 0}>
            <div class="flex justify-start">
              <div class="max-w-[95%] sm:max-w-[90%] w-full border-l-2 border-cf-accent/40 pl-3 ml-2">
                <Show when={agui.stepCount() > 0}>
                  <div class="mb-1 text-xs text-cf-text-muted">
                    Step {agui.stepCount()} {"\u00B7"} {agui.toolCalls().length} tool call
                    {agui.toolCalls().length !== 1 ? "s" : ""}
                  </div>
                </Show>
                <For each={agui.toolCalls()}>
                  {(tc) => (
                    <ToolCallCard
                      name={tc.name}
                      args={tc.args}
                      result={tc.result}
                      status={tc.status}
                      diff={tc.diff}
                      runId={activeConversation() ?? undefined}
                      callId={tc.callId}
                    />
                  )}
                </For>
              </div>
            </div>
          </Show>

          {/* Goal proposals from AG-UI events — inline approval cards */}
          <Show when={agui.goalProposals().length > 0}>
            <div class="flex justify-start">
              <div class="max-w-[90%] sm:max-w-[75%] w-full">
                <For each={agui.goalProposals()}>
                  {(proposal) => (
                    <GoalProposalCard
                      proposal={proposal}
                      projectId={props.projectId}
                      onApprove={(title) => sendChatMessage(`[Goal approved: ${title}]`)}
                      onReject={(title) => sendChatMessage(`[Goal rejected: ${title}]`)}
                    />
                  )}
                </For>
              </div>
            </div>
          </Show>

          {/* Permission request cards from AG-UI events (HITL approval) */}
          <For
            each={agui
              .permissionRequests()
              .filter((pr) => !agui.resolvedPermissions().has(pr.call_id))}
          >
            {(pr) => (
              <div class="flex justify-start">
                <div class="max-w-[90%] sm:max-w-[75%] w-full">
                  <PermissionRequestCard
                    projectId={props.projectId}
                    runId={pr.run_id}
                    callId={pr.call_id}
                    tool={pr.tool}
                    command={pr.command}
                    path={pr.path}
                    onResolved={() => {
                      agui.setResolvedPermissions((prev) => new Set([...prev, pr.call_id]));
                    }}
                  />
                </div>
              </div>
            )}
          </For>

          {/* Streaming assistant message from AG-UI text_message events */}
          <Show when={agui.streamingContent()}>
            {(content) => (
              <div class="flex justify-start">
                <div class="max-w-[90%] sm:max-w-[75%] rounded-cf-md px-4 py-2 text-sm bg-cf-bg-surface-alt text-cf-text-primary">
                  <Markdown content={content()} />
                  <StreamingCursor active={agui.agentRunning()} />
                </div>
              </div>
            )}
          </Show>

          {/* Error message when a run fails */}
          <Show when={agui.runError()}>
            {(error) => (
              <div class="flex justify-start">
                <div class="max-w-[90%] sm:max-w-[75%] rounded-cf-md px-4 py-2 text-sm bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-300 border border-red-300 dark:border-red-700">
                  <span class="font-medium">Error: </span>
                  {error()}
                </div>
              </div>
            )}
          </Show>

          {/* Action suggestions — shown when agent is idle and suggestions exist */}
          <Show when={!agui.agentRunning() && agui.actionSuggestions().length > 0}>
            <ActionBar
              rules={agui.actionSuggestions()}
              onAction={(action) => {
                agui.setActionSuggestions([]);
                if (action.action === "send_message") {
                  sendChatMessage(action.value);
                }
              }}
            />
          </Show>

          {/* Thinking indicator: shown when agent run is active but no text has streamed yet */}
          <Show when={(sending() || agui.agentRunning()) && !agui.streamingContent()}>
            <div class="flex justify-start">
              <div class="bg-cf-bg-surface-alt rounded-cf-md px-4 py-3 inline-flex items-center gap-2">
                <TypingIndicator />
                <span class="text-sm text-cf-text-tertiary">{t("chat.thinking")}</span>
              </div>
            </div>
          </Show>
        </div>

        {/* Contextual suggestions */}
        <ChatSuggestions
          activeTab={props.activeTab ?? "files"}
          onSelect={(text) => setInput(text)}
        />

        {/* Context files badges */}
        <Show when={contextFiles().length > 0}>
          <div class="flex flex-wrap gap-1 border-t border-cf-border px-3 pt-2">
            <For each={contextFiles()}>
              {(path) => (
                <TokenBadge type="@" label={path} onRemove={() => removeContextFile(path)} />
              )}
            </For>
          </div>
        </Show>

        {/* Input area */}
        <div class="border-t border-cf-border p-3 flex-shrink-0" data-shortcut-scope="chat">
          <input ref={chatFileInputRef} type="file" class="hidden" onChange={handleAttachChange} />
          <div class="flex gap-2 items-end">
            <Button
              variant="ghost"
              size="sm"
              class="flex-shrink-0"
              onClick={() => chatFileInputRef?.click()}
              disabled={attaching()}
              title={t("chat.attachFile")}
            >
              <svg
                xmlns="http://www.w3.org/2000/svg"
                viewBox="0 0 20 20"
                fill="currentColor"
                class="w-4 h-4"
              >
                <path
                  fill-rule="evenodd"
                  d="M15.621 4.379a3 3 0 0 0-4.242 0l-7 7a3 3 0 0 0 4.241 4.243l7-7a1.5 1.5 0 0 0-2.121-2.122l-7 7a.5.5 0 1 1-.707-.707l7-7a3 3 0 0 1 4.242 4.243l-7 7a5 5 0 0 1-7.071-7.071l7-7a1 1 0 0 1 1.414 1.414l-7 7a3 3 0 1 0 4.243 4.243l7-7a1.5 1.5 0 0 0 0-2.122Z"
                  clip-rule="evenodd"
                />
              </svg>
            </Button>
            {/* Design Canvas button */}
            <Button
              variant="ghost"
              size="sm"
              class="flex-shrink-0"
              onClick={() => setCanvasOpen(true)}
              title="Design Canvas"
              data-testid="canvas-open-btn"
            >
              <svg
                xmlns="http://www.w3.org/2000/svg"
                viewBox="0 0 20 20"
                fill="currentColor"
                class="w-4 h-4"
              >
                <path d="M15.993 1.385a1.87 1.87 0 0 1 2.623 2.622l-4.03 4.031-2.622-2.623 4.03-4.03ZM3.74 12.104l7.217-7.216 2.623 2.622-7.217 7.217H3.74v-2.623Z" />
                <path
                  fill-rule="evenodd"
                  d="M0 4a2 2 0 0 1 2-2h7a1 1 0 0 1 0 2H2v14h14v-7a1 1 0 1 1 2 0v7a2 2 0 0 1-2 2H2a2 2 0 0 1-2-2V4Z"
                  clip-rule="evenodd"
                />
              </svg>
            </Button>
            <div class="flex-1 min-w-0">
              <ChatInput
                value={input()}
                onInput={setInput}
                onSubmit={handleSend}
                placeholder={t("chat.placeholder")}
                disabled={sending()}
                projectId={props.projectId}
                conversations={(conversations() ?? []).map((c) => ({ id: c.id, title: c.title }))}
              />
            </div>
            <Button
              variant="primary"
              size="sm"
              class="flex-shrink-0"
              onClick={handleSend}
              disabled={sending() || !input().trim()}
            >
              {t("chat.send")}
            </Button>
          </div>
        </div>

        {/* Session usage footer */}
        <SessionFooter
          model={agui.sessionModel() || undefined}
          steps={agui.sessionSteps()}
          costUsd={agui.sessionCostUsd()}
          tokensUsed={agui.sessionTokensIn() + agui.sessionTokensOut()}
          tokensTotal={maxContextTokens()}
          visible={agui.sessionCostUsd() > 0 || agui.sessionSteps() > 0}
        />

        {/* Design Canvas modal */}
        <CanvasModal
          open={canvasOpen()}
          onClose={() => setCanvasOpen(false)}
          onExport={(canvasExports: CanvasExports) => {
            setCanvasOpen(false);
            const convId = activeConversation();
            if (!convId) return;

            const hasVision = modelSupportsVision(agui.sessionModel());
            const promptText = buildCanvasPrompt(
              canvasExports.ascii,
              canvasExports.json,
              input().trim(),
              hasVision,
            );

            // Build images array for vision-capable models with a valid PNG.
            const images: MessageImage[] = [];
            if (hasVision && canvasExports.png) {
              const base64Data = canvasExports.png.replace(/^data:image\/png;base64,/, "");
              images.push({
                data: base64Data,
                media_type: "image/png",
                alt_text: "Design canvas sketch",
              });
            }

            setInput("");
            setSending(true);
            agui.setRunError(null);
            void (async () => {
              try {
                await api.conversations.send(convId, {
                  content: promptText,
                  ...(images.length > 0 ? { images } : {}),
                });
                await refetchMessages();
                scrollToBottom();
              } catch {
                // Error handled by API layer toast.
              } finally {
                setSending(false);
              }
            })();
          }}
        />
      </Show>
    </div>
  );
}
