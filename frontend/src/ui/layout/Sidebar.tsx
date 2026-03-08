import { type JSX, Show, splitProps } from "solid-js";
import { Portal } from "solid-js/web";

import { useSidebar } from "~/components/SidebarProvider";
import { useI18n } from "~/i18n";

import { CollapseIcon, ExpandIcon } from "./NavIcons";

// ---------------------------------------------------------------------------
// Sidebar compound component (responsive: hidden / collapsed / expanded)
// ---------------------------------------------------------------------------

export interface SidebarProps {
  class?: string;
  children: JSX.Element;
}

function SidebarRoot(props: SidebarProps): JSX.Element {
  const [local] = splitProps(props, ["class", "children"]);
  const { collapsed, isMobile, mobileOpen, closeMobile } = useSidebar();

  // Mobile: overlay via Portal
  const MobileOverlay = (): JSX.Element => (
    <Portal>
      <Show when={mobileOpen()}>
        {/* Backdrop */}
        <div class="fixed inset-0 z-40 bg-black/50 transition-opacity" onClick={closeMobile} />
        {/* Drawer */}
        <aside
          class={
            "fixed inset-y-0 left-0 z-50 flex w-72 flex-col border-r border-cf-border bg-cf-bg-surface shadow-cf-lg transition-transform duration-200 ease-in-out" +
            (local.class ? " " + local.class : "")
          }
          style={{
            "padding-top": "var(--cf-safe-top)",
            "padding-bottom": "var(--cf-safe-bottom)",
          }}
          aria-label="Sidebar"
        >
          {local.children}
        </aside>
      </Show>
    </Portal>
  );

  // Tablet / Desktop: inline sidebar
  const InlineSidebar = (): JSX.Element => (
    <aside
      class={
        "flex flex-col border-r border-cf-border bg-cf-bg-surface transition-[width] duration-200 ease-in-out overflow-hidden " +
        (collapsed() ? "w-14" : "w-64") +
        (local.class ? " " + local.class : "")
      }
      aria-label="Sidebar"
    >
      {local.children}
    </aside>
  );

  return (
    <Show when={isMobile()} fallback={<InlineSidebar />}>
      <MobileOverlay />
    </Show>
  );
}

function SidebarHeader(props: SidebarProps): JSX.Element {
  const [local] = splitProps(props, ["class", "children"]);
  const { collapsed, toggle, isMobile, closeMobile } = useSidebar();
  const { t } = useI18n();

  return (
    <div
      class={
        "flex items-center " +
        (isMobile()
          ? "justify-between p-4"
          : collapsed()
            ? "flex-col gap-1 px-1 py-2"
            : "justify-between p-4") +
        (local.class ? " " + local.class : "")
      }
    >
      <Show when={!collapsed() || isMobile()}>
        <div class="min-w-0">{local.children}</div>
      </Show>
      <Show
        when={isMobile()}
        fallback={
          <button
            type="button"
            onClick={toggle}
            class="flex-shrink-0 rounded-cf-md p-1 min-h-[44px] min-w-[44px] flex items-center justify-center text-cf-text-muted hover:bg-cf-bg-surface-alt hover:text-cf-text-secondary transition-colors"
            aria-expanded={!collapsed()}
            aria-label={collapsed() ? t("sidebar.expand") : t("sidebar.collapse")}
            title={collapsed() ? t("sidebar.expand") : t("sidebar.collapse")}
          >
            <Show when={collapsed()} fallback={<CollapseIcon />}>
              <ExpandIcon />
            </Show>
          </button>
        }
      >
        <button
          type="button"
          onClick={closeMobile}
          class="flex-shrink-0 rounded-cf-md p-1 min-h-[44px] min-w-[44px] flex items-center justify-center text-cf-text-muted hover:bg-cf-bg-surface-alt hover:text-cf-text-secondary transition-colors"
          aria-label={t("sidebar.collapse")}
        >
          {"\u2715"}
        </button>
      </Show>
    </div>
  );
}

function SidebarNav(props: SidebarProps): JSX.Element {
  const [local] = splitProps(props, ["class", "children"]);
  const { collapsed, isMobile } = useSidebar();

  return (
    <nav
      class={
        "flex-1 overflow-y-auto " +
        (isMobile() ? "px-3" : collapsed() ? "px-1" : "px-3") +
        (local.class ? " " + local.class : "")
      }
      aria-label="Main navigation"
    >
      {local.children}
    </nav>
  );
}

function SidebarFooter(props: SidebarProps): JSX.Element {
  const [local] = splitProps(props, ["class", "children"]);
  const { collapsed, isMobile } = useSidebar();

  return (
    <div
      class={
        "border-t border-cf-border " +
        (isMobile() ? "p-4" : collapsed() ? "px-1 py-2" : "p-4") +
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
