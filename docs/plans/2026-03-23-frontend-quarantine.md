# Frontend Quarantine Admin Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create a new `/quarantine` admin-only page that wires 5 quarantine backend endpoints into a dashboard with stats cards, message table, approve/reject action buttons, and a detail modal.

**Architecture:** New top-level route with stats cards at top (pending/approved/rejected/expired counts), filterable table of quarantined messages, inline approve/reject actions, and a detail modal showing payload and risk factors. New API resource file, types in api/types.ts, i18n keys, nav link, route registration.

**Tech Stack:** SolidJS, TypeScript, Tailwind CSS

---

### Task 1: Add TypeScript types

**Files:**
- Modify: `frontend/src/api/types.ts`

- [ ] **Step 1: Add Quarantine types**

```typescript
/** Quarantine message status (matches Go domain/quarantine.Status) */
export type QuarantineStatus = "pending" | "approved" | "rejected" | "expired";

/** Matches Go domain/quarantine.Message */
export interface QuarantineMessage {
  id: string;
  tenant_id: string;
  project_id: string;
  subject: string;
  payload: string;
  trust_origin: string;
  trust_level: string;
  risk_score: number;
  risk_factors: string[];
  status: QuarantineStatus;
  reviewed_by: string;
  review_note: string;
  created_at: string;
  reviewed_at?: string;
  expires_at: string;
}

/** Matches Go domain/quarantine.Stats */
export interface QuarantineStats {
  pending: number;
  approved: number;
  rejected: number;
  expired: number;
}

/** Request body for approve/reject actions */
export interface QuarantineReviewRequest {
  reviewed_by: string;
  note: string;
}
```

---

### Task 2: Create API resource file

**Files:**
- Create: `frontend/src/api/resources/quarantine.ts`

- [ ] **Step 1: Create createQuarantineResource function**

```typescript
import type { CoreClient } from "../core";
import { url } from "../factory";
import type {
  QuarantineMessage,
  QuarantineReviewRequest,
  QuarantineStats,
  QuarantineStatus,
} from "../types";

export function createQuarantineResource(c: CoreClient) {
  return {
    list: (projectId: string, status?: QuarantineStatus, limit?: number, offset?: number) => {
      const params = new URLSearchParams({ project_id: projectId });
      if (status) params.set("status", status);
      if (limit !== undefined) params.set("limit", String(limit));
      if (offset !== undefined) params.set("offset", String(offset));
      return c.get<QuarantineMessage[]>(`/quarantine?${params.toString()}`);
    },

    get: (id: string) => c.get<QuarantineMessage>(url`/quarantine/${id}`),

    approve: (id: string, data: QuarantineReviewRequest) =>
      c.post<{ status: string }>(url`/quarantine/${id}/approve`, data),

    reject: (id: string, data: QuarantineReviewRequest) =>
      c.post<{ status: string }>(url`/quarantine/${id}/reject`, data),

    stats: (projectId: string) =>
      c.get<QuarantineStats>(`/quarantine/stats?project_id=${encodeURIComponent(projectId)}`),
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
export { createQuarantineResource } from "./quarantine";
```

- [ ] **Step 2: Register in client.ts**

Add import:
```typescript
import { createQuarantineResource } from "./resources/quarantine";
```

Add to `api` object:
```typescript
quarantine: createQuarantineResource(core),
```

---

### Task 4: Create Quarantine Page component

**Files:**
- Create: `frontend/src/features/quarantine/QuarantinePage.tsx`

- [ ] **Step 1: Create page skeleton with project selector and stats**

```typescript
import { createResource, createSignal, For, onMount, Show } from "solid-js";

import { api } from "~/api/client";
import type { Project, QuarantineMessage, QuarantineStats, QuarantineStatus } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useAsyncAction } from "~/hooks";
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
  StatCard,
  Table,
  Textarea,
} from "~/ui";
import type { TableColumn } from "~/ui/composites/Table";
```

- [ ] **Step 2: Implement stats cards row**

Fetch stats via `api.quarantine.stats(projectId)`. Display four StatCard components in a grid:
- Pending (yellow/warning color)
- Approved (green/success color)
- Rejected (red/error color)
- Expired (gray/muted color)

