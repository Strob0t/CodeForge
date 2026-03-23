# Frontend Microagents & Skills Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create a new `/microagents` page that wires 11 backend endpoints (6 microagent + 5 skill) into a tabbed UI with CRUD forms and a YAML/Markdown content editor for each entity.

**Architecture:** New top-level route with two tabs (Microagents | Skills), each with a Table + CRUD form. Microagent form includes trigger pattern regex input and prompt textarea. Skill form includes a content editor (YAML/Markdown), tags input, and an import-from-URL action. Both are project-scoped (project selector dropdown). New API resource file, types in api/types.ts, i18n keys, nav link, route registration.

**Tech Stack:** SolidJS, TypeScript, Tailwind CSS

---

### Task 1: Add TypeScript types

**Files:**
- Modify: `frontend/src/api/types.ts`

- [ ] **Step 1: Add Microagent types**

```typescript
/** Microagent type (matches Go domain/microagent.Type) */
export type MicroagentType = "knowledge" | "repo" | "task";

/** Matches Go domain/microagent.Microagent */
export interface Microagent {
  id: string;
  tenant_id: string;
  project_id: string;
  name: string;
  type: MicroagentType;
  trigger_pattern: string;
  description: string;
  prompt: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

/** Matches Go domain/microagent.CreateRequest */
export interface CreateMicroagentRequest {
  project_id?: string;
  name: string;
  type: MicroagentType;
  trigger_pattern: string;
  description: string;
  prompt: string;
}

/** Matches Go domain/microagent.UpdateRequest */
export interface UpdateMicroagentRequest {
  name?: string;
  trigger_pattern?: string;
  description?: string;
  prompt?: string;
  enabled?: boolean;
}
```

- [ ] **Step 2: Add Skill types**

```typescript
/** Matches Go domain/skill.Skill */
export interface Skill {
  id: string;
  tenant_id: string;
  project_id: string;
  name: string;
  type: string;
  description: string;
  language: string;
  content: string;
  tags: string[];
  source: string;
  source_url?: string;
  format_origin: string;
  status: string;
  usage_count: number;
  created_at: string;
}

/** Matches Go domain/skill.CreateRequest */
export interface CreateSkillRequest {
  project_id?: string;
  name: string;
  type: string;
  description: string;
  language: string;
  content: string;
  tags: string[];
  source?: string;
  source_url?: string;
  format_origin?: string;
}

/** Matches Go domain/skill.UpdateRequest */
export interface UpdateSkillRequest {
  name?: string;
  type?: string;
  description?: string;
  language?: string;
  content?: string;
  tags?: string[];
  status?: string;
}

/** Import skill request */
export interface ImportSkillRequest {
  source_url: string;
  project_id?: string;
}

/** Skill import rejection response */
export interface SkillImportRejection {
  error: string;
  score: number;
  factors: string[];
}
```

---

### Task 2: Create API resource file

**Files:**
- Create: `frontend/src/api/resources/microagents.ts`

- [ ] **Step 1: Create createMicroagentsResource and createSkillsResource functions**

```typescript
import type { CoreClient } from "../core";
import { url } from "../factory";
import type {
  CreateMicroagentRequest,
  CreateSkillRequest,
  ImportSkillRequest,
  Microagent,
  Skill,
  UpdateMicroagentRequest,
  UpdateSkillRequest,
} from "../types";

export function createMicroagentsResource(c: CoreClient) {
  return {
    list: (projectId: string) =>
      c.get<Microagent[]>(url`/projects/${projectId}/microagents`),

    get: (id: string) => c.get<Microagent>(url`/microagents/${id}`),

    create: (projectId: string, data: CreateMicroagentRequest) =>
      c.post<Microagent>(url`/projects/${projectId}/microagents`, data),

    update: (id: string, data: UpdateMicroagentRequest) =>
      c.put<Microagent>(url`/microagents/${id}`, data),

    delete: (id: string) => c.del<undefined>(url`/microagents/${id}`),
  };
}

export function createSkillsResource(c: CoreClient) {
  return {
    list: (projectId: string) =>
      c.get<Skill[]>(url`/projects/${projectId}/skills`),

    get: (id: string) => c.get<Skill>(url`/skills/${id}`),

    create: (projectId: string, data: CreateSkillRequest) =>
      c.post<Skill>(url`/projects/${projectId}/skills`, data),

    update: (id: string, data: UpdateSkillRequest) =>
      c.put<Skill>(url`/skills/${id}`, data),

    delete: (id: string) => c.del<undefined>(url`/skills/${id}`),

    import: (data: ImportSkillRequest) =>
      c.post<Skill>("/skills/import", data),
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
export { createMicroagentsResource, createSkillsResource } from "./microagents";
```

