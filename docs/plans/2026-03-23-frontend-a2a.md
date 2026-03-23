# Frontend A2A Federation Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create a new `/a2a` page that wires 12 A2A backend endpoints into a tabbed UI for managing remote agents, tasks, and push notification configs.

**Architecture:** New top-level route with three tabs (Agents | Tasks | Push Configs), each with Table + CRUD forms. New API resource file, types in api/types.ts, i18n keys, nav link, route registration.

**Tech Stack:** SolidJS, TypeScript, Tailwind CSS

---

### Task 1: Add TypeScript types

**Files:**
- Modify: `frontend/src/api/types.ts`

- [ ] **Step 1: Add A2A Remote Agent type**

```typescript
/** Matches Go domain/a2a.RemoteAgent */
export interface A2ARemoteAgent {
  id: string;
  name: string;
  url: string;
  description: string;
  trust_level: string;
  enabled: boolean;
  skills: string[];
  last_seen?: string;
  card_json?: string;
  tenant_id: string;
  created_at: string;
  updated_at: string;
}

/** Create remote agent request */
export interface CreateA2ARemoteAgentRequest {
  name: string;
  url: string;
  trust_level?: string;
}
```

- [ ] **Step 2: Add A2A Task type**

```typescript
/** A2A task state (matches Go domain/a2a.TaskState) */
export type A2ATaskState =
  | "submitted"
  | "working"
  | "completed"
  | "failed"
  | "canceled"
  | "rejected"
  | "input-required"
  | "auth-required";

/** A2A task direction */
export type A2ATaskDirection = "inbound" | "outbound";

/** Matches Go domain/a2a.A2ATask */
export interface A2ATask {
  id: string;
  context_id: string;
  state: A2ATaskState;
  direction: A2ATaskDirection;
  skill_id: string;
  trust_origin: string;
  trust_level: string;
  source_addr: string;
  project_id: string;
  remote_agent_id: string;
  tenant_id: string;
  metadata: Record<string, string>;
  error_message: string;
  version: number;
  created_at: string;
  updated_at: string;
}

/** Send A2A task request */
export interface SendA2ATaskRequest {
  skill_id: string;
  prompt: string;
}
```

- [ ] **Step 3: Add A2A Push Config type**

```typescript
/** Matches Go database.A2APushConfig */
export interface A2APushConfig {
  id: string;
  task_id: string;
  url: string;
  token: string;
  created_at: string;
}

/** Create push config request */
export interface CreateA2APushConfigRequest {
  url: string;
  token?: string;
}
```

---

### Task 2: Create API resource file

**Files:**
- Create: `frontend/src/api/resources/a2a.ts`

- [ ] **Step 1: Create createA2AResource function**

```typescript
import type { CoreClient } from "../core";
import { url } from "../factory";
import type {
  A2APushConfig,
  A2ARemoteAgent,
  A2ATask,
  CreateA2APushConfigRequest,
  CreateA2ARemoteAgentRequest,
  SendA2ATaskRequest,
} from "../types";

export function createA2AResource(c: CoreClient) {
  return {
    // Remote Agents
    listAgents: () => c.get<A2ARemoteAgent[]>("/a2a/agents"),

    registerAgent: (data: CreateA2ARemoteAgentRequest) =>
      c.post<A2ARemoteAgent>("/a2a/agents", data),

    deleteAgent: (id: string) => c.del<undefined>(url`/a2a/agents/${id}`),

    discoverAgent: (id: string) =>
      c.post<A2ARemoteAgent>(url`/a2a/agents/${id}/discover`),

    // Tasks
    listTasks: (state?: string, direction?: string) => {
      const params = new URLSearchParams();
      if (state) params.set("state", state);
      if (direction) params.set("direction", direction);
      const qs = params.toString();
      return c.get<A2ATask[]>(`/a2a/tasks${qs ? `?${qs}` : ""}`);
    },

    getTask: (id: string) => c.get<A2ATask>(url`/a2a/tasks/${id}`),

    cancelTask: (id: string) =>
      c.post<{ status: string }>(url`/a2a/tasks/${id}/cancel`),

    sendTask: (agentId: string, data: SendA2ATaskRequest) =>
      c.post<A2ATask>(url`/a2a/agents/${agentId}/send`, data),

    // Push Configs
    listPushConfigs: (taskId: string) =>
      c.get<A2APushConfig[]>(url`/a2a/tasks/${taskId}/push-config`),

    createPushConfig: (taskId: string, data: CreateA2APushConfigRequest) =>
      c.post<{ id: string }>(url`/a2a/tasks/${taskId}/push-config`, data),

    deletePushConfig: (id: string) =>
      c.del<undefined>(url`/a2a/push-config/${id}`),
  };
}
```

---

### Task 3: Register API resource

**Files:**
- Modify: `frontend/src/api/resources/index.ts`
- Modify: `frontend/src/api/client.ts`

- [ ] **Step 1: Export from index.ts**

Add to `frontend/src/api/resources/index.ts`:
```typescript
export { createA2AResource } from "./a2a";
```

