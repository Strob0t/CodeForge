# Mobile-Responsive Frontend — Design Document

**Date:** 2026-03-08
**Status:** Approved
**Scope:** Full mobile + tablet responsiveness for the entire CodeForge frontend (320px+)
**Approach:** Bottom-Up (Primitives → Composites → Layout Shell → Pages)
**Breaking Changes:** Allowed — all layouts optimized for best result + performance

---

## 1. Problem Statement

The CodeForge frontend was built desktop-first. Only 14 of ~95+ component files use any responsive Tailwind breakpoints (`sm:`, `md:`, `lg:`), totaling just 41 breakpoint usages. Key issues:

- **Sidebar** always visible, no hamburger menu, no overlay mode
- **ProjectDetailPage** uses 50/50 height split on mobile — unusable on phones
- **Fixed widths** (`w-96`, `w-80`, `w-64`) cause horizontal scrolling on phones
- **Touch targets** as small as 20px (WCAG minimum: 44px)
- **Text sizes** down to 10px (`text-[10px]`)
- **Grids** with hardcoded column counts (`grid-cols-4`) without responsive variants
- **No safe-area-inset** support for iOS notch/Dynamic Island
- **No `pointer: coarse`** detection for touch vs mouse input
- **Card/Table padding** not responsive

## 2. Target Devices

| Category | Viewport | Examples |
|----------|----------|----------|
| Phone | 320px–639px | iPhone SE, Android phones |
| Tablet | 640px–1023px | iPad Mini, iPad, Android tablets |
| Desktop | 1024px+ | Laptops, monitors |

Breakpoint mapping (Tailwind defaults):
- `sm:` = 640px (phone → tablet transition)
- `md:` = 768px (tablet midpoint)
- `lg:` = 1024px (tablet → desktop transition)
- `xl:` = 1280px (wide desktop)

## 3. Architecture

### 3.1 `useBreakpoint` Hook (NEW)

**File:** `frontend/src/hooks/useBreakpoint.ts`

Reactive breakpoint signals using `window.matchMedia` with SolidJS signals.

```ts
interface BreakpointState {
  isMobile: () => boolean;   // < 640px
  isTablet: () => boolean;   // 640px–1023px
  isDesktop: () => boolean;  // >= 1024px
  breakpoint: () => "mobile" | "tablet" | "desktop";
}
```

- Singleton pattern: one set of `matchMedia` listeners shared across the app
- Replaces `ProjectDetailPage.tsx`'s custom `isNarrow` signal (lines 82–98)
- Used by Sidebar, ProjectDetailPage, FilePanel, and any component needing JS-level breakpoint awareness

### 3.2 CSS Foundation (`index.css`)

Three additions:

```css
/* 1. Safe area insets for iOS notch/Dynamic Island */
:root {
  --cf-safe-top: env(safe-area-inset-top, 0px);
  --cf-safe-bottom: env(safe-area-inset-bottom, 0px);
  --cf-safe-left: env(safe-area-inset-left, 0px);
  --cf-safe-right: env(safe-area-inset-right, 0px);
}

/* 2. Touch-device minimum target sizes (WCAG 2.5.8) */
@media (pointer: coarse) {
  button, [role="button"], a[href],
  select, input[type="checkbox"], input[type="radio"] {
    min-height: 44px;
    min-width: 44px;
  }
}

/* 3. Hidden scrollbar for horizontal tab strips */
.scrollbar-none::-webkit-scrollbar { display: none; }
.scrollbar-none { -ms-overflow-style: none; scrollbar-width: none; }
```

### 3.3 Viewport Meta (`index.html`)

```html
<meta name="viewport" content="width=device-width, initial-scale=1.0, viewport-fit=cover" />
```

`viewport-fit=cover` enables `env(safe-area-inset-*)` on iOS.

---

## 4. Component Changes

### 4.1 Button (`ui/primitives/Button.tsx`)

Overhaul size classes for proper touch targets:

