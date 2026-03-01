import type { Accessor } from "solid-js";
import { createSignal } from "solid-js";

interface AsyncActionOptions {
  /** Called when the action throws. Defaults to extracting error.message. */
  onError?: (err: unknown) => void;
}

interface AsyncActionReturn<TArgs extends unknown[], TResult> {
  run: (...args: TArgs) => Promise<TResult | undefined>;
  loading: Accessor<boolean>;
  error: Accessor<string>;
  clearError: () => void;
}

/**
 * Eliminates the repetitive error/loading/try-catch-finally pattern.
 *
 * Usage:
 * ```ts
 * const { run, loading, error, clearError } = useAsyncAction(
 *   async (id: string) => { await api.delete(id); refetch(); }
 * );
 * ```
 */
export function useAsyncAction<TArgs extends unknown[], TResult>(
  action: (...args: TArgs) => Promise<TResult>,
  options?: AsyncActionOptions,
): AsyncActionReturn<TArgs, TResult> {
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal("");

  const run = async (...args: TArgs): Promise<TResult | undefined> => {
    setError("");
    setLoading(true);
    try {
      const result = await action(...args);
      return result;
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(msg);
      options?.onError?.(err);
      return undefined;
    } finally {
      setLoading(false);
    }
  };

  return { run, loading, error, clearError: () => setError("") };
}
