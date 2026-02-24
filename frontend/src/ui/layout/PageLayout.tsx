import { type JSX, Show, splitProps } from "solid-js";

export interface PageLayoutProps {
  title: string;
  description?: string;
  action?: JSX.Element;
  class?: string;
  children: JSX.Element;
}

export function PageLayout(props: PageLayoutProps): JSX.Element {
  const [local] = splitProps(props, ["title", "description", "action", "class", "children"]);

  return (
    <div class={local.class}>
      <div class="mb-6 flex items-start justify-between">
        <div>
          <h1 class="text-2xl font-bold text-cf-text-primary">{local.title}</h1>
          <Show when={local.description}>
            <p class="mt-1 text-sm text-cf-text-muted">{local.description}</p>
          </Show>
        </div>
        <Show when={local.action}>
          <div class="shrink-0">{local.action}</div>
        </Show>
      </div>
      {local.children}
    </div>
  );
}
