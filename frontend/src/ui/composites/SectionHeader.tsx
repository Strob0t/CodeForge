import { type JSX, Show, splitProps } from "solid-js";

export interface SectionHeaderProps {
  title: string;
  description?: string;
  action?: JSX.Element;
  class?: string;
}

export function SectionHeader(props: SectionHeaderProps): JSX.Element {
  const [local] = splitProps(props, ["title", "description", "action", "class"]);

  return (
    <div class={"flex items-start justify-between" + (local.class ? " " + local.class : "")}>
      <div>
        <h2 class="text-lg font-semibold text-cf-text-primary">{local.title}</h2>
        <Show when={local.description}>
          <p class="mt-0.5 text-sm text-cf-text-muted">{local.description}</p>
        </Show>
      </div>
      <Show when={local.action}>
        <div class="shrink-0">{local.action}</div>
      </Show>
    </div>
  );
}