| Size | Before | After |
|------|--------|-------|
| `xs` | `px-1.5 py-0.5 text-xs` (20-24px) | `px-2 py-1.5 text-xs min-h-[36px] rounded-cf-sm` |
| `sm` | `px-2.5 py-1 text-xs` (28-32px) | `px-3 py-2 text-sm min-h-[40px] rounded-cf-sm` |
| `md` | `px-4 py-2 text-sm` (36px) | `px-4 py-2.5 text-sm min-h-[44px] rounded-cf-md` |
| `lg` | `px-6 py-3 text-base` (44px) | `px-6 py-3 text-base min-h-[48px] rounded-cf-lg` |
| `icon` | `p-1` (24px) | `p-2 min-h-[40px] min-w-[40px] rounded-cf-sm` |

The global `@media (pointer: coarse)` rule in `index.css` further boosts all interactive elements to 44px minimum on touch devices.

### 4.2 NavLink (`ui/layout/NavLink.tsx`)

- Collapsed state: `p-2` → `p-2 min-h-[44px] min-w-[44px] flex items-center justify-center`
- Expanded state: `px-3 py-2` → `px-3 py-2.5 min-h-[44px]`

### 4.3 Modal (`ui/composites/Modal.tsx`)

- Margin: `mx-4` → `mx-3 sm:mx-4`
- Add safe-area bottom padding: `pb-[env(safe-area-inset-bottom)]` on content wrapper

### 4.4 Table (`ui/composites/Table.tsx`)

- Wrapper: `overflow-auto` → `overflow-x-auto`
- Cell padding: `px-4 py-2` → `px-3 py-2 sm:px-4` (both `<th>` and `<td>`)

### 4.5 Card (`ui/composites/Card.tsx`)

- `CardBody` padding: `px-4 py-4` → `px-3 py-3 sm:px-4 sm:py-4`

### 4.6 PageLayout (`ui/layout/PageLayout.tsx`)

Header layout responsive:

```
Before: flex items-start justify-between (single row always)

After:  flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between
        Title:  text-2xl → text-xl sm:text-2xl
        Action: shrink-0  → w-full sm:w-auto
```

Mobile: title stacks above action button (full width).
Desktop: side-by-side as before.

### 4.7 Text Size Fixes

| File | Before | After |
|------|--------|-------|
| `App.tsx:74` | `text-[10px]` | `text-xs` |
| `ChatPanel.tsx:310` | `text-[11px]` | `text-xs` |

---

## 5. Layout Shell Changes

### 5.1 Sidebar — 3-State Responsive (`SidebarProvider.tsx` + `Sidebar.tsx`)

| Screen | Behavior |
|--------|----------|
| Phone (< 640px) | **Hidden**. Hamburger button in main header. Tap opens as **full-width overlay** (`fixed inset-y-0 left-0 w-72 z-50`) with backdrop. Tap backdrop or nav item = close. |
| Tablet (640px–1023px) | **Collapsed** (`w-14`). Click toggle expands as overlay (`absolute z-40 w-64`). |
| Desktop (>= 1024px) | **Expanded** (`w-64`). Collapse toggle as before. |

**SidebarProvider changes:**
- Add `isMobile: () => boolean` signal (from `useBreakpoint`)
- Add `mobileOpen: () => boolean` + `openMobile()` / `closeMobile()` methods
- Auto-close mobile sidebar on route change

**Sidebar.tsx changes:**
- Mobile: render as `<Portal>` overlay with backdrop div
- Tablet: collapsed by default, overlay on expand
- Desktop: inline as before
- All states: smooth `transition-transform` animation

### 5.2 App Shell (`App.tsx`)

- **Hamburger button:** Rendered in `<main>` header when `isMobile()`, calls `openMobile()`
- **Main padding:** `p-6` → `p-3 pb-[env(safe-area-inset-bottom)] sm:p-4 lg:p-6`
- **Main margin:** No left margin on mobile (sidebar hidden), normal flow on desktop

---

## 6. Page-Level Changes

### 6.1 Grid Responsiveness — All Pages

