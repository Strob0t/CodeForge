# Project Detail Page — UX Evaluation Report

**Date:** 2026-03-24
**Project tested:** S2-cut-tool (fef1bdcf-21d2-4291-9442-81b11955252d)
**Model used:** lm_studio/qwen/qwen3-30b-a3b (conversation history from S2 autonomous run)
**Testplan:** `docs/testing/project-detail-ux-testplan.md`

---

## Summary

| Category | Features Tested | PASS | PARTIAL | FAIL | BROKEN |
|----------|----------------|------|---------|------|--------|
| Header & Actions | 6 | 4 | 0 | 2 | 0 |
| File Management | 5 | 5 | 0 | 0 | 0 |
| Goals | 5 | 5 | 0 | 0 | 0 |
| Roadmap & Planning | 5 | 5 | 0 | 0 | 0 |
| Agent & Execution | 5 | 5 | 0 | 0 | 0 |
| Intelligence & System | 5 | 5 | 0 | 0 | 0 |
| Chat | 5 | 5 | 0 | 0 | 0 |
| Cross-Cutting | 1 | 1 | 0 | 0 | 0 |
| **TOTAL** | **37** | **35** | **0** | **2** | **0** |

---

## Value Assessment

| Feature | Value | UX Quality | Recommendation |
|---------|-------|------------|----------------|
| Project Title + Git Status | ESSENTIAL | SMOOTH | KEEP AS-IS |
| Pull Button | USEFUL | ROUGH | IMPROVE (error message: "internal server error" → "No remote configured") |
| Auto-Agent | USEFUL | ROUGH | IMPROVE (500 error, no loading state, unclear purpose from label alone) |
| Design Canvas | USEFUL | SMOOTH | KEEP AS-IS (9 tools, export, Send to Agent) |
| Settings Popover | ESSENTIAL | SMOOTH | KEEP AS-IS (autonomy, MCP servers, cost summary, auto-close on save) |
| Progress Wizard | USEFUL | SMOOTH | KEEP AS-IS (smart panel navigation on step click, dismissable) |
| File Tree | ESSENTIAL | SMOOTH | KEEP AS-IS (expand/collapse, icons, selection highlight) |
| File Viewer (Monaco) | ESSENTIAL | SMOOTH | KEEP AS-IS (tabs, syntax highlighting, language badge) |
| File Filter | USEFUL | SMOOTH | KEEP AS-IS (real-time, regex support, match highlighting with `<mark>`) |
| File Toolbar | USEFUL | ADEQUATE | IMPROVE (icon-only buttons need tooltips — currently no labels) |
| Goals CRUD | ESSENTIAL | SMOOTH | KEEP AS-IS (kind dropdown with 5 types, ON toggle, toast) |
| Goal Delete | USEFUL | ROUGH | IMPROVE (no confirmation dialog — risky accidental deletion) |
| Roadmap Create | ESSENTIAL | SMOOTH | KEEP AS-IS (title+desc, toast, rich toolbar: Import Specs/PM, AI View, Sync) |
| Milestone + Feature | ESSENTIAL | SMOOTH | KEEP AS-IS (drag reorder, status badges, mark-as-done checkbox) |
| Feature Map | USEFUL | SMOOTH | KEEP AS-IS (card layout, drag-to-move, edit-on-click) |
| Tasks & Roadmap | MARGINAL | ADEQUATE | REDESIGN (unclear relationship to Roadmap panel; no chat suggestions appear) |
| Plans | MARGINAL | ADEQUATE | REDESIGN (unclear what a "Plan" is vs "Task" vs "Roadmap") |
| War Room | USEFUL | SMOOTH | KEEP AS-IS (good empty state, Shared Context section) |
| Sessions | MARGINAL | ADEQUATE | CONSIDER MERGING (unclear vs Conversations in chat header) |
| Trajectory | USEFUL | ADEQUATE | KEEP AS-IS (needs agent run data to evaluate fully) |
| Agents & Runs | ESSENTIAL | SMOOTH | KEEP AS-IS (3 sections: agents, run mgmt, network viz) |
| Code Intelligence | USEFUL | SMOOTH | KEEP AS-IS (repo map, graph explorer, LSP management) |
| Retrieval | USEFUL | SMOOTH | KEEP AS-IS (search simulator with weight sliders is excellent) |
| Boundaries | MARGINAL | ADEQUATE | IMPROVE (label unclear without docs; needs description text) |
| Audit Trail | USEFUL | ADEQUATE | KEEP AS-IS (action filter, contextual suggestions) |
| Policy | ESSENTIAL | SMOOTH | KEEP AS-IS (5 presets, custom rules, permission preview) |
| Chat Messages | ESSENTIAL | SMOOTH | KEEP AS-IS (markdown, tool call badges, model info) |
| Tool Call Cards | ESSENTIAL | SMOOTH | KEEP AS-IS (expandable, args/result sections) |
| Conversation Switching | ESSENTIAL | SMOOTH | KEEP AS-IS (tab-style buttons, active highlight) |
| Chat Suggestions | USEFUL | SMOOTH | KEEP AS-IS (context-aware per active panel) |
| Panel Collapse | USEFUL | SMOOTH | KEEP AS-IS (thin sidebar with label, expand button) |

