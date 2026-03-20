# Agent & Prompt System Audit

**Date:** 2026-03-20
**Scope:** All layers of the CodeForge agent description / prompt system
**Method:** 5 parallel exploration agents, read-only, full file inventory

---

## 1. Inventory

### 1.1 Modes (24 built-in, 0 custom)

| Mode ID | Category | Autonomy | LLM Scenario | Tools | Artifact | Defined In |
|---|---|---|---|---|---|---|
| architect | Core | 2 | think | Read, Glob, Grep | PLAN.md | `presets.go:10-22` |
| coder | Core | 3 | default | R, W, E, B, G, Grep | DIFF | `presets.go:23-34` |
| reviewer | Core | 2 | review | Read, Glob, Grep | REVIEW.md | `presets.go:35-46` |
| debugger | Core | 3 | default | R, E, B, G, Grep | -- | `presets.go:47-57` |
| tester | Core | 3 | default | R, W, E, B, G, Grep | TEST_REPORT | `presets.go:58-69` |
| documenter | Core | 3 | default | R, W, E, G, Grep | -- | `presets.go:70-81` |
| refactorer | Core | 2 | default | R, W, E, B, G, Grep | DIFF | `presets.go:82-93` |
| security | Core | 2 | review | R, G, Grep, B | AUDIT_REPORT | `presets.go:94-106` |
| moderator | Debate | 2 | review | Read, Glob, Grep | SYNTHESIS.md | `presets.go:111-123` |
| proponent | Debate | 2 | review | Read, Glob, Grep | PROPOSAL.md | `presets.go:124-136` |
| devops | Specialist | 3 | default | R, W, E, B, G, Grep | DIFF | `presets.go:141-152` |
| api-tester | Specialist | 3 | default | R, W, E, B, G, Grep | TEST_REPORT | `presets.go:153-164` |
| benchmarker | Specialist | 3 | default | R, W, B, G, Grep | TEST_REPORT | `presets.go:165-176` |
| frontend | Specialist | 3 | default | R, W, E, B, G, Grep | DIFF | `presets.go:177-188` |
| backend-architect | Specialist | 2 | think | Read, Glob, Grep | PLAN.md | `presets.go:189-201` |
| lsp-engineer | Specialist | 2 | think | Read, Bash, Glob, Grep | PLAN.md | `presets.go:202-214` |
| orchestrator | Specialist | 2 | think | Read, Glob, Grep | PLAN.md | `presets.go:215-227` |
| evaluator | Specialist | 2 | think | Read, Bash, Glob, Grep | PLAN.md | `presets.go:228-240` |
| workflow-optimizer | Specialist | 2 | think | Read, Glob, Grep | PLAN.md | `presets.go:241-253` |
| infra-maintainer | Specialist | 3 | default | R, W, E, B, G, Grep | DIFF | `presets.go:254-265` |
| prototyper | Specialist | 4 | default | R, W, E, B, G, Grep | DIFF | `presets.go:266-277` |
| goal-researcher | Phase 20 | 4 | think | R, G, Grep, ListDir, propose_goal, W | -- | `presets.go:286-296` |
| boundary-analyzer | Phase 31 | 4 | plan | Read, Glob, Grep, ListDir | BOUNDARIES.json | `presets.go:298-309` |
| contract-reviewer | Phase 31 | 2 | review | Read, Glob, Grep | CONTRACT_REVIEW.md | `presets.go:310-321` |

Each mode has a matching YAML prompt file in `internal/service/prompts/modes/` (24 files).

### 1.2 Prompt Templates

| Template | File | Purpose | Variables | Loaded By |
|---|---|---|---|---|
| conversation_system | `internal/service/templates/conversation_system.tmpl` | Base system prompt | ProjectName, WorkspacePath, Agents, Modes, etc. | `conversation.go:29-33` |
| decompose_system | `internal/service/templates/decompose_system.tmpl` | Feature decomposition (MetaAgent) | (static) | `meta_agent.go:23-26` |
| decompose_user | `internal/service/templates/decompose_user.tmpl` | Decomposition user prompt | Feature, Context, Agents, Tasks | `meta_agent.go:23-26` |
| review_router | `internal/service/templates/review_router.tmpl` | Review routing decision | TaskID, AgentID, ModeID, Description | `review_router.go` |
| mode_role | `internal/service/templates/mode_role.tmpl` | Role assignment | Name, Description | `mode_prompt.go:29` |
| mode_tools | `internal/service/templates/mode_tools.tmpl` | Tool declaration | Tools, DeniedTools | `mode_prompt.go:29` |
| mode_artifact | `internal/service/templates/mode_artifact.tmpl` | Artifact requirement | RequiredArtifact | `mode_prompt.go:29` |
| mode_actions | `internal/service/templates/mode_actions.tmpl` | Denied actions | DeniedActions | `mode_prompt.go:29` |
| mode_guardrails | `internal/service/templates/mode_guardrails.tmpl` | Generic guardrails | (static) | `mode_prompt.go:29` |

