# Project Workflow Redesign — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Improve the project detail UX with tab reordering, lifecycle progress indicator, proactive agent greeting, contextual chat suggestions, and empty state guidance.

**Architecture:** Pure frontend changes. No new API endpoints or DB migrations. All onboarding state derived from existing data (project object, goals, roadmap items, sessions). Greeting flag stored in localStorage.

**Tech Stack:** SolidJS, TypeScript, Tailwind CSS

**Design doc:** `docs/specs/2026-03-09-project-workflow-redesign-design.md`

---

### Task 1: Reorder project tabs

**Files:**
- Modify: `frontend/src/features/project/ProjectDetailPage.tsx:125-132` (LeftTab type)
- Modify: `frontend/src/features/project/ProjectDetailPage.tsx:133` (default tab signal)
- Modify: `frontend/src/features/project/ProjectDetailPage.tsx:509-574` (tab buttons)
- Modify: `frontend/src/features/project/ProjectDetailPage.tsx:597-640` (tab content Show blocks)

**Step 1: Reorder tab buttons (lines 509-574)**

Change the tab button order from: Roadmap, Feature Map, Files, War Room, Goals, Audit, Sessions, Trajectory
To: Files, Goals, Roadmap, Feature Map, War Room, Sessions, Trajectory, Audit

The Files tab (currently lines 525-534, wrapped in `<Show when={p().workspace_path}>`) moves to position 1.
Then Goals (currently 543-549), Roadmap (currently 509-516), Feature Map (currently 517-524),
War Room (currently 535-542), Sessions (currently 559-566), Trajectory (currently 567-574), Audit (currently 551-558).

**Step 2: Reorder tab content Show blocks (lines 597-640)**

Match the same order as the buttons. The `<Show when={leftTab() === "..."}>` blocks must follow:
files, goals, roadmap, featuremap, warroom, sessions, trajectory, audit.

**Step 3: Update default tab**

Change line 133 from:
```typescript
const [leftTab, setLeftTab] = createSignal<LeftTab>("roadmap");
```
To:
```typescript
const [leftTab, setLeftTab] = createSignal<LeftTab>("files");
```

The existing `createEffect` (lines 136-144) that auto-selects "files" when workspace exists is now redundant since "files" is the default. Remove lines 136-144.

**Step 4: Verify in browser**

Run: `cd frontend && npm run dev`
Open a project detail page. Verify tabs appear in new order: Files, Goals, Roadmap, Feature Map, War Room, Sessions, Trajectory, Audit.

**Step 5: Commit**

```bash
git add frontend/src/features/project/ProjectDetailPage.tsx
git commit -m "feat(ui): reorder project tabs to match natural workflow"
```

---

### Task 2: Add i18n keys for onboarding and empty states

**Files:**
- Modify: `frontend/src/i18n/en.ts`

**Step 1: Add all new i18n keys**

Add these keys to the translations object (find the `"goals.empty"` line ~1448 and add nearby, or append to the relevant section):

```typescript
// Onboarding progress
"onboarding.repoCloned": "Repo cloned",
"onboarding.stackDetected": "Stack detected",
"onboarding.goalsDefined": "Goals defined",
"onboarding.roadmapCreated": "Roadmap created",
"onboarding.firstRun": "First agent run",
"onboarding.dismiss": "Dismiss",

// Empty states
"empty.files": "No workspace linked. Clone a repo or adopt a local directory.",
"empty.files.action": "Setup Workspace",
"empty.goals": "No goals defined yet.",
"empty.goals.action": "Start a chat to define goals together",
"empty.roadmap": "Define goals first, then the agent can create a roadmap.",
"empty.roadmap.action": "Go to Goals",
"empty.featuremap": "Create a roadmap first to visualize features.",
"empty.featuremap.action": "Go to Roadmap",
"empty.warroom": "No active agents. Start a conversation to begin.",
"empty.warroom.action": "Open Chat",
"empty.sessions": "No agent sessions yet. Start a chat to get going.",
"empty.sessions.action": "Open Chat",
"empty.trajectory": "No trajectory data. Run an agent first.",
"empty.trajectory.action": "Go to Sessions",
"empty.audit": "No audit events recorded yet.",

// Chat suggestions
"chat.suggestion.explainStructure": "Explain the project structure",
"chat.suggestion.findEntryPoints": "Find entry points",
"chat.suggestion.defineGoals": "Help me define goals",
"chat.suggestion.setPriorities": "Set priorities",
"chat.suggestion.createRoadmap": "Create roadmap from goals",
"chat.suggestion.planMvp": "Plan MVP",
"chat.suggestion.analyzeFeatures": "Analyze features",
"chat.suggestion.showDeps": "Show dependencies",
"chat.suggestion.startAgent": "Start an agent",
"chat.suggestion.explainStatus": "Explain current status",
"chat.suggestion.summarizeSession": "Summarize last session",
"chat.suggestion.continueWork": "Continue where we left off",
"chat.suggestion.explainTrajectory": "Explain this trajectory",
"chat.suggestion.whatWentWrong": "What went wrong?",
"chat.suggestion.summarizeChanges": "Summarize recent changes",
"chat.suggestion.showSecurityEvents": "Show security events",
```

