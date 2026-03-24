# Project Detail Page — Comprehensive UX Test & Evaluation Plan

**Date:** 2026-03-24
**Type:** Interactive UX Test + Feature Value Assessment (Claude Code + Playwright-MCP)
**Scope:** `/projects/:id` — Project Detail Page with all panels, modals, and interactions
**Executor:** Claude Code via Playwright-MCP browser automation

---

## How to Use This Document

This is a **runbook for Claude Code sessions** using Playwright-MCP tools. Claude Code drives the CodeForge frontend interactively, tests every feature, and produces a structured evaluation report.

**Prerequisites:**
- Playwright-MCP connected (`/mcp` shows playwright-mcp)
- All services running (Go backend, frontend, Python worker, Docker services)
- At least one project exists with workspace, goals, and conversation history
- Logged in as `admin@localhost`

**Execution:**
- Follow phases A-H sequentially
- Each phase: execute Steps, check Validation, record Verdict
- For every feature: test interactions AND evaluate UX value
- Save report to `docs/testing/YYYY-MM-DD-project-detail-ux-report.md`

**Browser URL:** `http://host.docker.internal:3000` (Playwright-MCP Docker)

---

## Evaluation Framework

Every feature gets rated on 3 dimensions:

| Dimension | Scale | Question |
|-----------|-------|----------|
| **Functional** | PASS / PARTIAL / FAIL / BROKEN | Does it work correctly? |
| **Value** | ESSENTIAL / USEFUL / MARGINAL / UNNECESSARY | Does it add real value to the user? |
| **UX Quality** | SMOOTH / ADEQUATE / ROUGH / PAINFUL | Is the interaction intuitive and polished? |

**Evaluation template** (copy for each feature):
```
### [Feature Name]
- Functional: PASS/PARTIAL/FAIL/BROKEN
- Value: ESSENTIAL/USEFUL/MARGINAL/UNNECESSARY
- UX Quality: SMOOTH/ADEQUATE/ROUGH/PAINFUL
- Issues: [list any bugs, friction points, missing affordances]
- Recommendation: [KEEP AS-IS / IMPROVE / REDESIGN / REMOVE]
- Details: [what was tested, what happened, what should change]
```

**Friction indicators** (log these when observed):
- User would not know what to click next (missing affordance)
- Action requires too many clicks (> 3 for common operations)
- Error message is unhelpful or missing
- Loading state is absent or too long (> 3s with no feedback)
- Feature exists but is empty/broken/disconnected from backend
- Label/button text is confusing or misleading
- Layout breaks or content overflows

---

## State Variables

```
PROJECT_ID = ""        # UUID of test project
PROJECT_NAME = ""      # Display name
HAS_WORKSPACE = false  # Project has adopted workspace
HAS_GOALS = false      # Goals exist
HAS_ROADMAP = false    # Roadmap created
HAS_CONVERSATION = false # At least one conversation with messages
TOKEN = ""             # Auth token
```

---

## Phase 0: Setup & Navigation to Project

**Goal:** Navigate to a project detail page with data to test against.

### Steps

1. Login via `/login` (admin@localhost / Changeme123)
2. From Dashboard, click on an existing project card (or create one with workspace + goals)
3. `browser_snapshot` -> Verify ProjectDetailPage loaded:
   - Project name in header
   - Git branch badge
   - Left panel with "Files" tab
   - Chat panel on right
   - Progress wizard visible
4. Store `PROJECT_ID` from URL path
5. Record: `HAS_WORKSPACE`, `HAS_GOALS`, `HAS_ROADMAP`, `HAS_CONVERSATION`

**If no suitable project exists:**
- Create via API: `POST /projects` with `local_path` pointing to a real workspace
- Create a goal via API: `POST /projects/{id}/goals`
- Send at least one chat message to create conversation history

---

## Phase A: Project Header & Global Actions

**Goal:** Test all elements in the project header bar.

