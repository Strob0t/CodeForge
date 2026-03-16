import type { JSX } from "solid-js";
import { Show } from "solid-js";

import { useSidebar } from "~/components/SidebarProvider";
import { useBreakpoint } from "~/hooks/useBreakpoint";

interface NavSectionProps {
  label?: string;
  bottom?: boolean;
  children: JSX.Element;
}

export function NavSection(props: NavSectionProps): JSX.Element {
  const { collapsed } = useSidebar();
  const { isMobile } = useBreakpoint();

  return (
    <div class={props.bottom ? "mt-auto" : ""}>
      <Show when={props.label}>
        <Show
          when={!collapsed() || isMobile()}
          fallback={<div class="mx-2 my-2 border-t border-cf-border" />}
        >
          <div class="px-3 pt-4 pb-1 text-[10px] font-semibold uppercase tracking-wider text-cf-text-muted select-none">
            {props.label}
          </div>
        </Show>
      </Show>
      {props.children}
    </div>
  );
}
