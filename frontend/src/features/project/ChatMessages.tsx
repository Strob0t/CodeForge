import { For, Show } from "solid-js";

import type { ConversationMessage, MessageImage } from "~/api/types";
import type { AGUIGoalProposal, AGUIPermissionRequest } from "~/api/websocket";
import { useI18n } from "~/i18n";
import { Badge, StreamingCursor, TypingIndicator } from "~/ui";

import ActionBar from "./ActionBar";
import type { ActionRule } from "./actionRules";
import type { PlanStepState, ToolCallState } from "./chatPanelTypes";
import GoalProposalCard from "./GoalProposalCard";
import Markdown from "./Markdown";
import MessageBadge from "./MessageBadge";
import PermissionRequestCard from "./PermissionRequestCard";
import ToolCallCard from "./ToolCallCard";

interface ChatMessagesProps {
  projectId: string;
  messages: () => ConversationMessage[] | undefined;
  toolResultMap: () => Map<string, string>;
  activeConversation: () => string | null;
  // AG-UI streaming state
  streamingContent: () => string;
  agentRunning: () => boolean;
  runError: () => string | null;
  toolCalls: () => ToolCallState[];
  planSteps: () => PlanStepState[];
  goalProposals: () => AGUIGoalProposal[];
  permissionRequests: () => AGUIPermissionRequest[];
  resolvedPermissions: () => Set<string>;
  setResolvedPermissions: (fn: (prev: Set<string>) => Set<string>) => void;
  actionSuggestions: () => ActionRule[];
  setActionSuggestions: (fn: ActionRule[] | ((prev: ActionRule[]) => ActionRule[])) => void;
  stepCount: () => number;
  commandOutput: () => string | null;
  sending: () => boolean;
  // Callbacks
  sendChatMessage: (content: string) => void;
  // Ref setter for scroll container
  setContainerRef: (el: HTMLDivElement) => void;
}

function stepBadgeVariant(status: string): "info" | "success" | "danger" {
  if (status === "running") return "info";
  if (status === "completed") return "success";
  return "danger";
}

export default function ChatMessages(props: ChatMessagesProps) {
  const { t } = useI18n();

  return (
    <div ref={props.setContainerRef} class="flex-1 overflow-y-auto p-4">
      <ul class="space-y-4 list-none m-0 p-0">
        <For
          each={(props.messages() ?? []).filter((msg) => {
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
                        const result = props.toolResultMap().get(tc.id);
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
      <Show when={props.planSteps().length > 0}>
        <div class="flex flex-wrap gap-2 px-1">
          <For each={props.planSteps()}>
            {(step) => (
              <Badge variant={stepBadgeVariant(step.status)} pill>
                {step.name}
              </Badge>
            )}
          </For>
        </div>
      </Show>

      {/* Slash command output (e.g. /help, /cost) */}
      <Show when={props.commandOutput()}>
        {(output) => (
          <div class="flex justify-start">
            <div class="max-w-[90%] sm:max-w-[75%] rounded-cf-md px-4 py-2 text-sm bg-cf-bg-surface-alt text-cf-text-secondary border border-cf-border">
              <pre class="whitespace-pre-wrap font-mono text-xs">{output()}</pre>
            </div>
          </div>
        )}
      </Show>

      {/* Active tool calls from AG-UI events -- grouped with vertical line */}
      <Show when={props.toolCalls().length > 0}>
        <div class="flex justify-start">
          <div class="max-w-[95%] sm:max-w-[90%] w-full border-l-2 border-cf-accent/40 pl-3 ml-2">
            <Show when={props.stepCount() > 0}>
              <div class="mb-1 text-xs text-cf-text-muted">
                Step {props.stepCount()} {"\u00B7"} {props.toolCalls().length} tool call
                {props.toolCalls().length !== 1 ? "s" : ""}
              </div>
            </Show>
            <For each={props.toolCalls()}>
              {(tc) => (
                <ToolCallCard
                  name={tc.name}
                  args={tc.args}
                  result={tc.result}
                  status={tc.status}
                  diff={tc.diff}
                  runId={props.activeConversation() ?? undefined}
                  callId={tc.callId}
                />
              )}
            </For>
          </div>
        </div>
      </Show>

      {/* Goal proposals from AG-UI events -- inline approval cards */}
      <Show when={props.goalProposals().length > 0}>
        <div class="flex justify-start">
          <div class="max-w-[90%] sm:max-w-[75%] w-full">
            <For each={props.goalProposals()}>
              {(proposal) => (
                <GoalProposalCard
                  proposal={proposal}
                  projectId={props.projectId}
                  onApprove={(title) => props.sendChatMessage(`[Goal approved: ${title}]`)}
                  onReject={(title) => props.sendChatMessage(`[Goal rejected: ${title}]`)}
                />
              )}
            </For>
          </div>
        </div>
      </Show>

      {/* Permission request cards from AG-UI events (HITL approval) */}
      <For
        each={props
          .permissionRequests()
          .filter((pr) => !props.resolvedPermissions().has(pr.call_id))}
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
                  props.setResolvedPermissions((prev) => new Set([...prev, pr.call_id]));
                }}
              />
            </div>
          </div>
        )}
      </For>

      {/* Streaming assistant message from AG-UI text_message events */}
      <Show when={props.streamingContent()}>
        {(content) => (
          <div class="flex justify-start">
            <div class="max-w-[90%] sm:max-w-[75%] rounded-cf-md px-4 py-2 text-sm bg-cf-bg-surface-alt text-cf-text-primary">
              <Markdown content={content()} />
              <StreamingCursor active={props.agentRunning()} />
            </div>
          </div>
        )}
      </Show>

      {/* Error message when a run fails */}
      <Show when={props.runError()}>
        {(error) => (
          <div class="flex justify-start">
            <div class="max-w-[90%] sm:max-w-[75%] rounded-cf-md px-4 py-2 text-sm bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-300 border border-red-300 dark:border-red-700">
              <span class="font-medium">Error: </span>
              {error()}
            </div>
          </div>
        )}
      </Show>

      {/* Action suggestions -- shown when agent is idle and suggestions exist */}
      <Show when={!props.agentRunning() && props.actionSuggestions().length > 0}>
        <ActionBar
          rules={props.actionSuggestions()}
          onAction={(action) => {
            props.setActionSuggestions([]);
            if (action.action === "send_message") {
              props.sendChatMessage(action.value);
            }
          }}
        />
      </Show>

      {/* Thinking indicator: shown when agent run is active but no text has streamed yet */}
      <Show when={(props.sending() || props.agentRunning()) && !props.streamingContent()}>
        <div class="flex justify-start">
          <div class="bg-cf-bg-surface-alt rounded-cf-md px-4 py-3 inline-flex items-center gap-2">
            <TypingIndicator />
            <span class="text-sm text-cf-text-tertiary">{t("chat.thinking")}</span>
          </div>
        </div>
      </Show>
    </div>
  );
}
