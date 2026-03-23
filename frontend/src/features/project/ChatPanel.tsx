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
import { Button } from "~/ui";

import { buildCanvasPrompt, modelSupportsVision } from "../canvas/buildCanvasPrompt";
import { CanvasModal } from "../canvas/CanvasModal";
import type { CanvasExports } from "../canvas/canvasTypes";
import ChatInput from "../chat/ChatInput";
import { type CommandContext, executeCommand } from "../chat/commandExecutor";
import TokenBadge from "../chat/TokenBadge";
import ChatHeader from "./ChatHeader";
import ChatMessages from "./ChatMessages";
import ChatSuggestions from "./ChatSuggestions";
import { clearContextFiles, contextFiles, removeContextFile } from "./contextFilesStore";
import SessionFooter from "./SessionFooter";
import { useChatAGUI } from "./useChatAGUI";

interface ChatPanelProps {
  projectId: string;
  activeTab?: string;
}

export default function ChatPanel(props: ChatPanelProps) {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const { isRunActive } = useConversationRuns();

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
  // eslint-disable-next-line prefer-const -- SolidJS ref requires let
  let chatFileInputRef: HTMLInputElement | undefined = undefined;

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
        <ChatHeader
          projectId={props.projectId}
          activeConversation={activeConversation}
          setActiveConversation={setActiveConversation}
          conversations={conversations}
          refetchConversations={() => void refetchConversations()}
          session={session}
          refetchSession={() => void refetchSession()}
          refetchMessages={() => void refetchMessages()}
          agentRunning={agui.agentRunning}
          stepCount={agui.stepCount}
          runningCost={agui.runningCost}
          onStop={handleStop}
          sessionByConv={sessionByConv}
        />

        <ChatMessages
          projectId={props.projectId}
          messages={messages}
          toolResultMap={toolResultMap}
          activeConversation={activeConversation}
          streamingContent={agui.streamingContent}
          agentRunning={agui.agentRunning}
          runError={agui.runError}
          toolCalls={agui.toolCalls}
          planSteps={agui.planSteps}
          goalProposals={agui.goalProposals}
          permissionRequests={agui.permissionRequests}
          resolvedPermissions={agui.resolvedPermissions}
          setResolvedPermissions={agui.setResolvedPermissions}
          actionSuggestions={agui.actionSuggestions}
          setActionSuggestions={agui.setActionSuggestions}
          stepCount={agui.stepCount}
          commandOutput={agui.commandOutput}
          sending={sending}
          sendChatMessage={sendChatMessage}
          setContainerRef={(el) => {
            messagesContainerRef = el;
          }}
        />

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
