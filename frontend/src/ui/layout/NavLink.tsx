import { A } from "@solidjs/router";
import { type JSX, Show, splitProps } from "solid-js";

import { useSidebar } from "~/components/SidebarProvider";

export interface NavLinkProps {
  href: string;
  end?: boolean;
  icon?: JSX.Element;
  class?: string;
  children: JSX.Element;
}

export function NavLink(props: NavLinkProps): JSX.Element {
  const [local, rest] = splitProps(props, ["href", "end", "icon", "class", "children"]);
  const { collapsed } = useSidebar();

  return (
    <A
      {...rest}
      href={local.href}
      end={local.end}
      class={
        "group relative block rounded-cf-md text-sm font-medium text-cf-text-secondary hover:bg-cf-bg-surface-alt transition-colors " +
        (collapsed()
          ? "flex items-center justify-center p-2"
          : "flex items-center gap-2 px-3 py-2") +
        (local.class ? " " + local.class : "")
      }
      activeClass="bg-cf-bg-surface-alt text-cf-accent"
    >
      <Show when={local.icon}>
        <span class="flex-shrink-0">{local.icon}</span>
      </Show>
      <Show when={!collapsed()}>
        <span class="truncate">{local.children}</span>
      </Show>
      <Show when={collapsed()}>
        <span
          role="tooltip"
          class="pointer-events-none absolute left-full ml-2 whitespace-nowrap rounded-cf-md bg-cf-bg-surface-alt px-2 py-1 text-xs text-cf-text-primary shadow-md opacity-0 transition-opacity group-hover:opacity-100 z-50"
        >
          {local.children}
        </span>
      </Show>
    </A>
  );
}
