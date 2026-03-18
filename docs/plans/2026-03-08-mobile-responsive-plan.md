# Mobile-Responsive Frontend — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the entire CodeForge frontend fully usable on phones (320px+) and tablets (640px+) with optimized layouts, proper touch targets, and iOS safe-area support.

**Architecture:** Bottom-up approach: CSS foundation and shared hook first, then primitives (Button, NavLink), composites (Modal, Table, Card, PageLayout), layout shell (Sidebar, App), page-level grids, and finally complex pages (ProjectDetailPage, ChatPanel, FilePanel).

**Tech Stack:** SolidJS, Tailwind CSS v4, `window.matchMedia` for JS-level breakpoints

**Design Doc:** `docs/specs/2026-03-08-mobile-responsive-design.md`

---

## Task 1: CSS Foundation + `useBreakpoint` Hook

**Files:**
- Create: `frontend/src/hooks/useBreakpoint.ts`
- Modify: `frontend/src/index.css`
- Modify: `frontend/index.html:5`

**Step 1: Create `useBreakpoint` hook**

Create `frontend/src/hooks/useBreakpoint.ts`:

```ts
import { createSignal, onCleanup } from "solid-js";

type Breakpoint = "mobile" | "tablet" | "desktop";

interface BreakpointState {
  isMobile: () => boolean;
  isTablet: () => boolean;
  isDesktop: () => boolean;
  breakpoint: () => Breakpoint;
}

// Singleton state — shared across all consumers
const [bp, setBp] = createSignal<Breakpoint>(detectBreakpoint());

let listenersAttached = false;

function detectBreakpoint(): Breakpoint {
  if (typeof window === "undefined") return "desktop";
  if (window.matchMedia("(max-width: 639px)").matches) return "mobile";
  if (window.matchMedia("(max-width: 1023px)").matches) return "tablet";
  return "desktop";
}

function attachListeners(): void {
  if (listenersAttached) return;
  listenersAttached = true;

  const mqMobile = window.matchMedia("(max-width: 639px)");
  const mqTablet = window.matchMedia("(min-width: 640px) and (max-width: 1023px)");

  const update = () => setBp(detectBreakpoint());

  mqMobile.addEventListener("change", update);
  mqTablet.addEventListener("change", update);
}

export function useBreakpoint(): BreakpointState {
  attachListeners();

  return {
    isMobile: () => bp() === "mobile",
    isTablet: () => bp() === "tablet",
    isDesktop: () => bp() === "desktop",
    breakpoint: bp,
  };
}
```

**Step 2: Add CSS rules to `index.css`**

Append after the `.cf-spinner` block (after line 296) in `frontend/src/index.css`:

```css
/* ---------------------------------------------------------------------------
   Safe area insets for iOS notch / Dynamic Island
   --------------------------------------------------------------------------- */

:root {
  --cf-safe-top: env(safe-area-inset-top, 0px);
  --cf-safe-bottom: env(safe-area-inset-bottom, 0px);
  --cf-safe-left: env(safe-area-inset-left, 0px);
  --cf-safe-right: env(safe-area-inset-right, 0px);
}

/* ---------------------------------------------------------------------------
   Touch-device minimum target sizes (WCAG 2.5.8)
   --------------------------------------------------------------------------- */

@media (pointer: coarse) {
  button,
  [role="button"],
  a[href],
  select,
  input[type="checkbox"],
  input[type="radio"] {
    min-height: 44px;
    min-width: 44px;
  }
}

/* ---------------------------------------------------------------------------
   Hidden scrollbar for horizontal tab strips
   --------------------------------------------------------------------------- */

.scrollbar-none::-webkit-scrollbar {
  display: none;
}

.scrollbar-none {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
```

**Step 3: Update viewport meta tag**

In `frontend/index.html`, change line 5 from:

```html
<meta name="viewport" content="width=device-width, initial-scale=1.0" />
```

to:

```html
<meta name="viewport" content="width=device-width, initial-scale=1.0, viewport-fit=cover" />
```

**Step 4: Verify build compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: no errors

**Step 5: Commit**

