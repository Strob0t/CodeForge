import { A } from "@solidjs/router";
import { type JSX, Show, splitProps } from "solid-js";

import { useSidebar } from "~/components/SidebarProvider";
import { Tooltip } from "~/ui/primitives/Tooltip";

export interface NavLinkProps {
  href: string;
  end?: boolean;
  icon?: JSX.Element;
  label?: string;
  class?: string;
  children: JSX.Element;
}

export function NavLink(props: NavLinkProps): JSX.Element {
  const [local, rest] = splitProps(props, ["href", "end", "icon", "label", "class", "children"]);
  const { collapsed } = useSidebar();

  const LinkContent = (): JSX.Element => (
    <A
      {...rest}
      href={local.href}
      end={local.end}
      class={
        "block rounded-cf-md text-sm font-medium text-cf-text-secondary hover:bg-cf-bg-surface-alt transition-colors " +
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
    </A>
  );

  return (
    <Show when={collapsed() && local.label} fallback={<LinkContent />}>
      <Tooltip text={local.label ?? ""}>
        <LinkContent />
      </Tooltip>
    </Show>
  );
}
