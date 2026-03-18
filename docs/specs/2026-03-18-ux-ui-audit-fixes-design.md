# UX/UI Audit Fixes — Design Spec

**Date:** 2026-03-18
**Source:** `docs/ux-ui-audit.md` (automated Playwright audit)
**Scope:** Full coverage (17 atomic tasks across 3 layers)
**Execution:** Layer-cake — Quick Wins first, then Medium-Term, then Strategic

---

## Layer 1: Quick Wins (6 tasks, < 2h each)

### Q1: Add Favicon
- **Files:** `frontend/index.html`, new `frontend/public/favicon.svg`
- **What:** Create inline SVG anvil/forge icon. Add `<link rel="icon" type="image/svg+xml" href="/favicon.svg">` to HTML head.
- **Style:** Monochrome, works at 16x16 and 32x32. Uses `currentColor` or fixed dark color. Matches brand theme.
- **Note:** M6 (Sidebar Brand Mark) must reuse this same SVG design for visual consistency. Do Q1 first.
- **Verify:** No more 404 on `/favicon.ico`. Icon visible in browser tab.

### Q2: Per-Page Document Titles
- **Files:** All page components (15 files):
  - `frontend/src/features/dashboard/DashboardPage.tsx` — "Dashboard - CodeForge"
  - `frontend/src/features/activity/ActivityPage.tsx` — "Activity - CodeForge"
  - `frontend/src/features/llm/AIConfigPage.tsx` — "AI Config - CodeForge"
  - `frontend/src/features/costs/CostDashboardPage.tsx` — "Costs - CodeForge"
  - `frontend/src/features/knowledge/KnowledgePage.tsx` — "Knowledge - CodeForge"
  - `frontend/src/features/mcp/MCPServersPage.tsx` — "MCP Servers - CodeForge"
  - `frontend/src/features/prompts/PromptEditorPage.tsx` — "Prompts - CodeForge"
  - `frontend/src/features/settings/SettingsPage.tsx` — "Settings - CodeForge"
  - `frontend/src/features/benchmarks/BenchmarkPage.tsx` — "Benchmarks - CodeForge"
  - `frontend/src/features/project/ProjectDetailPage.tsx` — "{ProjectName} - CodeForge"
  - `frontend/src/features/channels/ChannelView.tsx` — "Channel - CodeForge"
  - `frontend/src/features/auth/LoginPage.tsx` — "Sign In - CodeForge"
  - `frontend/src/features/auth/SetupPage.tsx` — "Setup - CodeForge"
  - `frontend/src/features/auth/ChangePasswordPage.tsx` — "Change Password - CodeForge"
  - `frontend/src/features/NotFoundPage.tsx` — "Not Found - CodeForge"
- **What:** Add `onMount(() => { document.title = "PageName - CodeForge"; })` in each page component.
- **Verify:** Browser tab shows correct title per page. Navigate between pages — title updates.

### Q3: Fix Prompts "Preview" Button
- **Files:** `frontend/src/features/prompts/PromptEditorPage.tsx` (line ~145)
- **What:** Change the Preview button from `variant="ghost"` to `variant="secondary"` for visual consistency with the adjacent "Add Section" primary button.
- **Verify:** "Add Section" (primary/filled blue) and "Preview" (secondary/outlined) are visually distinct but both styled.

### Q4: Debounce WebSocket Reconnect Banner
- **Files:** `frontend/src/components/OfflineBanner.tsx`
- **What:** Add 2-second delay before showing "WebSocket disconnected. Reconnecting..." banner. Use a `setTimeout` + signal pattern: set `showBanner` signal to `true` only after 2s of continuous disconnection. If connection restores within 2s, clear the timeout and never show. Also suppress banner entirely during initial page load (first 3s after component mount via `onMount` timestamp check).
- **Verify:** Navigate between pages — no flash of disconnect banner. Disconnect WebSocket manually — banner appears after 2s delay.