### A1: Project Title & Git Status

1. `browser_snapshot` -> Verify:
   - Project name as `<h2>`
   - Git branch badge (e.g., "main")
   - Dirty/clean indicator
2. **Evaluate:** Is the git status immediately visible? Does the user understand what branch they're on?

### A2: Pull Button

1. `browser_click` -> "Pull" button
2. `browser_snapshot` -> What happens?
   - Toast notification? Loading state? Error?
3. **Evaluate:** Does Pull give feedback? What if there's no remote?

### A3: Auto-Agent Button

1. `browser_click` -> "Auto-Agent" button
2. `browser_snapshot` -> What happens?
   - Does it toggle? Open a modal? Start something?
3. **Evaluate:** Is the purpose clear from the button label? Does the user understand what Auto-Agent does?

### A4: Design Canvas Button

1. `browser_click` -> Canvas icon button
2. `browser_snapshot` -> CanvasModal should open
3. Test canvas tools if modal opens:
   - Select tool, draw a rectangle, add text
   - Export as PNG
   - Close modal
4. **Evaluate:** Is the canvas discoverable? Does it feel integrated or bolted-on?

### A5: Project Settings (Gear Icon)

1. `browser_click` -> Gear icon
2. `browser_snapshot` -> CompactSettingsPopover should open with:
   - Autonomy level dropdown
   - Policy preset dropdown
   - Execution mode
   - Other config fields
3. Change autonomy level -> Save
4. `browser_snapshot` -> Verify change persisted (close & reopen popover)
5. **Evaluate:** Are settings discoverable? Is the popover the right pattern vs. a full settings page?

### A6: Progress Wizard

1. `browser_snapshot` -> Wizard bar visible with steps:
   - Repo cloned (✓/○)
   - Stack detected (✓/○)
   - Goals defined (✓/○)
   - Roadmap created (✓/○)
   - First agent run (✓/○)
2. Click on an incomplete step (e.g., "Roadmap created")
3. `browser_snapshot` -> Does it navigate or guide the user?
4. Click the dismiss "×" button
5. `browser_snapshot` -> Wizard disappears?
6. Reload page -> Does wizard stay dismissed or return?
7. **Evaluate:** Does the wizard actually help? Is it annoying after first use? Should it be permanently dismissable?

---

## Phase B: Left Panel — File Management

**Goal:** Test the Files panel and all file operations.

### B1: File Tree Loading

1. Click "Files" tab (should be default)
2. `browser_snapshot` -> File tree with project files visible?
3. **Evaluate:** How long does it take to load? Is there a loading indicator?

### B2: File Tree Navigation

1. Click on a directory to expand it
2. Click on a file to open it
3. `browser_snapshot` -> File content visible in editor area?
4. Click on another file -> Does it switch?
5. **Evaluate:** Is navigation intuitive? Is the selected file highlighted?

### B3: File Operations

1. Click "New File" button in Files header
2. `browser_snapshot` -> Modal/form for creating file?
3. Enter filename and content, submit
4. `browser_snapshot` -> File appears in tree?
5. Test "Upload File" button
6. **Evaluate:** Are file operations discoverable? Is the create flow smooth?

### B4: File Search/Filter

1. Type in the "Filter files..." input
2. `browser_snapshot` -> Does tree filter in real-time?
3. Try regex pattern (e.g., `/\.py$/`)
4. **Evaluate:** Is filtering fast? Does regex work? Is the placeholder clear?

### B5: File Tree Toolbar

1. Test "Expand All" button
2. Test "Collapse All" button
3. Test resize handle (drag left panel wider/narrower)
4. Double-click resize handle -> Does it reset?
5. **Evaluate:** Are toolbar actions useful? Is the resize handle discoverable?

---

## Phase C: Left Panel — Goals

**Goal:** Test Goals panel CRUD and AI Discovery.

### C1: Switch to Goals Panel

