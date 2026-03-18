# UX/UI Audit Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix all 10 issues from the UX/UI audit and add strategic improvements (typography, micro-interactions, design system docs, onboarding).

**Architecture:** Pure frontend changes across 17 atomic tasks in 3 layers. All changes use existing SolidJS + Tailwind CSS v4 design token system. No new npm dependencies.

**Tech Stack:** SolidJS 1.9, Tailwind CSS v4, CSS custom properties (`--cf-*`), inline SVG, vanilla JS animations.

**Worktree:** `/workspaces/CodeForge/.worktrees/ux-ui-audit-fixes`
**Branch:** `feature/ux-ui-audit-fixes`
**Spec:** `docs/superpowers/specs/2026-03-18-ux-ui-audit-fixes-design.md`

---

## Layer 1: Quick Wins

### Task Q1: Add Favicon

**Files:**
- Create: `frontend/public/favicon.svg`
- Modify: `frontend/index.html`

- [ ] **Step 1: Create favicon SVG**

Create `frontend/public/favicon.svg` — a monochrome anvil icon:

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" fill="none" stroke="#073642" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
  <!-- Anvil body -->
  <path d="M6 20h20l2 4H4l2-4z" fill="#268bd2" stroke="#073642"/>
  <rect x="8" y="14" width="16" height="6" rx="1" fill="#073642"/>
  <!-- Anvil horn -->
  <path d="M8 17h-5c-1 0-1.5-1-1-2l3-1h3" fill="#073642"/>
  <!-- Hammer spark -->
  <line x1="20" y1="8" x2="22" y2="5" stroke="#268bd2" stroke-width="1.5"/>
  <line x1="23" y1="9" x2="26" y2="7" stroke="#268bd2" stroke-width="1.5"/>
  <line x1="17" y1="6" x2="18" y2="3" stroke="#268bd2" stroke-width="1.5"/>
  <!-- Base -->
  <rect x="10" y="24" width="12" height="3" rx="0.5" fill="#073642"/>
</svg>
```

- [ ] **Step 2: Add link to index.html**

In `frontend/index.html`, add inside `<head>`:

```html
<link rel="icon" type="image/svg+xml" href="/favicon.svg" />
```

- [ ] **Step 3: Verify**

Open browser — favicon visible in tab. No 404 on `/favicon.ico` in console.

- [ ] **Step 4: Commit**

```bash
git add frontend/public/favicon.svg frontend/index.html
git commit -m "feat(ui): add anvil favicon (Q1)"
```

---

### Task Q2: Per-Page Document Titles

**Files:**
- Modify: 15 page components (see list below)

- [ ] **Step 1: Add title to each page**

Add `onMount(() => { document.title = "X - CodeForge"; })` to each page component. Import `onMount` from `solid-js` if not already imported.

Pages and titles:
| File | Title |
|------|-------|
| `frontend/src/features/dashboard/DashboardPage.tsx` | "Dashboard - CodeForge" |
| `frontend/src/features/activity/ActivityPage.tsx` | "Activity - CodeForge" |
| `frontend/src/features/llm/AIConfigPage.tsx` | "AI Config - CodeForge" |
| `frontend/src/features/costs/CostDashboardPage.tsx` | "Costs - CodeForge" |
| `frontend/src/features/knowledge/KnowledgePage.tsx` | "Knowledge - CodeForge" |
| `frontend/src/features/mcp/MCPServersPage.tsx` | "MCP Servers - CodeForge" |
| `frontend/src/features/prompts/PromptEditorPage.tsx` | "Prompts - CodeForge" |
| `frontend/src/features/settings/SettingsPage.tsx` | "Settings - CodeForge" |
| `frontend/src/features/benchmarks/BenchmarkPage.tsx` | "Benchmarks - CodeForge" |
| `frontend/src/features/project/ProjectDetailPage.tsx` | `"${project.name} - CodeForge"` (dynamic) |
| `frontend/src/features/channels/ChannelView.tsx` | "Channel - CodeForge" |
| `frontend/src/features/auth/LoginPage.tsx` | "Sign In - CodeForge" |
| `frontend/src/features/auth/SetupPage.tsx` | "Setup - CodeForge" |
| `frontend/src/features/auth/ChangePasswordPage.tsx` | "Change Password - CodeForge" |
| `frontend/src/features/NotFoundPage.tsx` | "Not Found - CodeForge" |

- [ ] **Step 2: Verify**

Navigate between 3-4 pages — browser tab title updates correctly.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/features/
git commit -m "feat(ui): add per-page document titles (Q2)"
```

