import { createReconnectingWS } from "@solid-primitives/websocket";
import { createSignal, onCleanup } from "solid-js";

export interface WSMessage {
  type: string;
  payload: Record<string, unknown>;
}

function buildWSURL(): string {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${location.host}/ws`;
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

  return { connected, onMessage } as const;
}
