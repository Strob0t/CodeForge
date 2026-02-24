import {
  createContext,
  createSignal,
  For,
  type JSX,
  onCleanup,
  type ParentProps,
  useContext,
} from "solid-js";
import { Portal } from "solid-js/web";

import { useI18n } from "~/i18n";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type ToastLevel = "success" | "error" | "warning" | "info";

interface Toast {
  id: number;
  level: ToastLevel;
  message: string;
  dismissMs: number;
}

interface ToastContextValue {
  /** Show a toast. Returns the toast id (can be used to dismiss early). */
  show: (level: ToastLevel, message: string, dismissMs?: number) => number;
  dismiss: (id: number) => void;
}

// ---------------------------------------------------------------------------
// Context
// ---------------------------------------------------------------------------

const ToastContext = createContext<ToastContextValue>();

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used within <ToastProvider>");
  return ctx;
}

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

const MAX_VISIBLE = 3;
const DEFAULT_DISMISS_MS = 5000;

let nextId = 1;

export function ToastProvider(props: ParentProps): JSX.Element {
  const [toasts, setToasts] = createSignal<Toast[]>([]);
  const timers = new Map<number, ReturnType<typeof setTimeout>>();

  function dismiss(id: number) {
    clearTimeout(timers.get(id));
    timers.delete(id);
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }

  function show(
    level: ToastLevel,
    message: string,
    dismissMs: number = DEFAULT_DISMISS_MS,
  ): number {
    const id = nextId++;

    setToasts((prev) => {
      const next = [...prev, { id, level, message, dismissMs }];
      // Evict oldest when exceeding max
      while (next.length > MAX_VISIBLE) {
        const removed = next.shift();
        if (removed) {
          clearTimeout(timers.get(removed.id));
          timers.delete(removed.id);
        }
      }
      return next;
    });

    if (dismissMs > 0) {
      timers.set(
        id,
        setTimeout(() => dismiss(id), dismissMs),
      );
    }

    return id;
  }

  onCleanup(() => {
    for (const timer of timers.values()) clearTimeout(timer);
    timers.clear();
  });

  const ctx: ToastContextValue = { show, dismiss };

  return (
    <ToastContext.Provider value={ctx}>
      {props.children}
      <Portal>
        <div
          class="pointer-events-none fixed right-4 top-16 z-[60] flex w-80 flex-col gap-2"
          aria-live="polite"
        >
          <For each={toasts()}>
            {(toast) => <ToastItem toast={toast} onDismiss={() => dismiss(toast.id)} />}
          </For>
        </div>
      </Portal>
    </ToastContext.Provider>
  );
}

// ---------------------------------------------------------------------------
// Toast item
// ---------------------------------------------------------------------------

const levelStyles: Record<ToastLevel, { bg: string; icon: string }> = {
  success: {
    bg: "border-green-400 bg-green-50 text-green-800 dark:border-green-600 dark:bg-green-900/30 dark:text-green-300",
    icon: "\u2713",
  },
  error: {
    bg: "border-red-400 bg-red-50 text-red-800 dark:border-red-600 dark:bg-red-900/30 dark:text-red-300",
    icon: "\u2717",
  },
  warning: {
    bg: "border-yellow-400 bg-yellow-50 text-yellow-800 dark:border-yellow-600 dark:bg-yellow-900/30 dark:text-yellow-300",
    icon: "\u26A0",
  },
  info: {
    bg: "border-blue-400 bg-blue-50 text-blue-800 dark:border-blue-600 dark:bg-blue-900/30 dark:text-blue-300",
    icon: "\u2139",
  },
};

function ToastItem(props: { toast: Toast; onDismiss: () => void }): JSX.Element {
  const { t } = useI18n();
  const style = () => levelStyles[props.toast.level];

  return (
    <div
      role={props.toast.level === "error" ? "alert" : "status"}
      class={`pointer-events-auto flex items-start gap-2 rounded-lg border-l-4 p-3 shadow-md dark:shadow-gray-900/30 ${style().bg}`}
    >
      <span class="mt-0.5 text-sm font-bold" aria-hidden="true">
        {style().icon}
      </span>
      <p class="flex-1 text-sm">{props.toast.message}</p>
      <button
        type="button"
        class="ml-2 text-sm opacity-60 hover:opacity-100"
        onClick={() => props.onDismiss()}
        aria-label={t("toast.dismiss")}
      >
        &times;
      </button>
    </div>
  );
}