---

### Task Q3: Fix Prompts Preview Button

**Files:**
- Modify: `frontend/src/features/prompts/PromptEditorPage.tsx`

- [ ] **Step 1: Change variant**

Find the Preview button (~line 145). Change `variant="ghost"` to `variant="secondary"`.

- [ ] **Step 2: Verify**

Navigate to `/prompts` — "Add Section" (blue filled) and "Preview" (outlined) are both properly styled.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/features/prompts/PromptEditorPage.tsx
git commit -m "fix(ui): change Prompts Preview button to secondary variant (Q3)"
```

---

### Task Q4: Debounce WebSocket Reconnect Banner

**Files:**
- Modify: `frontend/src/components/OfflineBanner.tsx`

- [ ] **Step 1: Read current implementation**

Read `frontend/src/components/OfflineBanner.tsx` to understand the current signal/show pattern.

- [ ] **Step 2: Add debounce logic**

Add a 2-second delay before showing the banner. Pattern:
- Track `mountedAt = Date.now()` in `onMount`
- When disconnect signal fires, start a `setTimeout(2000)` instead of immediately showing
- If reconnect happens within 2s, `clearTimeout` — banner never appears
- Suppress entirely in first 3s after mount (`Date.now() - mountedAt < 3000`)
- Store timeout ref in a `let` variable, clear on reconnect or `onCleanup`

- [ ] **Step 3: Verify**

Navigate between pages — no flash of disconnect banner on page transitions.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/OfflineBanner.tsx
git commit -m "fix(ui): debounce WebSocket reconnect banner by 2s (Q4)"
```

---

### Task Q5: Fix KPI Text Truncation on Mobile

**Files:**
- Modify: `frontend/src/features/dashboard/KpiStrip.tsx`

- [ ] **Step 1: Read current implementation**

Read `frontend/src/features/dashboard/KpiStrip.tsx` to understand the KPI label rendering.

- [ ] **Step 2: Add mobile-friendly labels**

For each KPI item, add a `shortLabel` that is shown on mobile:
- "Success Rate (7d)" → `<span class="hidden sm:inline">Success Rate (7d)</span><span class="sm:hidden">Success 7d</span>`
- "Avg Cost/Run" → short: "Avg Cost"
- "Error Rate (24h)" → short: "Err 24h"
- "Tokens Today" → short: "Tokens"
- "Cost Today" → short: "Cost"
- "Active Runs" → short: "Runs"
- "Active Agents" → short: "Agents"

Also add `min-w-0` to each KPI item container to prevent overflow.

- [ ] **Step 3: Verify**

Resize browser to 375px width — all KPI labels fully visible.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/features/dashboard/KpiStrip.tsx
git commit -m "fix(ui): add abbreviated KPI labels for mobile viewport (Q5)"
```

---

### Task Q6: Hover Effects on Project Cards

**Files:**
- Modify: `frontend/src/features/dashboard/ProjectCard.tsx`

- [ ] **Step 1: Read current implementation**

Read `frontend/src/features/dashboard/ProjectCard.tsx` to understand the card structure and existing click handling.

- [ ] **Step 2: Add hover styles and click handler**

Add to the card root element:
- Classes: `hover:shadow-cf-md hover:border-cf-accent/30 transition-all duration-200 cursor-pointer`
- `onClick` handler: navigate to project detail, but guard against button clicks:

```tsx
const navigate = useNavigate();
const handleCardClick = (e: MouseEvent) => {
  // Don't navigate if clicking Edit/Delete buttons
  if ((e.target as HTMLElement).closest("button")) return;
  navigate(`/projects/${props.project.id}`);
};
```

- [ ] **Step 3: Verify**

Hover over project card — shadow lifts, border tints. Click card body — navigates. Click Edit/Delete — buttons work, no navigation.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/features/dashboard/ProjectCard.tsx
git commit -m "feat(ui): add hover effects and click-to-navigate on project cards (Q6)"
```

---

## Layer 2: Medium-Term

### Task M1: Empty State Illustrations

