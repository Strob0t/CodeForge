# CodeForge Unwired Features Report — Post-Remediation

**Date:** 2026-03-23 | **Branch:** staging | **Version:** 0.8.0 | **Status:** POST-REMEDIATION AUDIT

---

## Executive Summary

After merging 6 remediation branches, a full re-audit across all 5 architecture layers shows:

### What was FIXED (6 merges)
- **16 orphaned frontend components** -> ALL wired (0 remaining)
- **5 broken NATS subjects** (Python->Go) -> ALL subscribers added
- **prompt_scores table** -> Store layer implemented
- **PromptEvolution loop** -> HTTP endpoint + full loop wired
- **Dead code** (MemoryStore, BenchmarkRunner) -> Deleted
- **11 unused API methods** -> Removed
- **2 misplaced test files** -> Relocated

### What remains UNWIRED (newly discovered or pre-existing)

| Category | Count | Severity |
|----------|-------|----------|
| HTTP endpoints without frontend | **37+** across 6 features | HIGH |
| Broken NATS subjects | **9** critical | HIGH |
| Unwired service (PromptScoreCollector) | **1** | MEDIUM |
| SharedContext without HTTP endpoints | **1** | MEDIUM |
| Unused frontend API methods | **31** (stubs/redundant) | LOW |
| Python dead code | **3** items | LOW |

---

## Layer 1: HTTP Endpoints Without Frontend (37+)

Six entire backend feature areas have full HTTP handlers + routes but **zero frontend API resources**.

### 1. A2A Protocol (Phase 27) — 12 endpoints, CRITICAL

| Endpoint | Method | Handler |
|----------|--------|---------|
| /api/v1/a2a/agents | GET | ListA2AAgents |
| /api/v1/a2a/agents | POST | RegisterA2AAgent |
| /api/v1/a2a/agents/{id} | GET | GetA2AAgent |
| /api/v1/a2a/agents/{id} | DELETE | DeleteA2AAgent |
| /api/v1/a2a/tasks | POST | CreateA2ATask |
| /api/v1/a2a/tasks | GET | ListA2ATasks |
| /api/v1/a2a/tasks/{id} | GET | GetA2ATask |
| /api/v1/a2a/tasks/{id}/cancel | POST | CancelA2ATask |
| /api/v1/a2a/tasks/{id}/send | POST | SendA2AMessage |
| /api/v1/a2a/push-configs | GET | ListA2APushConfigs |
| /api/v1/a2a/push-configs | POST | CreateA2APushConfig |
| /api/v1/a2a/push-configs/{id} | DELETE | DeleteA2APushConfig |

### 2. Quarantine (Phase 23B) — 5 endpoints, CRITICAL

| Endpoint | Method | Handler |
|----------|--------|---------|
| /api/v1/quarantine | GET | ListQuarantineMessages |
| /api/v1/quarantine/{id} | GET | GetQuarantineMessage |
| /api/v1/quarantine/{id}/approve | POST | ApproveQuarantineMessage |
| /api/v1/quarantine/{id}/reject | POST | RejectQuarantineMessage |
| /api/v1/quarantine/stats | GET | GetQuarantineStats |

### 3. Microagents (Phase 22C) — 5 endpoints, HIGH

| Endpoint | Method | Handler |
|----------|--------|---------|
| /api/v1/microagents | GET | ListMicroagents |
| /api/v1/microagents | POST | CreateMicroagent |
| /api/v1/microagents/{id} | GET | GetMicroagent |
| /api/v1/microagents/{id} | PUT | UpdateMicroagent |
| /api/v1/microagents/{id} | DELETE | DeleteMicroagent |

### 4. Skills (Phase 22D) — 6 endpoints, HIGH

