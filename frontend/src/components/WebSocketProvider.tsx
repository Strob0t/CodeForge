import { createContext, createEffect, type JSX, on, useContext } from "solid-js";

import { getAccessToken } from "~/api/client";
import type { AGUIEventMap, AGUIEventType, WSMessage } from "~/api/websocket";
import { createCodeForgeWS } from "~/api/websocket";

interface WebSocketContextValue {
  connected: () => boolean;
  onMessage: (handler: (msg: WSMessage) => void) => () => void;
  onAGUIEvent: <T extends AGUIEventType>(
    type: T,
    handler: (payload: AGUIEventMap[T]) => void,
  ) => () => void;
}

const WebSocketContext = createContext<WebSocketContextValue>();

/**
 * Singleton WebSocket provider — creates exactly ONE connection for the
 * entire application. All components share it via `useWebSocket()`.
 *
 * Must be rendered inside `<AuthProvider>` so the auth token is available.
 */
export function WebSocketProvider(props: { children: JSX.Element }): JSX.Element {
  const ws = createCodeForgeWS();

  // When the auth token changes (refresh), close + reconnect with the new token.
  // The `reconnect` method on createCodeForgeWS handles this — we trigger it
  // by watching the token signal.
  createEffect(
    on(
      () => getAccessToken(),
      (token, prevToken) => {
        // Skip the initial run and only react to actual changes.
        if (prevToken !== undefined && token !== prevToken && token) {
          ws.reconnect();
        }
      },
    ),
  );

  return <WebSocketContext.Provider value={ws}>{props.children}</WebSocketContext.Provider>;
}

export function useWebSocket(): WebSocketContextValue {
  const ctx = useContext(WebSocketContext);
  if (!ctx) {
    throw new Error("useWebSocket must be used within a WebSocketProvider");
  }
  return ctx;
}