### Q5: Fix KPI Text Truncation on Mobile
- **Files:** `frontend/src/features/dashboard/KpiStrip.tsx` (lines ~53-77)
- **What:** Add `min-w-0` on KPI container items, `truncate` on label text. Provide short labels for mobile via a `shortLabel` prop or conditional rendering at `sm:` breakpoint. Abbreviations: "Success 7d" (was "Success Rate (7d)"), "Avg Cost" (was "Avg Cost/Run"), "Err 24h" (was "Error Rate (24h)"), "Tokens" (was "Tokens Today").
- **Verify:** Mobile viewport (375px) — all KPI labels fully visible, no truncation.

### Q6: Hover Effects on Project Cards
- **Files:** `frontend/src/features/dashboard/ProjectCard.tsx`
- **What:** Add to card wrapper: `hover:shadow-cf-md hover:border-cf-accent/30 transition-all duration-200 cursor-pointer`. Make entire card clickable via `onClick` handler that navigates to project detail, with `e.target` check to exclude clicks on Edit/Delete buttons (check if click target is inside a `button` element via `closest('button')`). Do NOT wrap in `<a>` — the card already contains `<A>` link and `<button>` elements; nesting would violate HTML spec.
- **Verify:** Hover over project card — shadow lifts, border tints blue. Click card body — navigates to project. Click Edit/Delete — does NOT navigate, buttons work normally.

---

## Layer 2: Medium-Term (7 tasks, 2-6h each)

### M1: Empty State Illustrations
- **Files:** `frontend/src/ui/composites/EmptyState.tsx` (add `illustration` prop), new `frontend/src/ui/icons/EmptyStateIcons.tsx`, update 6 pages:
  - `frontend/src/features/mcp/MCPServersPage.tsx` — server/plug icon, "Get started by adding your first MCP server"
  - `frontend/src/features/knowledge/KnowledgePage.tsx` — brain/book icon, "Create a knowledge base to enhance agent context"
  - `frontend/src/features/benchmarks/BenchmarkPage.tsx` — chart/trophy icon, "Run your first benchmark to evaluate agent quality"
  - `frontend/src/features/prompts/PromptEditorPage.tsx` — document/pen icon, "Add prompt sections to customize agent behavior"
  - `frontend/src/features/activity/ActivityPage.tsx` — timeline/pulse icon, "Activity will appear here as agents work"
  - `frontend/src/features/costs/CostDashboardPage.tsx` — coins/wallet icon, "Cost data will appear after your first agent run"
- **Pattern:** Add optional `illustration` prop (JSX.Element) to existing `EmptyState` component. Render above title. Each SVG is a named export from `EmptyStateIcons.tsx`.
- **Size:** Each SVG ~40-60 lines, 120x120px viewport, uses `currentColor` for outlines + `var(--cf-accent)` for accents.
- **Verify:** Each empty page shows illustration + guidance text. Both light and dark mode.

### M2: Page Transition Animations
- **Files:** New `frontend/src/ui/layout/PageTransition.tsx`, update `frontend/src/ui/layout/PageLayout.tsx`
- **What:** Create `<PageTransition>` wrapper that applies `animate-[cf-fade-in_0.15s_ease-out]` on mount. Implementation: use a `mounted` signal (initially `false`), set to `true` in `onMount`, apply animation class conditionally via `cx()`. Wrap the content area in `PageLayout` with this component. The existing `cf-fade-in` keyframe (index.css lines 416-423) handles the animation. No SolidJS `<Transition>` primitive (does not exist in SolidJS) — pure CSS + `onMount` signal.
- **Respects:** `prefers-reduced-motion` already handled in index.css (animations disabled).
- **Verify:** Navigate between pages — content fades in smoothly (~150ms).