**Step 2: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`

**Step 3: Commit**

```bash
git add frontend/src/i18n/en.ts
git commit -m "feat(i18n): add onboarding, empty state, and chat suggestion keys"
```

---

### Task 3: Create OnboardingProgress component

**Files:**
- Create: `frontend/src/features/project/OnboardingProgress.tsx`

**Step 1: Create the component**

```tsx
import { createMemo, createSignal, For, Show } from "solid-js";

import { useI18n } from "~/i18n";
import type { TranslationKey } from "~/i18n/en";

interface OnboardingStep {
  key: TranslationKey;
  done: boolean;
  action?: () => void;
}

interface OnboardingProgressProps {
  projectId: string;
  hasWorkspace: boolean;
  hasStack: boolean;
  hasGoals: boolean;
  hasRoadmap: boolean;
  hasRuns: boolean;
  onNavigate: (tab: string) => void;
}

export default function OnboardingProgress(props: OnboardingProgressProps) {
  const { t } = useI18n();

  const dismissKey = () => `codeforge:onboarding-dismissed:${props.projectId}`;
  const [dismissed, setDismissed] = createSignal(
    localStorage.getItem(dismissKey()) === "true",
  );

  const steps = createMemo<OnboardingStep[]>(() => [
    { key: "onboarding.repoCloned", done: props.hasWorkspace },
    { key: "onboarding.stackDetected", done: props.hasStack },
    {
      key: "onboarding.goalsDefined",
      done: props.hasGoals,
      action: () => props.onNavigate("goals"),
    },
    {
      key: "onboarding.roadmapCreated",
      done: props.hasRoadmap,
      action: () => props.onNavigate("roadmap"),
    },
    {
      key: "onboarding.firstRun",
      done: props.hasRuns,
      action: () => props.onNavigate("chat"),
    },
  ]);

  const allDone = createMemo(() => steps().every((s) => s.done));

  function handleDismiss() {
    localStorage.setItem(dismissKey(), "true");
    setDismissed(true);
  }

  return (
    <Show when={!dismissed() && !allDone()}>
      <div class="flex items-center gap-3 px-4 py-2 border-b border-cf-border bg-cf-bg-secondary text-xs">
        <For each={steps()}>
          {(step, i) => (
            <>
              <Show when={i() > 0}>
                <span class="text-cf-text-muted">&rarr;</span>
              </Show>
              <button
                class={
                  "flex items-center gap-1 rounded px-1.5 py-0.5 transition-colors " +
                  (step.done
                    ? "text-green-600"
                    : step.action
                      ? "text-cf-text-muted hover:text-cf-accent cursor-pointer"
                      : "text-cf-text-muted cursor-default")
                }
                onClick={() => step.action?.()}
                disabled={step.done || !step.action}
              >
                <span>{step.done ? "\u2713" : "\u25CB"}</span>
                <span>{t(step.key)}</span>
              </button>
            </>
          )}
        </For>
        <button
          class="ml-auto text-cf-text-muted hover:text-cf-text-primary"
          onClick={handleDismiss}
          title={t("onboarding.dismiss")}
        >
          &times;
        </button>
      </div>
    </Show>
  );
}
```

**Step 2: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`

**Step 3: Commit**

```bash
git add frontend/src/features/project/OnboardingProgress.tsx
git commit -m "feat(ui): add OnboardingProgress component"
```

---

### Task 4: Integrate OnboardingProgress into ProjectDetailPage

**Files:**
- Modify: `frontend/src/features/project/ProjectDetailPage.tsx`

**Step 1: Add import**

Add at the top imports section:
```typescript
import OnboardingProgress from "./OnboardingProgress";
```

**Step 2: Load onboarding data**

After the existing resource signals (around line 50-70 where `project`, `gitStatus` etc. are defined), add resources for goals, roadmap items, and sessions counts. Check what resources are already loaded on the page. If `goals`, `roadmap`, and `sessions` are not already fetched, add:

