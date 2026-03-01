import { createStore, type SetStoreFunction } from "solid-js/store";

interface FormStateReturn<T extends object> {
  state: T;
  setState: SetStoreFunction<T>;
  /** Reset the form to its default values. */
  reset: () => void;
  /** Populate the form with partial values (e.g. editing an existing entity). */
  populate: (values: Partial<T>) => void;
}

/**
 * Store-based form state with reset and populate helpers.
 *
 * Usage:
 * ```ts
 * const { state, setState, reset, populate } = useFormState({ name: "", email: "" });
 * ```
 */
export function useFormState<T extends object>(defaults: T): FormStateReturn<T> {
  const [state, setState] = createStore<T>({ ...defaults });

  const reset = () => {
    setState(() => ({ ...defaults }));
  };

  const populate = (values: Partial<T>) => {
    setState((prev) => ({ ...prev, ...values }));
  };

  return { state, setState, reset, populate };
}