| Endpoint | Method | Handler |
|----------|--------|---------|
| /api/v1/skills | GET | ListSkills |
| /api/v1/skills | POST | CreateSkill |
| /api/v1/skills/{id} | GET | GetSkill |
| /api/v1/skills/{id} | PUT | UpdateSkill |
| /api/v1/skills/{id} | DELETE | DeleteSkill |
| /api/v1/skills/import | POST | ImportSkill |

### 5. Prompt Evolution (Phase 33) — 4 endpoints, HIGH

| Endpoint | Method | Handler |
|----------|--------|---------|
| /api/v1/prompt-evolution/variants | GET | ListPromptEvolutionVariants |
| /api/v1/prompt-evolution/variants/{id}/promote | POST | PromotePromptEvolutionVariant |
| /api/v1/prompt-evolution/modes/{id}/revert | POST | RevertPromptEvolutionMode |
| /api/v1/prompt-evolution/reflect | POST | TriggerPromptEvolutionReflect |

### 6. Routing (Phase 29) — 5 endpoints, MEDIUM

| Endpoint | Method | Handler |
|----------|--------|---------|
| /api/v1/routing/stats | GET | GetRoutingStats |
| /api/v1/routing/outcomes | GET | ListRoutingOutcomes |
| /api/v1/routing/outcomes | POST | RecordRoutingOutcome |
| /api/v1/routing/benchmark/seed | POST | SeedRoutingBenchmark |
| /api/v1/routing/config | GET | GetRoutingConfig |

---

## Layer 2: Broken NATS Subjects (9 critical)

### Go publishes, Python never subscribes (4)

| Subject | Go Publisher | Impact |
|---------|-------------|--------|
| conversation.run.cancel | ConversationAgentService | Cancel requests ignored by Python worker |
| tasks.cancel | Agent backends (aider, goose, etc.) | Task cancel requests ignored |
| context.shared.updated | ContextService | Shared context changes not propagated |
| prompt.evolution.promoted/reverted | PromptEvolutionService | Python doesn't update active prompts |

### Neither side wired (3)

| Subject | Issue |
|---------|-------|
| runs.qualitygate.request/result | Go never publishes request, never subscribes to result |
| review.trigger.request | Go never publishes (review trigger service uses DB, not NATS) |
| review.approval.required | Neither side publishes or subscribes |

### Go never subscribes to result (2)

| Subject | Issue |
|---------|-------|
| evaluation.gemmas.result | Go publishes request but ignores result |
| agents.output | Go subscribes but Python never publishes |

---

## Layer 3: Frontend

### Components: ALL WIRED (0 orphaned)

All 133 components are imported and rendered. The 16 previously-orphaned components are confirmed wired via ProjectDetailPage tabs and ChatPanel integration.

### Unused API Methods: 31 remaining

| Category | Methods | Reason |
|----------|---------|--------|
| LSP stubs (5) | diagnostics, definition, references, symbols, hover | Feature not fully integrated |
| Review stubs (7) | listPolicies, createPolicy, getPolicy, updatePolicy, deletePolicy, list, get | Feature incomplete |
| Redundant singles (8) | conversations.get/delete/fork/rewind, modes.get, scopes.get/update, goals.get | Covered by list+filter |
| Incomplete workflows (6) | projects.checkout, roadmap.deleteMilestone/deleteFeature, benchmarks.listDatasets/getSuite, plans.planFeature | Workflow not exposed |
| Other (5) | costs.byToolForRun, sessions.get, knowledgeBases.get, scopes.graphSearch, conversations.delete | Low priority |

**Verdict:** These are intentional stubs or redundant methods — not bugs. Low priority cleanup.

---

## Layer 4: Database & Service

### Tables: ALL 63 WIRED (0 orphaned)

- `prompt_scores` now has store methods (InsertPromptScore, GetScoresByFingerprint, GetAggregatedScores)
- `graph_nodes/edges/metadata` confirmed used by Python GraphRAG

### Unwired Services (2)