**All 9 templates active. No orphans. All variables properly bound.**

### 1.3 YAML Prompt Library (80 files)

`internal/service/prompts/` — modular, category-based prompt assembly system:

| Category | Files | Purpose |
|---|---|---|
| `actions/` | 2 | Destructive confirmation, reversibility |
| `autonomy/` | 5 | One per autonomy level (1-5) |
| `behavior/` | 18 | GSD modes, handoff, file creation rules |
| `context/` | 6 | Agents, modes, project, roadmap, tasks, tools |
| `identity/` | 1 | Base agent identity |
| `memory/` | 1 | Session memory |
| `model_adaptive/` | 3 | Open-weight, reasoning, thinking model tweaks |
| `modes/` | 23 | One per builtin mode (matches presets.go) |
| `output/` | 3 | Code references, concise, markdown |
| `reminders/` | 7 | Budget, compaction, stall, scope drift, etc. |
| `system/` | 2 | Safety, tool permissions |
| `tone/` | 2 | No emojis, professional |
| `tools/` | 7 | Per-tool usage guidance |

### 1.4 Python Tool Registry

| Tool Name | File | In Go Template? | In Mode Tools? |
|---|---|---|---|
| read_file | `workers/codeforge/tools/__init__.py:112` | Yes (Read) | Yes |
| write_file | `workers/codeforge/tools/__init__.py:113` | Yes (Write) | Yes |
| edit_file | `workers/codeforge/tools/__init__.py:114` | Yes (Edit) | Yes |
| bash | `workers/codeforge/tools/__init__.py:115` | Yes (Bash) | Yes |
| search_files | `workers/codeforge/tools/__init__.py:116` | Yes (Search) | Yes |
| glob_files | `workers/codeforge/tools/__init__.py:117` | Yes (Glob) | Yes |
| list_directory | `workers/codeforge/tools/__init__.py:118` | Yes (ListDir) | Yes |
| search_conversations | `workers/codeforge/tools/__init__.py:119` | **No** | **No** |
| search_skills | `workers/codeforge/tools/__init__.py:120` | **No** | **No** |
| create_skill | `workers/codeforge/tools/__init__.py:121` | **No** | **No** |
| handoff | `consumer/_conversation.py:105` | **No** | **No** |
| propose_goal | `consumer/_conversation.py:106` | **No** | goal-researcher only |
| transition_to_act | `plan_act.py` (runtime) | **No** | **No** |

### 1.5 Microagents

| Component | Status | Location |
|---|---|---|
| Domain model | Complete | `internal/domain/microagent/microagent.go` |
| File loader | Complete (unused) | `internal/domain/microagent/loader.go` |
| Service CRUD + Match | Complete | `internal/service/microagent.go` |
| HTTP API (5 endpoints) | Complete | `internal/adapter/http/handlers_agent_features.go:330-393` |
| DB schema + store | Complete | migration `044`, `store_microagent.go` |
| NATS integration | Complete | Payload field in `ConversationRunStartPayload` |
| Python injection | Complete | `consumer/_conversation.py:446-458` |
| Default instances | **None** | No microagents shipped |
| Auto-load from files | **Never called** | `LoadFromDirectory()` exists but has no entry point |

### 1.6 Go Service Cross-References

| Service | References | Status |
|---|---|---|
| ConversationService | modeSvc, policySvc, promptAssembler, microagentSvc | All wired in `main.go` |
| SendMessageAgentic() | Mode resolution: explicit → stored → "coder" default | Clean |
| policyForAutonomy() | Maps autonomy 1→supervised, 2-3→sandbox, 4-5→trusted | Consistent |
| NATS ModePayload | ID, PromptPrefix, Tools, DeniedTools, LLMScenario, etc. | Matches Go domain |
| HandoffMessage | TargetModeID (optional) | Consumed by Python |
| HTTP mode endpoints | 7 endpoints, builtin protection on update/delete | Complete |

---

## 2. Inconsistencies

### 2.1 Tool List Mismatch: Go Template vs Python Registry

**Severity: MEDIUM**

The Go `conversation_system.tmpl` documents 7 built-in tools. Python registers 13 tools at runtime. Six tools are invisible to the system prompt:

