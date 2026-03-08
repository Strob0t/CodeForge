import { createContext, createSignal, type JSX, type ParentProps, useContext } from "solid-js";

import { SIDEBAR_COLLAPSED_KEY } from "~/config/constants";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface SidebarContextValue {
  collapsed: () => boolean;
  setCollapsed: (v: boolean) => void;
  toggle: () => void;
}

// ---------------------------------------------------------------------------
// Context
// ---------------------------------------------------------------------------

const SidebarContext = createContext<SidebarContextValue>();

export function useSidebar(): SidebarContextValue {
  const ctx = useContext(SidebarContext);
  if (!ctx) throw new Error("useSidebar must be used within <SidebarProvider>");
  return ctx;
}

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

function loadCollapsed(): boolean {
  if (typeof window === "undefined") return false;
  const stored = localStorage.getItem(SIDEBAR_COLLAPSED_KEY);
  if (stored !== null) return stored === "true";
  return window.matchMedia("(max-width: 768px)").matches;
}

export function SidebarProvider(props: ParentProps): JSX.Element {
  const [collapsed, setCollapsedSignal] = createSignal(loadCollapsed());

  function setCollapsed(v: boolean): void {
    setCollapsedSignal(v);
    localStorage.setItem(SIDEBAR_COLLAPSED_KEY, String(v));
  }

  function toggle(): void {
    setCollapsed(!collapsed());
  }

  const ctx: SidebarContextValue = { collapsed, setCollapsed, toggle };

  return <SidebarContext.Provider value={ctx}>{props.children}</SidebarContext.Provider>;
}
