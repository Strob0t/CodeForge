import type { AGUIGoalProposal, AGUIPermissionRequest } from "~/api/websocket";

import type { ActionRule } from "./actionRules";

export interface ToolCallState {
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

export interface PlanStepState {
  stepId: string;
  name: string;
  status: "running" | "completed" | "failed" | "cancelled" | "skipped";
}

/** All reactive state managed by the useChatAGUI hook. */
export interface ChatAGUIState {
  streamingContent: () => string;
  agentRunning: () => boolean;
  setAgentRunning: (v: boolean) => void;
  runError: () => string | null;
  setRunError: (v: string | null) => void;
  toolCalls: () => ToolCallState[];
  planSteps: () => PlanStepState[];
  goalProposals: () => AGUIGoalProposal[];
  permissionRequests: () => AGUIPermissionRequest[];
  resolvedPermissions: () => Set<string>;
  setResolvedPermissions: (fn: (prev: Set<string>) => Set<string>) => void;
  actionSuggestions: () => ActionRule[];
  setActionSuggestions: (fn: ActionRule[] | ((prev: ActionRule[]) => ActionRule[])) => void;
  stepCount: () => number;
  runningCost: () => number;
  sessionModel: () => string;
  sessionCostUsd: () => number;
  sessionTokensIn: () => number;
  sessionTokensOut: () => number;
  sessionSteps: () => number;
  commandOutput: () => string | null;
  setCommandOutput: (v: string | null) => void;
  sessionDiffs: () => {
    path: string;
    hunks: {
      old_start: number;
      old_lines: number;
      new_start: number;
      new_lines: number;
      old_content: string;
      new_content: string;
    }[];
  }[];
  stepHistory: () => {
    stepId: string;
    name: string;
    timestamp: string;
    status: "running" | "completed" | "failed" | "cancelled" | "skipped";
  }[];
}
