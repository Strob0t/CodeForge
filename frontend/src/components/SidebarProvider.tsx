import { useLocation } from "@solidjs/router";
import {
  createContext,
  createEffect,
  createSignal,
  type JSX,
  type ParentProps,
  useContext,
} from "solid-js";

import { SIDEBAR_COLLAPSED_KEY } from "~/config/constants";
import { useBreakpoint } from "~/hooks/useBreakpoint";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface SidebarContextValue {
  collapsed: () => boolean;
  setCollapsed: (v: boolean) => void;
  toggle: () => void;
  isMobile: () => boolean;
  mobileOpen: () => boolean;
  openMobile: () => void;
  closeMobile: () => void;
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
  // Default: collapsed on tablet, expanded on desktop
  return window.matchMedia("(max-width: 1023px)").matches;
}

export function SidebarProvider(props: ParentProps): JSX.Element {
  const { isMobile } = useBreakpoint();
  const location = useLocation();

  const [collapsed, setCollapsedSignal] = createSignal(loadCollapsed());
  const [mobileOpen, setMobileOpen] = createSignal(false);

  function setCollapsed(v: boolean): void {
    setCollapsedSignal(v);
    localStorage.setItem(SIDEBAR_COLLAPSED_KEY, String(v));
  }

  function toggle(): void {
    setCollapsed(!collapsed());
  }

  function openMobile(): void {
    setMobileOpen(true);
  }

  function closeMobile(): void {
    setMobileOpen(false);
  }

  // Auto-close mobile sidebar on route change
  createEffect(() => {
    void location.pathname; // track reactive dependency
    setMobileOpen(false);
  });

  const ctx: SidebarContextValue = {
    collapsed,
    setCollapsed,
    toggle,
    isMobile,
    mobileOpen,
    openMobile,
    closeMobile,
  };

  return <SidebarContext.Provider value={ctx}>{props.children}</SidebarContext.Provider>;
}