```bash
git add frontend/src/hooks/useBreakpoint.ts frontend/src/index.css frontend/index.html
git commit -m "feat(frontend): add useBreakpoint hook, CSS touch targets, safe-area insets"
```

---

## Task 2: Button Touch Targets

**Files:**
- Modify: `frontend/src/ui/primitives/Button.tsx:29-34,60`

**Step 1: Update size classes**

In `frontend/src/ui/primitives/Button.tsx`, replace the `sizeClasses` object (lines 29-34) with:

```ts
const sizeClasses: Record<ButtonSize, string> = {
  xs: "px-2 py-1.5 text-xs min-h-[36px] rounded-cf-sm",
  sm: "px-3 py-2 text-sm min-h-[40px] rounded-cf-sm",
  md: "px-4 py-2.5 text-sm min-h-[44px] rounded-cf-md",
  lg: "px-6 py-3 text-base min-h-[48px] rounded-cf-lg",
};
```

**Step 2: Update icon variant size**

In the same file, line 60, change the icon variant inline from:

```ts
(variant() === "icon" ? "p-1 rounded-cf-sm text-sm" : sizeClasses[size()])
```

to:

```ts
(variant() === "icon" ? "p-2 min-h-[40px] min-w-[40px] rounded-cf-sm text-sm" : sizeClasses[size()])
```

**Step 3: Verify build compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: no errors

**Step 4: Commit**

```bash
git add frontend/src/ui/primitives/Button.tsx
git commit -m "feat(frontend): increase button touch targets to WCAG 2.5.8 minimum"
```

---

## Task 3: NavLink Touch Targets

**Files:**
- Modify: `frontend/src/ui/layout/NavLink.tsx:26-30`

**Step 1: Update NavLink classes**

In `frontend/src/ui/layout/NavLink.tsx`, replace lines 26-30 (inside the `class` attribute of the `<A>` element):

From:

```ts
"block rounded-cf-md text-sm font-medium text-cf-text-secondary hover:bg-cf-bg-surface-alt transition-colors " +
(collapsed()
  ? "flex items-center justify-center p-2"
  : "flex items-center gap-2 px-3 py-2") +
```

To:

```ts
"block rounded-cf-md text-sm font-medium text-cf-text-secondary hover:bg-cf-bg-surface-alt transition-colors " +
(collapsed()
  ? "flex items-center justify-center p-2 min-h-[44px] min-w-[44px]"
  : "flex items-center gap-2 px-3 py-2.5 min-h-[44px]") +
```

**Step 2: Verify build compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: no errors

**Step 3: Commit**

```bash
git add frontend/src/ui/layout/NavLink.tsx
git commit -m "feat(frontend): increase NavLink touch targets to 44px minimum"
```

---

## Task 4: Modal Responsive

**Files:**
- Modify: `frontend/src/ui/composites/Modal.tsx:94,106`

**Step 1: Update modal content wrapper**

In `frontend/src/ui/composites/Modal.tsx`, line 94, change:

```ts
"relative mx-4 max-h-[85vh] w-full max-w-lg overflow-auto rounded-cf-lg border border-cf-border bg-cf-bg-surface shadow-cf-lg" +
```

to:

```ts
"relative mx-3 sm:mx-4 max-h-[85vh] w-full max-w-lg overflow-auto rounded-cf-lg border border-cf-border bg-cf-bg-surface shadow-cf-lg" +
```

**Step 2: Add safe-area padding to content**

In the same file, line 106, change:

```ts
<div class="p-4">{local.children}</div>
```

to:

```ts
<div class="p-4 pb-[max(1rem,env(safe-area-inset-bottom))]">{local.children}</div>
```

**Step 3: Verify build compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: no errors

**Step 4: Commit**

```bash
git add frontend/src/ui/composites/Modal.tsx
git commit -m "feat(frontend): responsive modal margins + iOS safe-area padding"
```

---

## Task 5: Table Responsive Padding

**Files:**
- Modify: `frontend/src/ui/composites/Table.tsx:34,45,79`

**Step 1: Fix overflow direction**

In `frontend/src/ui/composites/Table.tsx`, line 34, change:

```ts
"overflow-auto rounded-cf-lg border border-cf-border" +
```

to:

```ts
"overflow-x-auto rounded-cf-lg border border-cf-border" +
```

