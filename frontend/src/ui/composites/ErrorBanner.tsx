import type { Accessor, JSX } from "solid-js";
import { Show } from "solid-js";

import { Alert } from "../primitives/Alert";

interface ErrorBannerProps {
  error: Accessor<string>;
  onDismiss?: () => void;
  class?: string;
}

/**
 * Small component replacing the repetitive `<Show when={error()}><Alert variant="error">...</Alert></Show>` pattern.
 *
 * Usage:
 * ```tsx
 * <ErrorBanner error={error} onDismiss={clearError} />
 * ```
 */
export function ErrorBanner(props: ErrorBannerProps): JSX.Element {
  return (
    <Show when={props.error()}>
      <Alert variant="error" class={props.class ?? "mb-4"} onDismiss={props.onDismiss}>
        {props.error()}
      </Alert>
    </Show>
  );
}