```typescript
const [goals] = createResource(
  () => params.id,
  (pid) => api.goals.list(pid).catch(() => []),
);
const [roadmapItems] = createResource(
  () => params.id,
  (pid) => api.roadmap.listItems(pid).catch(() => []),
);
const [sessions] = createResource(
  () => params.id,
  (pid) => api.sessions.list(pid).catch(() => []),
);
```

Note: Check which of these are already loaded — `GoalsPanel`, `RoadmapPanel`, and `SessionPanel` fetch their own data internally. To avoid double-fetching, the OnboardingProgress can accept simple boolean props derived from the project object itself where possible, and use lightweight count endpoints or signals from child components. The simplest approach: load them at the page level and pass into child panels too if needed, or keep the child panels self-contained and only load counts here.

Preferred: Use lightweight calls just for boolean checks.

**Step 3: Place OnboardingProgress below the header bar**

Insert the component between the header bar `</div>` (around line 460) and the panels container. Inside the `{(p) => ( ... )}` block, right after the header `</div>`:

```tsx
<OnboardingProgress
  projectId={params.id}
  hasWorkspace={!!p().workspace_path}
  hasStack={!!p().config?.detected_languages}
  hasGoals={(goals() ?? []).length > 0}
  hasRoadmap={(roadmapItems() ?? []).length > 0}
  hasRuns={(sessions() ?? []).length > 0}
  onNavigate={(tab) => {
    if (tab === "chat") {
      // Focus chat panel — could scroll to chat or expand it
    } else {
      setLeftTab(tab as LeftTab);
    }
  }}
/>
```

**Step 4: Verify in browser**

Open a project. The progress bar should appear below the header showing which steps are complete.

**Step 5: Commit**

```bash
git add frontend/src/features/project/ProjectDetailPage.tsx
git commit -m "feat(ui): integrate OnboardingProgress into project detail page"
```

---

### Task 5: Create ChatSuggestions component

**Files:**
- Create: `frontend/src/features/project/ChatSuggestions.tsx`

**Step 1: Create the component**

```tsx
import { createMemo, For } from "solid-js";

import { useI18n } from "~/i18n";
import type { TranslationKey } from "~/i18n/en";

interface ChatSuggestionsProps {
  activeTab: string;
  onSend: (text: string) => void;
}

const SUGGESTIONS: Record<string, TranslationKey[]> = {
  files: ["chat.suggestion.explainStructure", "chat.suggestion.findEntryPoints"],
  goals: ["chat.suggestion.defineGoals", "chat.suggestion.setPriorities"],
  roadmap: ["chat.suggestion.createRoadmap", "chat.suggestion.planMvp"],
  featuremap: ["chat.suggestion.analyzeFeatures", "chat.suggestion.showDeps"],
  warroom: ["chat.suggestion.startAgent", "chat.suggestion.explainStatus"],
  sessions: ["chat.suggestion.summarizeSession", "chat.suggestion.continueWork"],
  trajectory: ["chat.suggestion.explainTrajectory", "chat.suggestion.whatWentWrong"],
  audit: ["chat.suggestion.summarizeChanges", "chat.suggestion.showSecurityEvents"],
};

export default function ChatSuggestions(props: ChatSuggestionsProps) {
  const { t } = useI18n();

  const suggestions = createMemo(() => SUGGESTIONS[props.activeTab] ?? []);

  return (
    <div class="flex gap-1.5 overflow-x-auto scrollbar-none px-3 py-1.5">
      <For each={suggestions()}>
        {(key) => (
          <button
            class="flex-shrink-0 rounded-full border border-cf-border bg-cf-bg-surface px-3 py-1 text-xs text-cf-text-muted hover:border-cf-accent hover:text-cf-accent transition-colors"
            onClick={() => props.onSend(t(key))}
          >
            {t(key)}
          </button>
        )}
      </For>
    </div>
  );
}
```

