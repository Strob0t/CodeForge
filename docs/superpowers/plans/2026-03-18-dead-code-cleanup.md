# Dead Code Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove all verified dead code from the Dead Code Audit v2 — 41 confirmed items across Python, TypeScript, and Go.

**Architecture:** Pure deletion plan. No new features, no refactoring. Each task removes one category of dead code and produces one atomic commit. Tasks are ordered smallest-blast-radius-first.

**Tech Stack:** Python 3.12, TypeScript/SolidJS, Go 1.25

**IMPORTANT — Cross-Language Audit Corrections:**
The cross-language agent reported ~35 findings, but many were FALSE POSITIVES. The following are NOT dead and MUST NOT be removed:
- ALL `runs.*` subjects + payloads (SubjectRunToolCallRequest/Response/Result, SubjectRunComplete, SubjectRunOutput, SubjectRunHeartbeat, SubjectRunCancel) — actively used by `internal/service/runtime.go`
- `SubjectTaskCancel` + `TaskCancelPayload` — used by 4 backend adapters (aider, goose, opencode, plandex)
- `SubjectTaskOutput` — subscribed in `internal/service/agent.go:227`
- `SubjectAgentOutput` — subscribed in `internal/service/agent.go:240`
- `SubjectConversationRunCancel` — published in `internal/service/conversation_agent.go:821`
- `SubjectConversationCompactRequest` — published in `internal/service/conversation.go:331`
- `SubjectSharedUpdated` + `SharedContextUpdatedPayload` — published in `internal/service/shared_context.go:61`
- `SubjectPromptEvolutionPromoted/Reverted` — published in `internal/service/prompt_evolution.go:162,197`
- `RunCompletePayload`, `RunOutputPayload`, `RunHeartbeatPayload`, `ToolCall*Payload` — all used by RuntimeService

---

### Task 1: Remove Python dead config fields

**Files:**
- Modify: `workers/codeforge/config.py:143-152`

- [ ] **Step 1: Remove dead config fields**

Remove these lines from `WorkerSettings.__init__()`:
```python
# Lines 143-152 — remove entirely:
self.core_url = _resolve_str("CODEFORGE_CORE_URL", None, "http://localhost:8080")
self.internal_key = _resolve_str("CODEFORGE_INTERNAL_KEY", None, "")
self.aider_path = os.environ.get("CODEFORGE_AIDER_PATH", "aider")
self.goose_path = os.environ.get("CODEFORGE_GOOSE_PATH", "goose")
self.opencode_path = os.environ.get("CODEFORGE_OPENCODE_PATH", "opencode")
self.plandex_path = os.environ.get("CODEFORGE_PLANDEX_PATH", "plandex")
self.openhands_url = os.environ.get("CODEFORGE_OPENHANDS_URL", "http://localhost:3000")
```

Also remove the comment on line 143 (`# Go Core connection...`) and the comment on line 147 (`# Agent backend CLI paths`).

- [ ] **Step 2: Verify no references**

Run: `cd /workspaces/CodeForge && grep -r "\.core_url\|\.internal_key\|\.aider_path\|\.goose_path\|\.opencode_path\|\.plandex_path\|\.openhands_url" workers/ --include="*.py" | grep -v config.py | grep -v __pycache__`
Expected: No output (zero references outside config.py)

