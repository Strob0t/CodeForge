import { type JSX, Show, splitProps } from "solid-js";

import { useSidebar } from "~/components/SidebarProvider";
import { useI18n } from "~/i18n";

import { CollapseIcon, ExpandIcon } from "./NavIcons";

// ---------------------------------------------------------------------------
// Sidebar compound component (collapsible)
// ---------------------------------------------------------------------------

export interface SidebarProps {
  class?: string;
  children: JSX.Element;
}

function SidebarRoot(props: SidebarProps): JSX.Element {
  const [local] = splitProps(props, ["class", "children"]);
  const { collapsed } = useSidebar();

  return (
    <aside
      class={
        "flex flex-col border-r border-cf-border bg-cf-bg-surface transition-[width] duration-200 ease-in-out overflow-hidden " +
        (collapsed() ? "w-12" : "w-64") +
        (local.class ? " " + local.class : "")
      }
      aria-label="Sidebar"
    >
      {local.children}
    </aside>
  );
}

function SidebarHeader(props: SidebarProps): JSX.Element {
  const [local] = splitProps(props, ["class", "children"]);
  const { collapsed, toggle } = useSidebar();
  const { t } = useI18n();

  return (
    <div
      class={
        "flex items-center " +
        (collapsed() ? "flex-col gap-1 px-1 py-2" : "justify-between p-4") +
        (local.class ? " " + local.class : "")
      }
    >
      <Show when={!collapsed()}>
        <div class="min-w-0">{local.children}</div>
      </Show>
      <button
        type="button"
        onClick={toggle}
        class="flex-shrink-0 rounded-cf-md p-1 text-cf-text-muted hover:bg-cf-bg-surface-alt hover:text-cf-text-secondary transition-colors"
        aria-expanded={!collapsed()}
        aria-label={collapsed() ? t("sidebar.expand") : t("sidebar.collapse")}
        title={collapsed() ? t("sidebar.expand") : t("sidebar.collapse")}
      >
        <Show when={collapsed()} fallback={<CollapseIcon />}>
          <ExpandIcon />
        </Show>
      </button>
    </div>
  );
}

function SidebarNav(props: SidebarProps): JSX.Element {
  const [local] = splitProps(props, ["class", "children"]);
  const { collapsed } = useSidebar();

  return (
    <nav
      class={"flex-1 " + (collapsed() ? "px-1" : "px-3") + (local.class ? " " + local.class : "")}
      aria-label="Main navigation"
    >
      {local.children}
    </nav>
  );
}

function SidebarFooter(props: SidebarProps): JSX.Element {
  const [local] = splitProps(props, ["class", "children"]);
  const { collapsed } = useSidebar();

  return (
    <div
      class={
        "border-t border-cf-border " +
        (collapsed() ? "px-1 py-2" : "p-4") +
        (local.class ? " " + local.class : "")
      }
    >
      {local.children}
    </div>
  );
}

export const Sidebar = Object.assign(SidebarRoot, {
  Header: SidebarHeader,
  Nav: SidebarNav,
  Footer: SidebarFooter,
});
