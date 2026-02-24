import { type JSX, Show, splitProps } from "solid-js";

export interface EmptyStateProps {
  title: string;
  description?: string;
  action?: JSX.Element;
  class?: string;
}

export function EmptyState(props: EmptyStateProps): JSX.Element {
  const [local] = splitProps(props, ["title", "description", "action", "class"]);

  return (
    <div
      class={
        "flex flex-col items-center justify-center py-12 text-center" +
        (local.class ? " " + local.class : "")
      }
    >
      <p class="text-lg font-medium text-cf-text-secondary">{local.title}</p>
      <Show when={local.description}>
        <p class="mt-1 text-sm text-cf-text-muted">{local.description}</p>
      </Show>
      <Show when={local.action}>
        <div class="mt-4">{local.action}</div>
      </Show>
    </div>
  );
}
