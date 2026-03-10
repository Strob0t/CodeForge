import {
  batch,
  createEffect,
  createResource,
  createSignal,
  For,
  onCleanup,
  onMount,
  Show,
} from "solid-js";

import { api } from "~/api/client";
import type { Conversation, ConversationMessage, Session } from "~/api/types";
import type { AGUIGoalProposal, AGUIPermissionRequest } from "~/api/websocket";
import { useConversationRuns } from "~/components/ConversationRunProvider";
import { useToast } from "~/components/Toast";
import { useWebSocket } from "~/components/WebSocketProvider";
import { useI18n } from "~/i18n";
import type { TranslationKey } from "~/i18n/en";
import { Badge, Button, CostDisplay } from "~/ui";

import ActionBar from "./ActionBar";
import type { ActionRule } from "./actionRules";
import { deriveActions } from "./actionRules";
import ChatSuggestions from "./ChatSuggestions";
import GoalProposalCard from "./GoalProposalCard";
import Markdown from "./Markdown";
import PermissionRequestCard from "./PermissionRequestCard";
import SessionPanel from "./SessionPanel";
import ToolCallCard from "./ToolCallCard";

interface ChatPanelProps {
  projectId: string;
  activeTab?: string;
}

interface ToolCallState {
  callId: string;
  name: string;
  args?: Record<string, unknown>;
  result?: string;
  status: "pending" | "running" | "completed" | "failed";
  diff?: {
    path: string;
    hunks: {
      old_start: number;
      old_lines: number;
      new_start: number;
      new_lines: number;
      old_content: string;
      new_content: string;
    }[];
  };
}

interface PlanStepState {
  stepId: string;
  name: string;
  status: "running" | "completed" | "failed" | "cancelled" | "skipped";
}

