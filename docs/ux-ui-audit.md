# CodeForge Frontend UX/UI Audit

**Date:** 2026-03-18
**Auditor:** Automated (Playwright MCP + Claude)
**Version:** v0.1.0
**Pages Evaluated:** 13 routes (+ responsive + dark mode)

---

## Executive Summary

CodeForge presents a **functional but visually undifferentiated** admin interface that scores **5.4/10 overall**. The warm beige/cream color palette is distinctive but borders on monotonous. The layout is clean and consistent across pages, with a well-structured sidebar navigation. However, the frontend suffers from generic typography, lack of visual hierarchy beyond headings, minimal micro-interactions, and a critical error on the Project Detail page. The dark mode is competent, and responsive behavior is surprisingly solid for an early-stage product.

---

## Page-by-Page Evaluation

### Login Page — Score: 5/10
- **Typography:** 4/10 — Generic serif heading ("Sign in to CodeForge") paired with system sans-serif body. The serif choice is unusual but not refined enough to feel intentional.
- **Color & Theme:** 5/10 — Warm cream background (#FDF6E3-ish) with a subtle card. Pleasant but forgettable.
- **Layout:** 6/10 — Centered card layout is clean. Good vertical spacing. The background image (bottom-right anvil silhouette) is a nice brand touch.
- **Navigation:** N/A — Single-purpose page.
- **Responsiveness:** 7/10 — Card adapts well, no overflow issues.
- **Interactions:** 4/10 — No visible focus ring on inputs, no password visibility toggle, no loading state animation on submit. Error toast appears with red background — functional but jarring.
- **Consistency:** 5/10 — Blue "Sign in" button matches the primary blue used elsewhere.
- **Accessibility:** 5/10 — Inputs have labels and required indicators. Missing visible focus states. Error displayed via alert role (good).

### Dashboard — Score: 6/10
- **Typography:** 5/10 — "Projects" heading is clean. KPI numbers are adequately sized. Project card titles use a workable hierarchy.
- **Color & Theme:** 5/10 — Cream background, light beige cards, steel-blue headings. Cohesive but low-energy. The red health dots on projects are the only accent color — they pop.
- **Layout:** 7/10 — Strong grid layout. KPI row with 7 metrics is well-spaced. Project cards in a 3-column grid at desktop. Activity + Charts in a 2-column bottom section. Good information density.
- **Navigation:** 7/10 — Sidebar with icon-only (collapsed) and icon+label (expanded) modes. Grouped sections (AI & Agents, Knowledge, Channels, System). Active state visible via background highlight.
- **Responsiveness:** 7/10 — Mobile: sidebar collapses to hamburger, cards stack vertically, KPI row becomes horizontal scroll. Tablet: maintains grid but reduces columns. No broken layouts.
- **Interactions:** 5/10 — Edit/Delete buttons are plain text links without hover indication in light mode. Chart tab switching works. 7d/30d toggle functional.
- **Consistency:** 7/10 — Card styling, button colors, and spacing are consistent.
- **Accessibility:** 6/10 — Skip-to-content link present. Sidebar has `complementary` role. Main content has `main` role. Chart alt text needs improvement.

### Project Detail — Score: 1/10 (CRITICAL ERROR)
- **All dimensions:** N/A — Page crashes with "Something went wrong — internal server error" upon navigation. The error boundary card is centered, uses red heading, blue "Try again" button. The error boundary itself is well-designed, but the page is completely non-functional.
- **Root cause:** Backend returns 500 on `/projects/:id/roadmap` and `/projects/:id/conversations` endpoints.

### Activity Page — Score: 5/10
- **Typography:** 5/10 — Clean heading and tab labels.
- **Color & Theme:** 5/10 — Same cream palette. Green "Live" badge is a nice touch. Disconnected state shows different badge color.
- **Layout:** 5/10 — Tab bar (Live / Audit Trail) + filter row + empty state. Very sparse when no data. Large empty area with a single centered message.
- **Navigation:** 6/10 — Tab navigation works. Filter dropdown for event types.
- **Responsiveness:** 6/10 — Adapts well, filter controls remain usable.
- **Interactions:** 5/10 — Pause/Clear buttons available. Event type combobox functional.
- **Consistency:** 6/10 — Matches overall styling patterns.
- **Accessibility:** 5/10 — Tab roles present. Live region for connection status via alert role.

### AI Config (Models Tab) — Score: 5/10
- **Typography:** 5/10 — Model names in bold are readable. Metadata rendered as inline code-like badges.
- **Color & Theme:** 5/10 — Badge-like tags for model properties (id, key, provider, costs) use subtle colored backgrounds. Functional but visually dense.
- **Layout:** 5/10 — Simple list of model cards. Each card shows all properties as tag badges. Gets visually noisy with many models.
- **Navigation:** 6/10 — Tab switching between Models/Modes works cleanly.
- **Responsiveness:** 5/10 — Tags wrap but can feel cramped on mobile.
- **Interactions:** 5/10 — "Discover Models" and "Add Model" buttons available. No inline editing or expand/collapse for model details.
- **Consistency:** 6/10 — Button styling matches the app.
- **Accessibility:** 5/10 — Loading state announced properly.

### AI Config (Modes Tab) — Score: 6/10
- **Typography:** 5/10 — Mode names bold and clear. "built-in" badge is a nice semantic tag.
- **Color & Theme:** 6/10 — Red background on "Denied Actions" tags (`rm -rf`, `curl | bash`) is effective — clearly signals danger. Tool badges in neutral. Good semantic color usage.
- **Layout:** 6/10 — Card layout per mode with clear sections: Tools, Denied Actions, LLM Scenario, Autonomy Level, Required Artifact. Well-organized.
- **Navigation:** 6/10 — "Add Mode" CTA prominent.
- **Responsiveness:** 5/10 — Tags wrap reasonably.
- **Interactions:** 5/10 — No inline editing. View-only with action buttons.
- **Consistency:** 6/10 — Same card pattern as Models.
- **Accessibility:** 5/10 — Role labels could be improved.

### Cost Dashboard — Score: 5/10
- **Typography:** 5/10 — Clean numbers in the KPI cards. "$0.00" is well-formatted.
- **Color & Theme:** 5/10 — 4 KPI cards in a 2x2 grid with cream background. Table below. Functional but uninspiring.
- **Layout:** 6/10 — KPI grid + data table is a solid dashboard pattern. Table has proper column headers.
- **Navigation:** 5/10 — No time range filter or breakdown options visible.
- **Responsiveness:** 6/10 — Table doesn't overflow. KPI cards stack on mobile.
- **Interactions:** 4/10 — Static display. No drill-down, no chart, no export. Just numbers and a table.
- **Consistency:** 6/10 — Card + table pattern matches the app.
- **Accessibility:** 6/10 — Proper `table` semantics with `columnheader` roles. Empty state message in table body.

### Knowledge Page — Score: 5/10
- **Typography:** 6/10 — Heading + subtitle ("Curated knowledge modules for agent context") is informative.
- **Color & Theme:** 5/10 — Same palette. Nothing distinctive.
- **Layout:** 5/10 — Tabs (Knowledge Bases / Scopes) + CTA button + empty state. Very minimal.
- **Navigation:** 6/10 — Tab structure for organization.
- **Responsiveness:** 6/10 — Clean on all sizes.
- **Interactions:** 4/10 — Only a "Create Knowledge Base" button. Empty state message could have an illustration or more guidance.
- **Consistency:** 6/10 — Same empty state pattern.
- **Accessibility:** 5/10 — Tab roles present.

### MCP Servers — Score: 5/10
- **Typography:** 6/10 — Heading + subtitle is clear and descriptive.
- **Color & Theme:** 5/10 — Minimal. Blue "Add Server" button.
- **Layout:** 5/10 — Header + empty state. Same sparse pattern.
- **Navigation:** 5/10 — No sub-navigation needed.
- **Responsiveness:** 6/10 — Adapts well.
- **Interactions:** 4/10 — Only "Add Server" button. Empty state needs onboarding guidance.
- **Consistency:** 6/10 — Matches pattern.
- **Accessibility:** 5/10 — Basic semantics.

### Prompt Sections — Score: 5/10
- **Typography:** 5/10 — Clean heading and subtitle.
- **Color & Theme:** 5/10 — Same cream.
- **Layout:** 5/10 — Scope dropdown + Add/Preview buttons + empty state.
- **Navigation:** 5/10 — Scope selector (Global dropdown) is useful.
- **Responsiveness:** 5/10 — Buttons stack oddly — "Add Section" is blue, "Preview" is unstyled text. Inconsistent.
- **Interactions:** 4/10 — Preview button exists but no content to preview.
- **Consistency:** 5/10 — "Preview" button lacks styling compared to "Add Section" — looks like an afterthought.
- **Accessibility:** 5/10 — Combobox role present.

### Benchmark Dashboard — Score: 6/10
- **Typography:** 6/10 — Clear heading with descriptive subtitle including "(dev-mode only)" — good contextual information.
- **Color & Theme:** 5/10 — Same palette.
- **Layout:** 6/10 — 5-tab navigation (Runs, Leaderboard, Cost Analysis, Multi-Compare, Suites) shows a feature-rich section. "New Run" CTA is prominent.
- **Navigation:** 7/10 — Strong tab structure for complex feature.
- **Responsiveness:** 6/10 — Tabs may become cramped on mobile.
- **Interactions:** 5/10 — "New Run" button available.
- **Consistency:** 6/10 — Tab + CTA pattern matches the app.
- **Accessibility:** 6/10 — Tab roles and selected states correct.

### Settings Page — Score: 6/10
- **Typography:** 5/10 — Section headings (General, Keyboard Shortcuts, VCS Accounts, Providers, LLM Proxy, Subscription Providers, API Keys, User Management, Developer Tools) create clear hierarchy.
- **Color & Theme:** 5/10 — Same cream. Provider lists use green "Connected" / grey "Disconnected" badges — functional.
- **Layout:** 7/10 — Long scrolling page with distinct sections. Each section is self-contained with appropriate form controls. Provider grid (Git, Agent Backends, Spec, PM) is well-organized.
- **Navigation:** 5/10 — No sticky section nav or anchor links for a very long page. User must scroll to find sections.
- **Responsiveness:** 6/10 — Form controls adapt. Table wraps appropriately.
- **Interactions:** 6/10 — Functional forms: dropdowns, checkboxes, text inputs, action buttons. Keyboard shortcut editor with "Edit" buttons per shortcut is a nice touch. GitHub Copilot shows "Connected" state with "Disconnect" action.
- **Consistency:** 6/10 — Consistent form patterns across sections.
- **Accessibility:** 6/10 — Form labels present. Combobox roles. Table semantics for user management.

### 404 Page — Score: 6/10
- **Typography:** 6/10 — "Page not found" heading is clear. Helpful subtitle text.
- **Color & Theme:** 5/10 — Cream background, centered content.
- **Layout:** 7/10 — Perfectly centered with heading + message + CTA button. Clean and purposeful.
- **Navigation:** 7/10 — "Back to Dashboard" button provides clear escape route.
- **Responsiveness:** 7/10 — Centered layout works at all sizes.
- **Interactions:** 5/10 — Single button, functional. No animation or illustration.
- **Consistency:** 6/10 — Blue button matches primary CTA.
- **Accessibility:** 6/10 — Heading hierarchy, button semantics.

---

## Cross-Cutting Assessment

| Dimension | Score | Justification |
|-----------|-------|---------------|
| **Information Architecture** | 7/10 | Sidebar grouping (AI & Agents, Knowledge, Channels, System) is logical. New users can find features. Missing breadcrumbs on sub-pages. |
| **Error Handling UX** | 4/10 | Error boundary exists and is well-styled, but Project Detail crashing is a critical failure. Empty states are text-only with no illustrations or guided onboarding. Toast errors are functional but lack detail. |
| **Performance Perception** | 5/10 | Loading spinner with "Loading models..." is present. No skeleton screens. No progressive loading. Charts render but with no animation. Content appears abruptly. |
| **Visual Identity** | 4/10 | Warm cream palette is distinctive but not memorable. Feels like a "parchment paper" admin template. The anvil/forge imagery on the login page is the only brand element. No logo in sidebar, just text "CodeForge v0.1.0". |
| **Delight Factor** | 3/10 | No micro-interactions, no transitions between pages, no hover animations on cards, no progress indicators, no illustrations in empty states. The UI is purely functional. |

---

## Responsive Audit

| Viewport | Score | Findings |
|----------|-------|----------|
| **Mobile (375x812)** | 6/10 | Sidebar collapses to hamburger menu. KPI row becomes horizontal scroll (truncates "Success R..." — text overflow issue). Project cards stack vertically and are usable. Edit/Delete buttons reachable. Chart renders but is very small. |
| **Tablet (768x1024)** | 7/10 | Good adaptation. Sidebar stays collapsed as icon strip. Full KPI row visible. Project cards in single column. Activity + Charts sections stack vertically. No broken layouts. |
| **Desktop (1280x800)** | 7/10 | Best experience. Sidebar expands with labels. 3-column project grid. 2-column bottom section (Activity + Charts). Good use of horizontal space. |

**Key responsive issues:**
- KPI badges text truncation on mobile ("Success R..." instead of "Success Rate (7d)")
- No responsive breakpoint between tablet icon sidebar and desktop full sidebar — could benefit from an intermediate state
- Chart Y-axis labels get cramped on narrow viewports

---

## Theme Audit

| Aspect | Light Mode | Dark Mode |
|--------|-----------|-----------|
| **Background** | Warm cream (#FDF6E3-ish) | Dark charcoal (#1a1a2e-ish) |
| **Cards** | Slightly darker cream with subtle border | Dark grey with visible borders |
| **Text contrast** | Steel-blue headings on cream — adequate but not WCAG AAA | Light text on dark — good contrast |
| **Accent colors** | Blue buttons, red delete/health dots | Same blue buttons, red dots maintain visibility |
| **Overall quality** | 5/10 — Pleasant but monotone | 6/10 — Actually better-looking; the dark background makes cards pop more and gives better visual hierarchy |

**Dark mode is slightly better than light mode** — the contrast between card surfaces and background is more pronounced, creating better visual hierarchy that the light mode lacks.

---

## Top 5 Strengths

1. **Consistent sidebar navigation** — Well-organized groups (AI & Agents, Knowledge, Channels, System), collapsible with icon-only mode, active state highlighting, WebSocket + API connection status indicators at bottom.
2. **Responsive layout foundations** — All pages adapt from 375px to 1280px without broken layouts. Hamburger menu on mobile, icon strip on tablet, full sidebar on desktop.
3. **Semantic HTML structure** — `complementary` role on sidebar, `main` role on content, `banner` on header, proper `table` semantics, `tab`/`tablist` roles, `alert` for errors. Good accessibility foundation.
4. **Functional completeness** — 13 distinct pages covering all four pillars. Settings page is particularly comprehensive with 9 sections including keyboard shortcuts editor and provider registry.
5. **Dark mode implementation** — Clean theme toggle (System/Light/Dark cycle), all components properly themed, no elements missed, better visual hierarchy than light mode.

---

## Top 10 Issues (Priority-Ranked)

| # | Issue | Severity | Affected Pages | Suggested Fix |
|---|-------|----------|---------------|---------------|
| 1 | **Project Detail page crashes** with "internal server error" on `/roadmap` and `/conversations` API calls | Critical | `/projects/:id` | Fix backend endpoints or add graceful degradation — show project info even if sub-resources fail |
| 2 | **No visual identity / brand presence** — looks like a generic admin template | Major | All pages | Add logo to sidebar, distinctive typography (replace system fonts with a characterful font pair), introduce a secondary accent color beyond blue |
| 3 | **Empty states are text-only** — "No MCP servers configured yet" with no illustration, no guidance, no quick-start | Major | Knowledge, MCP, Prompts, Benchmarks, Activity | Add empty state illustrations (SVG), onboarding text ("Get started by..."), and inline quick-action buttons |
| 4 | **No page transitions or micro-interactions** — UI feels static and lifeless | Major | All pages | Add fade/slide transitions between routes, hover effects on cards, subtle animations on KPI numbers, loading skeletons |
| 5 | **WebSocket "disconnected" banner persists** across most page loads — shows reconnection churn | Major | Activity, AI Config, Costs, Knowledge, MCP, Prompts, Benchmarks | Debounce the reconnection banner, or suppress it during initial page load. Only show after stable connection drops |
| 6 | **Settings page too long** with no section navigation — user must scroll through 9 sections | Major | `/settings` | Add sticky section sidebar or anchor nav at the top. Consider splitting into sub-tabs |
| 7 | **KPI text truncation on mobile** — "Success R..." instead of "Success Rate (7d)" | Minor | Dashboard (mobile) | Use abbreviations on mobile ("Success 7d") or show fewer KPIs with a "show more" toggle |
| 8 | **Inconsistent button styling** — "Preview" on Prompts page is unstyled text next to styled "Add Section" | Minor | Prompts | Apply consistent button variants: primary (filled blue), secondary (outlined), tertiary (text link) |
| 9 | **Models page visual noise** — property badges (id, key, provider, cost) create information overload | Minor | AI Config (Models) | Collapse model details by default, show only name + provider. Expand on click. |
| 10 | **No favicon** — browser returns 404 for `/favicon.ico` | Minor | All pages | Add a favicon (anvil/forge icon matching the login page brand image) |

---

## Recommendations

### Quick Wins (< 1 day effort)
- Add a favicon and update `<title>` per page (e.g., "Dashboard - CodeForge")
- Fix the inconsistent "Preview" button styling on Prompts page
- Suppress WebSocket reconnection banner during initial page load (debounce 2s)
- Add `text-overflow: ellipsis` with proper `max-width` on KPI labels for mobile
- Add hover effects on project cards (subtle shadow lift or border color change)

### Medium-Term (1-5 days)
- Design and add SVG empty state illustrations for all list pages
- Add page transition animations (route-level fade or slide)
- Add loading skeletons for data-heavy pages (Models, Settings, Cost Dashboard)
- Add section navigation to Settings page (sticky sidebar or tab groups)
- Implement card expand/collapse for Models page
- Fix Project Detail page backend errors for graceful degradation
- Add a proper logo and brand mark to the sidebar header

### Strategic (> 5 days)
- Commission a typography system: pick a distinctive display font for headings (e.g., JetBrains Mono, Space Mono, or a serif like Playfair Display) and a clean body font — replacing default system fonts
- Develop a proper design system with documented color tokens, spacing scale, component variants (button sizes, card styles, badge types)
- Add onboarding flow for first-time users: guided setup wizard connecting VCS accounts, configuring first model, creating first project
- Implement full keyboard navigation with visible focus rings and command palette (Ctrl+K is already bound)
- Add dashboard data visualizations: sparklines in KPI cards, project health trend graphs, animated chart transitions