- [ ] **Step 3: Run Python tests**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/ -x -q --no-header 2>&1 | tail -5`
Expected: All tests pass

- [ ] **Step 4: Commit**

```bash
git add workers/codeforge/config.py
git commit -m "chore: remove 7 dead config fields from WorkerSettings"
```

---

### Task 2: Remove Python dead functions and classes

**Files:**
- Modify: `workers/codeforge/evaluation/prompt_optimizer.py` (remove `PromptPatch` class lines 29-40, `_ANALYSIS_PROMPT` lines 59-82, `analyze_failures` lines 85-126, `analyze_failures_async` lines 129-183)
- Delete: `workers/codeforge/nats_utils.py`
- Modify: `workers/codeforge/tracing/propagation.py` (remove `start_consumer_span()`, lines 36-47)
- Modify: `workers/codeforge/health.py` (remove `serve()`, lines 26-29)
- Modify: `workers/codeforge/routing/models.py` (remove `CascadeConfig`, lines 92-97)
- Modify: `workers/codeforge/retrieval.py` (remove `_RetrieverConfig`, lines 359-365)

- [ ] **Step 1: Remove dead code from prompt_optimizer.py**

Remove ONLY these specific ranges:
- `PromptPatch` dataclass (lines 29-40)
- `_ANALYSIS_PROMPT` template string (lines 59-82)
- `analyze_failures()` function (lines 85-126)
- `analyze_failures_async()` function (lines 129-183)

**CRITICAL: Keep everything from line 186 onward!** The following are actively used:
- `_is_failed()` (line 186) — called by `reflect_on_failures_sync()` and `reflect_on_failures()`
- `_classify_failure_pattern()` (line 223) — called by `reflect_on_failures_sync()` and `_build_clusters_text()`
- `reflect_on_failures_sync()` (line 309) — fallback for `reflect_on_failures()`
- `reflect_on_failures()` (line 398) — imported by `workers/codeforge/consumer/_prompt_evolution.py`
- `handle_reflect_request()` (line 483) — tested in `workers/tests/evaluation/test_prompt_optimizer.py`

- [ ] **Step 2: Delete nats_utils.py**

Delete the entire file `workers/codeforge/nats_utils.py`. `nats_handler()` is its only export and is never imported.

- [ ] **Step 3: Remove `start_consumer_span()` from propagation.py**

Remove lines 36-47. Keep `extract_trace_context` and `inject_trace_context` which are used.

- [ ] **Step 4: Remove `serve()` from health.py**

Remove the `serve` function (lines 26-29). Keep `HealthHandler` class which is used by tests.

- [ ] **Step 5: Remove `CascadeConfig` from routing/models.py**

Remove lines 92-97. Keep `CascadeStep` and `CascadePlan` which are used.

- [ ] **Step 6: Remove `_RetrieverConfig` from retrieval.py**

Remove lines 359-365. The `HybridRetriever.__init__` constructs its own config inline.

- [ ] **Step 7: Verify no import breakage**

Run: `cd /workspaces/CodeForge && .venv/bin/python -c "from codeforge.consumer import TaskConsumer; print('OK')" 2>&1`
Expected: OK

- [ ] **Step 8: Run Python tests**

Run: `cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/ -x -q --no-header 2>&1 | tail -5`
Expected: All tests pass

- [ ] **Step 9: Commit**

```bash
git add workers/codeforge/evaluation/prompt_optimizer.py workers/codeforge/nats_utils.py workers/codeforge/tracing/propagation.py workers/codeforge/health.py workers/codeforge/routing/models.py workers/codeforge/retrieval.py
git commit -m "chore: remove 8 dead Python functions/classes (PromptPatch, analyze_failures, analyze_failures_async, nats_handler, start_consumer_span, serve, CascadeConfig, _RetrieverConfig)"
```

---

### Task 3: Remove TypeScript unused dependencies

**Files:**
- Modify: `frontend/package.json`

- [ ] **Step 1: Uninstall unused dependencies**

Run: `cd /workspaces/CodeForge/frontend && npm uninstall @monaco-editor/loader @solid-primitives/websocket`

Note: `@monaco-editor/loader` may remain as a transitive dep of `solid-monaco`. That's fine — we're removing the direct dependency declaration.

- [ ] **Step 2: Verify build**

Run: `cd /workspaces/CodeForge/frontend && npm run build 2>&1 | tail -5`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add frontend/package.json frontend/package-lock.json
git commit -m "chore: remove 2 unused npm deps (@monaco-editor/loader, @solid-primitives/websocket)"
```

---