```tsx
<div class="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-6">
  <StatCard label={t("quarantine.stats.pending")} value={stats()?.pending ?? 0} />
  <StatCard label={t("quarantine.stats.approved")} value={stats()?.approved ?? 0} />
  <StatCard label={t("quarantine.stats.rejected")} value={stats()?.rejected ?? 0} />
  <StatCard label={t("quarantine.stats.expired")} value={stats()?.expired ?? 0} />
</div>
```

- [ ] **Step 3: Implement message table with status filter**

Project selector dropdown at top (fetched via `api.projects.list()`).
Status filter: all / pending / approved / rejected / expired.

Table columns: Subject, Trust Origin, Trust Level, Risk Score (color-coded badge), Status (badge), Created At, Actions.

```tsx
const columns: TableColumn<QuarantineMessage>[] = [
  { key: "subject", label: t("quarantine.col.subject") },
  { key: "trust_origin", label: t("quarantine.col.trustOrigin") },
  { key: "risk_score", label: t("quarantine.col.riskScore"),
    render: (row) => (
      <Badge variant={row.risk_score > 0.7 ? "error" : row.risk_score > 0.4 ? "warning" : "success"}>
        {row.risk_score.toFixed(2)}
      </Badge>
    ) },
  { key: "status", label: t("quarantine.col.status"),
    render: (row) => <Badge variant={statusVariant(row.status)}>{row.status}</Badge> },
  { key: "created_at", label: t("quarantine.col.createdAt"),
    render: (row) => new Date(row.created_at).toLocaleString() },
];
```

- [ ] **Step 4: Implement approve/reject action buttons**

For rows with status "pending", show two action buttons:
- Approve (green): opens a small inline form for reviewer name and note, then calls `api.quarantine.approve(id, { reviewed_by, note })`.
- Reject (red): same form, calls `api.quarantine.reject(id, { reviewed_by, note })`.

On success, refetch the messages list and stats.

- [ ] **Step 5: Implement detail modal**

Clicking a row opens a detail modal/panel showing:
- Full payload (formatted JSON in a pre block)
- Risk factors (list of strings with warning badges)
- Trust origin and level
- Review info (reviewed_by, review_note, reviewed_at) if already reviewed
- Approve/Reject buttons if status is "pending"

```tsx
export default function QuarantinePage() {
  onMount(() => { document.title = "Quarantine - CodeForge"; });
  const { t } = useI18n();
  const { show: toast } = useToast();

  const [selectedProjectId, setSelectedProjectId] = createSignal("");
  const [statusFilter, setStatusFilter] = createSignal<QuarantineStatus | "">("");
  const [selectedMessage, setSelectedMessage] = createSignal<QuarantineMessage | null>(null);

  const [projects] = createResource(() => api.projects.list());
  const [stats, { refetch: refetchStats }] = createResource(
    () => selectedProjectId(),
    (pid) => pid ? api.quarantine.stats(pid) : undefined,
  );
  const [messages, { refetch: refetchMessages }] = createResource(
    () => ({ pid: selectedProjectId(), status: statusFilter() }),
    ({ pid, status }) =>
      pid ? api.quarantine.list(pid, status || undefined) : Promise.resolve([]),
  );

  return (
    <PageLayout title={t("quarantine.title")} description={t("quarantine.description")}>
      {/* Project selector + status filter + stats cards + table + detail modal */}
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
// -- Quarantine ---------------------------------------------------------------
"quarantine.title": "Quarantine",
"quarantine.description": "Review and manage quarantined messages flagged by the trust system.",
"quarantine.stats.pending": "Pending",
"quarantine.stats.approved": "Approved",
"quarantine.stats.rejected": "Rejected",
"quarantine.stats.expired": "Expired",
"quarantine.col.subject": "Subject",
"quarantine.col.trustOrigin": "Trust Origin",
"quarantine.col.trustLevel": "Trust Level",
"quarantine.col.riskScore": "Risk Score",
"quarantine.col.status": "Status",
"quarantine.col.createdAt": "Created",
"quarantine.col.expiresAt": "Expires",
"quarantine.selectProject": "Select a project",
"quarantine.filterStatus": "Filter by status",
"quarantine.all": "All statuses",
"quarantine.empty": "No quarantined messages found.",
"quarantine.detail.title": "Message Detail",
"quarantine.detail.payload": "Payload",
"quarantine.detail.riskFactors": "Risk Factors",
"quarantine.detail.reviewedBy": "Reviewed By",
"quarantine.detail.reviewNote": "Review Note",
"quarantine.detail.reviewedAt": "Reviewed At",
"quarantine.action.approve": "Approve",
"quarantine.action.reject": "Reject",
"quarantine.action.reviewerName": "Your name",
"quarantine.action.note": "Review note",
"quarantine.toast.approved": "Message approved and dispatched.",
"quarantine.toast.rejected": "Message rejected.",
"app.nav.quarantine": "Quarantine",
```

