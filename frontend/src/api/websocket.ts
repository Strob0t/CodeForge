import { createSignal, onCleanup } from "solid-js";

import { api, getAccessToken } from "~/api/client";
import { logError } from "~/lib/errorUtils";

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
  } catch (err) {
    logError("ws.parseWSMessage", err);
  }
  return null;
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
  | "agui.action_suggestion"
  | "agui.roadmap_proposal";

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
  /** Policy profile that asked; Allow-Always extends the project's copy of it. */
  profile?: string;
}
export interface AGUIActionSuggestion {
  run_id: string;
  label: string;
  action: string; // "send_message", "run_tool", "navigate"
  value: string;
}
export interface AGUIRoadmapProposal {
  run_id: string;
  proposal_id: string;
  action: "create_milestone" | "create_step";
  milestone_title: string;
  milestone_description?: string;
  milestone_sort_order?: number;
  step_title?: string;
  step_description?: string;
  step_sort_order?: number;
  step_complexity?: "trivial" | "simple" | "medium" | "complex";
  step_model_tier?: "weak" | "mid" | "strong";
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
  "agui.roadmap_proposal": AGUIRoadmapProposal;
}

/**
 * Narrow a WSMessage to a specific AG-UI event type.
 * Checks the type discriminator and the required `run_id` field shared by all AG-UI events.
 */
function isAGUIEvent<T extends AGUIEventType>(
  msg: WSMessage,
  type: T,
): msg is WSMessage & { type: T; payload: AGUIEventMap[T] } {
  return msg.type === type && typeof msg.payload.run_id === "string";
}

/** The parts of `window.location` a WebSocket URL is built from. */
export type WSLocation = Pick<Location, "protocol" | "host">;

/**
 * Builds the WebSocket URL for a single-use ticket from `POST /api/v1/ws/ticket`.
 * The access token never goes into the URL, where it would leak into server,
 * proxy and browser logs; a ticket is worthless once used or expired.
 */
export function buildWSURL(ticket: string, loc: WSLocation = location): string {
  const proto = loc.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${loc.host}/ws?ticket=${encodeURIComponent(ticket)}`;
}

/**
 * Creates a reconnecting WebSocket. Every connection attempt fetches a fresh
 * single-use ticket with the current access token, so reconnects after a
 * token refresh authenticate as the current session.
 *
 * NOTE: Do not call this directly from components — use `useWebSocket()` from
 * `~/components/WebSocketProvider` to share a single connection app-wide.
 */
export function createCodeForgeWS() {
  const RECONNECT_DELAY = 1000;
  const [connected, setConnected] = createSignal(false);

  let ws: WebSocket | null = null;
  let disposed = false;
  // Set by disconnect() (logout): no connection attempts until reconnect().
  let paused = false;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  // Incremented per connection attempt: a ticket that arrives after the
  // attempt was superseded (reconnect or cleanup) is discarded.
  let attempt = 0;
  const listeners: ((ev: MessageEvent) => void)[] = [];

  function scheduleReconnect(): void {
    if (disposed || paused) return;
    reconnectTimer = setTimeout(() => void connect(), RECONNECT_DELAY);
  }

  async function connect(): Promise<void> {
    if (disposed || paused) return;
    const current = ++attempt;

    if (!getAccessToken()) {
      // Not logged in yet — retry after delay.
      scheduleReconnect();
      return;
    }

    let ticket: string;
    try {
      ({ ticket } = await api.auth.wsTicket());
    } catch (err) {
      logError("ws.ticket", err);
      if (current === attempt) scheduleReconnect();
      return;
    }
    if (disposed || paused || current !== attempt) return;

    const socket = new WebSocket(buildWSURL(ticket));
    ws = socket;

    socket.addEventListener("open", () => {
      if (ws === socket) setConnected(true);
    });

    socket.addEventListener("close", () => {
      // A socket replaced by reconnect() or closed on cleanup stays closed.
      if (ws !== socket) return;
      ws = null;
      setConnected(false);
      scheduleReconnect();
    });

    socket.addEventListener("error", () => {
      // error is always followed by close, which triggers reconnect
    });

    socket.addEventListener("message", (ev) => {
      for (const listener of listeners) {
        listener(ev);
      }
    });
  }

  function closeCurrent(): void {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    const socket = ws;
    ws = null;
    socket?.close();
    setConnected(false);
  }

  void connect();

  onCleanup(() => {
    disposed = true;
    closeCurrent();
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
      if (isAGUIEvent(msg, type)) {
        handler(msg.payload);
      }
    });
  }

  /** Force-close and reconnect (e.g. after a login or a user change). */
  function reconnect(): void {
    if (disposed) return;
    paused = false;
    closeCurrent();
    void connect();
  }

  /**
   * Close the socket and stop reconnecting until reconnect() (logout). A
   * ticket-bound connection outlives the token it was issued for, so it must be
   * closed explicitly.
   */
  function disconnect(): void {
    paused = true;
    attempt++;
    closeCurrent();
  }

  return { connected, onMessage, onAGUIEvent, reconnect, disconnect } as const;
}