**Files:**
- Modify: `frontend/src/ui/composites/EmptyState.tsx`
- Create: `frontend/src/ui/icons/EmptyStateIcons.tsx`
- Modify: 6 page files

- [ ] **Step 1: Add illustration prop to EmptyState**

Read `frontend/src/ui/composites/EmptyState.tsx`. Add optional `illustration?: JSX.Element` prop. Render it above the title when provided.

- [ ] **Step 2: Create EmptyStateIcons.tsx**

Create `frontend/src/ui/icons/EmptyStateIcons.tsx` with 6 named SVG component exports. Each SVG is 120x120, uses `currentColor` for outlines and `var(--cf-accent)` for accents:
- `ServerPlugIcon` — for MCP Servers
- `BrainBookIcon` — for Knowledge
- `ChartTrophyIcon` — for Benchmarks
- `DocumentPenIcon` — for Prompts
- `TimelinePulseIcon` — for Activity
- `CoinsWalletIcon` — for Cost Dashboard

- [ ] **Step 3: Wire illustrations into pages**

Update each page's empty state to use `<EmptyState illustration={<Icon />} title="..." description="Get started by..." />`:
- `MCPServersPage.tsx` — `ServerPlugIcon`, "Get started by adding your first MCP server"
- `KnowledgePage.tsx` — `BrainBookIcon`, "Create a knowledge base to enhance agent context"
- `BenchmarkPage.tsx` — `ChartTrophyIcon`, "Run your first benchmark to evaluate agent quality"
- `PromptEditorPage.tsx` — `DocumentPenIcon`, "Add prompt sections to customize agent behavior"
- `ActivityPage.tsx` — `TimelinePulseIcon`, "Activity will appear here as agents work"
- `CostDashboardPage.tsx` — `CoinsWalletIcon`, "Cost data will appear after your first agent run"

- [ ] **Step 4: Verify**

Navigate to each empty page — illustration + guidance text visible. Check light + dark mode.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/ui/composites/EmptyState.tsx frontend/src/ui/icons/EmptyStateIcons.tsx frontend/src/features/
git commit -m "feat(ui): add SVG empty state illustrations for 6 pages (M1)"
```

---

### Task M2: Page Transition Animations

**Files:**
- Create: `frontend/src/ui/layout/PageTransition.tsx`
- Modify: `frontend/src/ui/layout/PageLayout.tsx`

- [ ] **Step 1: Create PageTransition component**

```tsx
import { type JSX, createSignal, onMount } from "solid-js";
import { cx } from "~/utils/cx";

interface Props { children: JSX.Element; class?: string; }

export function PageTransition(props: Props): JSX.Element {
  const [mounted, setMounted] = createSignal(false);
  onMount(() => setMounted(true));

  return (
    <div class={cx(
      mounted() && "animate-[cf-fade-in_0.15s_ease-out]",
      props.class
    )}>
      {props.children}
    </div>
  );
}
```

- [ ] **Step 2: Wrap PageLayout content**

Read `frontend/src/ui/layout/PageLayout.tsx`. Wrap the root `<div>` content in `<PageTransition>`.

- [ ] **Step 3: Verify**

Navigate between pages — content fades in smoothly (~150ms).

- [ ] **Step 4: Commit**

```bash
git add frontend/src/ui/layout/PageTransition.tsx frontend/src/ui/layout/PageLayout.tsx
git commit -m "feat(ui): add page transition fade-in animation (M2)"
```

---

### Task M3: Skeleton Loaders on Data-Heavy Pages

**Files:**
- Modify: `frontend/src/features/llm/AIConfigPage.tsx` (or sub-components)
- Modify: `frontend/src/features/costs/CostDashboardPage.tsx`
- Modify: `frontend/src/features/settings/SettingsPage.tsx`

- [ ] **Step 1: Read loading states in each file**

Read the 3 files and find where "Loading..." text or `<LoadingState>` is used.

- [ ] **Step 2: Replace with skeleton components**

- AI Config: Replace "Loading models..." with `<For each={[1,2,3]}>{() => <SkeletonCard />}</For>`
- Cost Dashboard: Replace empty table state with `<SkeletonTable rows={5} cols={5} />`
- Settings (LLM Proxy section): Replace "Checking connection..." with `<SkeletonCard />`

Import from `~/ui/composites/SkeletonCard` and `~/ui/composites/SkeletonTable`.

- [ ] **Step 3: Verify**

Reload each page — shimmer skeletons visible during data fetch.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/features/llm/ frontend/src/features/costs/ frontend/src/features/settings/
git commit -m "feat(ui): replace loading text with skeleton components (M3)"
```