1. Select "Goals" from "More panels..." dropdown
2. `browser_snapshot` -> GoalsPanel visible with existing goals?

### C2: View Existing Goals

1. `browser_snapshot` -> Each goal shows: title, description, kind badge, enabled toggle
2. Click on a goal to see details
3. **Evaluate:** Is the goal display informative enough? Is the hierarchy clear?

### C3: Add Goal Manually

1. Click "Add Goal" button
2. `browser_snapshot` -> Form with kind, title, content fields?
3. Fill form and submit
4. `browser_snapshot` -> New goal appears in list?
5. **Evaluate:** Is the form intuitive? Are field labels clear?

### C4: Toggle Goal On/Off

1. Click the ON/OFF toggle on a goal
2. `browser_snapshot` -> Toggle state changes?
3. **Evaluate:** What does enabling/disabling a goal mean to the user? Is this explained?

### C5: Delete Goal

1. Click delete (x) button on a goal
2. `browser_snapshot` -> Confirmation? Or immediate delete?
3. **Evaluate:** Is there an undo option? Should there be confirmation?

### C6: AI Discover

1. Click "AI Discover" button
2. `browser_snapshot` -> What happens?
   - Navigates to chat? Opens modal? Shows loading?
3. Wait for response (up to 60s for local models)
4. `browser_snapshot` -> GoalProposalCards appear?
5. **Evaluate:** Is the AI discovery flow clear? Does the user understand what's happening?

### C7: Detect Goals

1. Click "Detect Goals" button
2. `browser_snapshot` -> What happens?
3. **Evaluate:** What's the difference between "AI Discover" and "Detect Goals"? Is the distinction clear?

---

## Phase D: Left Panel — Roadmap & Features

### D1: Roadmap Panel (Empty State)

1. Select "Roadmap" from dropdown
2. `browser_snapshot` -> Create roadmap form visible?
3. **Evaluate:** Is the empty state helpful? Does "Go to Goals" make sense?

### D2: Create Roadmap

1. Fill title and description
2. Click "Create Roadmap"
3. `browser_snapshot` -> Roadmap appears with milestone creation UI?
4. **Evaluate:** Is the creation flow minimal and clear?

### D3: Add Milestones and Features

1. Click "Add Milestone"
2. Fill milestone title, submit
3. Click "Add Feature" within the milestone
4. Fill feature title, submit
5. `browser_snapshot` -> Hierarchical structure: Roadmap > Milestone > Features
6. **Evaluate:** Is the hierarchy intuitive? Can features be reordered?

### D4: Auto-Detect

1. Click "Auto-Detect" button
2. `browser_snapshot` -> What happens?
3. **Evaluate:** What does Auto-Detect do? Is the result useful?

### D5: Feature Map Panel

1. Select "Feature Map" from dropdown
2. `browser_snapshot` -> What renders?
3. **Evaluate:** What is this panel? Is it different from Roadmap? Is the distinction clear?

### D6: Tasks & Roadmap Panel

1. Select "Tasks & Roadmap" from dropdown
2. `browser_snapshot` -> What renders?
3. **Evaluate:** How does this differ from Roadmap? Is having both confusing?

### D7: Plans Panel

1. Select "Plans" from dropdown
2. `browser_snapshot` -> What renders?
3. **Evaluate:** What is a "Plan" vs a "Roadmap" vs a "Task"? Is the taxonomy clear?

---

## Phase E: Left Panel — Agent & Execution Panels

### E1: War Room

1. Select "War Room" from dropdown
2. `browser_snapshot` -> Active agents visualization? Shared context?
3. Click "Shared Context" expander
4. **Evaluate:** Is the War Room useful during multi-agent runs? Is it empty/useless when no agents are active?

### E2: Sessions Panel

1. Select "Sessions" from dropdown
2. `browser_snapshot` -> List of sessions?
3. Click on a session if any exist
4. **Evaluate:** What is a session vs a conversation? Is this panel necessary?

