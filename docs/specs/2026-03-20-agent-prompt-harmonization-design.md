# Agent & Prompt System Harmonization

**Date:** 2026-03-20
**Status:** Approved
**Audit:** `docs/audits/agent-prompt-system-audit.md`

---

## 1. Goals

Harmonize the CodeForge agent description and prompt system across all layers (Go, Python, YAML, DB, Frontend). Eliminate inconsistencies, fill gaps, remove dead code, unify naming conventions.

## 2. Decisions

| # | Decision | Rationale |
|---|---|---|
| D1 | Full scope: inconsistencies + gaps + refactoring | Technical debt compounds; fix now |
| D2 | Add 3 permanent tools to Go system prompt template | Go is source of truth; these tools are always available |
| D3 | Mode IDs: kebab-case → snake_case with DB migration | Unified convention across Go/Python/YAML |
| D4 | CLAUDE.md: correct "Tool Bundles" terminology | Tools are per-mode, no separate bundle YAML exists |
| D5 | Remove legacy template fallback | PromptAssembler is stable; one code path |

## 3. Affected Files

### 3.1 Mode ID Migration (kebab → snake_case)

8 mode IDs change. 16 stay unchanged (already no hyphen).

| Old ID | New ID |
|---|---|
| `api-tester` | `api_tester` |
| `backend-architect` | `backend_architect` |
| `lsp-engineer` | `lsp_engineer` |
| `workflow-optimizer` | `workflow_optimizer` |
| `infra-maintainer` | `infra_maintainer` |
| `goal-researcher` | `goal_researcher` |
| `boundary-analyzer` | `boundary_analyzer` |
| `contract-reviewer` | `contract_reviewer` |

**Go files to update:**

| File | Lines | What changes |
|---|---|---|
| `internal/domain/mode/presets.go` | 154, 190, 203, 242, 255, 287, 299, 311 | Mode `ID` strings |
| `internal/domain/mode/mode_test.go` | 206, 209-210, 213-214, 216-218 | Test map keys |
| `internal/domain/mode/presets_test.go` | 7, 24, 30 | `boundary-analyzer` references |
| `internal/domain/pipeline/presets.go` | 51, 60-61 | `boundary-analyzer`, `contract-reviewer` |
| `internal/domain/pipeline/presets_test.go` | 23-27 | Pipeline step assertions |
| `internal/service/context_budget.go` | 21-22 | Budget map keys |
| `internal/service/context_budget_test.go` | 161-162 | Test case names/IDs |
| `internal/service/mode_prompt_test.go` | 332-333 | Test expectations |
| `internal/service/prompt_system_test.go` | 21-26 | Mode ID list |
| `internal/service/conversation_test.go` | 638, 907, 926, 938 | Integration test IDs |
| `internal/adapter/http/handlers_goals.go` | 157 | `"goal-researcher"` literal |

**YAML files to update (8 mode files):**

| File | Line 1 | Line 8 |
|---|---|---|
| `internal/service/prompts/modes/api_tester.yaml` | `id: mode.api-tester` → `mode.api_tester` | `- api-tester` → `- api_tester` |
| `internal/service/prompts/modes/backend_architect.yaml` | same pattern | same pattern |
| `internal/service/prompts/modes/boundary_analyzer.yaml` | same pattern | same pattern |
| `internal/service/prompts/modes/contract_reviewer.yaml` | same pattern | same pattern |
| `internal/service/prompts/modes/goal_researcher.yaml` | same pattern | same pattern |
| `internal/service/prompts/modes/infra_maintainer.yaml` | same pattern | same pattern |
| `internal/service/prompts/modes/lsp_engineer.yaml` | same pattern | same pattern |
| `internal/service/prompts/modes/workflow_optimizer.yaml` | same pattern | same pattern |

**Python files to update:**

| File | Lines | What changes |
|---|---|---|
| `workers/codeforge/consumer/_review.py` | 29, 60, 64, 89, 110 | `boundary-analyzer` → `boundary_analyzer` |
| `workers/tests/consumer/test_review.py` | 3, 117, 126 | Same |

**Database migration:**

New migration `086_mode_id_snake_case.sql` (085 is the highest existing migration):