---

### Task M4: Settings Page Section Navigation

**Files:**
- Modify: `frontend/src/features/settings/SettingsPage.tsx`

- [ ] **Step 1: Read SettingsPage structure**

Read `frontend/src/features/settings/SettingsPage.tsx` — identify all 9 section heading elements.

- [ ] **Step 2: Add section IDs**

Add `id="settings-general"`, `id="settings-shortcuts"`, `id="settings-vcs"`, `id="settings-providers"`, `id="settings-proxy"`, `id="settings-subscriptions"`, `id="settings-apikeys"`, `id="settings-users"`, `id="settings-devtools"` to each section wrapper.

- [ ] **Step 3: Add sticky section nav**

Add a nav bar below the heading:

```tsx
const sections = [
  { id: "settings-general", label: "General" },
  { id: "settings-shortcuts", label: "Shortcuts" },
  { id: "settings-vcs", label: "VCS" },
  { id: "settings-providers", label: "Providers" },
  { id: "settings-proxy", label: "LLM Proxy" },
  { id: "settings-subscriptions", label: "Subscriptions" },
  { id: "settings-apikeys", label: "API Keys" },
  { id: "settings-users", label: "Users" },
  { id: "settings-devtools", label: "Dev Tools" },
];

const [activeSection, setActiveSection] = createSignal("settings-general");

onMount(() => {
  const observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (entry.isIntersecting) setActiveSection(entry.target.id);
      }
    },
    { rootMargin: "-20% 0px -80% 0px" }
  );
  for (const s of sections) {
    const el = document.getElementById(s.id);
    if (el) observer.observe(el);
  }
  onCleanup(() => observer.disconnect());
});
```

Render as:
```tsx
<nav class="sticky top-0 z-10 bg-cf-bg-primary/95 backdrop-blur-sm border-b border-cf-border overflow-x-auto whitespace-nowrap flex gap-1 py-2 mb-4">
  <For each={sections}>{(s) =>
    <button
      class={cx("px-3 py-1 text-sm rounded-cf-sm transition-colors",
        activeSection() === s.id ? "bg-cf-accent text-cf-accent-fg" : "text-cf-text-secondary hover:bg-cf-bg-surface-alt")}
      onClick={() => document.getElementById(s.id)?.scrollIntoView({ behavior: "smooth" })}
    >{s.label}</button>
  }</For>
</nav>
```

- [ ] **Step 4: Verify**

Scroll settings page — nav highlights active section. Click nav item — smooth scroll. Mobile: nav scrolls horizontally.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/settings/SettingsPage.tsx
git commit -m "feat(ui): add sticky section navigation to Settings page (M4)"
```

---

### Task M5: Project Detail Graceful Degradation

**Files:**
- Modify: `frontend/src/features/project/ProjectDetailPage.tsx`

- [ ] **Step 1: Read current data fetching**

Read `frontend/src/features/project/ProjectDetailPage.tsx` — understand how data is fetched and how the error boundary triggers.

- [ ] **Step 2: Refactor to independent resources**

Split the single data fetch into independent `createResource` calls per sub-section:
- Project info: `GET /api/v1/projects/:id` (required — if this fails, show full error)
- Conversations: `GET /api/v1/projects/:id/conversations` (optional — show ErrorBanner if fails)
- Roadmap: `GET /api/v1/projects/:id/roadmap` (optional — show ErrorBanner if fails)

Each optional section: `<Show when={!resource.error} fallback={<ErrorBanner error={resource.error} onRetry={refetch} />}>{content}</Show>`

- [ ] **Step 3: Verify**

Navigate to a project — page renders even when roadmap/conversations return 500. Working sections display normally. Failed sections show inline error with retry.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/features/project/ProjectDetailPage.tsx
git commit -m "fix(ui): graceful degradation on Project Detail page (M5)"
```

---

### Task M6: Sidebar Brand Mark

**Files:**
- Create: `frontend/src/ui/icons/CodeForgeLogo.tsx`
- Modify: `frontend/src/ui/layout/Sidebar.tsx`