### Task 4: Remove TypeScript dead exports (statusVariants)

**Files:**
- Modify: `frontend/src/config/statusVariants.ts:68-74,132-144`

- [ ] **Step 1: Remove 3 unused variant maps**

Remove `teamRoleVariant` (lines 68-74), `userRoleVariant` (lines 132-135), and `vcsProviderVariant` (lines 138-144).

- [ ] **Step 2: Verify no references**

Run: `cd /workspaces/CodeForge/frontend && grep -r "teamRoleVariant\|userRoleVariant\|vcsProviderVariant" src/ --include="*.ts" --include="*.tsx" | grep -v statusVariants.ts`
Expected: No output

- [ ] **Step 3: Verify build**

Run: `cd /workspaces/CodeForge/frontend && npm run build 2>&1 | tail -5`
Expected: Build succeeds

- [ ] **Step 4: Commit**

```bash
git add frontend/src/config/statusVariants.ts
git commit -m "chore: remove 3 unused badge variant maps (teamRole, userRole, vcsProvider)"
```

---

### Task 5: Remove TypeScript dead components and utilities

**Files:**
- Delete: `frontend/src/ui/composites/ResourceGuard.tsx`
- Delete: `frontend/src/ui/composites/SkeletonCard.tsx`
- Delete: `frontend/src/ui/composites/SkeletonChat.tsx`
- Delete: `frontend/src/ui/composites/SkeletonTable.tsx`
- Delete: `frontend/src/ui/composites/SkeletonText.tsx`
- Delete: `frontend/src/ui/primitives/PacmanSpinner.tsx`
- Delete: `frontend/src/ui/primitives/ProgressBar.tsx`
- Modify: `frontend/src/ui/composites/index.ts` (remove re-exports for deleted files)
- Modify: `frontend/src/ui/primitives/index.ts` (remove re-exports for deleted files)
- Modify: `frontend/src/ui/index.ts` (remove re-exports for deleted items)
- Modify: `frontend/src/index.css` (remove 3 unused CSS animations)
- Modify: `frontend/src/api/factory.ts` (remove `createCRUDClient`, `createNestedCRUDClient`, and their helper types)
- Modify: `frontend/src/utils/tabBadge.ts` (remove `getTabBadgeCount`)

- [ ] **Step 1: Delete 5 unused composite component files**

```bash
cd /workspaces/CodeForge/frontend
rm src/ui/composites/ResourceGuard.tsx
rm src/ui/composites/SkeletonCard.tsx
rm src/ui/composites/SkeletonChat.tsx
rm src/ui/composites/SkeletonTable.tsx
rm src/ui/composites/SkeletonText.tsx
```

- [ ] **Step 2: Delete 2 unused primitive component files**

```bash
rm src/ui/primitives/PacmanSpinner.tsx
rm src/ui/primitives/ProgressBar.tsx
```

- [ ] **Step 3: Update barrel exports**

In `frontend/src/ui/composites/index.ts`: Remove the 5 export lines for deleted files (ResourceGuard, SkeletonCard, SkeletonChat, SkeletonTable, SkeletonText).

In `frontend/src/ui/primitives/index.ts`: Remove the 2 export lines for PacmanSpinner and ProgressBar.

In `frontend/src/ui/index.ts`: Remove the re-exports for ResourceGuard, SkeletonCard, SkeletonChat, SkeletonTable, SkeletonText, PacmanSpinner, ProgressBar.

- [ ] **Step 4: Remove 3 unused CSS animations from index.css**

Remove these animation blocks from `frontend/src/index.css`:
- `cf-progress-slide` (lines 376-387, comment + keyframes)
- `cf-pacman-chomp` (lines 389-401, comment + keyframes)
- `cf-dot-orbit` (lines 403-410, keyframes)

Keep the `cf-fade-in` animation (line 416+) which is used for page transitions.

- [ ] **Step 5: Remove dead factory functions from factory.ts**

