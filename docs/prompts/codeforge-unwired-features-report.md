# CodeForge Unwired Features Report

**Date:** 2026-03-23 | **Branch:** staging | **Version:** 0.8.0 | **Status:** VERIFIED

---

## Executive Summary

Systematic audit across all 5 architecture layers (HTTP, NATS, Frontend, Database, Python Workers), followed by per-claim verification with grep evidence.

**Verified unwired features:**

- **16 orphaned frontend components** (5,093 lines of dead UI code) — ALL VERIFIED
- **22 unused frontend API methods** (defined but never called) — VERIFIED (original claim of 37 was wrong: 15 are actually used)
- **5 truly broken NATS subjects** (Python publishes, Go never subscribes) — VERIFIED (original claim of 12 was wrong: 3 cancel subjects work via runtime listeners, 4 are just undefined)
- **1 orphaned database table** (prompt_scores) — VERIFIED (original claim of 4 was wrong: graph_nodes/edges/metadata are used by Python GraphRAG)
- **2 orphaned Go service methods** (PromptEvolution loop) — VERIFIED
- **2 orphaned Python classes** (MemoryStore, BenchmarkRunner) — VERIFIED (original claim of 5 was wrong: RLVRExporter, TrajectoryExporter, compress_for_context are actively used)
- **2 misplaced test files** (in tools/ instead of tests/) — VERIFIED

---

## 1. Frontend: Orphaned Components — ALL 16 VERIFIED

All exist in `frontend/src/features/` but are never imported by any route or parent component. Verification: grepped entire `frontend/src/` for each component name — 0 imports found for all 16.

| Component | Lines | Purpose | Verified? |
|-----------|-------|---------|-----------|
| PlanPanel.tsx | 794 | Plan decomposition & execution | ORPHANED |
| PolicyPanel.tsx | 866 | Policy profile management | ORPHANED |
| ArchitectureGraph.tsx | 470 | Code architecture visualization | ORPHANED |
| RetrievalPanel.tsx | 408 | BM25S+semantic search simulator | ORPHANED |
| SearchSimulator.tsx | 376 | Retrieval tuning UI | ORPHANED |
| RunPanel.tsx | 325 | Agent run monitoring | ORPHANED |
| AgentNetwork.tsx | 350 | Multi-agent coordination viz | ORPHANED |
| AgentPanel.tsx | 256 | Individual agent state & dispatch | ORPHANED |
| LSPPanel.tsx | 133 | LSP diagnostics display | ORPHANED |
| MultiTerminal.tsx | 172 | Terminal multiplexing | ORPHANED |
| RepoMapPanel.tsx | 154 | Repo structure visualization | ORPHANED |
| LiveOutput.tsx | 97 | Real-time tool output streaming | ORPHANED |
| TaskPanel.tsx | 209 | Task creation UI | ORPHANED |
| CostBreakdown.tsx | 38 | Cost visualization (stub) | ORPHANED |
| DiffSummaryModal.tsx | 271 | Summarize file diff changes | ORPHANED |
| RewindTimeline.tsx | 174 | Rewind conversation UI | ORPHANED |

---

## 2. Frontend: Unused API Methods — 22 VERIFIED (not 37)

### Truly Unused (22 methods)

| Resource | Unused Methods |
|----------|---------------|
| tasks | get, events, context, buildContext, claim |
| files | test, index, symbols, references, hover, diagnostics, definition, detectStack, detectStackByPath, graphSearch |
| mcp | getServer, listProjectServers, assignToProject, unassignFromProject |
| policies | createPolicy, deletePolicy, updatePolicy |
| lsp | trigger, preview, definitions, references, symbols, diagnostics, hover |

### Actually Used (15 methods — originally reported as unused, CORRECTED)