**Step 2: Update header cell padding**

Line 45, change:

```ts
"px-4 py-2 text-xs font-medium uppercase tracking-wider text-cf-text-tertiary" +
```

to:

```ts
"px-3 py-2 sm:px-4 text-xs font-medium uppercase tracking-wider text-cf-text-tertiary" +
```

**Step 3: Update body cell padding**

Line 79, change:

```ts
"px-4 py-2 text-cf-text-primary" + (col.class ? " " + col.class : "")
```

to:

```ts
"px-3 py-2 sm:px-4 text-cf-text-primary" + (col.class ? " " + col.class : "")
```

**Step 4: Commit**

```bash
git add frontend/src/ui/composites/Table.tsx
git commit -m "feat(frontend): responsive table padding + explicit overflow-x"
```

---

## Task 6: Card Responsive Padding

**Files:**
- Modify: `frontend/src/ui/composites/Card.tsx:33,45,55`

**Step 1: Update CardHeader padding**

In `frontend/src/ui/composites/Card.tsx`, line 33, change:

```ts
<div class={"border-b border-cf-border px-4 py-3" + (local.class ? " " + local.class : "")}>
```

to:

```ts
<div class={"border-b border-cf-border px-3 py-3 sm:px-4" + (local.class ? " " + local.class : "")}>
```

**Step 2: Update CardBody padding**

Line 45, change:

```ts
return <div class={"px-4 py-4" + (local.class ? " " + local.class : "")}>{local.children}</div>;
```

to:

```ts
return <div class={"px-3 py-3 sm:px-4 sm:py-4" + (local.class ? " " + local.class : "")}>{local.children}</div>;
```

**Step 3: Update CardFooter padding**

Line 55, change:

```ts
<div class={"border-t border-cf-border px-4 py-3" + (local.class ? " " + local.class : "")}>
```

to:

```ts
<div class={"border-t border-cf-border px-3 py-3 sm:px-4" + (local.class ? " " + local.class : "")}>
```

**Step 4: Commit**

```bash
git add frontend/src/ui/composites/Card.tsx
git commit -m "feat(frontend): responsive Card padding (px-3 mobile, px-4 desktop)"
```

---

## Task 7: PageLayout Responsive Header

**Files:**
- Modify: `frontend/src/ui/layout/PageLayout.tsx:16-25`

**Step 1: Update header layout**

In `frontend/src/ui/layout/PageLayout.tsx`, replace lines 15-27 (the inner content):

```ts
return (
  <div class={local.class}>
    <div class="mb-6 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <h1 class="text-xl sm:text-2xl font-bold text-cf-text-primary">{local.title}</h1>
        <Show when={local.description}>
          <p class="mt-1 text-sm text-cf-text-muted">{local.description}</p>
        </Show>
      </div>
      <Show when={local.action}>
        <div class="w-full sm:w-auto shrink-0">{local.action}</div>
      </Show>
    </div>
    {local.children}
  </div>
);
```

**Step 2: Commit**

```bash
git add frontend/src/ui/layout/PageLayout.tsx
git commit -m "feat(frontend): responsive PageLayout header (stacked on mobile)"
```

---

## Task 8: SidebarProvider 3-State Logic

**Files:**
- Modify: `frontend/src/components/SidebarProvider.tsx`

**Step 1: Rewrite SidebarProvider with mobile support**

Replace the entire content of `frontend/src/components/SidebarProvider.tsx`:

```ts
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
    location.pathname; // track
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
```

**Step 2: Verify build compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: no errors

**Step 3: Commit**

```bash
git add frontend/src/components/SidebarProvider.tsx
git commit -m "feat(frontend): SidebarProvider with 3-state mobile/tablet/desktop logic"
```

---

## Task 9: Sidebar Responsive Rendering

**Files:**
- Modify: `frontend/src/ui/layout/Sidebar.tsx`

**Step 1: Rewrite Sidebar with 3-state rendering**

Replace the entire content of `frontend/src/ui/layout/Sidebar.tsx`:

