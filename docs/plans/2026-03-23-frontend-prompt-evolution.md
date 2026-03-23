# Frontend Prompt Evolution Tab — Implementation Plan

**Date:** 2026-03-23
**Goal:** Add an "Evolution" tab to the existing PromptEditorPage that exposes the 4 prompt-evolution endpoints.
**Depends on:** `docs/plans/2026-03-23-prompt-evolution-loop.md` (prompt_scores store layer + score wiring)

---

## Context

The prompt evolution backend is fully implemented:
- `internal/service/prompt_evolution.go` — service with TriggerReflection, PromoteVariant, RevertMode, GetStatus
- `internal/adapter/http/handlers_prompt_evolution.go` — 4 HTTP handlers
- `internal/adapter/http/routes.go` lines 310-316 — routes registered

Backend endpoints available:
| Method | Path | Handler | Purpose |
|--------|------|---------|---------|
| GET | `/prompt-evolution/status` | `GetPromptEvolutionStatus` | Config status + per-mode stats |
| POST | `/prompt-evolution/reflect` | `TriggerPromptEvolutionReflect` | Trigger reflection loop |
| POST | `/prompt-evolution/promote/{variantId}` | `PromotePromptEvolutionVariant` | Promote a candidate variant |
| POST | `/prompt-evolution/revert/{modeId}` | `RevertPromptEvolutionMode` | Revert mode to base prompts |

**Missing backend:** There is no `GET /prompt-evolution/variants` endpoint. The `GetStatus` endpoint returns `EvolutionStatus` (enabled, trigger, strategy, mode_status map) but not actual variant rows. A variants list endpoint must be added in Go first (Task 1).

The existing `PromptEditorPage.tsx` (`frontend/src/features/prompts/PromptEditorPage.tsx`) is a flat page with no tabs. It uses `PageLayout`, `useCRUDForm`, `useAsyncAction`, `useI18n`, `useToast`.

---

## Task 1: Add GET /prompt-evolution/variants backend endpoint

**Files:**
- Modify: `internal/adapter/http/handlers_prompt_evolution.go`
- Modify: `internal/adapter/http/routes.go`
- Modify: `internal/service/prompt_evolution.go`
- Modify: `internal/port/database/store.go` (if `GetVariantsByModeAndModel` is not already broad enough)

- [ ] **Step 1: Add `ListVariants` method to `PromptEvolutionService`**

Query all variants for the tenant, optionally filtered by `mode_id` and `status` query params. Use the existing `PromptEvolutionStore` interface. May need a new store method `ListAllVariants(ctx, tenantID)` if `GetVariantsByModeAndModel` requires both params.

- [ ] **Step 2: Add `ListPromptEvolutionVariants` handler**

```go
// GET /api/v1/prompt-evolution/variants?mode_id=&status=
func (h *Handlers) ListPromptEvolutionVariants(w http.ResponseWriter, r *http.Request) {
    // Parse query params: mode_id, status (optional filters)
    // Call evoSvc.ListVariants(ctx, tenantID, modeID, status)
    // Return JSON array of prompt.PromptVariant
}
```

- [ ] **Step 3: Register route**

In `routes.go`, add inside the prompt-evolution group:
```go
r.Get("/prompt-evolution/variants", h.ListPromptEvolutionVariants)
```

- [ ] **Step 4: Write handler tests**

- [ ] **Step 5: Verify**

```bash
go test ./internal/adapter/http/... -run TestListPromptEvolutionVariants -v
```

---

## Task 2: Create API resource file for prompt evolution

**Files:**
- Create: `frontend/src/api/resources/promptEvolution.ts`
- Modify: `frontend/src/api/resources/index.ts` (add export)
- Modify: `frontend/src/api/client.ts` (register on `api` object)
- Modify: `frontend/src/api/types.ts` (add response types)

- [ ] **Step 1: Add TypeScript types**

In `frontend/src/api/types.ts`:

```typescript
export interface PromptEvolutionStatus {
  enabled: boolean;
  trigger: string;
  strategy: string;
  mode_status?: Record<string, {
    mode_id: string;
    active_variant: string;
    candidate_count: number;
    total_trials: number;
    avg_score: number;
    strategy: string;
  }>;
}

export interface PromptVariant {
  id: string;
  tenant_id: string;
  mode_id: string;
  model_family: string;
  content: string;
  version: number;
  parent_id: string;
  mutation_source: string;
  promotion_status: "candidate" | "promoted" | "retired";
  trial_count: number;
  avg_score: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}
```