Remove `createCRUDClient` (lines 60-80) and `createNestedCRUDClient` (lines 94-115) and their associated interfaces (`CRUDConfig`, `NestedCRUDConfig`, `CRUDClient<T,TCreate>`, `NestedCRUDClient<T,TCreate>`, `RequestFn`). Keep the `url` template literal function which IS used.

- [ ] **Step 6: Remove getTabBadgeCount from tabBadge.ts**

Remove the `getTabBadgeCount()` function (lines 17-19). Keep `updateTabBadge` and `resetTabBadge` which are used.

- [ ] **Step 7: Verify build**

Run: `cd /workspaces/CodeForge/frontend && npm run build 2>&1 | tail -5`
Expected: Build succeeds

- [ ] **Step 8: Verify lint**

Run: `cd /workspaces/CodeForge/frontend && npm run lint 2>&1 | tail -10`
Expected: No new errors

- [ ] **Step 9: Commit**

```bash
git add -A frontend/src/
git commit -m "chore: remove 12 unused TS exports (5 composites, 2 primitives, 2 factory fns, 1 util, 3 CSS animations)"
```

---

### Task 6: Remove Go dead NATS subjects and schemas

**Files:**
- Modify: `internal/port/messagequeue/queue.go` (remove 4 dead subject constants)
- Modify: `internal/port/messagequeue/schemas.go` (remove 7 dead payload structs)
- Modify: `internal/port/messagequeue/validator.go` (remove references to deleted subjects/structs)
- Modify: `internal/port/messagequeue/validator_test.go` (remove tests for deleted subjects)
- Modify: `internal/adapter/nats/nats_test.go` (update `TestQueue_DLQ` to use a non-deleted subject)

**Verified dead subjects (ONLY these):**
- `SubjectTaskCreated` — never published
- `SubjectAgentStatus` — never published or subscribed
- `SubjectContextPacked` — never published or subscribed
- `SubjectAgentMessage` — never published or subscribed

**Verified dead payload structs (ONLY these 7):**
- `TaskCreatedPayload` — only in validator
- `TaskOutputPayload` — only in validator (agent.go uses `ws.TaskOutputEvent`, not this struct)
- `AgentStatusPayload` — only in validator
- `ContextPackedPayload` — never used
- `MCPServerStatusPayload` — never unmarshalled in Go
- `MCPToolDiscoveryPayload` — never unmarshalled in Go
- `MCPToolPayload` — only used by MCPToolDiscoveryPayload

**Trade-off note:** Removing the validator case for `SubjectTaskOutput` means messages on that subject will no longer be schema-validated by the Go validator. This is acceptable because `agent.go` unmarshals into `ws.TaskOutputEvent` (not `TaskOutputPayload`), so the validator struct was already not matching the actual deserialization type.

**DO NOT REMOVE (actively used):**
- ALL `runs.*` subjects and payloads — RuntimeService
- `SubjectTaskCancel` + `TaskCancelPayload` — backend adapters
- `SubjectTaskOutput` (subject constant) — agent.go subscriber (KEEP the constant, remove only the payload struct)
- `SubjectAgentOutput` — agent.go subscriber
- ALL MCP subject CONSTANTS (keep `SubjectMCPServerStatus`, `SubjectMCPToolDiscovery` for protocol docs)
- ALL `conversation.*` subjects — conversation_agent.go / conversation.go
- `SharedContextUpdatedPayload` — shared_context.go
- `PromptEvolution*` subjects — prompt_evolution.go
- `MCPServerDefPayload` — used in `ConversationRunStartPayload`

- [ ] **Step 1: Remove 4 dead subject constants from queue.go**

In `internal/port/messagequeue/queue.go`, remove:
- `SubjectTaskCreated = "tasks.created"` (line 37)
- `SubjectAgentStatus = "agents.status"` (line 42)
- `SubjectContextPacked = "context.packed"` (line 62)
- `SubjectAgentMessage = "agents.message"` (line 104)