```ts
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
        <div
          class="fixed inset-0 z-40 bg-black/50 transition-opacity"
          onClick={closeMobile}
        />
        {/* Drawer */}
        <aside
          class={
            "fixed inset-y-0 left-0 z-50 flex w-72 flex-col border-r border-cf-border bg-cf-bg-surface shadow-cf-lg transition-transform duration-200 ease-in-out" +
            (local.class ? " " + local.class : "")
          }
          style={{ "padding-top": "var(--cf-safe-top)", "padding-bottom": "var(--cf-safe-bottom)" }}
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
```

**Step 2: Verify build compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: no errors

**Step 3: Commit**

```bash
git add frontend/src/ui/layout/Sidebar.tsx
git commit -m "feat(frontend): responsive Sidebar — hidden+overlay on mobile, collapsed on tablet"
```

---

## Task 10: App Shell — Hamburger + Responsive Padding

**Files:**
- Modify: `frontend/src/App.tsx:71,74,102,108,235`

**Step 1: Add hamburger button and responsive padding**

In `frontend/src/App.tsx`:

1. Add import at top (after line 13):
```ts
import { useBreakpoint } from "~/hooks/useBreakpoint";
```

2. In the `AppShell` function, add breakpoint hook (after line 93, where `useSidebar` is):
```ts
const { isMobile } = useBreakpoint();
```

3. Line 74 — fix text-[10px] to text-xs. Change:
```ts
<span class="rounded bg-cf-bg-surface-alt px-1 py-0.5 text-[10px] font-medium uppercase">
```
to:
```ts
<span class="rounded bg-cf-bg-surface-alt px-1 py-0.5 text-xs font-medium uppercase">
```

4. Line 102 — wrap main content area. Change:
```ts
<div class="flex h-screen flex-col bg-cf-bg-primary text-cf-text-primary">
```
stays the same.

5. Line 235 — update main element. Change:
```ts
<main id="main-content" class="flex-1 overflow-auto p-6">
```
to:
```ts
<main id="main-content" class="flex-1 overflow-auto p-3 sm:p-4 lg:p-6" style={{ "padding-bottom": "max(0.75rem, var(--cf-safe-bottom))" }}>
  <Show when={isMobile()}>
    <div class="mb-3 flex items-center">
      <button
        type="button"
        onClick={() => { const { openMobile } = useSidebar(); openMobile(); }}
        class="rounded-cf-md p-2 min-h-[44px] min-w-[44px] flex items-center justify-center text-cf-text-secondary hover:bg-cf-bg-surface-alt transition-colors"
        aria-label="Open menu"
      >
        <svg class="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6.75h16.5M3.75 12h16.5m-16.5 5.25h16.5" />
        </svg>
      </button>
    </div>
  </Show>
  {props.children}
</main>
```

**Important:** The hamburger button must call `useSidebar().openMobile()`. Since `AppShell` already has access to `useSidebar` via imports, extract `openMobile` at function scope level instead of inline. Refactor the hamburger to use:

```ts
const { collapsed, openMobile } = useSidebar();  // destructure openMobile
```

Then the button onClick becomes: `onClick={openMobile}`

**Step 2: Verify build compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: no errors

**Step 3: Commit**

```bash
git add frontend/src/App.tsx
git commit -m "feat(frontend): hamburger menu on mobile, responsive main padding, safe-area bottom"
```

---

## Task 11: CostDashboardPage Responsive Grids

**Files:**
- Modify: `frontend/src/features/costs/CostDashboardPage.tsx:73,164`

**Step 1: Fix global totals grid**

Line 73, change:
```ts
<div class="mb-6 grid grid-cols-4 gap-4">
```
to:
```ts
<div class="mb-6 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
```

**Step 2: Fix project cost summary grid**

Line 164, change:
```ts
<div class="mb-4 grid grid-cols-4 gap-3">
```
to:
```ts
<div class="mb-4 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
```

**Step 3: Commit**

```bash
git add frontend/src/features/costs/CostDashboardPage.tsx
git commit -m "feat(frontend): responsive cost dashboard grids (1→2→4 columns)"
```

---

## Task 12: CostAnalysisView Responsive

**Files:**
- Modify: `frontend/src/features/benchmarks/CostAnalysisView.tsx:27,70`

**Step 1: Fix select width**

