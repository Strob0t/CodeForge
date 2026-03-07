import { createContext, createSignal, type JSX, onCleanup, useContext } from "solid-js";

import { useWebSocket } from "~/components/WebSocketProvider";

interface ConversationRunContextValue {
  activeRuns: () => Set<string>;
  isRunActive: (id: string) => boolean;
}

const ConversationRunContext = createContext<ConversationRunContextValue>();

/**
 * Global run tracker that survives page navigation.
 * Subscribes to AG-UI run_started/run_finished events at the app root.
 */
export function ConversationRunProvider(props: { children: JSX.Element }): JSX.Element {
  const { onAGUIEvent } = useWebSocket();
  const [activeRuns, setActiveRuns] = createSignal<Set<string>>(new Set());

  const offStarted = onAGUIEvent("agui.run_started", (ev) => {
    const runId = ev.run_id as string;
    setActiveRuns((prev) => {
      const next = new Set(prev);
      next.add(runId);
      return next;
    });
  });

  const offFinished = onAGUIEvent("agui.run_finished", (ev) => {
    const runId = ev.run_id as string;
    setActiveRuns((prev) => {
      const next = new Set(prev);
      next.delete(runId);
      return next;
    });
  });

  onCleanup(() => {
    offStarted();
    offFinished();
  });

  const value: ConversationRunContextValue = {
    activeRuns,
    isRunActive: (id: string) => activeRuns().has(id),
  };

  return (
    <ConversationRunContext.Provider value={value}>
      {props.children}
    </ConversationRunContext.Provider>
  );
}

export function useConversationRuns(): ConversationRunContextValue {
  const ctx = useContext(ConversationRunContext);
  if (!ctx) throw new Error("useConversationRuns must be used within a ConversationRunProvider");
  return ctx;
}