```sql
-- +goose Up
-- Convert kebab-case mode IDs to snake_case across all tables.
UPDATE conversations SET mode = REPLACE(mode, '-', '_') WHERE mode LIKE '%-%';
UPDATE agents SET mode_id = REPLACE(mode_id, '-', '_') WHERE mode_id LIKE '%-%';
UPDATE runs SET mode_id = REPLACE(mode_id, '-', '_') WHERE mode_id LIKE '%-%';
UPDATE plan_steps SET mode_id = REPLACE(mode_id, '-', '_') WHERE mode_id LIKE '%-%';
UPDATE prompt_scores SET mode_id = REPLACE(mode_id, '-', '_') WHERE mode_id LIKE '%-%';
UPDATE prompt_sections SET mode_id = REPLACE(mode_id, '-', '_') WHERE mode_id LIKE '%-%';

-- +goose Down
UPDATE conversations SET mode = REPLACE(mode, '_', '-')
  WHERE mode IN ('api_tester','backend_architect','lsp_engineer','workflow_optimizer','infra_maintainer','goal_researcher','boundary_analyzer','contract_reviewer');
UPDATE agents SET mode_id = REPLACE(mode_id, '_', '-')
  WHERE mode_id IN ('api_tester','backend_architect','lsp_engineer','workflow_optimizer','infra_maintainer','goal_researcher','boundary_analyzer','contract_reviewer');
UPDATE runs SET mode_id = REPLACE(mode_id, '_', '-')
  WHERE mode_id IN ('api_tester','backend_architect','lsp_engineer','workflow_optimizer','infra_maintainer','goal_researcher','boundary_analyzer','contract_reviewer');
UPDATE plan_steps SET mode_id = REPLACE(mode_id, '_', '-')
  WHERE mode_id IN ('api_tester','backend_architect','lsp_engineer','workflow_optimizer','infra_maintainer','goal_researcher','boundary_analyzer','contract_reviewer');
UPDATE prompt_scores SET mode_id = REPLACE(mode_id, '_', '-')
  WHERE mode_id IN ('api_tester','backend_architect','lsp_engineer','workflow_optimizer','infra_maintainer','goal_researcher','boundary_analyzer','contract_reviewer');
UPDATE prompt_sections SET mode_id = REPLACE(mode_id, '_', '-')
  WHERE mode_id IN ('api_tester','backend_architect','lsp_engineer','workflow_optimizer','infra_maintainer','goal_researcher','boundary_analyzer','contract_reviewer');
```

**Frontend:** No hardcoded kebab-case mode IDs found. Frontend uses mode data dynamically from API. No changes needed.

### 3.2 Tool Documentation

| File | Change |
|---|---|
| `internal/service/templates/conversation_system.tmpl` | Add `search_conversations`, `search_skills`, `create_skill` to tool docs |

### 3.3 Legacy Fallback Removal

| File | Lines | Change |
|---|---|---|
| `internal/service/conversation_agent.go` | 1006-1028 | Remove `if s.promptAssembler != nil` guard + legacy template block. Assembler is always set in `main.go:548-559`. Add `slog.Error` if assembler returns empty. |
| `internal/service/conversation.go` | 29-33 | Remove `//go:embed` for `conversationSystemTmpl` and related vars |
| `internal/service/templates/conversation_system.tmpl` | -- | Keep file for reference but remove `//go:embed` usage |

### 3.4 Microagent Gaps

| File | Change |
|---|---|
| `cmd/codeforge/main.go` | Wire `LoadFromDirectory(".codeforge/microagents")` on startup |
| `.codeforge/microagents/README.md` | Create with usage instructions |
| `.codeforge/microagents/error_helper.md` | Default microagent: error debugging |
| `.codeforge/microagents/test_helper.md` | Default microagent: test writing guidance |
| `.codeforge/microagents/security_checker.md` | Default microagent: security awareness |
| `.codeforge/microagents/commit_helper.md` | Default microagent: commit message guidance |
| `.codeforge/microagents/doc_helper.md` | Default microagent: documentation reminders |

### 3.5 Custom Modes Directory

| File | Change |
|---|---|
| `.codeforge/modes/README.md` | Create with custom mode instructions |
| `cmd/codeforge/main.go` | Wire custom mode loading from `.codeforge/modes/` |

### 3.6 Documentation

| File | Change |
|---|---|
| `CLAUDE.md` | Fix "Tool Bundles" → "per-mode Tool lists"; document naming convention; update Phase 31 mode IDs (line ~124) |
| `internal/domain/mode/mode.go` | Add comment documenting snake_case ID convention |
| `docs/features/04-agent-orchestration.md` | Update kebab-case mode IDs at lines 863-864, 895-896, 909 |
| `docs/todo.md` | Update mode IDs at lines 1165, 1284, 1484; mark harmonization tasks complete |

---

## 4. Atomic Work Plan

Each step is independently committable. Steps are ordered so the system stays functional after each commit.

### Phase A: Non-Breaking Preparation

**Step A1: Create directory scaffolding**
- Create `.codeforge/microagents/` with README
- Create `.codeforge/modes/` with README
- Create 5 default microagent `.md` files
- **Test:** Files exist, valid YAML front matter
- **Commit:** `chore: add .codeforge/microagents and modes directories with defaults`

