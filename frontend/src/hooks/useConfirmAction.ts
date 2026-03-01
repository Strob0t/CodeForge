import type { Accessor } from "solid-js";
import { createSignal } from "solid-js";

interface ConfirmActionReturn<T> {
  /** The item awaiting confirmation, or null. */
  target: Accessor<T | null>;
  /** Request confirmation for an item (shows the confirm dialog). */
  requestConfirm: (item: T) => void;
  /** Execute the confirmed action and clear the target. */
  confirm: () => Promise<void>;
  /** Cancel confirmation and clear the target. */
  cancel: () => void;
}

/**
 * Replaces the deleteTarget/confirm/cancel pattern used in CRUD pages.
 *
 * Usage:
 * ```ts
 * const del = useConfirmAction(async (item: Server) => {
 *   await api.delete(item.id);
 *   refetch();
 * });
 * // In JSX: onClick={() => del.requestConfirm(server)}
 * // ConfirmDialog: open={del.target() !== null} onConfirm={del.confirm} onCancel={del.cancel}
 * ```
 */
export function useConfirmAction<T>(action: (item: T) => Promise<void>): ConfirmActionReturn<T> {
  const [target, setTarget] = createSignal<T | null>(null);

  const requestConfirm = (item: T) => {
    setTarget(() => item);
  };

  const confirm = async () => {
    const item = target();
    if (item === null) return;
    setTarget(null);
    await action(item);
  };

  const cancel = () => {
    setTarget(null);
  };

  return { target, requestConfirm, confirm, cancel };
}