| File:Line | Before | After |
|-----------|--------|-------|
| `CostDashboardPage.tsx:73` | `grid-cols-4 gap-4` | `grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4` |
| `CostDashboardPage.tsx:164` | `grid-cols-4 gap-3` | `grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4` |
| `CostAnalysisView.tsx:70` | `grid-cols-2 gap-3` | `grid-cols-1 gap-3 sm:grid-cols-2` |
| `PromptEditorPage.tsx:191` | `grid-cols-2 gap-3` | `grid-cols-1 gap-3 sm:grid-cols-2` |
| `WarRoom.tsx:87` | `style="grid-template-columns: repeat(auto-fill, minmax(320px, 1fr))"` | `class="grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3"` (Tailwind classes, remove inline style) |

### 6.2 Fixed-Width Fixes

| File:Line | Before | After |
|-----------|--------|-------|
| `CompactSettingsPopover.tsx:85` | `w-96` | `w-[calc(100vw-2rem)] sm:w-96 max-w-96` |
| `CostAnalysisView.tsx:27` | `w-80` | `w-full sm:w-80` |
| `MultiCompareView.tsx:54` | `w-64 h-64 flex-shrink-0` | `w-full max-w-[16rem] h-auto aspect-square` |

### 6.3 ProjectDetailPage — Mobile-First Redesign

**Phone (< 640px): Tab-Switch instead of Split**

Instead of showing Roadmap + Chat simultaneously:

- **Bottom tab bar** with 2 primary tabs: "Panels" and "Chat"
- "Panels" shows the left panel content (Roadmap/FeatureMap/Files/WarRoom/Goals/Audit) fullscreen
- "Chat" shows ChatPanel fullscreen
- Sub-tabs (Roadmap, FeatureMap, etc.) rendered in a horizontally scrollable strip: `overflow-x-auto scrollbar-none whitespace-nowrap`
- Uses `useBreakpoint()` instead of custom `isNarrow` signal
- Draggable divider hidden on phone

**Tablet (640px–1023px): Adjustable Stacked Layout**

- Vertical split: 65% top (panels), 35% bottom (chat)
- Toggle button to swap ratio (35/65)
- Draggable divider hidden

**Desktop (>= 1024px): Unchanged**

- Side-by-side with drag divider, as currently implemented

**Header area (lines 358-394):**
- `flex items-center justify-between` → `flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between`
- Auto-Agent button + Settings gear: wrap in `flex flex-wrap gap-2`

### 6.4 ChatPanel Responsive

- Chat bubbles: `max-w-[75%]` → `max-w-[90%] sm:max-w-[75%]`
- Header buttons (Fork, Rewind, Stop): wrap in `flex flex-wrap gap-2`
- Agentic badge: `text-[11px]` → `text-xs`
- Input area: already responsive (`flex-1` textarea), no changes needed

### 6.5 FilePanel — Mobile Drawer

**Phone (< 640px):**
- File tree hidden by default
- Button at top: "Files" → opens file tree as `fixed inset-y-0 left-0 w-72 z-40` overlay
- Tap file → opens in editor, drawer auto-closes
- Draggable divider hidden

**Tablet+:** Unchanged (inline sidebar with drag divider)

---

## 7. Implementation Order (Bottom-Up)

### Step 1: Hook + CSS Foundation
- `frontend/src/hooks/useBreakpoint.ts` (new)
- `frontend/src/index.css` (safe area, touch targets, scrollbar-none)
- `frontend/index.html` (viewport-fit)

### Step 2: Primitives
- `frontend/src/ui/primitives/Button.tsx` (size classes)
- `frontend/src/ui/layout/NavLink.tsx` (touch targets)

### Step 3: Composites
- `frontend/src/ui/composites/Modal.tsx` (responsive margin, safe area)
- `frontend/src/ui/composites/Table.tsx` (overflow-x, responsive padding)
- `frontend/src/ui/composites/Card.tsx` (responsive padding)
- `frontend/src/ui/layout/PageLayout.tsx` (responsive header)

