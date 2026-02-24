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
    bg: "border-cf-success-border bg-cf-success-bg text-cf-success-fg",
    icon: "\u2713",
  },
  error: {
    bg: "border-cf-danger-border bg-cf-danger-bg text-cf-danger-fg",
    icon: "\u2717",
  },
  warning: {
    bg: "border-cf-warning-border bg-cf-warning-bg text-cf-warning-fg",
    icon: "\u26A0",
  },
  info: {
    bg: "border-cf-info-border bg-cf-info-bg text-cf-info-fg",
    icon: "\u2139",
  },
};

function ToastItem(props: { toast: Toast; onDismiss: () => void }): JSX.Element {
  const { t } = useI18n();
  const style = () => levelStyles[props.toast.level];

  return (
    <div
      role={props.toast.level === "error" ? "alert" : "status"}
      class={`pointer-events-auto flex items-start gap-2 rounded-cf-md border-l-4 p-3 shadow-cf-md ${style().bg}`}
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