### E3: Trajectory Panel

1. Select "Trajectory" from dropdown
2. `browser_snapshot` -> Run selector dropdown? Timeline visualization?
3. Select a run (if available)
4. `browser_snapshot` -> Step-by-step trajectory with tool calls, decisions?
5. **Evaluate:** Is the trajectory visualization useful for understanding what the agent did? Is the run selector intuitive?

### E4: Agents & Runs Panel

1. Select "Agents & Runs" from dropdown
2. `browser_snapshot` -> Agent list? Run history?
3. Click on a run if available
4. **Evaluate:** How does this differ from Trajectory? From Sessions?

### E5: Active Work Panel

1. `browser_snapshot` -> Is ActiveWorkPanel visible above chat when an agent is running?
2. **Evaluate:** Does it show useful information (step count, cost, duration)? Is it visible enough?

---

## Phase F: Left Panel — Intelligence & System Panels

### F1: Code Intelligence (LSP)

1. Select "Code Intelligence" from dropdown
2. `browser_snapshot` -> LSP status? Language servers? Diagnostics?
3. **Evaluate:** Is code intelligence useful? Does it show actionable information?

### F2: Retrieval Panel

1. Select "Retrieval" from dropdown
2. `browser_snapshot` -> Search interface? Index status?
3. Try a search query if available
4. **Evaluate:** Is the retrieval panel useful for the user or purely internal?

### F3: Boundaries Panel

1. Select "Boundaries" from dropdown
2. `browser_snapshot` -> Boundary detection results?
3. **Evaluate:** What are boundaries? Is this panel understandable without reading docs?

### F4: Audit Trail Panel

1. Select "Audit Trail" from dropdown
2. `browser_snapshot` -> Event log? Action filter?
3. **Evaluate:** Is the audit trail useful at project level? Or is the global /activity page sufficient?

### F5: Policy Panel

1. Select "Policy" from dropdown
2. `browser_snapshot` -> Policy list with presets and custom policies?
3. Click on a policy preset to expand/view
4. Click "New Policy" to create a custom rule
5. Click "Effective Permission Preview"
6. `browser_snapshot` -> Preview of merged policy rules?
7. **Evaluate:** Is the policy system understandable? Are presets clearly labeled? Is the preview useful?

---

## Phase G: Chat Panel — Full Interaction Test

### G1: Conversation List

1. `browser_snapshot` -> Chat header with conversation name and "New Conversation" button
2. Click "New Conversation" (+) button
3. `browser_snapshot` -> Conversation selector appears? (existing conversations listed)
4. Click back to original conversation
5. **Evaluate:** Is conversation switching smooth? Is the active conversation clear?

### G2: Chat Suggestions

1. With empty chat or new conversation:
2. `browser_snapshot` -> Quick action buttons visible? (e.g., "Explain the project structure", "Find entry points")
3. Click a suggestion button
4. `browser_snapshot` -> Does it populate the input? Send immediately?
5. **Evaluate:** Are suggestions context-aware? Do they change based on active panel?

### G3: Message Input

1. Click chat input textbox
2. Type a message
3. `browser_snapshot` -> Send button enables? Character/token count visible?
4. Press Shift+Enter -> New line in input?
5. Press Enter -> Message sends?
6. **Evaluate:** Is the input responsive? Is Shift+Enter discoverable?

### G4: Attach File

1. Click "Attach file" button
2. `browser_snapshot` -> File picker? Context file selector?
3. Attach a file
4. `browser_snapshot` -> File badge/chip visible in input area?
5. **Evaluate:** Is the attach flow intuitive? Can you remove an attached file?

### G5: Design Canvas from Chat

1. Click "Design Canvas" button in chat input area
2. `browser_snapshot` -> Canvas modal opens?
3. Draw something, close canvas
4. **Evaluate:** Does the canvas output get attached to the message? Is the flow clear?