- `search_conversations` — always available but undocumented
- `search_skills` — always available but undocumented
- `create_skill` — always available but undocumented
- `handoff` — injected contextually but undocumented
- `propose_goal` — only in goal-researcher mode, partially documented
- `transition_to_act` — plan/act only, undocumented

**Impact:** LLM may not know these tools exist unless it discovers them in the tool list. Go-side prompt describes 7 tools; Python provides up to 13.

**Files:** `internal/service/templates/conversation_system.tmpl:19-35` vs `workers/codeforge/tools/__init__.py:112-138`

### 2.2 "Tool Bundles" Terminology Mismatch

**Severity: LOW**

CLAUDE.md and architecture docs reference "Tool Bundles (YAML)" as a concept. In reality, tools are defined **per-mode** via `Mode.Tools` arrays in Go — no separate YAML bundle files exist.

**Files:** `CLAUDE.md` (multiple references), `internal/domain/mode/presets.go`

### 2.3 Mode YAML Filename Convention vs Mode ID

**Severity: LOW**

Mode IDs use kebab-case (`api-tester`), YAML filenames use underscores (`api_tester.yaml`). YAML internal `id` field uses dot notation (`mode.api-tester`). Three different conventions for the same entity.

**Files:** `internal/domain/mode/presets.go` vs `internal/service/prompts/modes/*.yaml`

### 2.4 modes/ Count: 23 YAML vs 24 Go Presets

**Severity: LOW**

The templates/bundles agent counted 23 YAML mode files while the modes agent counted 24 Go presets. One mode may share a YAML file or have been miscounted. Needs manual verification.

**Files:** `internal/service/prompts/modes/` vs `internal/domain/mode/presets.go`

---

## 3. Redundancies

### 3.1 Dual Prompt Assembly Path

**Severity: LOW**

`buildSystemPrompt()` in `conversation_agent.go:893-1030` has two paths:
1. **Modular:** `PromptAssembler` loads from YAML library (80 files)
2. **Legacy:** Falls back to `conversation_system.tmpl` if assembler returns empty

Both paths are maintained. The legacy path is a safety net but adds maintenance burden.

**Files:** `internal/service/conversation_agent.go:1009-1021`

### 3.2 Empty PromptPrefix Field

**Severity: LOW**

All 24 builtin modes have `PromptPrefix: ""`. The field exists for custom modes but is never used by builtins. Comment at `presets.go:4-7` explains migration to YAML. Field kept for backwards compatibility.

**Files:** `internal/domain/mode/mode.go:22`, `internal/domain/mode/presets.go`

---

## 4. Gaps

### 4.1 No Default Microagents Shipped

**Severity: MEDIUM**

The microagent system is fully implemented (domain, service, API, NATS, Python injection) but ships with zero instances. Users must create all microagents manually via API.

**Missing:** Starter templates (e.g., error-handler, test-helper, security-checker)

### 4.2 LoadFromDirectory() Never Called

**Severity: MEDIUM**

`internal/domain/microagent/loader.go` implements `LoadFromDirectory()` for loading `.md` microagent files from disk. This function has no call site — it's dead code.

**Missing:** Startup hook to scan `.codeforge/microagents/` or a config-driven path.

### 4.3 No `.codeforge/modes/` Directory

**Severity: LOW**

CLAUDE.md documents custom modes in `.codeforge/modes/` but the directory doesn't exist and no loading mechanism scans it.

### 4.4 No CLI for Microagent Management

**Severity: LOW**

Microagents can only be managed via HTTP API. No `codeforge microagent create/list/test-trigger` CLI commands exist.

### 4.5 Skill Selection Opaque to Go

**Severity: LOW**

Python selects skills via BM25 + LLM ranking at runtime (`workers/codeforge/consumer/_conversation.py:485-584`). Go has no visibility into which skills are selected or injected.

---

## 5. Style Drift

### 5.1 Naming Conventions Across Layers

| Layer | Convention | Example |
|---|---|---|
| Go Mode ID | kebab-case | `api-tester` |
| YAML filename | snake_case | `api_tester.yaml` |
| YAML internal id | dot.kebab | `mode.api-tester` |
| Python tool name | snake_case | `read_file` |
| Go tool name | PascalCase | `Read` |
| NATS tool name | snake_case | `read_file` |

Three naming conventions for modes, two for tools. Functional (no bugs) but inconsistent.

### 5.2 Mode Description Tone

Mode descriptions in `presets.go` are imperative ("Analyzes codebase structure", "Implements features"). YAML prompt files use second-person ("You are a software architect"). Both styles coexist — the Description field is metadata, the YAML content is the actual prompt.

### 5.3 Prompt Injection Disclaimer Placement