Keep all other subject constants including `SubjectTaskOutput`, `SubjectMCPServerStatus`, `SubjectMCPToolDiscovery`.

- [ ] **Step 2: Remove 7 dead payload structs from schemas.go**

In `internal/port/messagequeue/schemas.go`, remove:
- `TaskCreatedPayload` struct (lines 9-15) and its comment
- `TaskOutputPayload` struct (lines 30-36) and its comment
- `AgentStatusPayload` struct (lines 44-49) and its comment
- `ContextPackedPayload` struct (lines 193-199) and its comment
- `MCPServerStatusPayload` struct (lines 415-421) and its comment
- `MCPToolPayload` struct (lines 423-429) and its comment
- `MCPToolDiscoveryPayload` struct (lines 431-436) and its comment

Keep `MCPServerDefPayload` (used in `ConversationRunStartPayload`).

- [ ] **Step 3: Update validator.go**

In `internal/port/messagequeue/validator.go`, remove the switch cases that reference deleted subjects/structs:
- `case subject == SubjectTaskCreated:` + `target = &TaskCreatedPayload{}`
- `case subject == SubjectTaskOutput:` + `target = &TaskOutputPayload{}`
- `case subject == SubjectAgentStatus:` + `target = &AgentStatusPayload{}`

- [ ] **Step 4: Update validator_test.go**

Remove test cases that reference deleted subjects/structs:
- `TestValidate` cases for `SubjectTaskCreated`
- `TestValidate` cases for `SubjectTaskOutput`
- `TestValidate` cases for `SubjectAgentStatus`
- Test `TestValidateFieldConstraints` and `TestValidatePartialPayload` that use `SubjectTaskCreated`

Keep `TestValidate` case for `SubjectTaskCancel` (subject still exists).

- [ ] **Step 5: Update nats_test.go**

In `internal/adapter/nats/nats_test.go`, the `TestQueue_DLQ` test (line 157) uses `SubjectTaskCreated`. Change it to use `SubjectTaskCancel` (or any other subject that still has validator coverage). Update the test's comment and the published payload to match `TaskCancelPayload` structure.

- [ ] **Step 6: Verify Go build**

Run: `cd /workspaces/CodeForge && go build ./...`
Expected: Build succeeds

- [ ] **Step 7: Run Go tests for messagequeue package**

Run: `cd /workspaces/CodeForge && go test ./internal/port/messagequeue/ -v -count=1 2>&1 | tail -20`
Expected: All remaining tests pass

- [ ] **Step 8: Run Go tests for nats adapter**

Run: `cd /workspaces/CodeForge && go test ./internal/adapter/nats/ -v -count=1 -run TestQueue_DLQ 2>&1 | tail -10`
Expected: Test passes

- [ ] **Step 9: Run full Go test suite**

Run: `cd /workspaces/CodeForge && go test ./... -count=1 2>&1 | tail -30`
Expected: All tests pass

- [ ] **Step 10: Commit**

```bash
git add internal/port/messagequeue/ internal/adapter/nats/nats_test.go
git commit -m "chore: remove 4 dead NATS subjects + 7 dead payload structs + update validator"
```

---

## Summary

| Task | Items | Blast Radius | Commit |
|------|-------|-------------|--------|
| 1. Python dead config | 7 fields | Zero | 1 |
| 2. Python dead functions | 8 functions/classes | Zero | 1 |
| 3. TS unused deps | 2 packages | Zero | 1 |
| 4. TS dead exports | 3 variant maps | Zero | 1 |
| 5. TS dead components + utils | 12 exports + 7 files + 3 CSS | Low | 1 |
| 6. Go dead NATS | 4 subjects + 7 structs | Medium | 1 |
| **Total** | **41 verified items** | | **6 commits** |

**Note:** The original audit reported 66 items, but 25 cross-language items were false positives (the "old run protocol" is actively used by Go's RuntimeService). This plan covers only the 41 verified-dead items.
