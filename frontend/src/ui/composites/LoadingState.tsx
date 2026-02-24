import { type JSX, Show, splitProps } from "solid-js";

import { Spinner } from "../primitives/Spinner";

export interface LoadingStateProps {
  message?: string;
  class?: string;
}

export function LoadingState(props: LoadingStateProps): JSX.Element {
  const [local] = splitProps(props, ["message", "class"]);

  return (
    <div
      class={
        "flex flex-col items-center justify-center py-12" + (local.class ? " " + local.class : "")
      }
    >
      <Spinner size="lg" />
      <Show when={local.message}>
        <p class="mt-3 text-sm text-cf-text-muted">{local.message}</p>
      </Show>
    </div>
  );
}