| Method | Where Used | Evidence |
|--------|-----------|---------|
| agents.dispatch | AgentPanel.tsx | `await api.agents.dispatch(agentId, taskId)` |
| agents.stop | AgentPanel.tsx | `await api.agents.stop(agentId, taskId)` |
| agents.active | WarRoom.tsx | `await api.agents.active(id)` |
| benchmarks.cancelRun | BenchmarkPage.tsx | `await api.benchmarks.cancelRun(id)` |
| benchmarks.exportResultsUrl | CostAnalysisView.tsx | `href={api.benchmarks.exportResultsUrl(...)}` |
| benchmarks.exportTrainingUrl | CostAnalysisView.tsx | `href={api.benchmarks.exportTrainingUrl(...)}` |
| benchmarks.listDatasets | — | Type definitions only (borderline) |
| benchmarks.getSuite | — | Type definitions only (borderline) |
| roadmap.deleteFeature | — | i18n strings only (borderline) |
| roadmap.deleteMilestone | — | i18n strings only (borderline) |
| roadmap.importPMItems | RoadmapPanel.tsx | `await api.roadmap.importPMItems(...)` |
| roadmap.syncToFile | RoadmapPanel.tsx | `await api.roadmap.syncToFile(projectId)` |
| mcp.attachToScope | ScopesPage.tsx | `await api.knowledgeBases.attachToScope(...)` |
| mcp.detachFromScope | ScopesPage.tsx | `await api.knowledgeBases.detachFromScope(...)` |
| mcp.listByScope | ScopesPage.tsx | `api.knowledgeBases.listByScope(...)` |

**Note:** agents.dispatch/stop/active are used inside orphaned components (AgentPanel.tsx, WarRoom.tsx). If those components were wired up, these API methods would be active too.

---

## 3. NATS: Broken Subjects — 5 VERIFIED (not 12)

### Truly Broken: Python publishes, Go never subscribes (5 subjects)

| Subject | Python Publisher | Go Subscriber | Impact |
|---------|-----------------|---------------|--------|
| conversation.compact.complete | _compact.py:56 | MISSING | Compaction results lost |
| review.trigger.complete | _review.py:120-124 | MISSING | Review results lost |
| backends.health.result | _backend_health.py:44-46 | MISSING | Health check results lost |
| prompt.evolution.reflect.complete | _prompt_evolution.py:136-140 | MISSING | Reflection results lost |
| prompt.evolution.mutate.complete | _prompt_evolution.py:118-122 | MISSING | Mutation results lost |

### CORRECTED: Actually Working (3 subjects — originally reported as broken)

| Subject | How It Works |
|---------|-------------|
| conversation.run.cancel | Python subscribes via **runtime cancel_listener** (not static subscription) |
| a2a.task.cancel | Python subscribes via **registered handler** in consumer init |
| runs.cancel | Python subscribes via **runtime cancel_listener** (same as conversation) |

### Not Implemented (4 subjects — constants defined, never used anywhere)

| Subject | Status |
|---------|--------|
| review.boundary.analyzed | Constant only, never published or subscribed |
| review.approval.response | Constant only, never published or subscribed |
| mcp.server.status | Constant only, never published or subscribed |
| mcp.tools.discovered | Constant only, never published or subscribed |

---

## 4. Database: Orphaned Tables — 1 VERIFIED (not 4)

### CORRECTED: graph_nodes, graph_edges, graph_metadata are ACTIVE

Original report claimed these 3 tables were orphaned. **Wrong.** They are actively used by the Python GraphRAG module:

| Table | Python Usage | File |
|-------|-------------|------|
| graph_nodes | DELETE, INSERT, SELECT | workers/codeforge/graphrag.py:472-626 |
| graph_edges | DELETE, INSERT, SELECT | workers/codeforge/graphrag.py:472-680 |
| graph_metadata | INSERT with ON CONFLICT UPDATE | workers/codeforge/graphrag.py:516-526 |

These tables have no Go store methods because the GraphService delegates all DB work to Python via NATS. This is **by design** in Approach C architecture (Go owns control plane, Python owns runtime).

### Truly Orphaned: prompt_scores (1 table)

| Aspect | Finding |
|--------|---------|
| Migration | 078_prompt_evolution.sql |
| Go queries | NONE |
| Python queries | NONE |
| Store methods | NONE |
| Schema updates | Migration 086 updates mode_id values (data-only) |
| **Verdict** | Table created but never read or written by application code |

---

## 5. Go Service: Orphaned Methods — 2 VERIFIED