**1. PromptScoreCollector — NEVER INSTANTIATED**
- File: `internal/service/prompt_score.go`
- 8 methods (RecordScore, RecordBenchmarkScore, RecordSuccessScore, RecordCostScore, RecordUserFeedback, RecordEfficiencyScore, CompositeScoreForFingerprint, ScoreCountForFingerprint)
- Store methods exist, tests exist, but `NewPromptScoreCollector()` is never called in `main.go`
- **Impact:** Prompt evolution has no signal collection — variants can't be scored

**2. SharedContextService — NO HTTP ENDPOINTS**
- File: `internal/service/shared_context.go`
- Methods: InitForTeam, AddItem, Get
- Called internally by ContextOptimizerService only
- Tables `shared_contexts` + `shared_context_items` exist with store methods
- **Impact:** Team shared context is not manageable via API

---

## Layer 5: Python Workers

### Previous fixes CONFIRMED
- MemoryStore: deleted
- BenchmarkRunner: deleted
- Test files: relocated to workers/tests/

### New issues (3)

| Item | File | Issue |
|------|------|-------|
| FilesystemStateEvaluator | evaluation/evaluators/filesystem_state.py | Defined but never instantiated in _benchmark.py |
| run_streaming_subprocess() | backends/_streaming.py | Dead code, never imported |
| test_graphrag.py | workers/codeforge/test_graphrag.py | Orphaned test file (should be in tests/) |

---

## Cross-Reference: Priority Remediation

### Tier 1: CRITICAL (broken functionality)

| Issue | Break Type | Fix |
|-------|-----------|-----|
| 37+ HTTP endpoints without frontend | MISSING_FRONTEND | Create frontend API resources + pages for A2A, Quarantine, Microagents, Skills, PromptEvolution, Routing |
| PromptScoreCollector not instantiated | MISSING_WIRING | Call NewPromptScoreCollector(store) in main.go, inject into handlers |
| 4 NATS subjects (Go->Python ignored) | MISSING_SUBSCRIBER | Add Python cancel listeners for conversation.run.cancel, tasks.cancel |

### Tier 2: HIGH (incomplete features)

| Issue | Break Type | Fix |
|-------|-----------|-----|
| QualityGate NATS pair disconnected | MISSING_PUBLISHER | Wire Go to publish runs.qualitygate.request |
| GEMMAS eval result ignored | MISSING_SUBSCRIBER | Add Go subscriber for evaluation.gemmas.result |
| SharedContext no HTTP endpoints | MISSING_HANDLER | Add 3 REST endpoints for team context CRUD |
| FilesystemStateEvaluator unused | MISSING_REGISTRATION | Add to evaluator builder in _benchmark.py |

### Tier 3: LOW (cleanup)

| Issue | Fix |
|-------|-----|
| 31 unused frontend API methods | Remove or document as planned |
| run_streaming_subprocess() | Delete dead code |
| test_graphrag.py misplaced | Move to workers/tests/ |
| prompt.evolution.promoted/reverted events | Add Python listener or document as fire-and-forget |

---

## Comparison: Before vs After Remediation

| Metric | Before (6 merges) | After (re-audit) |
|--------|-------------------|-------------------|
| Orphaned frontend components | 16 | **0** |
| Unused frontend API methods | 22 verified | **31** (more found in deeper audit) |
| Broken NATS subjects | 5 verified | **9** (deeper audit found more) |
| Orphaned DB tables | 1 (prompt_scores) | **0** |
| Orphaned Go service methods | 2 | **0** (but PromptScoreCollector unwired) |
| Orphaned Python classes | 2 | **0** (but 3 minor items) |
| **HTTP endpoints without frontend** | not audited | **37+** (NEW finding) |
| **Unwired services** | not audited | **2** (NEW finding) |

**Key insight:** The first audit focused on already-implemented code that wasn't connected. The re-audit reveals a deeper layer: **entire backend features (A2A, Quarantine, Skills, Microagents) that have zero frontend presence.** These represent the largest remaining gap — 37+ production-ready endpoints with no UI.