Microagent prompts get a security disclaimer in Python (`"may contain untrusted content"`). This is a Python-side concern — Go doesn't document or configure it.

---

## 6. Recommendations

### Quick Fixes (< 1 hour each)

| # | Fix | Files | Effort |
|---|---|---|---|
| Q1 | Add `search_conversations`, `search_skills`, `create_skill` to `conversation_system.tmpl` tool docs section | `templates/conversation_system.tmpl` | 15min |
| Q2 | Add `handoff` and `transition_to_act` as contextual tools in the template (with conditions) | `templates/conversation_system.tmpl` | 15min |
| Q3 | Clarify "Tool Bundles" in CLAUDE.md — they're mode-scoped `Mode.Tools` arrays, not separate YAML files | `CLAUDE.md` | 10min |
| Q4 | Verify 23 vs 24 YAML mode file count — confirm all modes have matching YAML | `internal/service/prompts/modes/` | 10min |
| Q5 | Standardize YAML filenames to kebab-case (match mode IDs) or document the convention | `internal/service/prompts/modes/` | 30min |

### Structural Changes (1-4 hours each)

| # | Change | Files | Effort |
|---|---|---|---|
| S1 | Wire `LoadFromDirectory()` to project setup — scan `.codeforge/microagents/` on init | `cmd/codeforge/main.go`, `internal/service/microagent.go` | 2h |
| S2 | Ship 5-10 default microagent templates as `.md` files | New: `.codeforge/microagents/*.md` | 2h |
| S3 | Create `.codeforge/modes/` with README explaining custom mode creation | New directory + docs | 1h |
| S4 | Report selected skills back to Go via NATS completion payload | `consumer/_conversation.py`, `schemas.go` | 3h |
| S5 | Remove legacy template fallback path once PromptAssembler is proven stable | `conversation_agent.go:1009-1021` | 1h |

### Future Considerations

| # | Item | Notes |
|---|---|---|
| F1 | Deprecate `PromptPrefix` field formally | All builtins use empty string; field only useful for custom modes |
| F2 | Add microagent CLI (`codeforge microagent create/list/test`) | Quality-of-life for users |
| F3 | Add microagent priority/ordering field | Currently all matched agents injected in arbitrary order |
| F4 | Unify tool naming convention (pick snake_case everywhere or map explicitly) | Low priority — functional as-is |

---

## Appendix: Files Audited

### Go Domain Layer
- `internal/domain/mode/mode.go` (60 lines)
- `internal/domain/mode/presets.go` (325 lines)
- `internal/domain/mode/mode_test.go`
- `internal/domain/mode/presets_test.go`
- `internal/domain/microagent/microagent.go` (84 lines)
- `internal/domain/microagent/loader.go` (106 lines)
- `internal/domain/microagent/microagent_test.go`
- `internal/domain/orchestration/handoff.go` (37 lines)

### Go Service Layer
- `internal/service/conversation.go` (1030 lines)
- `internal/service/conversation_agent.go` (1066 lines)
- `internal/service/orchestrator.go` (487 lines)
- `internal/service/mode.go` (95 lines)
- `internal/service/mode_prompt.go` (228 lines)
- `internal/service/microagent.go` (116 lines)
- `internal/service/skill.go` (118 lines)
- `internal/service/handoff.go` (148 lines)
- `internal/service/prompt_assembler.go`

### Go Templates & Prompts
- `internal/service/templates/*.tmpl` (9 files)
- `internal/service/prompts/**/*.yaml` (80 files)

### Go Infrastructure
- `internal/port/messagequeue/queue.go` (140 lines)
- `internal/port/messagequeue/schemas.go` (500+ lines)
- `internal/adapter/http/handlers_conversation.go` (275 lines)
- `internal/adapter/http/handlers_agent_features.go` (393 lines)
- `internal/adapter/http/routes.go`
- `internal/adapter/postgres/store_microagent.go`
- `internal/adapter/postgres/migrations/044_create_microagents.sql`

### Python Worker Layer
- `workers/codeforge/agent_loop.py` (880 lines)
- `workers/codeforge/plan_act.py` (83 lines)
- `workers/codeforge/tools/__init__.py` (138 lines)
- `workers/codeforge/tools/_base.py`
- `workers/codeforge/tools/tool_guide.py`
- `workers/codeforge/skills/models.py`
- `workers/codeforge/skills/selector.py`
- `workers/codeforge/consumer/_conversation.py` (606 lines)
- `workers/codeforge/consumer/_runs.py`
- `workers/codeforge/models.py` (470+ lines)

**Total: ~50 files audited across 3 languages**