| Method | Service | Evidence |
|--------|---------|---------|
| TriggerReflection | PromptEvolutionService | grep found 0 callers (only definition) |
| HandleMutateComplete | PromptEvolutionService | grep found 0 callers (only definition) |

PromoteVariant and RevertMode ARE wired (called from handlers_prompt_evolution.go). Only the automatic evolution loop is disconnected.

---

## 6. Python Workers: Orphaned Code — 2 VERIFIED (not 5)

### Truly Orphaned (2 classes)

| Class | File | Evidence |
|-------|------|---------|
| MemoryStore | memory/storage.py | Exported in `__init__.py` but **never instantiated**. Tests only inspect source via `inspect.getsource()`. System uses ExperiencePool instead. |
| BenchmarkRunner | evaluation/runner.py | Only used in tests. **Superseded** by BaseBenchmarkRunner + subclasses (Simple/ToolUse/Agent runners) in production. |

### CORRECTED: Actually Used (3 items — originally reported as orphaned)

| Class/Function | Production Usage | Test Usage |
|---------------|-----------------|------------|
| RLVRExporter | Exported in `__init__.py` | 6 instantiations in test_rlvr_exporter.py |
| TrajectoryExporter | Exported in `__init__.py` | 12 instantiations in test_trajectory_exporter.py |
| compress_for_context | **4 production calls** in logprob_verifier.py, trajectory_verifier.py, llm_judge.py | 9 test calls |

### Misplaced Test Files (verified)

| File | Location | Should Be |
|------|----------|-----------|
| test_diff_output.py | workers/codeforge/tools/ | workers/tests/ |
| test_lint.py | workers/codeforge/tools/ | workers/tests/ |

Both are real pytest files (contain `@pytest.mark.asyncio`, test classes).

---

## 7. Corrected Summary

| Layer | Original Claim | Verified Count | Accuracy |
|-------|---------------|---------------|----------|
| Orphaned Frontend Components | 16 | **16** | 100% |
| Unused Frontend API Methods | 37 | **22** | 59% |
| Broken NATS Subjects | 12 | **5 broken + 4 undefined** | 42% |
| Orphaned DB Tables | 4 | **1** | 25% |
| Orphaned Go Methods | 2 | **2** | 100% |
| Orphaned Python Classes | 5 | **2** | 40% |
| Misplaced Test Files | 2 | **2** | 100% |

### Root Cause of Report Errors

1. **DB Tables:** Initial search only checked Go code. Python GraphRAG uses these tables directly via psycopg — the Go store layer is bypassed by design (Approach C).
2. **NATS Subjects:** Initial search only checked static subscription registrations. Python uses dynamic runtime `start_cancel_listener()` for cancel subjects.
3. **Python Classes:** Initial search missed test instantiations and cross-module production calls.
4. **API Methods:** Initial search missed usage inside orphaned components (which are defined but not routed — the API calls exist in code but are unreachable).

---

## Recommendations

### Remove Dead Code
1. Delete `evaluation/runner.py` (legacy BenchmarkRunner, replaced by runners/)
2. Delete `memory/storage.py` (MemoryStore, replaced by ExperiencePool)
3. Move `tools/test_diff_output.py` and `tools/test_lint.py` to `workers/tests/`

### Wire Up Orphaned Frontend Components
The 16 components represent ~5,000 lines of implemented UI. Priority candidates:
1. **PlanPanel** (794 LOC) — plan decomposition is a core feature
2. **PolicyPanel** (866 LOC) — policy management has full backend support
3. **RunPanel** (325 LOC) — agent run monitoring is essential for UX
4. **AgentPanel + AgentNetwork** (606 LOC) — agent dispatch/coordination

### Fix Broken NATS Subjects
5 subjects where Python publishes results but Go ignores them:
1. `conversation.compact.complete` — add Go subscriber to update conversation state
2. `backends.health.result` — add Go subscriber to feed health dashboard
3. `review.trigger.complete` — add Go subscriber for review pipeline
4. `prompt.evolution.reflect/mutate.complete` — add Go subscriber for evolution loop

### Decide: Keep or Drop
1. **prompt_scores** table — implement store layer OR create drop migration
2. **4 undefined NATS subjects** — implement OR remove constants
3. **22 unused API methods** — wire to UI OR remove from frontend API layer
