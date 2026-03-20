import { createSignal, onCleanup } from "solid-js";

import { getAccessToken } from "~/api/client";

export interface WSMessage {
  type: string;
  payload: Record<string, unknown>;
}

/**
 * Parse a raw WebSocket MessageEvent into a typed WSMessage.
 * Returns null if the data is not valid JSON or lacks the expected shape.
 */
export function parseWSMessage(data: unknown): WSMessage | null {
  if (typeof data !== "string") return null;
  try {
    const parsed: unknown = JSON.parse(data);
    if (
      typeof parsed === "object" &&
      parsed !== null &&
      "type" in parsed &&
      typeof (parsed as Record<string, unknown>).type === "string"
    ) {
      const msg = parsed as Record<string, unknown>;
      return {
        type: msg.type as string,
        payload:
          typeof msg.payload === "object" && msg.payload !== null
            ? (msg.payload as Record<string, unknown>)
            : {},
      };
    }
  } catch {
    // Malformed JSON
  }
  return null;
}

/**
 * Validate that a WSMessage payload matches a specific AG-UI event type.
 * Checks for the presence of the required `run_id` field shared by all AG-UI events.
 */
function isAGUIPayload(payload: Record<string, unknown>): boolean {
  return typeof payload.run_id === "string";
}

// AG-UI event types following the CopilotKit AG-UI specification.
export type AGUIEventType =
  | "agui.run_started"
  | "agui.run_finished"
  | "agui.text_message"
  | "agui.tool_call"
  | "agui.tool_result"
  | "agui.state_delta"
  | "agui.step_started"
  | "agui.step_finished"
  | "agui.goal_proposal"
  | "agui.permission_request"
  | "agui.action_suggestion";

export interface AGUIRunStarted {
  run_id: string;
  thread_id?: string;
  agent_name?: string;
}
export interface AGUIRunFinished {
  run_id: string;
  status: string;
  error?: string;
  model?: string;
  cost_usd?: number;
  tokens_in?: number;
  tokens_out?: number;
  steps?: number;
}
export interface AGUITextMessage {
  run_id: string;
  role: string;
  content: string;
}
export interface AGUIToolCall {
  run_id: string;
  call_id: string;
  name: string;
  args: string;
}
export interface AGUIToolResult {
  run_id: string;
  call_id: string;
  result: string;
  error?: string;
  cost_usd?: number;
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
export interface AGUIStateDelta {
  run_id: string;
  delta: string;
}
export interface AGUIStepStarted {
  run_id: string;
  step_id: string;
  name: string;
}
export interface AGUIStepFinished {
  run_id: string;
  step_id: string;
  status: string;
}
export interface AGUIGoalProposal {
  run_id: string;
  proposal_id: string;
  action: "create" | "update" | "delete";
  kind: "vision" | "requirement" | "constraint" | "state" | "context";
  title: string;
  content: string;
  priority: number;
  goal_id?: string;
}
export interface AGUIPermissionRequest {
  run_id: string;
  call_id: string;
  tool: string;
  command?: string;
  path?: string;
}
export interface AGUIActionSuggestion {
  run_id: string;
  label: string;
  action: string; // "send_message", "run_tool", "navigate"
  value: string;
}

/** Discriminated map from AG-UI event type to its typed payload. */
export interface AGUIEventMap {
  "agui.run_started": AGUIRunStarted;
  "agui.run_finished": AGUIRunFinished;
  "agui.text_message": AGUITextMessage;
  "agui.tool_call": AGUIToolCall;
  "agui.tool_result": AGUIToolResult;
  "agui.state_delta": AGUIStateDelta;
  "agui.step_started": AGUIStepStarted;
  "agui.step_finished": AGUIStepFinished;
  "agui.goal_proposal": AGUIGoalProposal;
  "agui.permission_request": AGUIPermissionRequest;
  "agui.action_suggestion": AGUIActionSuggestion;
}

function buildWSURL(): string {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const token = getAccessToken();
  const qs = token ? `?token=${encodeURIComponent(token)}` : "";
  return `${proto}//${location.host}/ws${qs}`;
}

/**
 * Creates a reconnecting WebSocket that rebuilds the URL (with a fresh token)
 * on every reconnection attempt. This ensures the auth token is always current.
 *
 * NOTE: Do not call this directly from components — use `useWebSocket()` from
 * `~/components/WebSocketProvider` to share a single connection app-wide.
 */
export function createCodeForgeWS() {
  const RECONNECT_DELAY = 1000;
  const [connected, setConnected] = createSignal(false);

  let ws: WebSocket | null = null;
  let disposed = false;
  let manualReconnect = false;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  const listeners: ((ev: MessageEvent) => void)[] = [];

  function connect(): void {
    if (disposed) return;

    const token = getAccessToken();
    if (!token) {
      // No token yet — retry after delay.
      reconnectTimer = setTimeout(connect, RECONNECT_DELAY);
      return;
    }

    const url = buildWSURL();
    ws = new WebSocket(url);

    ws.addEventListener("open", () => setConnected(true));

    ws.addEventListener("close", () => {
      setConnected(false);
      if (!disposed && !manualReconnect) {
        reconnectTimer = setTimeout(connect, RECONNECT_DELAY);
      }
    });

    ws.addEventListener("error", () => {
      // error is always followed by close, which triggers reconnect
    });

    ws.addEventListener("message", (ev) => {
      for (const listener of listeners) {
        listener(ev);
      }
    });
  }

  connect();

  onCleanup(() => {
    disposed = true;
    if (reconnectTimer) clearTimeout(reconnectTimer);
    ws?.close();
  });

  function onMessage(handler: (msg: WSMessage) => void): () => void {
    const listener = (ev: MessageEvent): void => {
      const msg = parseWSMessage(ev.data);
      if (msg !== null) {
        handler(msg);
      }
      // Silently ignore malformed WebSocket messages (empty frames, non-JSON data).
    };

    listeners.push(listener);

    return () => {
      const idx = listeners.indexOf(listener);
      if (idx >= 0) listeners.splice(idx, 1);
    };
  }

  /** Subscribe to a specific AG-UI event type with full type safety. */
  function onAGUIEvent<T extends AGUIEventType>(
    type: T,
    handler: (payload: AGUIEventMap[T]) => void,
  ): () => void {
    return onMessage((msg) => {
      if (msg.type === type && isAGUIPayload(msg.payload)) {
        // Safe cast: type discriminator + run_id validation ensures correct shape.
        handler(msg.payload as AGUIEventMap[T]);
      }
    });
  }

  /** Force-close and reconnect (e.g. after token refresh). */
  function reconnect(): void {
    if (disposed) return;
    // Prevent the close handler from scheduling a competing reconnect.
    manualReconnect = true;
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    ws?.close();
    manualReconnect = false;
    connect();
  }

  return { connected, onMessage, onAGUIEvent, reconnect } as const;
}