**Step 2: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`

**Step 3: Commit**

```bash
git add frontend/src/features/project/ChatSuggestions.tsx
git commit -m "feat(ui): add ChatSuggestions component with tab-aware prompts"
```

---

### Task 6: Integrate ChatSuggestions into ChatPanel + pass activeTab

**Files:**
- Modify: `frontend/src/features/project/ChatPanel.tsx:25-27` (props interface)
- Modify: `frontend/src/features/project/ChatPanel.tsx:670-693` (input area)
- Modify: `frontend/src/features/project/ProjectDetailPage.tsx` (pass activeTab prop)

**Step 1: Add activeTab prop to ChatPanel**

In `ChatPanel.tsx`, update the props interface (line 25-27):
```typescript
interface ChatPanelProps {
  projectId: string;
  activeTab?: string;
}
```

**Step 2: Import and place ChatSuggestions**

Add import at top of `ChatPanel.tsx`:
```typescript
import ChatSuggestions from "./ChatSuggestions";
```

Place the suggestions component above the textarea row. Find the input area (around line 640-693 area, the `<div class="flex items-end gap-2 ...">` that wraps textarea + send button). Insert just before that div:

```tsx
<ChatSuggestions
  activeTab={props.activeTab ?? "files"}
  onSend={(text) => {
    setInput(text);
    handleSend();
  }}
/>
```

Note: `onSend` sets the input and immediately sends. Since `handleSend` reads from `input()` signal and `setInput` is synchronous in SolidJS, this will work. However, verify that `handleSend` trims and checks for empty — if `setInput` hasn't flushed before `handleSend` reads `input()`, use a direct approach instead:

```tsx
onSend={async (text) => {
  const content = text.trim();
  if (!content || !activeConversation() || sending()) return;
  setSending(true);
  setRunError(null);
  try {
    const convId = activeConversation();
    if (!convId) return;
    await api.conversations.send(convId, { content });
    await refetchMessages();
    scrollToBottom();
  } catch {
    // handled by API layer
  } finally {
    setSending(false);
  }
}}
```

Simpler alternative: just set the input and let the user press Send:
```tsx
onSend={(text) => setInput(text)}
```

This is safer and gives the user control. Use this simpler approach.

**Step 3: Pass activeTab from ProjectDetailPage to ChatPanel**

In `ProjectDetailPage.tsx`, find where `<ChatPanel projectId={params.id} />` is rendered (around line 690). Add the prop:
```tsx
<ChatPanel projectId={params.id} activeTab={leftTab()} />
```

**Step 4: Verify in browser**

Open a project. Switch between tabs. Chat suggestions should update based on the active tab.

**Step 5: Commit**

```bash
git add frontend/src/features/project/ChatPanel.tsx frontend/src/features/project/ProjectDetailPage.tsx
git commit -m "feat(ui): integrate contextual chat suggestions based on active tab"
```

---

### Task 7: Add empty states to tab panels

**Files:**
- Modify: `frontend/src/features/project/GoalsPanel.tsx`
- Modify: `frontend/src/features/project/RoadmapPanel.tsx`
- Modify: `frontend/src/features/project/FeatureMapPanel.tsx`
- Modify: `frontend/src/features/project/WarRoom.tsx`
- Modify: `frontend/src/features/project/SessionPanel.tsx`
- Modify: `frontend/src/features/project/TrajectoryPanel.tsx`
- Modify: `frontend/src/features/project/FilePanel.tsx`
- Modify: `frontend/src/features/audit/AuditTable.tsx`

For each panel, the approach is the same:
1. Find the existing empty state (if any) or the place where an empty list would render
2. Replace with a styled empty state that includes a message and an action link

**Empty state pattern** (reuse across all panels):

```tsx
<div class="flex flex-col items-center justify-center gap-3 py-12 text-center">
  <p class="text-sm text-cf-text-muted">{t("empty.<tab>")}</p>
  <button
    class="text-sm text-cf-accent hover:underline"
    onClick={() => /* action */}
  >
    {t("empty.<tab>.action")}
  </button>
</div>
```

**Step 1: GoalsPanel**

Find the existing empty message (line ~197 — `{t("goals.empty")}`). Replace with:
```tsx
<div class="flex flex-col items-center justify-center gap-3 py-12 text-center">
  <p class="text-sm text-cf-text-muted">{t("empty.goals")}</p>
  <button
    class="text-sm text-cf-accent hover:underline"
    onClick={() => props.onNavigate?.("chat")}
  >
    {t("empty.goals.action")}
  </button>
</div>
```

This requires adding `onNavigate?: (target: string) => void` to GoalsPanel's props and passing it from ProjectDetailPage.

**Step 2: RoadmapPanel**

Find where the empty roadmap list renders. Add empty state with link to Goals tab.
Requires `onNavigate` prop same as GoalsPanel.

**Step 3: FeatureMapPanel**

Find empty state. Add link to Roadmap tab.
Requires `onNavigate` prop.

**Step 4: WarRoom, SessionPanel, TrajectoryPanel**

Add empty states with appropriate messages and actions.
WarRoom and SessionPanel link to chat. TrajectoryPanel links to Sessions tab.

**Step 5: FilePanel**

Find where the panel shows when no workspace exists. It should already be hidden (tab is conditional on `workspace_path`), but add an empty state for when workspace exists but has no files.

**Step 6: AuditTable**

Find empty state. Add message only (no action link needed).

**Step 7: Pass onNavigate prop from ProjectDetailPage**

For each panel that needs it, add `onNavigate` prop. In ProjectDetailPage, pass:
```tsx
<GoalsPanel
  projectId={params.id}
  onNavigate={(target) => {
    if (target === "chat") { /* focus chat */ }
    else setLeftTab(target as LeftTab);
  }}