### G6: Message Display

1. Scroll through conversation history
2. `browser_snapshot` -> Messages show:
   - User messages (right-aligned or distinct style)
   - Assistant messages with markdown rendering
   - Tool call badges (expandable?)
   - Model name badge
   - Cost badge (if applicable)
3. Click on a tool call badge to expand
4. `browser_snapshot` -> Tool call details visible (arguments, result)?
5. **Evaluate:** Is the message display readable? Are tool calls too prominent or too hidden?

### G7: Tool Call Cards

1. Find a tool call in the conversation (write_file, bash, etc.)
2. Click to expand
3. `browser_snapshot` -> Arguments visible? Result/output visible? Diff preview for edits?
4. **Evaluate:** Are tool calls informative? Is there too much or too little detail?

### G8: Slash Commands

1. Type `/` in the chat input
2. `browser_snapshot` -> Autocomplete dropdown with available commands?
   - `/compact`, `/rewind`, `/clear`, `/help`, `/mode`, `/model`
3. Select a command (e.g., `/help`)
4. `browser_snapshot` -> Command executed? Output shown?
5. **Evaluate:** Are slash commands discoverable? Is the autocomplete responsive?

### G9: @/# References

1. Type `@` in chat input
2. `browser_snapshot` -> Autocomplete for file references?
3. Type `#` in chat input
4. `browser_snapshot` -> Autocomplete for goals/features?
5. **Evaluate:** Are smart references working? Is the autocomplete fast?

### G10: Send Agentic Message (if LLM available)

1. Type a coding task (e.g., "List all Python files in the workspace")
2. Send message
3. `browser_wait_for` -> Agent response (streaming text, tool calls)
4. `browser_snapshot` -> Streaming indicator? Step counter? Agentic badge?
5. **Evaluate:** Is the agentic flow clear? Can the user tell the agent is working? Is there a stop button?

### G11: HITL Permission Cards

1. If a PermissionRequestCard appears during agent execution:
2. `browser_snapshot` -> Card shows: tool name, command, countdown timer, Allow/Deny/Always buttons?
3. Click "Allow"
4. `browser_snapshot` -> Card resolves, execution continues?
5. **Evaluate:** Is the permission UI clear? Is the countdown stressful? Are the options understandable?

---

## Phase H: Cross-Cutting Concerns

### H1: Panel Collapse/Expand

1. Click "Collapse" button (<<) to collapse left panel
2. `browser_snapshot` -> Left panel hidden? Chat panel takes full width?
3. Click expand to restore
4. **Evaluate:** Is the collapse toggle discoverable? Does the layout adjust smoothly?

### H2: Mobile Responsiveness

1. `browser_resize` -> width: 375, height: 812 (iPhone)
2. `browser_snapshot` -> Mobile layout?
   - Panels/Chat toggle?
   - Hamburger menu?
   - Touch-friendly targets (min 44px)?
3. Navigate between panels and chat on mobile
4. `browser_resize` -> restore to 1280x800
5. **Evaluate:** Is mobile usable? Or is it desktop-only?

### H3: Error States

1. Navigate to a project that has no workspace:
   - `browser_snapshot` -> How does FilePanel handle missing workspace?
2. Navigate to a non-existent project ID:
   - `browser_snapshot` -> 404 page? Error message?
3. **Evaluate:** Are error states informative? Do they suggest next steps?

### H4: Loading States

1. Observe each panel switch — is there a loading indicator?
2. Observe chat message send — is there a typing/streaming indicator?
3. Observe file tree load — is there a skeleton or spinner?
4. **Evaluate:** Are all async operations covered with loading states?

### H5: Keyboard Navigation

1. Tab through the page
2. Can you reach all interactive elements?
3. Can you switch panels with keyboard?
4. Is focus management correct after modal close?
5. **Evaluate:** Is the page keyboard-accessible?

---

## Report Template