### Step 4: Layout Shell
- `frontend/src/components/SidebarProvider.tsx` (3-state logic)
- `frontend/src/ui/layout/Sidebar.tsx` (hidden/collapsed/expanded)
- `frontend/src/App.tsx` (hamburger, responsive padding, safe area)

### Step 5: Page Grids + Fixed Widths
- `frontend/src/features/costs/CostDashboardPage.tsx` (2x grid)
- `frontend/src/features/benchmarks/CostAnalysisView.tsx` (grid + fixed width)
- `frontend/src/features/benchmarks/MultiCompareView.tsx` (SVG responsive)
- `frontend/src/features/prompts/PromptEditorPage.tsx` (grid)
- `frontend/src/features/project/WarRoom.tsx` (grid classes)
- `frontend/src/features/project/CompactSettingsPopover.tsx` (responsive width)

### Step 6: Complex Pages
- `frontend/src/features/project/ProjectDetailPage.tsx` (mobile tab-switch, header)
- `frontend/src/features/project/ChatPanel.tsx` (responsive bubbles, header, text fix)
- `frontend/src/features/project/FilePanel.tsx` (mobile drawer)

### Step 7: Text Fixes
- `frontend/src/App.tsx:74` (`text-[10px]` → `text-xs`)
- `frontend/src/features/project/ChatPanel.tsx:310` (`text-[11px]` → `text-xs`)

---

## 8. Files Changed Summary

| # | File | Change Type |
|---|------|-------------|
| 1 | `frontend/src/hooks/useBreakpoint.ts` | **New file** |
| 2 | `frontend/index.html` | viewport-fit |
| 3 | `frontend/src/index.css` | Safe area, touch targets, scrollbar-none |
| 4 | `frontend/src/ui/primitives/Button.tsx` | Size classes overhaul |
| 5 | `frontend/src/ui/layout/NavLink.tsx` | Touch target sizing |
| 6 | `frontend/src/ui/composites/Modal.tsx` | Responsive margin + safe area |
| 7 | `frontend/src/ui/composites/Table.tsx` | overflow-x, responsive padding |
| 8 | `frontend/src/ui/composites/Card.tsx` | Responsive body padding |
| 9 | `frontend/src/ui/layout/PageLayout.tsx` | Responsive header layout |
| 10 | `frontend/src/components/SidebarProvider.tsx` | 3-state mobile logic |
| 11 | `frontend/src/ui/layout/Sidebar.tsx` | Hidden/collapsed/expanded states |
| 12 | `frontend/src/App.tsx` | Hamburger, padding, safe area, text fix |
| 13 | `frontend/src/features/costs/CostDashboardPage.tsx` | 2x responsive grid |
| 14 | `frontend/src/features/benchmarks/CostAnalysisView.tsx` | Grid + fixed width |
| 15 | `frontend/src/features/benchmarks/MultiCompareView.tsx` | SVG responsive |
| 16 | `frontend/src/features/prompts/PromptEditorPage.tsx` | Responsive grid |
| 17 | `frontend/src/features/project/WarRoom.tsx` | Tailwind grid classes |
| 18 | `frontend/src/features/project/CompactSettingsPopover.tsx` | Responsive width |
| 19 | `frontend/src/features/project/ProjectDetailPage.tsx` | Mobile tab-switch redesign |
| 20 | `frontend/src/features/project/ChatPanel.tsx` | Responsive bubbles + header |
| 21 | `frontend/src/features/project/FilePanel.tsx` | Mobile drawer |

**Total: 21 files (1 new, 20 modified)**

---

## 9. Testing Strategy

- **Visual regression:** Playwright screenshots at 320px, 640px, 768px, 1024px, 1440px
- **Touch target validation:** Chrome DevTools device toolbar, iOS Safari, Android Chrome
- **Safe area:** iOS Simulator with notch device
- **Existing E2E tests:** Must pass unchanged (chromium at default viewport)
- **Focus areas:** Sidebar open/close, ProjectDetailPage tab switching, ChatPanel input on mobile