### M3: Skeleton Loaders on Data-Heavy Pages
- **Files:**
  - `frontend/src/features/llm/AIConfigPage.tsx` — replace "Loading models..." with `<SkeletonCard />` x3
  - `frontend/src/features/costs/CostDashboardPage.tsx` — replace empty table with `<SkeletonTable />`
  - `frontend/src/features/settings/SettingsPage.tsx` — replace "Checking connection..." (LLM Proxy) with `<SkeletonCard />`
- **What:** Use existing `SkeletonCard` (`frontend/src/ui/composites/SkeletonCard.tsx`) and `SkeletonTable` (`frontend/src/ui/composites/SkeletonTable.tsx`) in loading states. Wrap in `<Show when={loading()} fallback={<ActualContent />}>`.
- **Verify:** Reload each page — shimmer skeletons visible during data fetch.

### M4: Settings Page Section Navigation
- **Files:** `frontend/src/features/settings/SettingsPage.tsx`, `frontend/src/ui/layout/Section.tsx` (add `id` prop if not present)
- **What:** Add sticky horizontal nav bar at top of settings content (below heading). Lists all 9 sections: General, Keyboard Shortcuts, VCS Accounts, Providers, LLM Proxy, Subscriptions, API Keys, User Management, Developer Tools. Each section element gets `id="settings-{slug}"`. Click scrolls via `scrollIntoView({ behavior: 'smooth' })`. Active section highlighted via `IntersectionObserver` with `rootMargin: "-20% 0px -80% 0px"`.
- **Styling:** `sticky top-0 z-10 bg-cf-bg-primary/95 backdrop-blur-sm border-b border-cf-border` with horizontal scroll (`overflow-x-auto whitespace-nowrap`) for mobile overflow.
- **Verify:** Scroll settings page — nav highlights active section. Click nav item — smooth scroll to section. Mobile: nav scrolls horizontally.

### M5: Project Detail Graceful Degradation
- **Files:** `frontend/src/features/project/ProjectDetailPage.tsx`
- **What:** Refactor data fetching to load each sub-resource independently (project info, conversations, roadmap, files, audit). Use independent `createResource` per section instead of a single fetch-all. If a sub-resource fails, show `ErrorBanner` (from `frontend/src/ui/composites/ErrorBanner.tsx`) inline for that section only with a "Retry" button. Always render project header + available sections.
- **Verify:** Navigate to project detail — page renders even when `/roadmap` or `/conversations` return 500. Failed sections show error banner with retry button. Working sections display normally.

### M6: Sidebar Brand Mark
- **Files:** New `frontend/src/ui/icons/CodeForgeLogo.tsx`, update `frontend/src/ui/layout/Sidebar.tsx` (Sidebar.Header section)
- **What:** Create inline SVG logo — reuse Q1's anvil/forge design, optimized for sidebar (24x24 collapsed, 28x28 expanded). Replace plain text "CodeForge v0.1.0" in `Sidebar.Header` with: `<CodeForgeLogo />` + "CodeForge" text (hidden when collapsed via `Show when={!collapsed()}`). Version number in `Tooltip` on hover (using existing Tooltip component).
- **Dependency:** Must use same visual as Q1 favicon for brand consistency. Do Q1 first.
- **Verify:** Sidebar expanded: logo + "CodeForge" text. Collapsed: logo only. Hover on collapsed: tooltip shows "CodeForge v0.1.0".

### M7: Models Page Collapse/Expand (Audit Issue #9)
- **Files:** `frontend/src/features/llm/ModelsPage.tsx` (or wherever model cards are rendered in AI Config)
- **What:** Show models collapsed by default: display only model name + provider badge. Click to expand and show all property badges (id, key, cost per token, etc.). Use a `details/summary` HTML pattern or a signal-based toggle. Add "Expand All / Collapse All" toggle button at the top.
- **Verify:** AI Config Models tab — models show name + provider only. Click a model — details expand. "Expand All" shows all details.

---

## Layer 3: Strategic (4 tasks, 1-3 days each)

