import { createContext, createSignal, type JSX, type ParentProps, useContext } from "solid-js";

import { ConfirmDialog } from "~/ui";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface ConfirmOptions {
  title: string;
  message: string | JSX.Element;
  variant?: "danger" | "primary";
  confirmLabel?: string;
  cancelLabel?: string;
}

interface ConfirmContextValue {
  /** Show a confirmation dialog. Resolves `true` on confirm, `false` on cancel. */
  confirm: (options: ConfirmOptions) => Promise<boolean>;
}

// ---------------------------------------------------------------------------
// Context
// ---------------------------------------------------------------------------

const ConfirmContext = createContext<ConfirmContextValue>();

export function useConfirm(): ConfirmContextValue {
  const ctx = useContext(ConfirmContext);
  if (!ctx) throw new Error("useConfirm must be used within <ConfirmProvider>");
  return ctx;
}

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

interface PendingConfirm {
  options: ConfirmOptions;
  resolve: (value: boolean) => void;
}

export function ConfirmProvider(props: ParentProps): JSX.Element {
  const [pending, setPending] = createSignal<PendingConfirm | null>(null);

  function confirm(options: ConfirmOptions): Promise<boolean> {
    return new Promise<boolean>((resolve) => {
      setPending({ options, resolve });
    });
  }

  function handleConfirm() {
    const p = pending();
    if (p) {
      setPending(null);
      p.resolve(true);
    }
  }

  function handleCancel() {
    const p = pending();
    if (p) {
      setPending(null);
      p.resolve(false);
    }
  }

  const ctx: ConfirmContextValue = { confirm };

  return (
    <ConfirmContext.Provider value={ctx}>
      {props.children}
      <ConfirmDialog
        open={pending() !== null}
        title={pending()?.options.title ?? ""}
        message={pending()?.options.message ?? ""}
        variant={pending()?.options.variant ?? "primary"}
        confirmLabel={pending()?.options.confirmLabel}
        cancelLabel={pending()?.options.cancelLabel}
        onConfirm={handleConfirm}
        onCancel={handleCancel}
      />
    </ConfirmContext.Provider>
  );
}