---

## Top 5 Issues (by impact)

1. **Pull button returns "internal server error" for local-only projects** — Backend should return a meaningful error like "No remote configured" instead of 500. Affects every local workspace. FIX: check for remote before attempting pull; return 400 with clear message.

2. **Auto-Agent button returns 500** — Clicking it triggers an API call that fails silently. No loading state, no explanation of what it does. FIX: add loading spinner, tooltip explaining the feature, handle error gracefully.

3. **Goal delete has no confirmation** — Single click permanently removes a goal. No undo, no "Are you sure?" FIX: add confirmation dialog matching the project delete pattern.

4. **Tasks & Roadmap vs Roadmap vs Plans — unclear taxonomy** — Three panels with overlapping concepts. User cannot tell which to use. CONSIDER: merge Tasks into Roadmap panel, or add descriptions explaining the difference.

5. **Sessions panel unclear vs Chat conversations** — "Sessions" and "Conversations" seem to serve similar purposes. User doesn't know which to check. CONSIDER: merge into a single unified view or add clear labels explaining the distinction.

---

## Top 5 UX Wins (what works well)

1. **Context-aware chat suggestions** — Chat input shows different quick actions based on active panel (Goals: "Help me define goals"; Roadmap: "Create roadmap from goals"; War Room: "Start an agent"; Audit: "Show security events"). This is genuinely useful and well-executed.

2. **Progress Wizard with smart navigation** — Clicking a wizard step (e.g., "Roadmap created") automatically switches the left panel to the relevant tab. Elegant guidance without being intrusive. Dismissable with ×.

3. **File filter with match highlighting** — Typing in the filter input instantly filters the tree with highlighted matches using `<mark>` tags. Regex support. Auto-expands folders to show matches. Professional quality.

4. **Retrieval Search Simulator** — BM25/Semantic weight sliders, Top K, Token Budget, Agent search checkbox, GraphRAG toggle. Lets power users debug exactly what context an agent would receive. Outstanding debugging tool.

5. **Settings Popover auto-close on save** — Change autonomy → Save → toast confirmation → popover auto-closes. Clean flow with no manual dismissal needed. MCP server checkboxes are a nice addition.

---

## Recommendations Summary

### Must Fix (blocking usability)
- Pull button: return meaningful error for local-only projects (not 500)
- Auto-Agent: add error handling, loading state, and tooltip

### Should Improve (friction reduction)
- Goal delete: add confirmation dialog
- File toolbar: add tooltips to icon-only buttons (Expand All, Collapse All, Upload, New File)
- Boundaries panel: add subtitle explaining what boundaries are

### Nice to Have (polish)
- Chat suggestions: ensure they appear for ALL panels (Tasks & Roadmap currently shows none)
- Keyboard shortcut to switch panels (e.g., Ctrl+1-9)

### Consider Removing or Merging (adds complexity without clear value)
- **Sessions** panel: merge with conversation list in chat header, or clarify distinction
- **Tasks & Roadmap** panel: merge task list into Roadmap panel as a sub-tab
- **Plans** panel: consider making this a sub-tab of Agents & Runs instead of standalone

---

## Panel Count Assessment

The project detail page has **15 panels** in the dropdown. This is a lot of choices. Strategic advisor assessment:

| Category | Panels | Verdict |
|----------|--------|---------|
| **Core (keep)** | Files, Goals, Roadmap, Feature Map | 4 — essential for every user |
| **Execution (keep)** | War Room, Agents & Runs, Policy | 3 — essential for agent workflows |
| **Intelligence (keep for power users)** | Code Intelligence, Retrieval, Trajectory | 3 — valuable but advanced |
| **Merge candidates** | Sessions → into Chat header, Audit Trail → into Activity page, Tasks → into Roadmap, Plans → into Agents | 4 — could reduce to sub-tabs |
| **Niche** | Boundaries | 1 — only useful during contract review pipeline |

**Recommendation:** Consider grouping panels into categories in the dropdown (e.g., "─── Planning ───", "─── Agent ───", "─── Debug ───") to reduce cognitive load. Or use a popover grid instead of a flat dropdown.