- [ ] **Step 1: Create logo component**

Create `frontend/src/ui/icons/CodeForgeLogo.tsx` — reuse Q1's anvil SVG design, sized for sidebar (accepts `size` prop, default 24). Use `currentColor` for theme compatibility.

- [ ] **Step 2: Update Sidebar header**

Read `frontend/src/ui/layout/Sidebar.tsx`. Find where "CodeForge v0.1.0" is rendered in the header. Replace with:
- `<CodeForgeLogo size={collapsed() ? 24 : 28} />`
- `<Show when={!collapsed()}><span class="font-bold text-cf-text-primary">CodeForge</span></Show>`
- Wrap version in `<Tooltip content="CodeForge v0.1.0">` on the logo (for collapsed state)

- [ ] **Step 3: Verify**

Sidebar expanded: logo + "CodeForge" text. Collapsed: logo only. Hover collapsed logo: tooltip shows version.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/ui/icons/CodeForgeLogo.tsx frontend/src/ui/layout/Sidebar.tsx
git commit -m "feat(ui): add anvil brand mark to sidebar header (M6)"
```

---

### Task M7: Models Page Collapse/Expand

**Files:**
- Modify: The models list component in AI Config (find the exact file by reading `AIConfigPage.tsx`)

- [ ] **Step 1: Read models rendering**

Read `frontend/src/features/llm/AIConfigPage.tsx` and trace how model cards are rendered.

- [ ] **Step 2: Add collapse/expand**

For each model card:
- Default state: show only model name (h3) + provider badge
- Collapsed: hide all property badges (id, key, costs)
- Toggle via click on card or a chevron button
- Use a `Set<string>` signal tracking expanded model IDs

Add "Expand All / Collapse All" toggle at the top:
```tsx
const [expandedAll, setExpandedAll] = createSignal(false);
<Button variant="ghost" size="xs" onClick={() => setExpandedAll(!expandedAll())}>
  {expandedAll() ? "Collapse All" : "Expand All"}
</Button>
```

- [ ] **Step 3: Verify**

AI Config Models tab — models show name + provider only by default. Click model — details expand. "Expand All" shows all.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/features/llm/
git commit -m "feat(ui): collapsible model cards on AI Config page (M7)"
```

---

## Layer 3: Strategic

### Task S1: Typography System

**Files:**
- Create: `frontend/public/fonts/` (woff2 files)
- Modify: `frontend/src/index.css`

- [ ] **Step 1: Download fonts**

Download woff2 files for the display font (e.g. Outfit or Plus Jakarta Sans) and IBM Plex Sans (body). Place in `frontend/public/fonts/`. Get Regular (400), Medium (500), Bold (700) weights.

- [ ] **Step 2: Add @font-face rules**

In `frontend/src/index.css`, add before the `@theme` block:

```css
@font-face {
  font-family: "Outfit";
  src: url("/fonts/Outfit-Regular.woff2") format("woff2");
  font-weight: 400;
  font-style: normal;
  font-display: swap;
}
/* ... repeat for 500, 700 weights */

@font-face {
  font-family: "IBM Plex Sans";
  src: url("/fonts/IBMPlexSans-Regular.woff2") format("woff2");
  font-weight: 400;
  font-style: normal;
  font-display: swap;
}
/* ... repeat for 500, 700 weights */
```

- [ ] **Step 3: Add CSS variables and apply**

In `@theme` block:
```css
--cf-font-display: "Outfit", system-ui, sans-serif;
--cf-font-body: "IBM Plex Sans", system-ui, sans-serif;
```

In global styles:
```css
body { font-family: var(--cf-font-body); }
h1, h2, h3, h4 { font-family: var(--cf-font-display); }
```

- [ ] **Step 4: Verify**

Check all 13 pages in light + dark mode. Headings use display font, body uses body font. No FOUT.

- [ ] **Step 5: Commit**

```bash
git add frontend/public/fonts/ frontend/src/index.css
git commit -m "feat(ui): add Outfit + IBM Plex Sans typography system (S1)"
```

---

### Task S2: Micro-Interactions & Polish

**Files:** See spec for per-sub-item file list.

- [ ] **Step 1: Button press feedback**