export default function ChatPanel(props: ChatPanelProps) {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const { onAGUIEvent } = useWebSocket();
  const { isRunActive } = useConversationRuns();

  const [forkLoading, setForkLoading] = createSignal(false);
  const [rewindLoading, setRewindLoading] = createSignal(false);
  const [resumeLoading, setResumeLoading] = createSignal(false);
  const [showSessionHistory, setShowSessionHistory] = createSignal(false);

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

  // Streaming text from AG-UI text_message events, appended to the bottom of the chat
  const [streamingContent, setStreamingContent] = createSignal("");
  // Track whether the assistant is actively processing via run_started / run_finished
  const [agentRunning, setAgentRunning] = createSignal(false);
  // Error message from a failed run, shown as a system message in the chat
  const [runError, setRunError] = createSignal<string | null>(null);

  // Tool call tracking from AG-UI events
  const [toolCalls, setToolCalls] = createSignal<ToolCallState[]>([]);

  // Plan step tracking from AG-UI events
  const [planSteps, setPlanSteps] = createSignal<PlanStepState[]>([]);

  // Goal proposals from AG-UI events (rendered inline as approval cards)
  const [goalProposals, setGoalProposals] = createSignal<AGUIGoalProposal[]>([]);

  // Permission requests from AG-UI events (HITL approval cards)
  const [permissionRequests, setPermissionRequests] = createSignal<AGUIPermissionRequest[]>([]);
  const [resolvedPermissions, setResolvedPermissions] = createSignal<Set<string>>(new Set());

  // Action suggestions from AG-UI events and rule-based derivation
  const [actionSuggestions, setActionSuggestions] = createSignal<ActionRule[]>([]);

  // Agentic mode tracking: step counter and running cost
  const [stepCount, setStepCount] = createSignal(0);
  const [runningCost, setRunningCost] = createSignal(0);

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

  // Restore agentRunning state from global tracker when navigating back.
  createEffect(() => {
    const convId = activeConversation();
    if (convId && isRunActive(convId)) {
      setAgentRunning(true);
    }
  });

  // Proactive greeting on first chat open for a project
  const greetingKey = () => `codeforge:greeted:${props.projectId}`;
  const [greeted, setGreeted] = createSignal(localStorage.getItem(greetingKey()) === "true");

  createEffect(() => {
    const convId = activeConversation();
    const msgs = messages();
    const key = greetingKey();
    // Trigger greeting only when:
    // 1. Conversation is loaded
    // 2. No messages exist yet (fresh conversation)
    // 3. Not already greeted for this project
    // 4. Not currently sending
    if (convId && msgs && msgs.length === 0 && !greeted() && !sending()) {
      setGreeted(true);
      localStorage.setItem(key, "true");
      const greetingPrompt =
        "[Project Onboarding] Please greet me and summarize what you know about this project " +
        "(tech stack, structure, any detected specs or goals). " +
        "Then help me define goals and create an MVP plan.";
      setSending(true);
      api.conversations
        .send(convId, { content: greetingPrompt })
        .then(() => refetchMessages())
        .then(() => scrollToBottom())
        .catch(() => {
          // If greeting fails, allow retry next time
          localStorage.removeItem(key);
          setGreeted(false);
        })
        .finally(() => setSending(false));
    }
  });

  // --- AG-UI event subscriptions ---

  // When a run starts for the active conversation, show the thinking indicator
  // eslint-disable-next-line solid/reactivity -- event handler, not tracked scope
  const cleanupRunStarted = onAGUIEvent("agui.run_started", (payload) => {
    const runId = payload.run_id as string;
    if (runId === activeConversation()) {
      batch(() => {
        setAgentRunning(true);
        setStreamingContent("");
        setRunError(null);
        setStepCount(0);
        setRunningCost(0);
        setGoalProposals([]);
        setActionSuggestions([]);
      });
    }
  });

  // When a text_message arrives for the active conversation, update streaming content
  // eslint-disable-next-line solid/reactivity -- event handler, not tracked scope
  const cleanupTextMessage = onAGUIEvent("agui.text_message", (payload) => {
    const runId = payload.run_id as string;
    if (runId === activeConversation()) {
      const content = payload.content as string;
      setStreamingContent((prev) => prev + content);
      scrollToBottom();
    }
  });

  // When a tool call starts, add it to the tool calls list and increment step counter
  // eslint-disable-next-line solid/reactivity -- event handler, not tracked scope
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
      setStepCount((n) => n + 1);
      scrollToBottom();
    }
  });

  // When a tool result arrives, update the corresponding tool call and track cost
  // eslint-disable-next-line solid/reactivity -- event handler, not tracked scope
  const cleanupToolResult = onAGUIEvent("agui.tool_result", (payload) => {
    const runId = payload.run_id as string;
    if (runId === activeConversation()) {
      const callId = payload.call_id as string;
      const error = payload.error as string | undefined;
      const diff = payload.diff as ToolCallState["diff"] | undefined;
      setToolCalls((prev) =>
        prev.map((tc) =>
          tc.callId === callId
            ? {
                ...tc,
                result: payload.result as string,
                status: error ? "failed" : "completed",
                diff,
              }
            : tc,
        ),
      );
      // Track running cost if the event carries it
      if (typeof payload.cost_usd === "number") {
        setRunningCost((prev) => prev + (payload.cost_usd as number));
      }
      // Derive rule-based action suggestions from tool result
      const tc = toolCalls().find((t) => t.callId === callId);
      if (tc) {
        const derived = deriveActions(tc.name, (payload.result as string) ?? "");
        if (derived.length > 0) {
          setActionSuggestions((prev) => {
            const labels = new Set(prev.map((a) => a.label));
            return [...prev, ...derived.filter((d) => !labels.has(d.label))];
          });
        }
      }
    }
  });

  // When a run finishes, clear streaming state and refetch persisted messages
  // eslint-disable-next-line solid/reactivity -- event handler, not tracked scope
  const cleanupRunFinished = onAGUIEvent("agui.run_finished", (payload) => {
    const runId = payload.run_id as string;
    if (runId === activeConversation()) {
      const status = payload.status as string;
      const errorMsg = payload.error as string | undefined;
      batch(() => {
        setAgentRunning(false);
        setStreamingContent("");
        setToolCalls([]);
        setPlanSteps([]);
        setStepCount(0);
        setRunningCost(0);
        setPermissionRequests([]);
        setResolvedPermissions(new Set<string>());

        if (status === "failed" && errorMsg) {
          setRunError(errorMsg);
        } else if (status === "cancelled") {
          setRunError("Run was cancelled.");
        }
      });

      void refetchMessages();
      void refetchSession();
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

  // When the agent proposes a goal, add it to the proposal list for user approval
  // eslint-disable-next-line solid/reactivity -- event handler, not tracked scope
  const cleanupGoalProposal = onAGUIEvent("agui.goal_proposal", (payload) => {
    if (payload.run_id === activeConversation()) {
      setGoalProposals((prev) => [...prev, payload]);
      scrollToBottom();
    }
  });

  // When the agent requests permission (HITL), show an approval card
  // eslint-disable-next-line solid/reactivity -- event handler, not tracked scope
  const cleanupPermissionRequest = onAGUIEvent("agui.permission_request", (payload) => {
    if (payload.run_id === activeConversation()) {
      setPermissionRequests((prev) => [...prev, payload]);
      scrollToBottom();
    }
  });

  // When the agent suggests a follow-up action
  // eslint-disable-next-line solid/reactivity -- event handler, not tracked scope
  const cleanupActionSuggestion = onAGUIEvent("agui.action_suggestion", (payload) => {
    if (payload.run_id === activeConversation()) {
      const suggestion: ActionRule = {
        label: payload.label as string,
        action: payload.action as ActionRule["action"],
        value: payload.value as string,
      };
      setActionSuggestions((prev) => [...prev, suggestion]);
    }
  });

  onCleanup(() => {
    cleanupRunStarted();
    cleanupTextMessage();
    cleanupToolCall();
    cleanupToolResult();
    cleanupRunFinished();
    cleanupStepStarted();
    cleanupStepFinished();
    cleanupGoalProposal();
    cleanupPermissionRequest();
    cleanupActionSuggestion();
  });

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

    setInput("");
    setSending(true);
    setRunError(null);
    try {
      const convId = activeConversation();
      if (!convId) return;
      // All paths now dispatch via NATS (202 Accepted).
      // Results stream via AG-UI WebSocket events; messages are
      // refetched when run_finished arrives.
      await api.conversations.send(convId, { content });
      // User message is now persisted in DB — show it immediately.
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
        {/* Chat header with agentic mode indicator */}
        <div class="flex items-center justify-between border-b border-cf-border px-4 py-2">
          <div class="flex items-center gap-2">
            <span class="text-sm font-medium text-cf-text-primary">{t("chat.tab")}</span>
            <Show when={agentRunning()}>
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
            <Show when={!agentRunning() && session()}>
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
                !agentRunning() &&
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
            <Show when={agentRunning() && stepCount() > 0}>
              <span class="text-xs text-cf-text-muted">Step {stepCount()}</span>
            </Show>
            {/* Running cost during agentic turn */}
            <Show when={agentRunning() && runningCost() > 0}>
              <CostDisplay usd={runningCost()} class="text-xs text-cf-text-muted" />
            </Show>
            {/* Stop button during active agentic runs */}
            <Show when={agentRunning()}>
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
        <div class="flex-1 overflow-y-auto p-4 space-y-4">
          <For each={messages() ?? []}>
            {(msg) => (
              <div class={`flex ${msg.role === "user" ? "justify-end" : "justify-start"}`}>
                <div
                  class={`max-w-[90%] sm:max-w-[75%] rounded-cf-md px-4 py-2 text-sm ${
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

          {/* Active tool calls from AG-UI events — grouped with vertical line */}
          <Show when={toolCalls().length > 0}>
            <div class="flex justify-start">
              <div class="max-w-[90%] sm:max-w-[75%] w-full border-l-2 border-cf-accent/40 pl-3 ml-2">
                <Show when={stepCount() > 0}>
                  <div class="mb-1 text-xs text-cf-text-muted">
                    Step {stepCount()} {"\u00B7"} {toolCalls().length} tool call
                    {toolCalls().length !== 1 ? "s" : ""}
                  </div>
                </Show>
                <For each={toolCalls()}>
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
          <Show when={goalProposals().length > 0}>
            <div class="flex justify-start">
              <div class="max-w-[90%] sm:max-w-[75%] w-full">
                <For each={goalProposals()}>
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
          <For each={permissionRequests().filter((pr) => !resolvedPermissions().has(pr.call_id))}>
            {(pr) => (
              <div class="flex justify-start">
                <div class="max-w-[90%] sm:max-w-[75%] w-full">
                  <PermissionRequestCard
                    runId={pr.run_id}
                    callId={pr.call_id}
                    tool={pr.tool}
                    command={pr.command}
                    path={pr.path}
                    onResolved={() => {
                      setResolvedPermissions((prev) => new Set([...prev, pr.call_id]));
                    }}
                  />
                </div>
              </div>
            )}
          </For>

          {/* Streaming assistant message from AG-UI text_message events */}
          <Show when={streamingContent()}>
            {(content) => (
              <div class="flex justify-start">
                <div class="max-w-[90%] sm:max-w-[75%] rounded-cf-md px-4 py-2 text-sm bg-cf-bg-surface-alt text-cf-text-primary">
                  <Markdown content={content()} />
                  <div class="mt-1 text-xs opacity-60">{t("chat.streaming")}</div>
                </div>
              </div>
            )}
          </Show>

          {/* Error message when a run fails */}
          <Show when={runError()}>
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
          <Show when={!agentRunning() && actionSuggestions().length > 0}>
            <ActionBar
              rules={actionSuggestions()}
              onAction={(action) => {
                setActionSuggestions([]);
                if (action.action === "send_message") {
                  sendChatMessage(action.value);
                }
              }}
            />
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

        {/* Contextual suggestions */}
        <ChatSuggestions
          activeTab={props.activeTab ?? "files"}
          onSelect={(text) => setInput(text)}
        />

        {/* Input area */}
        <div class="border-t border-cf-border p-3 flex-shrink-0" data-shortcut-scope="chat">
          <input ref={chatFileInputRef} type="file" class="hidden" onChange={handleAttachChange} />
          <div class="flex gap-2">
            <Button
              variant="ghost"
              size="sm"
              class="self-end flex-shrink-0"
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
