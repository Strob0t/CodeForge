import { createReconnectingWS } from "@solid-primitives/websocket";
import { createSignal, onCleanup } from "solid-js";

import { getAccessToken } from "~/api/client";

export interface WSMessage {
  type: string;
  payload: Record<string, unknown>;
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
  | "agui.step_finished";

export interface AGUIRunStarted {
  run_id: string;
  thread_id?: string;
  agent_name?: string;
}
export interface AGUIRunFinished {
  run_id: string;
  status: string;
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

function buildWSURL(): string {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const token = getAccessToken();
  const qs = token ? `?token=${encodeURIComponent(token)}` : "";
  return `${proto}//${location.host}/ws${qs}`;
}

export function createCodeForgeWS() {
  const ws = createReconnectingWS(buildWSURL(), undefined, {
    delay: 1000,
    retries: Infinity,
  });

  const [connected, setConnected] = createSignal(false);

  ws.addEventListener("open", () => setConnected(true));
  ws.addEventListener("close", () => setConnected(false));

  // Poll readyState as fallback for reconnection state changes
  const interval = setInterval(() => {
    setConnected(ws.readyState === WebSocket.OPEN);
  }, 2000);

  onCleanup(() => clearInterval(interval));

  function onMessage(handler: (msg: WSMessage) => void): () => void {
    const listener = (ev: MessageEvent) => {
      const data = JSON.parse(ev.data as string) as WSMessage;
      handler(data);
    };

    ws.addEventListener("message", listener);

    return () => {
      ws.removeEventListener("message", listener);
    };
  }

  /** Subscribe to a specific AG-UI event type. Returns an unsubscribe function. */
  function onAGUIEvent(
    type: AGUIEventType,
    handler: (payload: Record<string, unknown>) => void,
  ): () => void {
    return onMessage((msg) => {
      if (msg.type === type) {
        handler(msg.payload);
      }
    });
  }

  return { connected, onMessage, onAGUIEvent } as const;
}
