import { createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { Conversation, Session } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import type { TranslationKey } from "~/i18n/en";
import { Badge, Button, CostDisplay } from "~/ui";

import SessionPanel from "./SessionPanel";

interface ChatHeaderProps {
  projectId: string;
  activeConversation: () => string | null;
  setActiveConversation: (id: string | null) => void;
  conversations: () => Conversation[] | undefined;
  refetchConversations: () => void;
  session: () => Session | null | undefined;
  refetchSession: () => void;
  refetchMessages: () => void;
  agentRunning: () => boolean;
  stepCount: () => number;
  runningCost: () => number;
  onStop: () => void;
  sessionByConv: () => Map<string, Session>;
  onShowRewindTimeline: () => void;
  hasStepHistory: boolean;
}

export default function ChatHeader(props: ChatHeaderProps) {
  const { t } = useI18n();
  const { show: toast } = useToast();

  const [forkLoading, setForkLoading] = createSignal(false);
  const [rewindLoading, setRewindLoading] = createSignal(false);
  const [resumeLoading, setResumeLoading] = createSignal(false);
  const [showSessionHistory, setShowSessionHistory] = createSignal(false);

  return (
    <>
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
                  props.refetchConversations();
                  props.setActiveConversation(conv.id);
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
          <Show when={props.agentRunning()}>
            <span class="inline-flex items-center gap-1 rounded-full bg-cf-accent/10 px-2 py-0.5 text-xs font-medium text-cf-accent">
              <span class="inline-block h-1.5 w-1.5 rounded-full bg-cf-accent animate-pulse" />
              Agentic
            </span>
          </Show>
        </div>
        <div class="flex flex-wrap items-center gap-2 sm:gap-3">
          {/* Session status badge */}
          <Show when={props.session()}>
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
          <Show when={!props.agentRunning() && props.session()}>
            <Button
              variant="secondary"
              size="sm"
              class="text-xs px-2 py-0.5"
              disabled={forkLoading()}
              onClick={() => {
                const convId = props.activeConversation();
                if (!convId) return;
                setForkLoading(true);
                void api.conversations
                  .fork(convId)
                  .then(
                    () => {
                      props.refetchSession();
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
                const convId = props.activeConversation();
                if (!convId) return;
                setRewindLoading(true);
                void api.conversations
                  .rewind(convId)
                  .then(
                    () => {
                      props.refetchSession();
                      props.refetchMessages();
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
              !props.agentRunning() &&
              props.session()?.current_run_id &&
              (props.session()?.status === "paused" || props.session()?.status === "completed")
            }
          >
            <Button
              variant="secondary"
              size="sm"
              class="text-xs px-2 py-0.5"
              disabled={resumeLoading()}
              onClick={() => {
                const runId = props.session()?.current_run_id;
                if (!runId) return;
                setResumeLoading(true);
                void api.runs
                  .resume(runId)
                  .then(
                    () => {
                      props.refetchSession();
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
          {/* Timeline button (visible when step history exists) */}
          <Show when={props.hasStepHistory}>
            <Button
              variant="secondary"
              size="sm"
              class="text-xs px-2 py-0.5"
              onClick={props.onShowRewindTimeline}
            >
              Timeline
            </Button>
          </Show>
          {/* Session History toggle */}
          <Show when={props.session()}>
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
          <Show when={props.agentRunning() && props.stepCount() > 0}>
            <span class="text-xs text-cf-text-muted">Step {props.stepCount()}</span>
          </Show>
          {/* Running cost during agentic turn */}
          <Show when={props.agentRunning() && props.runningCost() > 0}>
            <CostDisplay usd={props.runningCost()} class="text-xs text-cf-text-muted" />
          </Show>
          {/* Stop button during active agentic runs */}
          <Show when={props.agentRunning()}>
            <Button
              variant="primary"
              size="sm"
              class="bg-red-600 hover:bg-red-700 text-white text-xs px-2 py-0.5"
              onClick={props.onStop}
            >
              {"\u25A0"} Stop
            </Button>
          </Show>
        </div>
      </div>

      {/* Conversation selector with session status dots */}
      <Show when={(props.conversations() ?? []).length > 1}>
        <div class="flex items-center gap-1 border-b border-cf-border px-3 py-1.5 overflow-x-auto scrollbar-none">
          <For each={props.conversations() ?? []}>
            {(conv) => {
              const convSession = () => props.sessionByConv().get(conv.id);
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
                    props.activeConversation() === conv.id
                      ? "bg-cf-accent/15 text-cf-accent font-medium"
                      : "text-cf-text-muted hover:text-cf-text-primary hover:bg-cf-bg-hover"
                  }`}
                  onClick={() => props.setActiveConversation(conv.id)}
                  title={convSession() ? `Session: ${convSession()?.status}` : undefined}
                >
                  <Show when={convSession()}>
                    <span class={`inline-block h-2 w-2 rounded-full flex-shrink-0 ${dotColor()}`} />
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
    </>
  );
}