- [ ] **Step 2: Register in client.ts**

Add import:
```typescript
import { createA2AResource } from "./resources/a2a";
```

Add to `api` object:
```typescript
a2a: createA2AResource(core),
```

---

### Task 4: Create A2A Page component

**Files:**
- Create: `frontend/src/features/a2a/A2APage.tsx`

- [ ] **Step 1: Create page with three tabs**

```typescript
import { createResource, createSignal, For, onMount, Show } from "solid-js";

import { api } from "~/api/client";
import type {
  A2APushConfig,
  A2ARemoteAgent,
  A2ATask,
  CreateA2ARemoteAgentRequest,
  SendA2ATaskRequest,
} from "~/api/types";
import { useToast } from "~/components/Toast";
import { useAsyncAction, useCRUDForm } from "~/hooks";
import { useI18n } from "~/i18n";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  ErrorBanner,
  FormField,
  Input,
  LoadingState,
  PageLayout,
  Select,
  Table,
} from "~/ui";
import type { TableColumn } from "~/ui/composites/Table";
```

- [ ] **Step 2: Implement Agents tab content**

Table columns: Name, URL, Trust Level, Enabled, Skills, Last Seen, Actions (discover/delete).
CRUD form for registering new agents with fields: name, url, trust_level (select: untrusted/partial/verified/full).

- [ ] **Step 3: Implement Tasks tab content**

Table columns: ID, State (Badge), Direction (Badge), Skill ID, Remote Agent ID, Source, Created At, Actions (cancel for non-terminal states).
Filter selects at top: state (all/submitted/working/completed/failed/canceled/rejected/input-required/auth-required), direction (all/inbound/outbound).
Send Task form: select agent from agents list, skill_id input, prompt textarea.

- [ ] **Step 4: Implement Push Configs tab content**

Select a task first (dropdown from tasks list), then show push configs table.
Table columns: ID, URL, Token (masked), Created At, Actions (delete).
Create form: url input, token input.

- [ ] **Step 5: Wire tab switching**

```tsx
type A2ATab = "agents" | "tasks" | "push-configs";

export default function A2APage() {
  onMount(() => { document.title = "A2A Federation - CodeForge"; });
  const { t } = useI18n();
  const [activeTab, setActiveTab] = createSignal<A2ATab>("agents");

  return (
    <PageLayout title={t("a2a.title")} description={t("a2a.description")}>
      <div class="flex gap-2 mb-4 border-b border-cf-border">
        <For each={["agents", "tasks", "push-configs"] as const}>
          {(tab) => (
            <button
              class={`px-4 py-2 text-sm font-medium border-b-2 transition-colors ${
                activeTab() === tab
                  ? "border-cf-accent text-cf-accent"
                  : "border-transparent text-cf-text-muted hover:text-cf-text-primary"
              }`}
              onClick={() => setActiveTab(tab)}
            >
              {t(`a2a.tab.${tab}`)}
            </button>
          )}
        </For>
      </div>

      <Show when={activeTab() === "agents"}>
        <AgentsTab />
      </Show>
      <Show when={activeTab() === "tasks"}>
        <TasksTab />
      </Show>
      <Show when={activeTab() === "push-configs"}>
        <PushConfigsTab />
      </Show>
    </PageLayout>
  );
}
```

---

### Task 5: Add i18n keys

**Files:**
- Modify: `frontend/src/i18n/en.ts`
- Modify: `frontend/src/i18n/locales/de.ts`

- [ ] **Step 1: Add English keys**

```typescript
// -- A2A Federation -----------------------------------------------------------
"a2a.title": "A2A Federation",
"a2a.description": "Manage remote A2A agents, tasks, and push notification configs.",
"a2a.tab.agents": "Agents",
"a2a.tab.tasks": "Tasks",
"a2a.tab.push-configs": "Push Configs",
"a2a.agents.name": "Name",
"a2a.agents.url": "URL",
"a2a.agents.trustLevel": "Trust Level",
"a2a.agents.enabled": "Enabled",
"a2a.agents.skills": "Skills",
"a2a.agents.lastSeen": "Last Seen",
"a2a.agents.register": "Register Agent",
"a2a.agents.discover": "Discover",
"a2a.agents.empty": "No remote agents registered.",
"a2a.agents.toast.registered": "Remote agent registered.",
"a2a.agents.toast.deleted": "Remote agent deleted.",
"a2a.agents.toast.discovered": "Agent card refreshed.",
"a2a.tasks.state": "State",
"a2a.tasks.direction": "Direction",
"a2a.tasks.skillId": "Skill ID",
"a2a.tasks.remoteAgent": "Remote Agent",
"a2a.tasks.source": "Source",
"a2a.tasks.cancel": "Cancel",
"a2a.tasks.send": "Send Task",
"a2a.tasks.prompt": "Prompt",
"a2a.tasks.empty": "No A2A tasks found.",
"a2a.tasks.toast.cancelled": "Task cancelled.",
"a2a.tasks.toast.sent": "Task sent to remote agent.",
"a2a.tasks.filterState": "Filter by state",
"a2a.tasks.filterDirection": "Filter by direction",
"a2a.tasks.all": "All",
"a2a.pushConfigs.url": "Webhook URL",
"a2a.pushConfigs.token": "Token",
"a2a.pushConfigs.selectTask": "Select a task",
"a2a.pushConfigs.create": "Create Push Config",
"a2a.pushConfigs.empty": "No push configs for this task.",
"a2a.pushConfigs.toast.created": "Push config created.",
"a2a.pushConfigs.toast.deleted": "Push config deleted.",
"app.nav.a2a": "A2A Federation",
```