- [ ] **Step 2: Create resource file**

```typescript
// frontend/src/api/resources/promptEvolution.ts
import type { CoreClient } from "../core";
import { url } from "../factory";
import type { PromptEvolutionStatus, PromptVariant } from "../types";

export function createPromptEvolutionResource(c: CoreClient) {
  return {
    status: () => c.get<PromptEvolutionStatus>("/prompt-evolution/status"),

    variants: (modeId?: string, status?: string) => {
      const params = new URLSearchParams();
      if (modeId) params.set("mode_id", modeId);
      if (status) params.set("status", status);
      const qs = params.toString();
      return c.get<PromptVariant[]>(`/prompt-evolution/variants${qs ? `?${qs}` : ""}`);
    },

    promote: (variantId: string) =>
      c.post<{ status: string; variant_id: string }>(
        url`/prompt-evolution/promote/${variantId}`,
      ),

    revert: (modeId: string) =>
      c.post<{ status: string; mode_id: string }>(
        url`/prompt-evolution/revert/${modeId}`,
      ),

    reflect: (data: { mode_id: string; model_family: string; current_prompt: string; failures?: object[] }) =>
      c.post<{ status: string }>("/prompt-evolution/reflect", data),
  };
}
```

- [ ] **Step 3: Register in client.ts**

Add to `api` object:
```typescript
promptEvolution: createPromptEvolutionResource(core),
```

- [ ] **Step 4: Export from index.ts**

```typescript
export { createPromptEvolutionResource } from "./promptEvolution";
```

---

## Task 3: Convert PromptEditorPage to tabbed layout

**Files:**
- Modify: `frontend/src/features/prompts/PromptEditorPage.tsx`

The existing page has no tab system. Introduce a simple signal-based tab switcher (no external lib needed).

- [ ] **Step 1: Add tab state**

```typescript
const [activeTab, setActiveTab] = createSignal<"sections" | "evolution">("sections");
```

- [ ] **Step 2: Wrap existing content in a `SectionsTab` component**

Extract the current page body (scope selector, preview panel, form, section list) into a local `SectionsTab` component function. This keeps the diff minimal.

- [ ] **Step 3: Add tab bar**

Below the `PageLayout` title, add tab buttons:
```tsx
<div class="mb-4 flex border-b border-cf-border">
  <button
    class={`px-4 py-2 text-sm font-medium ${activeTab() === "sections" ? "border-b-2 border-cf-accent text-cf-accent" : "text-cf-text-muted"}`}
    onClick={() => setActiveTab("sections")}
  >
    {t("prompts.tabs.sections")}
  </button>
  <button
    class={`px-4 py-2 text-sm font-medium ${activeTab() === "evolution" ? "border-b-2 border-cf-accent text-cf-accent" : "text-cf-text-muted"}`}
    onClick={() => setActiveTab("evolution")}
  >
    {t("prompts.tabs.evolution")}
  </button>
</div>
```

- [ ] **Step 4: Conditionally render tabs**

```tsx
<Show when={activeTab() === "sections"}>
  <SectionsTab />
</Show>
<Show when={activeTab() === "evolution"}>
  <EvolutionTab />
</Show>
```

---

## Task 4: Build EvolutionTab component

**Files:**
- Create: `frontend/src/features/prompts/EvolutionTab.tsx`
- Modify: `frontend/src/features/prompts/PromptEditorPage.tsx` (import EvolutionTab)

The Evolution tab has three sections:
1. **Status section** — evolution config status from `GET /prompt-evolution/status`
2. **Variants table** — list from `GET /prompt-evolution/variants` with promote/revert actions
3. **Trigger Reflection form** — calls `POST /prompt-evolution/reflect`

- [ ] **Step 1: Status section**

```tsx
const [status] = createResource(() => api.promptEvolution.status());
```

Display: enabled badge, trigger type, strategy. If `mode_status` map is present, show per-mode stats (active variant, candidate count, avg score).

- [ ] **Step 2: Variants table**

```tsx
const [variants, { refetch }] = createResource(() => api.promptEvolution.variants());
```

Table columns:
| Mode | Model Family | Status | Version | Avg Score | Trials | Content (truncated) | Actions |