**Step A2: Add 3 tools to system prompt template**
- Edit `internal/service/templates/conversation_system.tmpl`
- Add `search_conversations`, `search_skills`, `create_skill` descriptions
- **Test:** Template renders without error (existing tests)
- **Commit:** `fix(prompt): document search_conversations, search_skills, create_skill in system prompt`

**Step A3: Document naming convention**
- Add comment in `internal/domain/mode/mode.go` explaining snake_case convention
- **Test:** `go build ./...`
- **Commit:** `docs: document snake_case mode ID convention in mode.go`

### Phase B: Mode ID Migration (single atomic commit)

**Step B1: Migrate all mode IDs kebab → snake_case**

All changes in one commit to avoid inconsistent state:

1. `internal/domain/mode/presets.go` — 8 ID strings
2. `internal/domain/mode/mode_test.go` — test map keys
3. `internal/domain/mode/presets_test.go` — assertions
4. `internal/domain/pipeline/presets.go` — pipeline step ModeIDs
5. `internal/domain/pipeline/presets_test.go` — assertions
6. `internal/service/context_budget.go` — budget map keys
7. `internal/service/context_budget_test.go` — test data
8. `internal/service/mode_prompt_test.go` — test expectations
9. `internal/service/prompt_system_test.go` — mode ID list
10. `internal/service/conversation_test.go` — integration tests
11. `internal/adapter/http/handlers_goals.go` — goal-researcher literal
12. 8 YAML files in `internal/service/prompts/modes/` — `id:` and `conditions.modes:`
13. `workers/codeforge/consumer/_review.py` — boundary-analyzer references
14. `workers/tests/consumer/test_review.py` — test assertions
15. New migration: `internal/adapter/postgres/migrations/086_mode_id_snake_case.sql`
16. `docs/features/04-agent-orchestration.md` — mode IDs in context budget / Phase 31 docs
17. `CLAUDE.md` — Phase 31 mode ID references
18. `docs/todo.md` — completed task entries with old IDs

- **Test:** `go test ./internal/...` + `cd workers && python -m pytest tests/consumer/test_review.py`
- **Commit:** `refactor(modes): migrate mode IDs from kebab-case to snake_case`

### Phase C: Cleanup & Wiring

**Step C1: Remove legacy template fallback**
1. Edit `internal/service/conversation_agent.go` — remove fallback block (lines 1006-1028), make assembler the only path, add `slog.Error` for empty result
2. Edit `internal/service/conversation.go` — remove `//go:embed` for `conversationSystemTmpl` and `conversationTmpl` var
3. Keep `conversation_system.tmpl` file (used in tests and as reference)
- **Test:** `go test ./internal/service/... -run Conversation`
- **Commit:** `refactor(prompt): remove legacy template fallback, PromptAssembler is sole path`

**Step C2: Wire microagent auto-loading**
1. Edit `cmd/codeforge/main.go` — after microagentSvc init, call `LoadFromDirectory(".codeforge/microagents")` and create each via `microagentSvc.Create()`
2. Skip duplicates (name already exists)
- **Test:** Start server, verify microagents loaded via `GET /api/v1/projects/{id}/microagents`
- **Commit:** `feat(microagent): auto-load .codeforge/microagents/*.md on startup`

**Step C3: Wire custom mode loading**
1. Edit `cmd/codeforge/main.go` — scan `.codeforge/modes/*.yaml`, parse, register via `modeSvc.Register()`
2. Skip if name conflicts with builtin
- **Test:** Place test YAML in `.codeforge/modes/`, verify via `GET /api/v1/modes`
- **Commit:** `feat(modes): auto-load custom modes from .codeforge/modes/`

### Phase D: Documentation

**Step D1: Update CLAUDE.md and audit docs**
1. CLAUDE.md: Fix "Tool Bundles" description, update naming convention section
2. `docs/todo.md`: Mark harmonization tasks complete
3. `docs/audits/agent-prompt-system-audit.md`: Add "Resolution" section
- **Commit:** `docs: update CLAUDE.md and audit for prompt system harmonization`

---

## 5. Verification

After all steps:

1. `go build ./cmd/codeforge/` — compiles
2. `go test ./internal/...` — all pass
3. `cd workers && python -m pytest` — all pass
4. `pre-commit run --all-files` — clean
5. Manual: Start server, verify `/api/v1/modes` returns snake_case IDs
6. Manual: Verify microagents loaded via API
7. Manual: Send agentic message, confirm system prompt includes all 10 tool descriptions

## 6. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| DB migration on production data | Reversible (goose down); tested on dev DB first |
| NATS in-flight messages with old IDs | Deploy during low traffic; old messages expire via AckWait |
| PromptAssembler empty on edge case | `slog.Error` + minimal fallback string (project name only) |
| Custom mode file parse errors | Log warning, skip file, don't crash startup |