- [ ] **Step 2: Add German keys**

```typescript
// -- A2A Federation -----------------------------------------------------------
"a2a.title": "A2A-Foederation",
"a2a.description": "Verwaltung von Remote-A2A-Agenten, Aufgaben und Push-Konfigurationen.",
"a2a.tab.agents": "Agenten",
"a2a.tab.tasks": "Aufgaben",
"a2a.tab.push-configs": "Push-Konfigurationen",
"a2a.agents.name": "Name",
"a2a.agents.url": "URL",
"a2a.agents.trustLevel": "Vertrauensstufe",
"a2a.agents.enabled": "Aktiviert",
"a2a.agents.skills": "Faehigkeiten",
"a2a.agents.lastSeen": "Zuletzt gesehen",
"a2a.agents.register": "Agent registrieren",
"a2a.agents.discover": "Erkennen",
"a2a.agents.empty": "Keine Remote-Agenten registriert.",
"a2a.agents.toast.registered": "Remote-Agent registriert.",
"a2a.agents.toast.deleted": "Remote-Agent geloescht.",
"a2a.agents.toast.discovered": "Agent-Karte aktualisiert.",
"a2a.tasks.state": "Status",
"a2a.tasks.direction": "Richtung",
"a2a.tasks.skillId": "Skill-ID",
"a2a.tasks.remoteAgent": "Remote-Agent",
"a2a.tasks.source": "Quelle",
"a2a.tasks.cancel": "Abbrechen",
"a2a.tasks.send": "Aufgabe senden",
"a2a.tasks.prompt": "Prompt",
"a2a.tasks.empty": "Keine A2A-Aufgaben gefunden.",
"a2a.tasks.toast.cancelled": "Aufgabe abgebrochen.",
"a2a.tasks.toast.sent": "Aufgabe an Remote-Agent gesendet.",
"a2a.tasks.filterState": "Nach Status filtern",
"a2a.tasks.filterDirection": "Nach Richtung filtern",
"a2a.tasks.all": "Alle",
"a2a.pushConfigs.url": "Webhook-URL",
"a2a.pushConfigs.token": "Token",
"a2a.pushConfigs.selectTask": "Aufgabe auswaehlen",
"a2a.pushConfigs.create": "Push-Konfiguration erstellen",
"a2a.pushConfigs.empty": "Keine Push-Konfigurationen fuer diese Aufgabe.",
"a2a.pushConfigs.toast.created": "Push-Konfiguration erstellt.",
"a2a.pushConfigs.toast.deleted": "Push-Konfiguration geloescht.",
"app.nav.a2a": "A2A-Foederation",
```

---

### Task 6: Register route and nav link

**Files:**
- Modify: `frontend/src/index.tsx`
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Add route in index.tsx**

Add import:
```typescript
import A2APage from "./features/a2a/A2APage.tsx";
```

Add route (after `/mcp` route):
```tsx
<Route path="/a2a" component={A2APage} />
```

- [ ] **Step 2: Add to KNOWN_ROUTES in App.tsx**

```typescript
const KNOWN_ROUTES = new Set([
  "/",
  "/projects",
  "/costs",
  "/ai",
  "/activity",
  "/knowledge",
  "/mcp",
  "/a2a",       // <-- add
  "/prompts",
  "/settings",
  "/benchmarks",
]);
```

- [ ] **Step 3: Add NavLink in App.tsx sidebar**

Add in the "AI & Agents" NavSection (after MCP NavLink):
```tsx
<NavLink href="/a2a" icon={<A2AIcon />} label={t("app.nav.a2a")}>
  {t("app.nav.a2a")}
</NavLink>
```

Create a simple A2A icon (inline SVG or use an existing icon with a network/federation motif).

---

### Task 7: Verify and commit

- [ ] **Step 1: Build check**

```bash
cd frontend && npm run build 2>&1 | tail -20
```

- [ ] **Step 2: Type check**

```bash
cd frontend && npx tsc --noEmit 2>&1 | tail -20
```

- [ ] **Step 3: Commit**

```
feat: add A2A Federation page with agents, tasks, and push configs

Wire 12 A2A backend endpoints (GET/POST/DELETE /a2a/agents,
GET/POST /a2a/tasks, cancel, send, push-config CRUD) into a new
/a2a page with three tabs. Includes API resource, types, i18n
(en+de), route registration, and sidebar nav link.
```
