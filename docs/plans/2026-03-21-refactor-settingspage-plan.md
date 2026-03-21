# SettingsPage Decomposition — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Decompose `SettingsPage.tsx` (1056 lines, 24 signals) into 9 focused section components, each owning its own state and API calls.

**Architecture:** SettingsPage becomes a thin layout with sidebar navigation + section routing. Each section (General, Shortcuts, VCS, Providers, LLM Proxy, Subscriptions, API Keys, Users, Dev Tools) becomes its own component file. Shared section types go in `settingsTypes.ts`. All existing tests and E2E specs must continue to pass.

**Tech Stack:** SolidJS, TypeScript, Vitest. Run tests: `cd frontend && npm test`

---

## File Structure

| File | Responsibility | Lines (est.) |
|------|---------------|-------------|
| `SettingsPage.tsx` | Layout shell: sidebar nav, section routing, IntersectionObserver | ~80 |
| `settingsTypes.ts` (NEW) | Shared types, section definitions | ~20 |
| `GeneralSection.tsx` (NEW) | Theme, language, autonomy, default model settings | ~120 |
| `VCSSection.tsx` (NEW) | VCS accounts CRUD, device flow, provider selection | ~150 |
| `ProvidersSection.tsx` (NEW) | LLM provider/model management | ~100 |
| `ProxySection.tsx` (NEW) | LiteLLM proxy config + status | ~80 |
| `SubscriptionsSection.tsx` (NEW) | Subscription provider management | ~80 |
| `APIKeysSection.tsx` (NEW) | API key CRUD, copy, revoke | ~100 |
| `UsersSection.tsx` (NEW) | User management, role changes | ~120 |
| `DevToolsSection.tsx` (NEW) | Developer tools, debug endpoints | ~80 |
| `ShortcutsSection.tsx` | Already extracted — no changes needed | existing |

---

## Task 1: Extract settingsTypes.ts + Section Definitions

**Files:**
- Create: `frontend/src/features/settings/settingsTypes.ts`

- [ ] **Step 1: Read SettingsPage.tsx sections array**

Read lines 61-71 to understand section structure.

- [ ] **Step 2: Create settingsTypes.ts**

```typescript
export interface SettingsSection {
  id: string;
  label: string;
}

export const SETTINGS_SECTIONS: SettingsSection[] = [
  { id: "settings-general", label: "General" },
  { id: "settings-shortcuts", label: "Shortcuts" },
  { id: "settings-vcs", label: "VCS" },
  { id: "settings-providers", label: "Providers" },
  { id: "settings-proxy", label: "LLM Proxy" },
  { id: "settings-subscriptions", label: "Subscriptions" },
  { id: "settings-apikeys", label: "API Keys" },
  { id: "settings-users", label: "Users" },
  { id: "settings-devtools", label: "Dev Tools" },
];
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/features/settings/settingsTypes.ts
git commit -m "refactor: extract SettingsPage types and section definitions"
```

---

## Task 2: Extract GeneralSection

**Files:**
- Create: `frontend/src/features/settings/GeneralSection.tsx`
- Modify: `frontend/src/features/settings/SettingsPage.tsx`

- [ ] **Step 1: Read SettingsPage.tsx to identify the General section**

Find the JSX block with `id="settings-general"` and all its associated state/handlers.

- [ ] **Step 2: Create GeneralSection.tsx**

Move the General section JSX + state (theme, language, autonomy, default model) into its own component. It should own its `createResource` and `createSignal` calls.

- [ ] **Step 3: Replace in SettingsPage.tsx**

Replace the General section block with `<GeneralSection />`.

- [ ] **Step 4: Run tests**

```bash
cd frontend && npm test
```

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/settings/GeneralSection.tsx frontend/src/features/settings/SettingsPage.tsx
git commit -m "refactor: extract GeneralSection from SettingsPage"
```

---

## Task 3: Extract VCSSection

**Files:**
- Create: `frontend/src/features/settings/VCSSection.tsx`
- Modify: `frontend/src/features/settings/SettingsPage.tsx`

- [ ] **Step 1: Identify VCS section in SettingsPage.tsx**

Find `id="settings-vcs"` block + all VCS-related state (accounts, deviceFlow, pollTimer).

- [ ] **Step 2: Create VCSSection.tsx**

Move VCS account CRUD, device flow polling, provider selector into own component.

- [ ] **Step 3: Replace in SettingsPage.tsx + run tests**

- [ ] **Step 4: Commit**

```bash
git add frontend/src/features/settings/VCSSection.tsx frontend/src/features/settings/SettingsPage.tsx
git commit -m "refactor: extract VCSSection from SettingsPage"
```

---

## Task 4: Extract Remaining 6 Sections (Batch)

**Files:**
- Create: `frontend/src/features/settings/ProvidersSection.tsx`
- Create: `frontend/src/features/settings/ProxySection.tsx`
- Create: `frontend/src/features/settings/SubscriptionsSection.tsx`
- Create: `frontend/src/features/settings/APIKeysSection.tsx`
- Create: `frontend/src/features/settings/UsersSection.tsx`
- Create: `frontend/src/features/settings/DevToolsSection.tsx`
- Modify: `frontend/src/features/settings/SettingsPage.tsx`

- [ ] **Step 1: Extract each section**

For each remaining section:
1. Find the `id="settings-*"` block in SettingsPage.tsx
2. Identify associated state (signals, resources, handlers)
3. Move into own component file
4. Replace in SettingsPage with `<XSection />`

- [ ] **Step 2: Run tests after each extraction**

```bash
cd frontend && npm test
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/features/settings/*.tsx
git commit -m "refactor: extract Providers, Proxy, Subscriptions, APIKeys, Users, DevTools sections"
```

---

## Task 5: Add Tests for New Section Components

**Files:**
- Modify: `frontend/src/features/settings/settings.test.ts`

- [ ] **Step 1: Extend settings.test.ts**

```typescript
import { describe, it, expect } from "vitest";

describe("Settings Feature", () => {
  it("should export SettingsPage", async () => {
    const mod = await import("./SettingsPage");
    expect(mod.default).toBeDefined();
  });

  it("should export GeneralSection", async () => {
    const mod = await import("./GeneralSection");
    expect(mod.default || mod.GeneralSection).toBeDefined();
  });

  it("should export VCSSection", async () => {
    const mod = await import("./VCSSection");
    expect(mod.default || mod.VCSSection).toBeDefined();
  });

  // ... same for all 6 remaining sections

  it("should export SETTINGS_SECTIONS", async () => {
    const mod = await import("./settingsTypes");
    expect(mod.SETTINGS_SECTIONS).toHaveLength(9);
  });
});
```

- [ ] **Step 2: Run tests**

```bash
cd frontend && npm test
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/features/settings/settings.test.ts
git commit -m "test: unit tests for all extracted SettingsPage sections"
```

---

## Task 6: Verify & Cleanup

- [ ] **Step 1: Verify SettingsPage.tsx is under 100 lines**

```bash
wc -l frontend/src/features/settings/SettingsPage.tsx
```
Expected: ~80 lines (down from 1056).

- [ ] **Step 2: Run full test suite + type check**

```bash
cd frontend && npm test && npx tsc --noEmit
```

- [ ] **Step 3: Final commit**

```bash
git add frontend/src/features/settings/
git commit -m "refactor: SettingsPage decomposition complete — 1056 LOC → 10 focused modules"
```