Line 27, change:
```ts
class="w-80"
```
to:
```ts
class="w-full sm:w-80"
```

**Step 2: Fix token totals grid**

Line 70, change:
```ts
<div class="grid grid-cols-2 gap-3">
```
to:
```ts
<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
```

**Step 3: Commit**

```bash
git add frontend/src/features/benchmarks/CostAnalysisView.tsx
git commit -m "feat(frontend): responsive CostAnalysisView select + grid"
```

---

## Task 13: MultiCompareView Responsive SVG

**Files:**
- Modify: `frontend/src/features/benchmarks/MultiCompareView.tsx:53-54`

**Step 1: Fix radar chart container**

Line 53-54, change:
```ts
<div class="flex items-center gap-4">
  <svg viewBox="0 0 300 300" class="h-64 w-64 flex-shrink-0">
```
to:
```ts
<div class="flex flex-col items-center gap-4 sm:flex-row">
  <svg viewBox="0 0 300 300" class="w-full max-w-[16rem] aspect-square flex-shrink-0">
```

**Step 2: Commit**

```bash
git add frontend/src/features/benchmarks/MultiCompareView.tsx
git commit -m "feat(frontend): responsive radar chart SVG (full-width on mobile)"
```

---

## Task 14: PromptEditorPage Responsive Grid

**Files:**
- Modify: `frontend/src/features/prompts/PromptEditorPage.tsx:191`

**Step 1: Fix section form grid**

Line 191, change:
```ts
<div class="grid grid-cols-2 gap-3">
```
to:
```ts
<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
```

**Step 2: Commit**

```bash
git add frontend/src/features/prompts/PromptEditorPage.tsx
git commit -m "feat(frontend): responsive prompt editor form grid"
```

---

## Task 15: WarRoom Responsive Grid

**Files:**
- Modify: `frontend/src/features/project/WarRoom.tsx:85-87`

**Step 1: Replace inline style with Tailwind classes**

Lines 85-87, change:
```ts
<div
  class="grid gap-4"
  style={{ "grid-template-columns": "repeat(auto-fill, minmax(320px, 1fr))" }}
>
```
to:
```ts
<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
```

**Step 2: Commit**

```bash
git add frontend/src/features/project/WarRoom.tsx
git commit -m "feat(frontend): responsive WarRoom grid (Tailwind classes instead of inline style)"
```

---

## Task 16: CompactSettingsPopover Responsive Width

**Files:**
- Modify: `frontend/src/features/project/CompactSettingsPopover.tsx:85`

**Step 1: Fix popover width**

Line 85, change:
```ts
class="absolute right-0 top-full mt-2 w-96 rounded-cf-md border border-cf-border bg-cf-bg-surface shadow-cf-lg z-50 p-4"
```
to:
```ts
class="absolute right-0 top-full mt-2 w-[calc(100vw-2rem)] sm:w-96 max-w-96 rounded-cf-md border border-cf-border bg-cf-bg-surface shadow-cf-lg z-50 p-4"
```

**Step 2: Commit**

```bash
git add frontend/src/features/project/CompactSettingsPopover.tsx
git commit -m "feat(frontend): responsive CompactSettingsPopover width"
```

---

## Task 17: ProjectDetailPage Mobile Redesign

**Files:**
- Modify: `frontend/src/features/project/ProjectDetailPage.tsx`

This is the largest change. Key modifications:

**Step 1: Replace `isNarrow` with `useBreakpoint`**

Remove the custom `isNarrow` signal (lines 82, 93-98) and replace with:

```ts
import { useBreakpoint } from "~/hooks/useBreakpoint";
```

Inside the component function, replace:
```ts
const [isNarrow, setIsNarrow] = createSignal(false);
```
and the `onMount` matchMedia block with:
```ts
const { isMobile, isDesktop } = useBreakpoint();
```

Add a signal for the mobile view toggle:
```ts
type MobileView = "panels" | "chat";
const [mobileView, setMobileView] = createSignal<MobileView>("panels");
```

**Step 2: Responsive header (line 307)**

Change:
```ts
<div class="flex items-center justify-between px-4 py-3 border-b border-cf-border flex-shrink-0">
```
to:
```ts
<div class="flex flex-col gap-2 px-3 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-4 border-b border-cf-border flex-shrink-0">
```