/>
```

Do the same for RoadmapPanel, FeatureMapPanel, WarRoom, SessionPanel, TrajectoryPanel.

**Step 8: Verify in browser**

Open a project with no goals. The Goals tab should show the empty state with a chat link. Same for other tabs.

**Step 9: Commit**

```bash
git add frontend/src/features/project/*.tsx frontend/src/features/audit/AuditTable.tsx
git commit -m "feat(ui): add empty states with navigation links to all tab panels"
```

---

### Task 8: Add proactive agent greeting on first chat open

**Files:**
- Modify: `frontend/src/features/project/ChatPanel.tsx`

**Step 1: Add greeting trigger logic**

After the existing `createEffect` blocks (around line 134-139), add greeting logic:

```typescript
// Proactive greeting on first chat open
const greetingKey = () => `codeforge:greeted:${props.projectId}`;
const [greeted, setGreeted] = createSignal(
  localStorage.getItem(greetingKey()) === "true",
);

createEffect(() => {
  const convId = activeConversation();
  const msgs = messages();
  // Trigger greeting only when:
  // 1. Conversation is loaded
  // 2. No messages exist yet (fresh conversation)
  // 3. Not already greeted for this project
  // 4. Not currently sending
  if (convId && msgs && msgs.length === 0 && !greeted() && !sending()) {
    setGreeted(true);
    localStorage.setItem(greetingKey(), "true");
    // Send a system-triggered greeting prompt
    const greetingPrompt =
      "You are opening this project for the first time with the user. " +
      "Greet them briefly, summarize what you know about this project " +
      "(tech stack, structure, any detected specs or goals), " +
      "and ask what they'd like to achieve. " +
      "Guide them toward defining goals and creating an MVP plan.";
    setSending(true);
    api.conversations
      .send(convId, { content: greetingPrompt, role: "system" })
      .then(() => refetchMessages())
      .then(() => scrollToBottom())
      .catch(() => {
        // If greeting fails, allow retry next time
        localStorage.removeItem(greetingKey());
        setGreeted(false);
      })
      .finally(() => setSending(false));
  }
});
```

**Important:** Check whether `api.conversations.send()` supports a `role` field. If it only accepts `content`, the greeting prompt needs to be sent as a user message with a special prefix that the agent loop recognizes, or sent via a different mechanism. Verify the API contract:
- Check `frontend/src/api/client.ts` for the `send` method signature
- Check the Go handler and NATS payload for role support
- If role is not supported, send as a user message: `"[Project Onboarding] Please greet me and summarize what you know about this project. Then help me define goals and create an MVP plan."`

**Step 2: Verify in browser**

Create a new project, clone a repo, open the chat for the first time. The agent should send a greeting. Refresh the page — greeting should not repeat.

**Step 3: Commit**

```bash
git add frontend/src/features/project/ChatPanel.tsx
git commit -m "feat(ui): add proactive agent greeting on first chat open"
```

---

### Task 9: Run linting, formatting, and verify

**Step 1: Run pre-commit**

```bash
poetry run pre-commit run --all-files
```

Fix any issues found.

**Step 2: Run TypeScript check**

```bash
cd frontend && npx tsc --noEmit
```

Fix any type errors.

**Step 3: Run ESLint**

```bash
cd frontend && npx eslint src/features/project/*.tsx src/features/audit/AuditTable.tsx --fix
```

**Step 4: Final commit if fixes were needed**

```bash
git add -A
git commit -m "fix: lint and formatting fixes for project workflow redesign"
```

---

### Task 10: Update documentation

**Files:**
- Modify: `docs/todo.md` — mark project workflow redesign as completed
- Modify: `docs/project-status.md` — add milestone entry

**Step 1: Update todo.md**

Add entry for completed work or mark existing entry as done.

**Step 2: Update project-status.md**

Add note about the project workflow redesign completion.

**Step 3: Commit and push**

```bash
git add docs/
git commit -m "docs: update todo and project status for workflow redesign"
git push origin staging
```