Actions column:
- **Promote** button (only for `candidate` status) — calls `api.promptEvolution.promote(id)`, then `refetch()`
- **Revert Mode** button (only for `promoted` status) — calls `api.promptEvolution.revert(modeId)`, then `refetch()`

Use `Badge` component for status: candidate=neutral, promoted=success, retired=warning.

- [ ] **Step 3: Trigger Reflection form**

Form fields:
- `mode_id` (required, text input or dropdown from modes list via `api.modes.list()`)
- `model_family` (required, select: openai/anthropic/google/meta/local)
- `current_prompt` (required, textarea)
- Submit button "Trigger Reflection"

On submit: call `api.promptEvolution.reflect(data)`, toast success (202 Accepted).

- [ ] **Step 4: Wire confirm dialog for promote/revert**

Both promote and revert should use `useConfirm()` before executing, since they change active prompts.

---

## Task 5: Add i18n keys

**Files:**
- Modify: `frontend/src/i18n/en.ts`
- Modify: `frontend/src/i18n/locales/de.ts`

- [ ] **Step 1: Add English keys**

```typescript
"prompts.tabs.sections": "Sections",
"prompts.tabs.evolution": "Evolution",
"prompts.evolution.title": "Prompt Evolution",
"prompts.evolution.status": "Evolution Status",
"prompts.evolution.enabled": "Enabled",
"prompts.evolution.trigger": "Trigger",
"prompts.evolution.strategy": "Strategy",
"prompts.evolution.variants": "Variants",
"prompts.evolution.empty": "No variants yet",
"prompts.evolution.emptyDescription": "Trigger a reflection to generate prompt variants.",
"prompts.evolution.promote": "Promote",
"prompts.evolution.revert": "Revert Mode",
"prompts.evolution.triggerReflection": "Trigger Reflection",
"prompts.evolution.field.modeId": "Mode ID",
"prompts.evolution.field.modelFamily": "Model Family",
"prompts.evolution.field.currentPrompt": "Current Prompt",
"prompts.evolution.confirm.promote": "Promote this variant? It will replace the currently active prompt for this mode.",
"prompts.evolution.confirm.revert": "Revert this mode to base prompts? All promoted variants will be retired.",
"prompts.evolution.promoted": "Variant promoted successfully.",
"prompts.evolution.reverted": "Mode reverted to base prompts.",
"prompts.evolution.reflectionTriggered": "Reflection triggered. Variants will appear after processing.",
"prompts.evolution.error.promoteFailed": "Failed to promote variant.",
"prompts.evolution.error.revertFailed": "Failed to revert mode.",
"prompts.evolution.error.reflectFailed": "Failed to trigger reflection.",
```

- [ ] **Step 2: Add German translations (de.ts)**

---

## Task 6: Tests

**Files:**
- Modify: `frontend/src/features/prompts/prompts.test.ts`

- [ ] **Step 1: Add unit tests for EvolutionTab**

Test: renders status, renders variants table, promote button calls API, revert button calls API, reflection form validates required fields.

- [ ] **Step 2: Verify**

```bash
cd frontend && npm test -- --filter prompts
```

---

## Task 7: Final commit

```
feat: add Evolution tab to PromptEditorPage

- Add GET /prompt-evolution/variants backend endpoint
- Create promptEvolution API resource (status/variants/promote/revert/reflect)
- Convert PromptEditorPage to tabbed layout (Sections + Evolution)
- Build EvolutionTab with variants table, status display, reflection trigger
- Add i18n keys for prompt evolution UI
```

---

## File Reference

| File | Action |
|------|--------|
| `internal/adapter/http/handlers_prompt_evolution.go` | Modify (add ListVariants handler) |
| `internal/adapter/http/routes.go` | Modify (add variants route) |
| `internal/service/prompt_evolution.go` | Modify (add ListVariants method) |
| `frontend/src/api/resources/promptEvolution.ts` | Create |
| `frontend/src/api/resources/index.ts` | Modify |
| `frontend/src/api/client.ts` | Modify |
| `frontend/src/api/types.ts` | Modify |
| `frontend/src/features/prompts/PromptEditorPage.tsx` | Modify (add tabs) |
| `frontend/src/features/prompts/EvolutionTab.tsx` | Create |
| `frontend/src/i18n/en.ts` | Modify |
| `frontend/src/i18n/locales/de.ts` | Modify |
| `frontend/src/features/prompts/prompts.test.ts` | Modify |