Save as `docs/testing/YYYY-MM-DD-project-detail-ux-report.md`:

```markdown
# Project Detail Page — UX Evaluation Report

**Date:** YYYY-MM-DD
**Project tested:** [name] (UUID)
**Model used:** [if agentic tests ran]

## Summary

| Category | Features Tested | PASS | PARTIAL | FAIL | BROKEN |
|----------|----------------|------|---------|------|--------|
| Header & Actions | N | | | | |
| File Management | N | | | | |
| Goals | N | | | | |
| Roadmap & Planning | N | | | | |
| Agent & Execution | N | | | | |
| Intelligence & System | N | | | | |
| Chat | N | | | | |
| Cross-Cutting | N | | | | |

## Value Assessment

| Feature | Value Rating | Recommendation |
|---------|-------------|----------------|
| [feature] | ESSENTIAL/USEFUL/MARGINAL/UNNECESSARY | KEEP/IMPROVE/REDESIGN/REMOVE |

## Top 5 Issues (by impact)

1. [Most impactful issue — what, why, fix]
2. ...

## Top 5 UX Wins (what works well)

1. [Best UX moment — what, why it works]
2. ...

## Detailed Feature Evaluations

[One evaluation block per feature using the template above]

## Recommendations Summary

### Must Fix (blocking usability)
- ...

### Should Improve (friction reduction)
- ...

### Nice to Have (polish)
- ...

### Consider Removing (adds complexity without value)
- ...
```

---

## Key Files Reference

| Component | File |
|-----------|------|
| ProjectDetailPage | `frontend/src/features/project/ProjectDetailPage.tsx` |
| FilePanel | `frontend/src/features/project/FilePanel.tsx` |
| GoalsPanel | `frontend/src/features/project/GoalsPanel.tsx` |
| RoadmapPanel | `frontend/src/features/project/RoadmapPanel.tsx` |
| FeatureMapPanel | `frontend/src/features/project/FeatureMapPanel.tsx` |
| ChatPanel | `frontend/src/features/project/ChatPanel.tsx` |
| ChatMessages | `frontend/src/features/project/ChatMessages.tsx` |
| ChatInput | `frontend/src/features/chat/ChatInput.tsx` |
| ChatSuggestions | `frontend/src/features/project/ChatSuggestions.tsx` |
| ActiveWorkPanel | `frontend/src/features/project/ActiveWorkPanel.tsx` |
| PermissionRequestCard | `frontend/src/features/project/PermissionRequestCard.tsx` |
| ToolCallCard | `frontend/src/features/project/ToolCallCard.tsx` |
| GoalProposalCard | `frontend/src/features/project/GoalProposalCard.tsx` |
| CompactSettingsPopover | `frontend/src/features/project/CompactSettingsPopover.tsx` |
| PolicyPanel | `frontend/src/features/project/PolicyPanel.tsx` |
| WarRoom (War Room) | `frontend/src/features/project/WarRoom.tsx` |
| SessionPanel | `frontend/src/features/project/SessionPanel.tsx` |
| TrajectoryPanel | `frontend/src/features/project/TrajectoryPanel.tsx` |
| AgentPanel | `frontend/src/features/project/AgentPanel.tsx` |
| BoundariesPanel | `frontend/src/features/project/BoundariesPanel.tsx` |
| LSPPanel (Code Intelligence) | `frontend/src/features/project/LSPPanel.tsx` |
| RetrievalPanel | `frontend/src/features/project/RetrievalPanel.tsx` |
| PlanPanel | `frontend/src/features/project/PlanPanel.tsx` |
| TaskPanel | `frontend/src/features/project/TaskPanel.tsx` |
| CanvasModal | `frontend/src/features/canvas/CanvasModal.tsx` |
| DiffSummaryModal | `frontend/src/features/chat/DiffSummaryModal.tsx` |
| RewindTimeline | `frontend/src/features/chat/RewindTimeline.tsx` |
