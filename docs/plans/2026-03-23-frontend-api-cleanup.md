# Frontend API Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove frontend API methods that are defined but never called, after accounting for the newly-wired components from plans #4 and #5.

**Architecture:** After wiring 16 orphaned components, re-verify which API methods are still unused, then remove them from the API resource files.

**Tech Stack:** TypeScript, SolidJS

**Depends on:** feat/frontend-project-panels (#4) and feat/frontend-chat-panels (#5)

---

### Task 1: Re-verify which API methods are still unused

**CRITICAL:** Plans #4 and #5 wired 16 orphaned components. Some previously-unused API methods are now reachable:
- `lsp.*` (7 methods) — now used by LSPPanel.tsx (wired in #4)
- `policies.createPolicy/deletePolicy/updatePolicy` (3 methods) — now used by PolicyPanel.tsx (wired in #4)
- `files.graphSearch` (1 method) — now used by ArchitectureGraph.tsx (wired in #4)

**Files to check:**
- `frontend/src/api/resources/` — all API resource files

- [ ] **Step 1: Grep each API method across frontend/src/ (excluding api/ definitions)**

For each method listed below, grep to confirm it's STILL unused after #4 and #5:

**Tasks (5):** `tasks.get`, `tasks.events`, `tasks.context`, `tasks.buildContext`, `tasks.claim`
**Files (9):** `files.test`, `files.index`, `files.symbols`, `files.references`, `files.hover`, `files.diagnostics`, `files.definition`, `files.detectStack`, `files.detectStackByPath`
**MCP (4):** `mcp.getServer`, `mcp.listProjectServers`, `mcp.assignToProject`, `mcp.unassignFromProject`

Expected: ~18 methods still truly unused.

- [ ] **Step 2: Document which methods are now used (do NOT remove these)**

Methods now used after wiring: `lsp.*` (7), `policies.create/delete/update` (3), `files.graphSearch` (1).

---

### Task 2: Remove unused tasks API methods

**Files:**
- Modify: `frontend/src/api/resources/tasks.ts`

- [ ] **Step 1: Remove methods: get, events, context, buildContext, claim**

Keep `list`, `create`, and any other methods that ARE used.

- [ ] **Step 2: Type check**

```bash
cd frontend && npx tsc --noEmit 2>&1 | grep -i error | head -20
```

---

### Task 3: Remove unused files API methods

**Files:**
- Modify: `frontend/src/api/resources/files.ts`

- [ ] **Step 1: Remove methods: test, index, symbols, references, hover, diagnostics, definition, detectStack, detectStackByPath**

Keep `graphSearch`, `repoMap`, `readFile`, `writeFile`, `deleteFile`, `renameFile`, `listTree`, and any other used methods.

- [ ] **Step 2: Type check**

```bash
cd frontend && npx tsc --noEmit 2>&1 | grep -i error | head -20
```

---

### Task 4: Remove unused mcp API methods

**Files:**
- Modify: `frontend/src/api/resources/mcp.ts`

- [ ] **Step 1: Remove methods: getServer, listProjectServers, assignToProject, unassignFromProject**

Keep `listServers`, `createServer`, `testServer`, `updateServer`, `deleteServer`, `listTools`, `attachToScope`, `detachFromScope`, `listByScope`, and any other used methods.

- [ ] **Step 2: Type check**

```bash
cd frontend && npx tsc --noEmit 2>&1 | grep -i error | head -20
```

---

### Task 5: Full verification and commit

- [ ] **Step 1: Build check**

```bash
cd frontend && npm run build 2>&1 | tail -20
```

- [ ] **Step 2: Type check**

```bash
cd frontend && npx tsc --noEmit 2>&1 | tail -20
```

- [ ] **Step 3: Grep to confirm no dangling references**

```bash
grep -r "tasks\.get\b\|tasks\.events\|tasks\.context\|tasks\.buildContext\|tasks\.claim" frontend/src/ --include="*.ts" --include="*.tsx" | grep -v "api/resources"
grep -r "files\.test\b\|files\.index\b\|files\.symbols\b\|files\.references\b\|files\.hover\b\|files\.diagnostics\b\|files\.definition\b\|files\.detectStack" frontend/src/ --include="*.ts" --include="*.tsx" | grep -v "api/resources"
grep -r "mcp\.getServer\b\|mcp\.listProjectServers\|mcp\.assignToProject\|mcp\.unassignFromProject" frontend/src/ --include="*.ts" --include="*.tsx" | grep -v "api/resources"
```

All should return empty.

- [ ] **Step 4: Commit**

```
refactor: remove 18 unused frontend API methods

Remove dead API methods that have no callers in the frontend:
- tasks: get, events, context, buildContext, claim
- files: test, index, symbols, references, hover, diagnostics,
  definition, detectStack, detectStackByPath
- mcp: getServer, listProjectServers, assignToProject,
  unassignFromProject

Note: lsp.*, policies.create/delete/update, and files.graphSearch
are now used by newly-wired components (LSPPanel, PolicyPanel,
ArchitectureGraph) and are kept.
```
