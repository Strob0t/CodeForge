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
  const ws = createReconnectingWS(buildWSURL, undefined, {
    delay: 1000,
    retries: Infinity,
  });

  const [connected, setConnected] = createSignal(false);

  const onOpen = () => setConnected(true);
  const onClose = () => setConnected(false);

  // The reconnecting WS returns a reactive `state` accessor
  // but we also track open/close via the underlying WebSocket events.
  // createReconnectingWS returns a getter for the underlying WebSocket instance.

  // Poll state from the reactive primitive
  const interval = setInterval(() => {
    setConnected(ws() !== undefined && ws()?.readyState === WebSocket.OPEN);
  }, 2000);

  onCleanup(() => clearInterval(interval));

  function onMessage(handler: (msg: WSMessage) => void): () => void {
    const listener = (ev: MessageEvent) => {
      const data = JSON.parse(ev.data as string) as WSMessage;
      handler(data);
    };

    // Since the WS instance may reconnect, we need to track it reactively.
    // For now, add listener to the current instance and re-add on reconnect.
    let currentWS: WebSocket | undefined;

    const check = setInterval(() => {
      const instance = ws();
      if (instance && instance !== currentWS) {
        currentWS?.removeEventListener("message", listener);
        instance.addEventListener("open", onOpen);
        instance.addEventListener("close", onClose);
        instance.addEventListener("message", listener);
        currentWS = instance;
      }
    }, 500);

    return () => {
      clearInterval(check);
      currentWS?.removeEventListener("message", listener);
      currentWS?.removeEventListener("open", onOpen);
      currentWS?.removeEventListener("close", onClose);
    };
  }

  return { connected, onMessage } as const;
}