- [ ] **Step 2: Register in client.ts**

Add import:
```typescript
import { createMicroagentsResource, createSkillsResource } from "./resources/microagents";
```

Add to `api` object:
```typescript
microagents: createMicroagentsResource(core),
skills: createSkillsResource(core),
```

---

### Task 4: Create Microagents & Skills Page component

**Files:**
- Create: `frontend/src/features/microagents/MicroagentsPage.tsx`

- [ ] **Step 1: Create page skeleton with project selector and tabs**

```typescript
import { createResource, createSignal, For, onMount, Show } from "solid-js";

import { api } from "~/api/client";
import type {
  CreateMicroagentRequest,
  CreateSkillRequest,
  Microagent,
  MicroagentType,
  Project,
  Skill,
  UpdateMicroagentRequest,
  UpdateSkillRequest,
} from "~/api/types";
import { useToast } from "~/components/Toast";
import { useAsyncAction, useCRUDForm } from "~/hooks";
import { useI18n } from "~/i18n";
import {
  Badge,
  Button,
  Card,
  ConfirmDialog,
  EmptyState,
  ErrorBanner,
  FormField,
  Input,
  LoadingState,
  PageLayout,
  Select,
  Table,
  Textarea,
} from "~/ui";
import type { TableColumn } from "~/ui/composites/Table";
```

- [ ] **Step 2: Implement page with tab switching and project selector**

```tsx
type MicroagentsTab = "microagents" | "skills";

export default function MicroagentsPage() {
  onMount(() => { document.title = "Microagents & Skills - CodeForge"; });
  const { t } = useI18n();
  const [activeTab, setActiveTab] = createSignal<MicroagentsTab>("microagents");
  const [selectedProjectId, setSelectedProjectId] = createSignal("");

  const [projects] = createResource(() => api.projects.list());

  return (
    <PageLayout title={t("microagents.title")} description={t("microagents.description")}>
      {/* Project selector */}
      <div class="mb-4">
        <Select
          value={selectedProjectId()}
          onChange={(v) => setSelectedProjectId(v)}
          options={(projects() ?? []).map((p: Project) => ({ value: p.id, label: p.name }))}
          placeholder={t("microagents.selectProject")}
        />
      </div>

      {/* Tabs */}
      <div class="flex gap-2 mb-4 border-b border-cf-border">
        <For each={["microagents", "skills"] as const}>
          {(tab) => (
            <button
              class={`px-4 py-2 text-sm font-medium border-b-2 transition-colors ${
                activeTab() === tab
                  ? "border-cf-accent text-cf-accent"
                  : "border-transparent text-cf-text-muted hover:text-cf-text-primary"
              }`}
              onClick={() => setActiveTab(tab)}
            >
              {t(`microagents.tab.${tab}`)}
            </button>
          )}
        </For>
      </div>

      <Show when={selectedProjectId()}>
        <Show when={activeTab() === "microagents"}>
          <MicroagentsTab projectId={selectedProjectId()} />
        </Show>
        <Show when={activeTab() === "skills"}>
          <SkillsTab projectId={selectedProjectId()} />
        </Show>
      </Show>
      <Show when={!selectedProjectId()}>
        <EmptyState message={t("microagents.selectProjectFirst")} />
      </Show>
    </PageLayout>
  );
}
```