### S1: Typography System
- **Files:** `frontend/src/index.css`, `frontend/index.html`, new `frontend/public/fonts/` directory
- **What:** Introduce a distinctive font pair:
  - **Display (headings):** Self-hosted woff2. A sans-serif with character that fits "forge/workshop" brand — e.g. **Outfit** (geometric, modern, open-source), **Plus Jakarta Sans** (distinctive, clean), or **Sora** (technical, geometric). NOT monospace for headings (monospace reads "code editor", not "product"), NOT Inter/Roboto/Arial/Space Grotesk.
  - **Body:** **IBM Plex Sans** — clean, excellent readability, technically-minded but not generic.
- **Implementation:**
  - Download woff2 files, add to `frontend/public/fonts/`
  - Define `@font-face` rules in index.css with `font-display: swap` (prevents FOUT)
  - Add CSS variables: `--cf-font-display`, `--cf-font-body`
  - Update `@theme` block in index.css
  - Apply: `font-display` to h1-h4 via `h1,h2,h3,h4 { font-family: var(--cf-font-display) }`, `font-body` to `body`
- **Verify:** All 13 pages in light + dark mode — headings use display font, body uses body font. No FOUT.

### S2: Micro-Interactions & Polish
- **Files (explicit per sub-task):**
  - (a) Card hover lift: `frontend/src/ui/composites/Card.tsx`, `frontend/src/features/dashboard/ProjectCard.tsx`
  - (b) Button press: `frontend/src/ui/primitives/Button.tsx`
  - (c) Tab underline: `frontend/src/ui/primitives/Tabs.tsx` (or wherever tabs are implemented)
  - (d) KPI count-up: `frontend/src/features/dashboard/KpiStrip.tsx`
  - (e) Toast entrance: `frontend/src/components/Toast.tsx`
  - (f) Modal entrance: `frontend/src/ui/composites/Modal.tsx`
  - (g) Sidebar group collapse: `frontend/src/ui/layout/Sidebar.tsx`
- **What:** Systematic polish pass, all CSS-only (no motion library):
  - (a) Card hover lift: add `hover:-translate-y-0.5 transition-transform duration-200`
  - (b) Button press: add `active:scale-[0.98] transition-transform` to base button class
  - (c) Tab active indicator: animated underline via `transition-all duration-200` on active tab border-bottom
  - (d) KPI count-up: simple `requestAnimationFrame` counter animating from 0 to value on dashboard mount (~300ms duration)
  - (e) Toast entrance: `translate-x-full -> translate-x-0` transition on mount via signal + `transition-transform duration-300`
  - (f) Modal entrance: `opacity-0 scale-95 -> opacity-100 scale-100` transition via signal + `transition-all duration-200`
  - (g) Sidebar group collapse: `grid-template-rows: 0fr -> 1fr` transition for nav section groups
- **Constraints:** All respect `prefers-reduced-motion`. No external animation library.
- **Verify:** Interactive check per interaction in browser. Each should feel smooth, not jarring.

