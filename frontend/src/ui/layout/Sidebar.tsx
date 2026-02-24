import { type JSX, splitProps } from "solid-js";

// ---------------------------------------------------------------------------
// Sidebar compound component
// ---------------------------------------------------------------------------

export interface SidebarProps {
  class?: string;
  children: JSX.Element;
}

function SidebarRoot(props: SidebarProps): JSX.Element {
  const [local] = splitProps(props, ["class", "children"]);
  return (
    <aside
      class={
        "flex w-64 flex-col border-r border-cf-border bg-cf-bg-surface" +
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
  return <div class={"p-4" + (local.class ? " " + local.class : "")}>{local.children}</div>;
}

function SidebarNav(props: SidebarProps): JSX.Element {
  const [local] = splitProps(props, ["class", "children"]);
  return (
    <nav
      class={"flex-1 px-3" + (local.class ? " " + local.class : "")}
      aria-label="Main navigation"
    >
      {local.children}
    </nav>
  );
}

function SidebarFooter(props: SidebarProps): JSX.Element {
  const [local] = splitProps(props, ["class", "children"]);
  return (
    <div class={"border-t border-cf-border p-4" + (local.class ? " " + local.class : "")}>
      {local.children}
    </div>
  );
}

export const Sidebar = Object.assign(SidebarRoot, {
  Header: SidebarHeader,
  Nav: SidebarNav,
  Footer: SidebarFooter,
});
