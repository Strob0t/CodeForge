import { createContext, createEffect, type JSX, on, useContext } from "solid-js";

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
 * The socket is authenticated by a single-use ticket and bound to the user and
 * tenant it was issued for, so an access-token refresh does not affect it. It
 * follows the session instead: `sessionUserID` is the ID of the user the socket
 * is for, or null when there must be no socket (logged out, or the password
 * must be changed, which the ticket endpoint refuses). The socket is closed on
 * null and reopened when the ID changes.
 */
export function WebSocketProvider(props: {
  sessionUserID: () => string | null;
  children: JSX.Element;
}): JSX.Element {
  const ws = createCodeForgeWS();

  createEffect(
    on(
      () => props.sessionUserID(),
      (id, prevID) => {
        if (id === prevID) return;
        if (id === null) {
          ws.disconnect();
        } else if (prevID !== undefined) {
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