And the action buttons container (line 322):
```ts
<div class="flex items-center gap-2">
```
to:
```ts
<div class="flex flex-wrap items-center gap-2">
```

**Step 3: Scrollable sub-tab strip (line 441)**

Change:
```ts
<div class="flex items-center gap-1">
```
to:
```ts
<div class="flex items-center gap-1 overflow-x-auto scrollbar-none">
```

And add `flex-shrink-0 whitespace-nowrap` to each tab Button's class prop inside this strip.

**Step 4: Mobile layout — tab-switch instead of split**

Replace the main layout section (line 421 onward). The key logic:

- `isMobile()`: Show **either** panels or chat fullscreen, with a bottom tab bar to switch
- `!isDesktop()` (tablet): Vertical stack with 65/35 split
- `isDesktop()`: Side-by-side with drag divider (unchanged)

The bottom tab bar for mobile:
```tsx
<Show when={isMobile()}>
  <div class="flex border-t border-cf-border flex-shrink-0">
    <button
      type="button"
      class={`flex-1 py-3 text-sm font-medium text-center min-h-[48px] transition-colors ${
        mobileView() === "panels"
          ? "text-cf-accent border-t-2 border-cf-accent bg-cf-bg-surface"
          : "text-cf-text-muted hover:text-cf-text-secondary"
      }`}
      onClick={() => setMobileView("panels")}
    >
      {t("detail.tab.panels")}
    </button>
    <button
      type="button"
      class={`flex-1 py-3 text-sm font-medium text-center min-h-[48px] transition-colors ${
        mobileView() === "chat"
          ? "text-cf-accent border-t-2 border-cf-accent bg-cf-bg-surface"
          : "text-cf-text-muted hover:text-cf-text-secondary"
      }`}
      onClick={() => setMobileView("chat")}
    >
      {t("chat.tab")}
    </button>
  </div>
</Show>
```

**Step 5: Replace all `isNarrow()` references**

- `isNarrow()` in flex-col check → `!isDesktop()`
- `isNarrow()` in height style → `!isDesktop()`
- `!isNarrow()` for divider → `isDesktop()`
- On mobile, conditionally show panels vs chat based on `mobileView()`

**Step 6: Add i18n keys**

Add `"detail.tab.panels": "Panels"` to EN and DE locale files (if not already present).

**Step 7: Verify build compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: no errors

**Step 8: Commit**

```bash
git add frontend/src/features/project/ProjectDetailPage.tsx frontend/src/i18n/en.ts frontend/src/i18n/de.ts
git commit -m "feat(frontend): ProjectDetailPage mobile tab-switch, responsive header, scrollable tabs"
```

---

## Task 18: ChatPanel Responsive

**Files:**
- Modify: `frontend/src/features/project/ChatPanel.tsx:310,316,339,392,425,450,462`

**Step 1: Fix agentic badge text size**

Line 310, change:
```ts
text-[11px]
```
to:
```ts
text-xs
```

**Step 2: Responsive header buttons**

Line 316, change:
```ts
<div class="flex items-center gap-3">
```
to:
```ts
<div class="flex flex-wrap items-center gap-2 sm:gap-3">
```

**Step 3: Responsive chat bubbles**

All `max-w-[75%]` occurrences (lines 392, 425, 450, 462) change to:
```ts
max-w-[90%] sm:max-w-[75%]
```

**Step 4: Commit**

```bash
git add frontend/src/features/project/ChatPanel.tsx
git commit -m "feat(frontend): responsive ChatPanel bubbles, header, text fix"
```

---

## Task 19: FilePanel Mobile Drawer

**Files:**
- Modify: `frontend/src/features/project/FilePanel.tsx`

**Step 1: Add `useBreakpoint` import**

```ts
import { useBreakpoint } from "~/hooks/useBreakpoint";
```

**Step 2: Add mobile state inside the component**

Inside the component function, add:
```ts
const { isMobile } = useBreakpoint();
const [fileDrawerOpen, setFileDrawerOpen] = createSignal(false);
```

**Step 3: Wrap file tree in conditional rendering**