- [ ] **Step 2: Add German keys**

```typescript
// -- Quarantine ---------------------------------------------------------------
"quarantine.title": "Quarantaene",
"quarantine.description": "Ueberpruefen und Verwalten von Nachrichten, die vom Vertrauenssystem markiert wurden.",
"quarantine.stats.pending": "Ausstehend",
"quarantine.stats.approved": "Genehmigt",
"quarantine.stats.rejected": "Abgelehnt",
"quarantine.stats.expired": "Abgelaufen",
"quarantine.col.subject": "Betreff",
"quarantine.col.trustOrigin": "Vertrauensherkunft",
"quarantine.col.trustLevel": "Vertrauensstufe",
"quarantine.col.riskScore": "Risikobewertung",
"quarantine.col.status": "Status",
"quarantine.col.createdAt": "Erstellt",
"quarantine.col.expiresAt": "Laeuft ab",
"quarantine.selectProject": "Projekt auswaehlen",
"quarantine.filterStatus": "Nach Status filtern",
"quarantine.all": "Alle Status",
"quarantine.empty": "Keine Nachrichten in Quarantaene gefunden.",
"quarantine.detail.title": "Nachrichtendetail",
"quarantine.detail.payload": "Inhalt",
"quarantine.detail.riskFactors": "Risikofaktoren",
"quarantine.detail.reviewedBy": "Geprueft von",
"quarantine.detail.reviewNote": "Pruefnotiz",
"quarantine.detail.reviewedAt": "Geprueft am",
"quarantine.action.approve": "Genehmigen",
"quarantine.action.reject": "Ablehnen",
"quarantine.action.reviewerName": "Ihr Name",
"quarantine.action.note": "Pruefnotiz",
"quarantine.toast.approved": "Nachricht genehmigt und weitergeleitet.",
"quarantine.toast.rejected": "Nachricht abgelehnt.",
"app.nav.quarantine": "Quarantaene",
```

---

### Task 6: Register route and nav link

**Files:**
- Modify: `frontend/src/index.tsx`
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Add route in index.tsx**

Add import:
```typescript
import QuarantinePage from "./features/quarantine/QuarantinePage.tsx";
```

Add route (after `/settings` route):
```tsx
<Route path="/quarantine" component={QuarantinePage} />
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
  "/prompts",
  "/settings",
  "/benchmarks",
  "/quarantine",  // <-- add
]);
```

- [ ] **Step 3: Add NavLink in App.tsx sidebar**

Add in the "System" NavSection (after Settings NavLink). This is an admin-only page so it should be gated behind admin role check if available, or always shown (backend enforces admin-only via RequireRole middleware).

```tsx
<NavLink href="/quarantine" icon={<QuarantineIcon />} label={t("app.nav.quarantine")}>
  {t("app.nav.quarantine")}
</NavLink>
```

Create a simple quarantine icon (inline SVG with a shield/exclamation motif).

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
feat: add Quarantine admin dashboard with stats, table, and review actions

Wire 5 quarantine backend endpoints (list, get, approve, reject, stats)
into a new /quarantine page. Stats cards show pending/approved/rejected/
expired counts. Table with risk score badges, status filter, project
selector. Approve/reject actions with reviewer name and note. Detail
modal shows payload and risk factors. Includes API resource, types,
i18n (en+de), route registration, and sidebar nav link.
```
