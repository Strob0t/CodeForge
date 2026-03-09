# Project Workflow Redesign

**Date:** 2026-03-09
**Status:** Approved
**Scope:** Frontend UX improvements for the project detail experience

## Problem

The current project detail page lacks guidance for new users. Eight tabs without clear ordering, no indication of project setup progress, and no proactive help from the agent make the experience disorienting — especially for first-time users.

## Design Decisions

### 1. Tab Reordering

Reorder project detail tabs to follow the natural workflow: understand context, set goals, plan, work, review.

**Current order:** Roadmap, Feature Map, Files, War Room, Goals, Audit, Sessions, Trajectory

**New order:**

| # | Tab | Purpose |
|---|-----|---------|
| 1 | Files | Understand what's in the repo |
| 2 | Goals | Define what to achieve |
| 3 | Roadmap | Plan how to get there |
| 4 | Feature Map | Visual feature overview |
| 5 | War Room | Observe active agent work |
| 6 | Sessions | Review past agent sessions |
| 7 | Trajectory | Detailed agent replay |
| 8 | Audit | Admin/compliance log |

**Implementation:** Reorder tab buttons and tab content `<Show>` blocks in `ProjectDetailPage.tsx` (lines 507-634).

### 2. Lifecycle Progress Indicator

A horizontal progress bar below the project header showing setup and onboarding state.

**Steps:**

```
Repo cloned -> Stack detected -> Goals defined -> Roadmap created -> First agent run
```

**Behavior:**

- Each step is automatically marked complete based on project state:
  - "Repo cloned" = `project.workspace_path` is not empty
  - "Stack detected" = project has stack detection results (config or detected languages)
  - "Goals defined" = project has at least one goal in GoalsPanel
  - "Roadmap created" = project has at least one roadmap item
  - "First agent run" = project has at least one session/run
- Incomplete steps are clickable: navigate to the relevant tab or open chat
- The bar disappears when all steps are complete (or user clicks "Dismiss")
- Dismiss state is persisted per project (localStorage or project config)

**Data source:** A new lightweight API endpoint `GET /api/v1/projects/{id}/onboarding-status` that returns boolean flags for each step. Alternatively, derive from existing data already loaded on the project detail page (goals count, roadmap count, sessions count, workspace_path).

**Preferred approach:** Derive from data already available on the page — no new endpoint needed. The progress indicator is a pure frontend component that reads from existing resources/signals.

### 3. Proactive Agent Greeting

When the chat panel of a project is opened for the first time, the agent sends a proactive greeting message.

**Trigger:** First time the chat is opened for a given project + user combination. A `greeted` flag is stored per project/user.

**Flow:**

1. User opens project chat (right panel) for the first time
2. Frontend checks `greeted` flag (from project config or localStorage)
3. If `greeted === false`:
   - Frontend sends a system-level message to the agent loop containing project context:
     - Stack detection results (languages, frameworks, tools)
     - Repo stats (file count, size, last commit)
     - Any detected specs or README summary
     - Detected goals (if any from auto-discovery)
   - The agent receives this context and generates a greeting that:
     - Summarizes what it found in the repo
     - Asks the user about their goals and intentions
     - Guides toward MVP definition and roadmap creation
   - Results from the conversation (goals, roadmap items) are persisted to the respective tabs
4. Set `greeted = true` for this project/user

**Storage:** `greeted` flag stored in localStorage as `codeforge:greeted:{projectId}`. No backend change needed for the flag itself.

**System prompt addition:** A greeting-specific system prompt template that includes project context and instructs the agent to summarize findings and guide the user through goal/roadmap definition.

### 4. Contextual Chat Prompts

The chat panel shows context-aware suggestion chips based on the currently active tab.

**Prompt mapping:**

| Active Tab | Suggestions |
|------------|-------------|
| Files | "Explain the project structure", "Find entry points" |
| Goals | "Help me define goals", "Set priorities" |
| Roadmap | "Create roadmap from goals", "Plan MVP" |
| Feature Map | "Analyze features", "Show dependencies" |
| War Room | "Start an agent", "Explain current status" |
| Sessions | "Summarize last session", "Continue where we left off" |
| Trajectory | "Explain this trajectory", "What went wrong?" |
| Audit | "Summarize recent changes", "Show security events" |

**Behavior:**

- Chips appear above the chat input field as a horizontal scrollable row
- Clicking a chip inserts its text as the user message and sends it
- Chips are always visible (not only when chat is empty) but are subtle/compact
- The active tab name is communicated to the chat panel via a signal/prop

### 5. Empty State Tab Links

Empty tabs display a helpful message with a link to the next logical step in the workflow.

**Messages:**

| Tab (empty) | Message | Action |
|-------------|---------|--------|
| Files | "No workspace linked. Clone a repo or adopt a local directory." | Button: "Setup Workspace" (triggers clone/adopt) |
| Goals | "No goals defined yet." | Link: "Start a chat to define goals together" (opens chat with prompt) |
| Roadmap | "Define goals first, then the agent can create a roadmap." | Link: "Go to Goals" (switches tab) |
| Feature Map | "Create a roadmap first to visualize features." | Link: "Go to Roadmap" (switches tab) |
| War Room | "No active agents. Start a conversation to begin." | Link: "Open Chat" (focuses chat input) |
| Sessions | "No agent sessions yet. Start a chat to get going." | Link: "Open Chat" (focuses chat input) |
| Trajectory | "No trajectory data. Run an agent first." | Link: "Go to Sessions" (switches tab) |
| Audit | "No audit events recorded yet." | (no action needed) |

**Behavior:**

- Empty states replace the current empty/blank content in each tab
- Links/buttons trigger tab switches or focus the chat input
- Empty states disappear as soon as content exists

## Files to Modify

| File | Changes |
|------|---------|
| `frontend/src/features/project/ProjectDetailPage.tsx` | Tab reorder, progress indicator component, active tab signal to chat |
| `frontend/src/features/project/ChatPanel.tsx` | Contextual prompt chips, greeting trigger logic |
| `frontend/src/features/project/GoalsPanel.tsx` | Empty state with chat link |
| `frontend/src/features/project/RoadmapPanel.tsx` | Empty state with Goals link |
| `frontend/src/features/project/FeatureMapPanel.tsx` | Empty state with Roadmap link |
| `frontend/src/features/project/WarRoom.tsx` | Empty state with chat link |
| `frontend/src/features/project/SessionPanel.tsx` | Empty state with chat link |
| `frontend/src/features/project/TrajectoryPanel.tsx` | Empty state with Sessions link |
| `frontend/src/features/project/FilePanel.tsx` | Empty state with setup action |
| `frontend/src/features/audit/AuditTable.tsx` | Empty state message |

## New Components

| Component | Purpose |
|-----------|---------|
| `frontend/src/features/project/OnboardingProgress.tsx` | Lifecycle progress indicator bar |
| `frontend/src/features/project/ChatSuggestions.tsx` | Contextual chat prompt chips |

## Out of Scope

- Backend API changes (all data derived from existing endpoints)
- New database tables or migrations
- Changes to the agent loop or LLM prompts (greeting uses existing agentic conversation flow with a system message)
- Global sidebar reordering
- Mobile-specific layout changes (responsive behavior stays the same)