- [ ] **Step 3: Implement MicroagentsTab component**

Table columns: Name, Type (badge), Trigger Pattern (monospace), Enabled (badge), Description (truncated), Created At, Actions (edit/delete).

CRUD form fields:
- name (Input, required)
- type (Select: knowledge/repo/task, required)
- trigger_pattern (Input monospace, required, hint: "Regex pattern that activates this microagent")
- description (Textarea)
- prompt (Textarea, large, required -- the microagent's instruction prompt)
- enabled (checkbox, default true for create, shown only in edit mode)

```tsx
function MicroagentsTab(props: { projectId: string }) {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [microagents, { refetch }] = createResource(
    () => props.projectId,
    (pid) => api.microagents.list(pid),
  );

  const FORM_DEFAULTS = {
    name: "", type: "knowledge" as MicroagentType,
    trigger_pattern: "", description: "", prompt: "",
  };

  const crud = useCRUDForm(FORM_DEFAULTS, async (ma: Microagent) => {
    await api.microagents.delete(ma.id);
    toast("success", t("microagents.toast.deleted"));
    refetch();
  });

  // Create handler
  async function handleCreate() {
    const data: CreateMicroagentRequest = {
      name: crud.form.name, type: crud.form.type,
      trigger_pattern: crud.form.trigger_pattern,
      description: crud.form.description, prompt: crud.form.prompt,
    };
    await api.microagents.create(props.projectId, data);
    toast("success", t("microagents.toast.created"));
    crud.cancelForm(); refetch();
  }

  // Update handler
  async function handleUpdate() {
    if (!crud.editItem()) return;
    const data: UpdateMicroagentRequest = {
      name: crud.form.name, trigger_pattern: crud.form.trigger_pattern,
      description: crud.form.description, prompt: crud.form.prompt,
    };
    await api.microagents.update((crud.editItem() as Microagent).id, data);
    toast("success", t("microagents.toast.updated"));
    crud.cancelForm(); refetch();
  }

  // Table + form rendering
  // ...
}
```

- [ ] **Step 4: Implement SkillsTab component**

Table columns: Name, Type (badge: workflow/pattern), Language, Status (badge: draft/active/disabled), Source, Tags (badge list), Usage Count, Created At, Actions (edit/delete).

CRUD form fields:
- name (Input, required)
- type (Select: workflow/pattern)
- description (Textarea, required)
- language (Input, e.g. "python", "go", "typescript")
- content (Textarea, large, monospace -- the skill content/code, required)
- tags (Input, comma-separated, converted to string array)
- status (Select: draft/active/disabled, shown only in edit mode)

Import action: Button "Import from URL" opens a small form with source_url input. Calls `api.skills.import({ source_url, project_id })`. Shows rejection details if safety check fails (score + factors).

```tsx
function SkillsTab(props: { projectId: string }) {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [skills, { refetch }] = createResource(
    () => props.projectId,
    (pid) => api.skills.list(pid),
  );

  const FORM_DEFAULTS = {
    name: "", type: "workflow", description: "",
    language: "", content: "", tags: "",
  };

  const crud = useCRUDForm(FORM_DEFAULTS, async (sk: Skill) => {
    await api.skills.delete(sk.id);
    toast("success", t("skills.toast.deleted"));
    refetch();
  });

  // Import state
  const [showImport, setShowImport] = createSignal(false);
  const [importUrl, setImportUrl] = createSignal("");

  async function handleImport() {
    const sk = await api.skills.import({
      source_url: importUrl(), project_id: props.projectId,
    });
    toast("success", t("skills.toast.imported"));
    setShowImport(false); setImportUrl(""); refetch();
  }

  // Create handler
  async function handleCreate() {
    const data: CreateSkillRequest = {
      name: crud.form.name, type: crud.form.type,
      description: crud.form.description, language: crud.form.language,
      content: crud.form.content,
      tags: crud.form.tags.split(",").map((s: string) => s.trim()).filter(Boolean),
    };
    await api.skills.create(props.projectId, data);
    toast("success", t("skills.toast.created"));
    crud.cancelForm(); refetch();
  }

  // Update handler
  async function handleUpdate() {
    if (!crud.editItem()) return;
    const data: UpdateSkillRequest = {
      name: crud.form.name, type: crud.form.type,
      description: crud.form.description, language: crud.form.language,
      content: crud.form.content,
      tags: crud.form.tags.split(",").map((s: string) => s.trim()).filter(Boolean),
    };
    await api.skills.update((crud.editItem() as Skill).id, data);
    toast("success", t("skills.toast.updated"));
    crud.cancelForm(); refetch();
  }

  // Table + form + import dialog rendering
  // ...
}
```

---

### Task 5: Add i18n keys

**Files:**
- Modify: `frontend/src/i18n/en.ts`
- Modify: `frontend/src/i18n/locales/de.ts`

- [ ] **Step 1: Add English keys**

```typescript
// -- Microagents & Skills -----------------------------------------------------
"microagents.title": "Microagents & Skills",
"microagents.description": "Manage trigger-based microagents and reusable skill patterns.",
"microagents.tab.microagents": "Microagents",
"microagents.tab.skills": "Skills",
"microagents.selectProject": "Select a project",
"microagents.selectProjectFirst": "Select a project to manage microagents and skills.",
"microagents.col.name": "Name",
"microagents.col.type": "Type",
"microagents.col.triggerPattern": "Trigger Pattern",
"microagents.col.enabled": "Enabled",
"microagents.col.description": "Description",
"microagents.col.createdAt": "Created",
"microagents.form.name": "Name",
"microagents.form.type": "Type",
"microagents.form.triggerPattern": "Trigger Pattern",
"microagents.form.triggerPatternHint": "Regex pattern that activates this microagent.",
"microagents.form.description": "Description",
"microagents.form.prompt": "Prompt",
"microagents.form.promptHint": "Instruction prompt injected when trigger matches.",
"microagents.create": "Create Microagent",
"microagents.empty": "No microagents found for this project.",
"microagents.toast.created": "Microagent created.",
"microagents.toast.updated": "Microagent updated.",
"microagents.toast.deleted": "Microagent deleted.",
"skills.col.name": "Name",
"skills.col.type": "Type",
"skills.col.language": "Language",
"skills.col.status": "Status",
"skills.col.source": "Source",
"skills.col.tags": "Tags",
"skills.col.usageCount": "Usage",
"skills.col.createdAt": "Created",
"skills.form.name": "Name",
"skills.form.type": "Type",
"skills.form.description": "Description",
"skills.form.language": "Language",
"skills.form.content": "Content",
"skills.form.contentHint": "Skill content (YAML, Markdown, or code).",
"skills.form.tags": "Tags",
"skills.form.tagsHint": "Comma-separated tags.",
"skills.form.status": "Status",
"skills.create": "Create Skill",
"skills.import": "Import from URL",
"skills.import.url": "Source URL",
"skills.import.urlHint": "URL to a .yaml, .md, or .mdc skill file.",
"skills.import.rejected": "Import rejected: {{error}} (score: {{score}})",
"skills.empty": "No skills found for this project.",
"skills.toast.created": "Skill created.",
"skills.toast.updated": "Skill updated.",
"skills.toast.deleted": "Skill deleted.",
"skills.toast.imported": "Skill imported successfully.",
"app.nav.microagents": "Microagents",
```

- [ ] **Step 2: Add German keys**

```typescript
// -- Microagents & Skills -----------------------------------------------------
"microagents.title": "Mikroagenten & Skills",
"microagents.description": "Verwaltung von triggerbasierten Mikroagenten und wiederverwendbaren Skill-Mustern.",
"microagents.tab.microagents": "Mikroagenten",
"microagents.tab.skills": "Skills",
"microagents.selectProject": "Projekt auswaehlen",
"microagents.selectProjectFirst": "Waehlen Sie ein Projekt, um Mikroagenten und Skills zu verwalten.",
"microagents.col.name": "Name",
"microagents.col.type": "Typ",
"microagents.col.triggerPattern": "Trigger-Muster",
"microagents.col.enabled": "Aktiviert",
"microagents.col.description": "Beschreibung",
"microagents.col.createdAt": "Erstellt",
"microagents.form.name": "Name",
"microagents.form.type": "Typ",
"microagents.form.triggerPattern": "Trigger-Muster",
"microagents.form.triggerPatternHint": "Regex-Muster, das diesen Mikroagenten aktiviert.",
"microagents.form.description": "Beschreibung",
"microagents.form.prompt": "Prompt",
"microagents.form.promptHint": "Anweisungs-Prompt, der bei Trigger-Uebereinstimmung eingefuegt wird.",
"microagents.create": "Mikroagent erstellen",
"microagents.empty": "Keine Mikroagenten fuer dieses Projekt gefunden.",
"microagents.toast.created": "Mikroagent erstellt.",
"microagents.toast.updated": "Mikroagent aktualisiert.",
"microagents.toast.deleted": "Mikroagent geloescht.",
"skills.col.name": "Name",
"skills.col.type": "Typ",
"skills.col.language": "Sprache",
"skills.col.status": "Status",
"skills.col.source": "Quelle",
"skills.col.tags": "Tags",
"skills.col.usageCount": "Nutzung",
"skills.col.createdAt": "Erstellt",
"skills.form.name": "Name",
"skills.form.type": "Typ",
"skills.form.description": "Beschreibung",
"skills.form.language": "Sprache",
"skills.form.content": "Inhalt",
"skills.form.contentHint": "Skill-Inhalt (YAML, Markdown oder Code).",
"skills.form.tags": "Tags",
"skills.form.tagsHint": "Kommagetrennte Tags.",
"skills.form.status": "Status",
"skills.create": "Skill erstellen",
"skills.import": "Von URL importieren",
"skills.import.url": "Quell-URL",
"skills.import.urlHint": "URL zu einer .yaml-, .md- oder .mdc-Skill-Datei.",
"skills.import.rejected": "Import abgelehnt: {{error}} (Bewertung: {{score}})",
"skills.empty": "Keine Skills fuer dieses Projekt gefunden.",
"skills.toast.created": "Skill erstellt.",
"skills.toast.updated": "Skill aktualisiert.",
"skills.toast.deleted": "Skill geloescht.",
"skills.toast.imported": "Skill erfolgreich importiert.",
"app.nav.microagents": "Mikroagenten",
```

---

### Task 6: Register route and nav link

**Files:**
- Modify: `frontend/src/index.tsx`
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Add route in index.tsx**

Add import:
```typescript
import MicroagentsPage from "./features/microagents/MicroagentsPage.tsx";
```

Add route (after `/mcp` route):
```tsx
<Route path="/microagents" component={MicroagentsPage} />
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
  "/microagents",  // <-- add
  "/prompts",
  "/settings",
  "/benchmarks",
]);
```

- [ ] **Step 3: Add NavLink in App.tsx sidebar**

Add in the "AI & Agents" NavSection (after MCP NavLink):
```tsx
<NavLink href="/microagents" icon={<MicroagentsIcon />} label={t("app.nav.microagents")}>
  {t("app.nav.microagents")}
</NavLink>
```

Create a simple microagents icon (inline SVG with a small-agent/puzzle motif).

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
feat: add Microagents & Skills page with CRUD, import, and content editor

Wire 11 backend endpoints (microagents: list, get, create, update,
delete; skills: list, get, create, update, delete, import) into a
new /microagents page with two tabs. Microagent tab has trigger pattern
and prompt editor. Skills tab has content editor and import-from-URL
with safety rejection display. Includes API resource, types, i18n
(en+de), route registration, and sidebar nav link.
```