In `frontend/src/ui/primitives/Button.tsx`, add `active:scale-[0.98] transition-transform` to base button classes.

- [ ] **Step 2: Card hover lift**

In `frontend/src/ui/composites/Card.tsx`, add `hover:-translate-y-0.5 transition-transform duration-200` to root Card class.

- [ ] **Step 3: Toast entrance animation**

In `frontend/src/components/Toast.tsx`, add entrance transition: start with `translate-x-full opacity-0`, transition to `translate-x-0 opacity-100` via a mounted signal.

- [ ] **Step 4: Modal entrance animation**

In `frontend/src/ui/composites/Modal.tsx`, add entrance transition: start with `opacity-0 scale-95`, transition to `opacity-100 scale-100` via a mounted signal + `transition-all duration-200`.

- [ ] **Step 5: Tab underline animation**

Find the tab component. Add `transition-all duration-200` to the active tab's border-bottom indicator.

- [ ] **Step 6: KPI count-up animation**

In `frontend/src/features/dashboard/KpiStrip.tsx`, add a simple `requestAnimationFrame` counter that animates numeric values from 0 to target over ~300ms on mount.

- [ ] **Step 7: Verify all interactions**

Test each: button press, card hover, toast, modal, tab switch, KPI load. All should feel smooth.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/ui/ frontend/src/components/Toast.tsx frontend/src/features/dashboard/
git commit -m "feat(ui): add micro-interactions — press, hover, transitions (S2)"
```

---

### Task S3: Design System Documentation

**Files:**
- Create: `frontend/src/ui/DESIGN-SYSTEM.md`
- Create: `frontend/src/features/dev/DesignSystemPage.tsx`
- Modify: `frontend/src/index.tsx`

- [ ] **Step 1: Create DesignSystemPage component**

Build a comprehensive component gallery page rendering all UI primitives and composites with example usage. Group by: Colors, Typography, Buttons, Badges, Cards, Alerts, EmptyState, Skeletons, Inputs, StatusDots, Spinners, Toasts, Modals, Tooltips.

- [ ] **Step 2: Add dev-mode route**

In `frontend/src/index.tsx`, add:
```tsx
<Route path="/design-system" component={DesignSystemPage} />
```

Guard in the component with `devMode()` check — redirect to `/` if not dev mode.

- [ ] **Step 3: Write DESIGN-SYSTEM.md**

Document: token naming conventions (`--cf-{category}-{variant}`), component API summary, usage guidelines, theme customization.

- [ ] **Step 4: Verify**

Navigate to `/design-system` — all components render in both light and dark mode.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/dev/ frontend/src/ui/DESIGN-SYSTEM.md frontend/src/index.tsx
git commit -m "feat(ui): add living design system page and documentation (S3)"
```

---

### Task S4: Onboarding Wizard

**Files:**
- Create: `frontend/src/features/onboarding/OnboardingWizard.tsx`
- Create: `frontend/src/features/onboarding/steps/ConnectCodeStep.tsx`
- Create: `frontend/src/features/onboarding/steps/ConfigureAIStep.tsx`
- Create: `frontend/src/features/onboarding/steps/CreateProjectStep.tsx`
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Create step components**

Each step is a standalone form (~50-80 lines) calling API endpoints directly:
- `ConnectCodeStep`: Provider dropdown + Token input → `POST /api/v1/vcs-accounts`
- `ConfigureAIStep`: Model name input + "Discover" button → `POST /api/v1/models/discover`
- `CreateProjectStep`: Name + Repo URL → `POST /api/v1/projects`

Each exports `{ component, title, description }`.

- [ ] **Step 2: Create OnboardingWizard**

Full-screen overlay with centered card (max-w-lg). Step indicator dots. Navigation buttons. Skip link. Store completion in localStorage `codeforge-onboarding-completed`.

- [ ] **Step 3: Wire into App.tsx**

Show wizard after login when: `!localStorage.getItem("codeforge-onboarding-completed")` AND projects API returns empty list. Render as overlay above main content.

- [ ] **Step 4: Verify**

Clear localStorage, ensure 0 projects — wizard appears. Complete steps — wizard dismissed. Reload — stays dismissed. "Skip" also dismisses.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/onboarding/ frontend/src/App.tsx
git commit -m "feat(ui): add 3-step onboarding wizard for first-time users (S4)"
```
