import { A } from "@solidjs/router";
import { type JSX, Show, splitProps } from "solid-js";

import { useSidebar } from "~/components/SidebarProvider";
import { useBreakpoint } from "~/hooks/useBreakpoint";
import { Tooltip } from "~/ui/primitives/Tooltip";
import { cx } from "~/utils/cx";

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
  const { isMobile } = useBreakpoint();

  const showLabels = (): boolean => !collapsed() || isMobile();

  const LinkContent = (): JSX.Element => (
    <A
      {...rest}
      href={local.href}
      end={local.end}
      class={cx(
        "block rounded-cf-md text-sm font-medium text-cf-text-secondary hover:bg-cf-bg-surface-alt transition-colors",
        showLabels()
          ? "flex items-center gap-2 px-3 py-2.5 min-h-[44px]"
          : "flex items-center justify-center p-2 min-h-[44px] min-w-[44px]",
        local.class,
      )}
      activeClass="bg-cf-bg-surface-alt text-cf-accent"
    >
      <Show when={local.icon}>
        <span class="flex-shrink-0">{local.icon}</span>
      </Show>
      <Show when={showLabels()}>
        <span class="truncate">{local.children}</span>
      </Show>
    </A>
  );

  return (
    <Show when={!showLabels() && local.label} fallback={<LinkContent />}>
      <Tooltip text={local.label ?? ""}>
        <LinkContent />
      </Tooltip>
    </Show>
  );
}
