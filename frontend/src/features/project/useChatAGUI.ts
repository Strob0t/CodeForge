import { batch, createSignal, onCleanup } from "solid-js";

import type { AGUIGoalProposal, AGUIPermissionRequest } from "~/api/websocket";
import { useWebSocket } from "~/components/WebSocketProvider";

import type { ActionRule } from "./actionRules";
import { deriveActions } from "./actionRules";
import type {
  ChatAGUIState,
  PlanStepState,
  RoadmapProposalState,
  ToolCallState,
} from "./chatPanelTypes";

interface UseChatAGUIOptions {
  activeConversation: () => string | null;
  scrollToBottom: () => void;
  refetchMessages: () => void;
  refetchSession: () => void;
}

export function useChatAGUI(opts: UseChatAGUIOptions): ChatAGUIState {
  const { onAGUIEvent } = useWebSocket();

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

  // Roadmap proposals from AG-UI events (rendered inline as approval cards)
  const [roadmapProposals, setRoadmapProposals] = createSignal<RoadmapProposalState[]>([]);

  // Permission requests from AG-UI events (HITL approval cards)
  const [permissionRequests, setPermissionRequests] = createSignal<AGUIPermissionRequest[]>([]);
  const [resolvedPermissions, setResolvedPermissions] = createSignal<Set<string>>(new Set());

  // Action suggestions from AG-UI events and rule-based derivation
  const [actionSuggestions, setActionSuggestions] = createSignal<ActionRule[]>([]);

  // Command output display (for /help, /cost etc.)
  const [commandOutput, setCommandOutput] = createSignal<string | null>(null);

  // Session-level diff accumulator: persists file diffs across tool calls for /diff command
  interface SessionDiffEntry {
    path: string;
    hunks: {
      old_start: number;
      old_lines: number;
      new_start: number;
      new_lines: number;
      old_content: string;
      new_content: string;
    }[];
  }
  const [sessionDiffs, setSessionDiffs] = createSignal<SessionDiffEntry[]>([]);

  // Session-level step history accumulator: persists step events for /rewind timeline
  interface StepHistoryEntry {
    stepId: string;
    name: string;
    timestamp: string;
    status: "running" | "completed" | "failed" | "cancelled" | "skipped";
  }
  const [stepHistory, setStepHistory] = createSignal<StepHistoryEntry[]>([]);

  // Agentic mode tracking: step counter and running cost
  const [stepCount, setStepCount] = createSignal(0);
  const [runningCost, setRunningCost] = createSignal(0);

  // Session-level cumulative usage (persists across runs, shown in SessionFooter)
  const [sessionModel, setSessionModel] = createSignal("");
  const [sessionCostUsd, setSessionCostUsd] = createSignal(0);
  const [sessionTokensIn, setSessionTokensIn] = createSignal(0);
  const [sessionTokensOut, setSessionTokensOut] = createSignal(0);
  const [sessionSteps, setSessionSteps] = createSignal(0);

  // --- AG-UI event subscriptions ---

  // When a run starts for the active conversation, show the thinking indicator

  const cleanupRunStarted = onAGUIEvent("agui.run_started", (payload) => {
    const runId = payload.run_id as string;
    if (runId === opts.activeConversation()) {
      batch(() => {
        setAgentRunning(true);
        setStreamingContent("");
        setRunError(null);
        setStepCount(0);
        setRunningCost(0);
        setGoalProposals([]);
        setRoadmapProposals([]);
        setActionSuggestions([]);
      });
    }
  });

  // When a text_message arrives for the active conversation, update streaming content

  const cleanupTextMessage = onAGUIEvent("agui.text_message", (payload) => {
    const runId = payload.run_id as string;
    if (runId === opts.activeConversation()) {
      const content = payload.content as string;
      setStreamingContent((prev) => prev + content);
      opts.scrollToBottom();
    }
  });

  // When a tool call starts, add it to the tool calls list and increment step counter

  const cleanupToolCall = onAGUIEvent("agui.tool_call", (payload) => {
    const runId = payload.run_id as string;
    if (runId === opts.activeConversation()) {
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
      opts.scrollToBottom();
    }
  });

  // When a tool result arrives, update the corresponding tool call and track cost
  // eslint-disable-next-line solid/reactivity -- event handler, not tracked scope
  const cleanupToolResult = onAGUIEvent("agui.tool_result", (payload) => {
    const runId = payload.run_id as string;
    if (runId === opts.activeConversation()) {
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
      // Accumulate file diffs for /diff summary
      if (diff) {
        setSessionDiffs((prev) => {
          const idx = prev.findIndex((d) => d.path === diff.path);
          if (idx >= 0) {
            const next = [...prev];
            next[idx] = diff;
            return next;
          }
          return [...prev, diff];
        });
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

  const cleanupRunFinished = onAGUIEvent("agui.run_finished", (payload) => {
    const runId = payload.run_id as string;
    if (runId === opts.activeConversation()) {
      const status = payload.status as string;
      const errorMsg = payload.error as string | undefined;
      // Extract usage data from run_finished payload
      const model = (payload.model as string) || "";
      const costUsd = (payload.cost_usd as number) || 0;
      const tokensIn = (payload.tokens_in as number) || 0;
      const tokensOut = (payload.tokens_out as number) || 0;
      const steps = (payload.steps as number) || 0;

      batch(() => {
        setAgentRunning(false);
        setStreamingContent("");
        setToolCalls([]);
        setPlanSteps([]);
        setStepCount(0);
        setRunningCost(0);
        setPermissionRequests([]);
        setResolvedPermissions(new Set<string>());

        // Accumulate session-level usage
        if (model) setSessionModel(model);
        setSessionCostUsd((prev) => prev + costUsd);
        setSessionTokensIn((prev) => prev + tokensIn);
        setSessionTokensOut((prev) => prev + tokensOut);
        setSessionSteps((prev) => prev + steps);

        if (status === "failed" && errorMsg) {
          setRunError(errorMsg);
        } else if (status === "cancelled") {
          setRunError("Run was cancelled.");
        }
      });

      opts.refetchMessages();
      opts.refetchSession();
    }
  });

  // When a plan step starts, add it to the step tracker
  const cleanupStepStarted = onAGUIEvent("agui.step_started", (payload) => {
    const stepId = payload.step_id as string;
    const name = payload.name as string;
    setPlanSteps((prev) => [...prev, { stepId, name, status: "running" }]);
    // Accumulate into persistent step history for /rewind timeline
    setStepHistory((prev) => [
      ...prev,
      { stepId, name, status: "running", timestamp: new Date().toISOString() },
    ]);
  });

  // When a plan step finishes, update its status
  const cleanupStepFinished = onAGUIEvent("agui.step_finished", (payload) => {
    const stepId = payload.step_id as string;
    const status = payload.status as PlanStepState["status"];
    setPlanSteps((prev) => prev.map((s) => (s.stepId === stepId ? { ...s, status } : s)));
    // Update persistent step history
    setStepHistory((prev) => prev.map((s) => (s.stepId === stepId ? { ...s, status } : s)));
  });

  // When the agent proposes a goal, add it to the proposal list for user approval

  const cleanupGoalProposal = onAGUIEvent("agui.goal_proposal", (payload) => {
    if (payload.run_id === opts.activeConversation()) {
      setGoalProposals((prev) => [...prev, payload]);
      opts.scrollToBottom();
    }
  });

  // When the agent proposes a roadmap change, add it to the proposal list for user approval

  const cleanupRoadmapProposal = onAGUIEvent("agui.roadmap_proposal", (ev) => {
    if (ev.run_id !== opts.activeConversation()) return;
    setRoadmapProposals((prev) => [
      ...prev,
      {
        proposalId: ev.proposal_id,
        action: ev.action,
        milestoneTitle: ev.milestone_title,
        milestoneDescription: ev.milestone_description,
        stepTitle: ev.step_title,
        stepDescription: ev.step_description,
        stepComplexity: ev.step_complexity,
        stepModelTier: ev.step_model_tier,
        status: "pending" as const,
      },
    ]);
    opts.scrollToBottom();
  });

  // When the agent proposes a roadmap change, add it to the proposal list for user approval

  // When the agent requests permission (HITL), show an approval card

  const cleanupPermissionRequest = onAGUIEvent("agui.permission_request", (payload) => {
    if (payload.run_id === opts.activeConversation()) {
      setPermissionRequests((prev) => [...prev, payload]);
      opts.scrollToBottom();
    }
  });

  // When the agent suggests a follow-up action

  const cleanupActionSuggestion = onAGUIEvent("agui.action_suggestion", (payload) => {
    if (payload.run_id === opts.activeConversation()) {
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
    cleanupRoadmapProposal();
    cleanupPermissionRequest();
    cleanupActionSuggestion();
  });

  return {
    streamingContent,
    agentRunning,
    setAgentRunning,
    runError,
    setRunError,
    toolCalls,
    planSteps,
    goalProposals,
    roadmapProposals,
    permissionRequests,
    resolvedPermissions,
    setResolvedPermissions: (fn: (prev: Set<string>) => Set<string>) =>
      setResolvedPermissions((prev) => fn(prev)),
    actionSuggestions,
    setActionSuggestions: (fn: ActionRule[] | ((prev: ActionRule[]) => ActionRule[])) => {
      if (typeof fn === "function") {
        setActionSuggestions(fn);
      } else {
        setActionSuggestions(fn);
      }
    },
    stepCount,
    runningCost,
    sessionModel,
    sessionCostUsd,
    sessionTokensIn,
    sessionTokensOut,
    sessionSteps,
    commandOutput,
    setCommandOutput,
    sessionDiffs,
    stepHistory,
  };
}