The existing layout (line 252 onward) is: file-tree sidebar | drag handle | editor.

For mobile, change to:
- Show a "Files" button at the top of the editor area
- File tree rendered as `fixed` overlay when `fileDrawerOpen()`
- On file select: close drawer and open file

Wrap the file tree sidebar `<div>` and drag handle in a `<Show when={!isMobile()}>` block.

Add mobile overlay before the editor area:
```tsx
<Show when={isMobile()}>
  {/* Mobile file tree toggle */}
  <Show when={!fileDrawerOpen()}>
    <button
      type="button"
      class="flex items-center gap-2 px-3 py-2 text-sm text-cf-text-secondary hover:bg-cf-bg-surface-alt border-b border-cf-border w-full min-h-[44px]"
      onClick={() => setFileDrawerOpen(true)}
    >
      <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M2.25 12.75V12A2.25 2.25 0 0 1 4.5 9.75h15A2.25 2.25 0 0 1 21.75 12v.75m-8.69-6.44-2.12-2.12a1.5 1.5 0 0 0-1.061-.44H4.5A2.25 2.25 0 0 0 2.25 6v12a2.25 2.25 0 0 0 2.25 2.25h15A2.25 2.25 0 0 0 21.75 18V9a2.25 2.25 0 0 0-2.25-2.25h-5.379a1.5 1.5 0 0 1-1.06-.44Z" />
      </svg>
      Files
    </button>
  </Show>

  {/* Mobile file tree overlay */}
  <Show when={fileDrawerOpen()}>
    <div class="fixed inset-0 z-40 bg-black/50" onClick={() => setFileDrawerOpen(false)} />
    <div class="fixed inset-y-0 left-0 z-50 w-72 flex flex-col border-r border-cf-border bg-cf-bg-surface shadow-cf-lg">
      <FileTreeProvider>
        <div class="flex items-center justify-between p-2 border-b border-cf-border">
          <span class="text-sm font-medium px-2">Files</span>
          <button
            type="button"
            class="p-2 min-h-[44px] min-w-[44px] flex items-center justify-center rounded-cf-md text-cf-text-muted hover:bg-cf-bg-surface-alt"
            onClick={() => setFileDrawerOpen(false)}
          >
            {"\u2715"}
          </button>
        </div>
        <SidebarHeader projectId={props.projectId} />
        <SearchInput />
        <div class="flex-1 overflow-y-auto">
          <FileTree
            projectId={props.projectId}
            onFileSelect={(path) => { openFile(path); setFileDrawerOpen(false); }}
            selectedPath={activeTab() ?? undefined}
          />
        </div>
      </FileTreeProvider>
    </div>
  </Show>
</Show>
```

**Step 4: Verify build compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: no errors

**Step 5: Commit**

```bash
git add frontend/src/features/project/FilePanel.tsx
git commit -m "feat(frontend): FilePanel mobile drawer overlay for file tree"
```

---

## Task 20: Final Verification + Pre-Commit

**Step 1: Run TypeScript check**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: no errors

**Step 2: Run ESLint**

Run: `cd /workspaces/CodeForge/frontend && npx eslint src/ --ext .ts,.tsx`
Expected: no errors (or only pre-existing warnings)

**Step 3: Run pre-commit hooks**

Run: `cd /workspaces/CodeForge && pre-commit run --all-files`
Expected: all pass

**Step 4: Run existing E2E tests (smoke)**

Run: `cd /workspaces/CodeForge/frontend && npx playwright test --project=chromium --grep "health|login" --timeout 30000`
Expected: existing tests still pass

**Step 5: Update documentation**

Update `docs/todo.md` — add entry under appropriate section:
```markdown
- [x] (2026-03-08) Mobile-responsive frontend — Tailwind breakpoints for phone/tablet (useBreakpoint hook, sidebar hamburger/overlay, responsive grids, touch targets, ProjectDetailPage mobile tab-switch, FilePanel mobile drawer, safe-area insets)
```

Update `docs/project-status.md` — add milestone entry.

**Step 6: Final commit**

```bash
git add docs/todo.md docs/project-status.md
git commit -m "docs: mark mobile-responsive frontend as complete"
git push
```