### S3: Design System Documentation
- **Files:** New `frontend/src/ui/DESIGN-SYSTEM.md`, new `frontend/src/features/dev/DesignSystemPage.tsx`, `frontend/src/index.tsx`
- **What:** Create living style guide page at `/design-system` (dev-mode only, guarded by `devMode()` check):
  - Color palette: render all `--cf-*` color variables as swatches
  - Typography: show heading scale (h1-h4) + body text + monospace
  - Buttons: render all variants x sizes in a grid
  - Badges: all variants + pill mode
  - Cards: compound card with header/body/footer
  - Alerts: all 4 variants
  - EmptyState: with and without illustration (requires M1's `illustration` prop)
  - Skeletons: all variants (text, rect, circle, table, card, chat)
  - Inputs: default, error, disabled, monospace
  - StatusDot: all colors with labels
  - Spinner: all sizes
  - Toast: trigger buttons for each level
  - Modal: trigger button to open demo modal
  - Tooltip: hover targets for each placement
- **Route:** Add `<Route path="/design-system" component={DesignSystemPage} />` to index.tsx, only rendered when `APP_ENV=development`
- **Markdown:** Write `DESIGN-SYSTEM.md` documenting token naming (`--cf-{category}-{variant}`), component API conventions, usage guidelines
- **Dependencies:** Implicitly depends on M1 (EmptyState illustration prop) — satisfied since Layer 2 precedes Layer 3.
- **Verify:** Navigate to `/design-system` — all components render. Both light and dark mode.

### S4: Onboarding Wizard
- **Files:** New `frontend/src/features/onboarding/OnboardingWizard.tsx`, new `frontend/src/features/onboarding/steps/ConnectCodeStep.tsx`, `ConfigureAIStep.tsx`, `CreateProjectStep.tsx`, `frontend/src/App.tsx`
- **What:** 3-step wizard shown on first login when no projects exist:
  1. **"Connect your code"** — Simplified VCS account form (Provider dropdown + Token input + "Add Account" button). Self-contained, NOT reusing SettingsPage inline — build a lightweight standalone form that calls the same API endpoints (`POST /api/v1/vcs-accounts`).
  2. **"Configure AI"** — Model provider selection (dropdown of available providers + "Test Connection" button). Self-contained, calls `POST /api/v1/models` or `POST /api/v1/models/discover`.
  3. **"Create your first project"** — Project name + repo URL + "Create" button. Self-contained, calls `POST /api/v1/projects`.
- **Rationale:** Building standalone step forms avoids hidden dependency on extracting forms from existing pages. Each step is ~50-80 lines, calling existing API endpoints directly.
- **Trigger:** Check `localStorage` key `codeforge-onboarding-completed` AND `GET /api/v1/projects` returns empty list
- **UX:** Full-screen overlay with centered card (max-w-lg), step indicator dots (1/3, 2/3, 3/3), "Skip setup" link at bottom, "Next" / "Back" buttons. Fade transition between steps.
- **Verify:** Clear localStorage, ensure 0 projects — wizard appears after login. Complete all 3 steps — wizard dismissed. Reload — wizard does not reappear. "Skip" dismisses permanently.

---

## Execution Order

```
Layer 1 (all parallel):     Q1  Q2  Q3  Q4  Q5  Q6
                              |
Layer 2 (all parallel):     M1  M2  M3  M4  M5  M6  M7
                              |                        |
                              | (Q1 must precede M6)   |
                              |                        |
Layer 3 (partially ordered): S4 ──────────────────────┐
                             S1 → S2 → S3 ────────────┘ merge
```

**Note:** S1 and S2 are sequential because font sizing affects animation feel. S3 documents the final state after S1+S2. S4 is independent and can run in parallel with S1.

## Constraints

- NO new npm dependencies (all CSS/SVG/vanilla JS)
- All animations respect `prefers-reduced-motion`
- All changes must work in both light and dark mode
- All new components use existing design tokens (`--cf-*`)
- All new components follow existing patterns (SolidJS signals, `cx()` utility, compound components)
- SVG illustrations use `currentColor` + CSS variables for theme compatibility

## Audit Coverage Matrix

| Audit Issue | Spec Task | Status |
|---|---|---|
| #1 Project Detail crashes | M5 | Covered |
| #2 No visual identity/brand | M6 + S1 | Covered |
| #3 Empty states text-only | M1 | Covered |
| #4 No transitions/micro-interactions | M2 + S2 | Covered |
| #5 WebSocket banner flashes | Q4 | Covered |
| #6 Settings page too long | M4 | Covered |
| #7 KPI text truncation | Q5 | Covered |
| #8 Inconsistent button styling | Q3 | Covered |
| #9 Models page visual noise | M7 | Covered |
| #10 No favicon | Q1 | Covered |
