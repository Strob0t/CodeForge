# Contract-First Review/Refactor System — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Contract-First Review/Refactor pipeline to CodeForge that automatically detects boundary files, reviews cross-layer contracts, proposes refactorings, and pauses for human approval on large changes.

**Architecture:** Hybrid Pipeline-Native + Agent-Driven. Go Core orchestrates the 4-step pipeline deterministically. Python/LLM provides boundary analysis, contract review, and refactoring content. Threshold-based HITL pauses for user approval when diffs exceed configurable limits.

**Tech Stack:** Go 1.25, PostgreSQL 18 (pgx v5, goose migrations), NATS JetStream, SolidJS + TypeScript, Python 3.12

**Spec:** `docs/specs/2026-03-15-contract-first-review-refactor-design.md`

---

## File Structure

### New Files

| File | Responsibility |
|---|---|
| `internal/domain/boundary/boundary.go` | Domain types: ProjectBoundaryConfig, BoundaryFile, BoundaryType, validation |
| `internal/domain/boundary/boundary_test.go` | Domain type validation tests |
| `internal/service/boundary.go` | BoundaryService: CRUD, trigger LLM analysis |
| `internal/service/boundary_test.go` | BoundaryService unit tests |
| `internal/service/review_trigger.go` | ReviewTriggerService: cascade trigger + dedup |
| `internal/service/review_trigger_test.go` | ReviewTriggerService unit tests |
| `internal/service/diff_impact.go` | DiffImpactScorer: threshold-based HITL decision |
| `internal/service/diff_impact_test.go` | DiffImpactScorer unit tests |
| `internal/adapter/http/handlers_review.go` | HTTP handlers: boundaries, triggers, approval |
| `internal/adapter/postgres/store_boundary.go` | PostgreSQL queries for boundaries |
| `internal/adapter/postgres/store_review_trigger.go` | PostgreSQL queries for trigger dedup |
| `internal/adapter/postgres/migrations/073_create_project_boundaries.sql` | Boundaries table |
| `internal/adapter/postgres/migrations/074_create_review_triggers.sql` | Review triggers table |
| `workers/codeforge/consumer/_review.py` | Python NATS handler for review events |
| `workers/tests/test_review_consumer.py` | Python consumer tests |
| `frontend/src/features/project/RefactorApproval.tsx` | HITL approval UI with diff preview |
| `frontend/src/features/project/BoundariesPanel.tsx` | Boundary config viewer/editor |

### Modified Files

| File | Change |
|---|---|
| `internal/domain/mode/presets.go` | Add boundary-analyzer + contract-reviewer modes |
| `internal/domain/pipeline/presets.go` | Add review-refactor template |
| `internal/domain/plan/plan.go` | Add StepStatusWaitingApproval constant |
| `internal/port/database/store.go` | Add boundary + review trigger store methods |
| `internal/adapter/nats/nats.go` | Add `"review.>"` to JetStream stream Subjects |
| `internal/port/messagequeue/queue.go` | Add review.* NATS subjects |
| `internal/port/messagequeue/schemas.go` | Add review payload structs |
| `internal/service/orchestrator.go` | Handle waiting_approval in advancePlan() |
| `internal/service/context_budget.go` | Add PhaseAwareContextBudget() |
| `internal/adapter/http/routes.go` | Register new review/boundary routes |
| `workers/codeforge/consumer/_subjects.py` | Add review.* subject constants + stream wildcard |
| `workers/codeforge/consumer/__init__.py` | Register review subscription |
| `workers/codeforge/models.py` | Add ReviewTriggerPayload Pydantic model |

---

## Chunk 1: Domain Layer + DB Migrations

### Task 1: Boundary Domain Model

**Files:**
- Create: `internal/domain/boundary/boundary.go`
- Create: `internal/domain/boundary/boundary_test.go`

- [ ] **Step 1: Write failing tests for BoundaryType validation**

```go
// internal/domain/boundary/boundary_test.go
package boundary

import "testing"

func TestBoundaryTypeValid(t *testing.T) {
	tests := []struct {
		name    string
		bt      BoundaryType
		wantErr bool
	}{
		{"api", BoundaryTypeAPI, false},
		{"data", BoundaryTypeData, false},
		{"inter-service", BoundaryTypeInterService, false},
		{"cross-language", BoundaryTypeCrossLanguage, false},
		{"empty", BoundaryType(""), true},
		{"invalid", BoundaryType("foobar"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.bt.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./internal/domain/boundary/ -v -run TestBoundaryTypeValid`
Expected: FAIL (package does not exist yet)

- [ ] **Step 3: Write BoundaryType implementation**

```go
// internal/domain/boundary/boundary.go
package boundary

import (
	"errors"
	"strconv"
	"time"
)

// BoundaryType classifies the kind of boundary a file represents.
type BoundaryType string

const (
	BoundaryTypeAPI           BoundaryType = "api"
	BoundaryTypeData          BoundaryType = "data"
	BoundaryTypeInterService  BoundaryType = "inter-service"
	BoundaryTypeCrossLanguage BoundaryType = "cross-language"
)

var validBoundaryTypes = map[BoundaryType]bool{
	BoundaryTypeAPI:           true,
	BoundaryTypeData:          true,
	BoundaryTypeInterService:  true,
	BoundaryTypeCrossLanguage: true,
}

func (bt BoundaryType) Validate() error {
	if !validBoundaryTypes[bt] {
		return errors.New("invalid boundary type: " + string(bt))
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./internal/domain/boundary/ -v -run TestBoundaryTypeValid`
Expected: PASS

- [ ] **Step 5: Write failing tests for BoundaryFile and ProjectBoundaryConfig validation**

```go
// Add to boundary_test.go
func TestBoundaryFileValidate(t *testing.T) {
	tests := []struct {
		name    string
		bf      BoundaryFile
		wantErr bool
	}{
		{"valid", BoundaryFile{Path: "api/schema.proto", Type: BoundaryTypeAPI}, false},
		{"with counterpart", BoundaryFile{Path: "models.py", Type: BoundaryTypeData, Counterpart: "types.ts"}, false},
		{"empty path", BoundaryFile{Path: "", Type: BoundaryTypeAPI}, true},
		{"invalid type", BoundaryFile{Path: "foo.go", Type: BoundaryType("bad")}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.bf.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProjectBoundaryConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ProjectBoundaryConfig
		wantErr bool
	}{
		{"valid", ProjectBoundaryConfig{
			ProjectID: "proj-1",
			Boundaries: []BoundaryFile{{Path: "a.proto", Type: BoundaryTypeAPI}},
		}, false},
		{"empty project id", ProjectBoundaryConfig{ProjectID: ""}, true},
		{"nil boundaries ok", ProjectBoundaryConfig{ProjectID: "proj-1"}, false},
		{"empty tenant id", ProjectBoundaryConfig{ProjectID: "proj-1", TenantID: ""}, false}, // tenant validation at service layer
		{"invalid boundary", ProjectBoundaryConfig{
			ProjectID:  "proj-1",
			Boundaries: []BoundaryFile{{Path: "", Type: BoundaryTypeAPI}},
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
```

- [ ] **Step 6: Write BoundaryFile and ProjectBoundaryConfig implementation**

```go
// Add to boundary.go

// BoundaryFile represents a single boundary file in a project.
type BoundaryFile struct {
	Path         string       `json:"path"`
	Type         BoundaryType `json:"type"`
	Counterpart  string       `json:"counterpart,omitempty"`
	AutoDetected bool         `json:"auto_detected"`
}

func (bf BoundaryFile) Validate() error {
	if bf.Path == "" {
		return errors.New("boundary file path must not be empty")
	}
	return bf.Type.Validate()
}

// ProjectBoundaryConfig stores detected boundary files for a project.
type ProjectBoundaryConfig struct {
	ProjectID    string         `json:"project_id"`
	TenantID     string         `json:"tenant_id"`
	Boundaries   []BoundaryFile `json:"boundaries"`
	LastAnalyzed time.Time      `json:"last_analyzed"`
	Version      int            `json:"version"`
}

func (c ProjectBoundaryConfig) Validate() error {
	if c.ProjectID == "" {
		return errors.New("project_id must not be empty")
	}
	for i, bf := range c.Boundaries {
		if err := bf.Validate(); err != nil {
			return errors.New("boundary[" + strconv.Itoa(i) + "]: " + err.Error())
		}
	}
	return nil
}
```

- [ ] **Step 7: Run all domain tests**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./internal/domain/boundary/ -v`
Expected: PASS (all tests)

- [ ] **Step 8: Commit**

```bash
cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && git add internal/domain/boundary/ && git commit -m "feat(boundary): add domain model with BoundaryType, BoundaryFile, ProjectBoundaryConfig

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Plan Domain — Add waiting_approval Status

**Files:**
- Modify: `internal/domain/plan/plan.go`

- [ ] **Step 1: Write failing test for StepStatusWaitingApproval**

```go
// Add to existing plan_test.go (or create if not exists)
func TestStepStatusWaitingApprovalIsNotTerminal(t *testing.T) {
	if StepStatusWaitingApproval.IsTerminal() {
		t.Error("waiting_approval should not be a terminal status")
	}
}

func TestStepStatusWaitingApprovalValue(t *testing.T) {
	if StepStatusWaitingApproval != "waiting_approval" {
		t.Errorf("expected 'waiting_approval', got %q", StepStatusWaitingApproval)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./internal/domain/plan/ -v -run TestStepStatusWaitingApproval`
Expected: FAIL (undefined: StepStatusWaitingApproval)

- [ ] **Step 3: Add StepStatusWaitingApproval constant to plan.go**

Add after `StepStatusCancelled`:
```go
StepStatusWaitingApproval StepStatus = "waiting_approval"
```

The existing `IsTerminal()` method already returns `false` for unknown values, so `waiting_approval` is automatically non-terminal.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./internal/domain/plan/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && git add internal/domain/plan/ && git commit -m "feat(plan): add StepStatusWaitingApproval for HITL pause

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 3: DB Migrations

**Files:**
- Create: `internal/adapter/postgres/migrations/073_create_project_boundaries.sql`
- Create: `internal/adapter/postgres/migrations/074_create_review_triggers.sql`

- [ ] **Step 1: Create project_boundaries migration**

```sql
-- +goose Up
CREATE TABLE project_boundaries (
    project_id   UUID        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    tenant_id    UUID        NOT NULL,
    boundaries   JSONB       NOT NULL DEFAULT '[]'::jsonb,
    last_analyzed TIMESTAMPTZ,
    version      INT         NOT NULL DEFAULT 1,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id)
);

CREATE INDEX idx_project_boundaries_tenant ON project_boundaries(tenant_id);

-- +goose Down
DROP TABLE IF EXISTS project_boundaries;
```

- [ ] **Step 2: Create review_triggers migration**

```sql
-- +goose Up
CREATE TABLE review_triggers (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id   UUID        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    tenant_id    UUID        NOT NULL,
    commit_sha   TEXT        NOT NULL,
    source       TEXT        NOT NULL CHECK (source IN ('pipeline-completion', 'branch-merge', 'manual')),
    plan_id      UUID,
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_review_triggers_dedup
    ON review_triggers(project_id, commit_sha, triggered_at DESC);
CREATE INDEX idx_review_triggers_tenant
    ON review_triggers(tenant_id);

-- +goose Down
DROP TABLE IF EXISTS review_triggers;
```

- [ ] **Step 3: Verify migrations parse correctly**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && ls internal/adapter/postgres/migrations/07*.sql`
Expected: Lists 073 and 074 files

- [ ] **Step 4: Commit**

```bash
cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && git add internal/adapter/postgres/migrations/073_*.sql internal/adapter/postgres/migrations/074_*.sql && git commit -m "feat(db): add project_boundaries and review_triggers tables

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 4: NATS Subjects (Go + Python)

**Files:**
- Modify: `internal/port/messagequeue/queue.go`
- Modify: `workers/codeforge/consumer/_subjects.py`
- Modify: `internal/port/messagequeue/schemas.go`
- Modify: `workers/codeforge/models.py`

- [ ] **Step 1: Add Go NATS subject constants**

Add to `internal/port/messagequeue/queue.go` in the const block:
```go
// Review/Refactor subjects
SubjectReviewTriggerRequest  = "review.trigger.request"
SubjectReviewTriggerComplete = "review.trigger.complete"
SubjectReviewBoundaryAnalyzed = "review.boundary.analyzed"
SubjectReviewApprovalRequired = "review.approval.required"
SubjectReviewApprovalResponse = "review.approval.response"
```

- [ ] **Step 1b: Add `"review.>"` to Go JetStream stream config**

In `internal/adapter/nats/nats.go`, add `"review.>"` to the `Subjects` slice in the JetStream StreamConfig (alongside `"tasks.>"`, `"benchmark.>"`, etc.). Without this, the NATS stream will reject all `review.*` messages.

- [ ] **Step 2: Add Python NATS subject constants**

Add to `workers/codeforge/consumer/_subjects.py`:

In `STREAM_SUBJECTS` list, add `"review.>"`.

Add constants:
```python
# Review/Refactor subjects
SUBJECT_REVIEW_TRIGGER_REQUEST = "review.trigger.request"
SUBJECT_REVIEW_TRIGGER_COMPLETE = "review.trigger.complete"
SUBJECT_REVIEW_BOUNDARY_ANALYZED = "review.boundary.analyzed"
SUBJECT_REVIEW_APPROVAL_REQUIRED = "review.approval.required"
SUBJECT_REVIEW_APPROVAL_RESPONSE = "review.approval.response"
```

- [ ] **Step 3: Add Go NATS payload structs**

Add to `internal/port/messagequeue/schemas.go`:
```go
type ReviewTriggerRequestPayload struct {
	ProjectID string `json:"project_id"`
	TenantID  string `json:"tenant_id"`
	CommitSHA string `json:"commit_sha"`
	Source    string `json:"source"` // "pipeline-completion" | "branch-merge" | "manual"
}

type ReviewBoundaryAnalyzedPayload struct {
	ProjectID  string `json:"project_id"`
	TenantID   string `json:"tenant_id"`
	Boundaries []struct {
		Path         string `json:"path"`
		Type         string `json:"type"`
		Counterpart  string `json:"counterpart,omitempty"`
		AutoDetected bool   `json:"auto_detected"`
	} `json:"boundaries"`
}

type ReviewApprovalRequiredPayload struct {
	RunID     string `json:"run_id"`
	ProjectID string `json:"project_id"`
	TenantID  string `json:"tenant_id"`
	DiffStats struct {
		FilesChanged  int  `json:"files_changed"`
		LinesAdded    int  `json:"lines_added"`
		LinesRemoved  int  `json:"lines_removed"`
		CrossLayer    bool `json:"cross_layer"`
		Structural    bool `json:"structural"`
	} `json:"diff_stats"`
	ImpactLevel string `json:"impact_level"` // "low" | "medium" | "high"
}

type ReviewApprovalResponsePayload struct {
	RunID    string `json:"run_id"`
	Decision string `json:"decision"` // "approve" | "reject" | "partial"
	Reason   string `json:"reason,omitempty"`
}
```

- [ ] **Step 4: Add Python Pydantic models**

Add to `workers/codeforge/models.py`:
```python
class ReviewTriggerRequestPayload(BaseModel):
    project_id: str
    tenant_id: str
    commit_sha: str
    source: str  # "pipeline-completion" | "branch-merge" | "manual"

class BoundaryEntry(BaseModel):
    path: str
    type: str
    counterpart: str = ""
    auto_detected: bool = True

class ReviewBoundaryAnalyzedPayload(BaseModel):
    project_id: str
    tenant_id: str
    boundaries: list[BoundaryEntry]
```

- [ ] **Step 5: Verify Go compiles**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go build ./internal/port/messagequeue/...`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && git add internal/port/messagequeue/queue.go internal/port/messagequeue/schemas.go workers/codeforge/consumer/_subjects.py workers/codeforge/models.py && git commit -m "feat(nats): add review.* subjects and payload schemas (Go + Python)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 5: Mode Presets — boundary-analyzer + contract-reviewer

**Files:**
- Modify: `internal/domain/mode/presets.go`

- [ ] **Step 1: Write failing test for new modes**

```go
// Add to existing mode test file or create internal/domain/mode/presets_test.go
func TestBuiltinModesContainReviewRefactorModes(t *testing.T) {
	modes := BuiltinModes()
	found := map[string]bool{"boundary-analyzer": false, "contract-reviewer": false}
	for _, m := range modes {
		if _, ok := found[m.ID]; ok {
			found[m.ID] = true
		}
	}
	for id, ok := range found {
		if !ok {
			t.Errorf("mode %q not found in BuiltinModes()", id)
		}
	}
}

func TestBoundaryAnalyzerModeProperties(t *testing.T) {
	modes := BuiltinModes()
	var ba Mode
	for _, m := range modes {
		if m.ID == "boundary-analyzer" {
			ba = m
			break
		}
	}
	if ba.ID == "" {
		t.Fatal("boundary-analyzer mode not found")
	}
	if ba.LLMScenario != "plan" {
		t.Errorf("LLMScenario = %q, want 'plan'", ba.LLMScenario)
	}
	if ba.Autonomy != 4 {
		t.Errorf("Autonomy = %d, want 4", ba.Autonomy)
	}
	if ba.RequiredArtifact != "BOUNDARIES.json" {
		t.Errorf("RequiredArtifact = %q, want 'BOUNDARIES.json'", ba.RequiredArtifact)
	}
	// Must be read-only: no Write, Edit, Bash
	for _, tool := range ba.DeniedTools {
		// DeniedTools should contain Write, Edit, Bash
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./internal/domain/mode/ -v -run TestBuiltinModesContainReviewRefactorModes`
Expected: FAIL

- [ ] **Step 3: Add boundary-analyzer and contract-reviewer modes to presets.go**

Add to the `BuiltinModes()` slice in `presets.go`:

```go
{
	ID:               "boundary-analyzer",
	Name:             "Boundary Analyzer",
	Description:      "Analyzes project structure to identify boundary files between modules, services, and language layers.",
	Builtin:          true,
	Tools:            []string{"Read", "Glob", "Grep", "ListDir"},
	DeniedTools:      []string{"Write", "Edit", "Bash"},
	RequiredArtifact: "BOUNDARIES.json",
	LLMScenario:      "plan",
	Autonomy:         4,
	PromptPrefix: commonRules + `# Boundary Analyzer Mode

You analyze project structure to identify boundary files — files that define contracts between modules, services, or language layers.

## What to look for:
- API schemas: OpenAPI/Swagger specs, Protobuf (.proto), GraphQL schemas, JSON Schema
- Data layer: ORM models, DB migration files, shared type definitions
- Inter-service: Message queue schemas, event definitions, RPC/gRPC interfaces
- Cross-language: Shared JSON contracts, TypeScript type files matching backend models

## Output format:
Return a BOUNDARIES.json file with this structure:
` + "```json\n" + `[
  {"path": "relative/path/to/file", "type": "api|data|inter-service|cross-language", "counterpart": "optional/counterpart/path", "auto_detected": true}
]
` + "```\n",
},
{
	ID:               "contract-reviewer",
	Name:             "Contract Reviewer",
	Description:      "Reviews cross-layer contracts for consistency between boundary files.",
	Builtin:          true,
	Tools:            []string{"Read", "Glob", "Grep"},
	DeniedTools:      []string{"Write", "Edit", "Bash"},
	RequiredArtifact: "CONTRACT_REVIEW.md",
	LLMScenario:      "review",
	Autonomy:         2,
	PromptPrefix: commonRules + `# Contract Reviewer Mode

You review cross-layer contracts for consistency. You receive a list of boundary files and their counterparts.

## Your job:
1. Read each boundary file and its counterpart
2. Check for inconsistencies: mismatched field names, type drift, missing fields, deprecated fields still in use
3. Report findings as a structured review

## Output format:
Create CONTRACT_REVIEW.md with:
- PASS or FAIL overall status
- Per-boundary-pair findings with severity (critical/warning/info)
- Specific line references where drift exists
`,
},
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./internal/domain/mode/ -v -run TestBuiltinModesContain`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && git add internal/domain/mode/ && git commit -m "feat(modes): add boundary-analyzer and contract-reviewer mode presets

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 6: Pipeline Template — review-refactor

**Files:**
- Modify: `internal/domain/pipeline/presets.go`

- [ ] **Step 1: Write failing test**

```go
// Add to pipeline presets test or create presets_test.go
func TestBuiltinTemplatesContainReviewRefactor(t *testing.T) {
	templates := BuiltinTemplates()
	var found bool
	for _, tmpl := range templates {
		if tmpl.ID == "review-refactor" {
			found = true
			if tmpl.Protocol != plan.ProtocolSequential {
				t.Errorf("protocol = %q, want sequential", tmpl.Protocol)
			}
			if len(tmpl.Steps) != 4 {
				t.Errorf("steps = %d, want 4", len(tmpl.Steps))
			}
			if tmpl.Steps[0].ModeID != "boundary-analyzer" {
				t.Errorf("step[0].ModeID = %q, want boundary-analyzer", tmpl.Steps[0].ModeID)
			}
			if tmpl.Steps[1].ModeID != "contract-reviewer" {
				t.Errorf("step[1].ModeID = %q, want contract-reviewer", tmpl.Steps[1].ModeID)
			}
			if tmpl.Steps[2].ModeID != "reviewer" {
				t.Errorf("step[2].ModeID = %q, want reviewer", tmpl.Steps[2].ModeID)
			}
			if tmpl.Steps[3].ModeID != "refactorer" {
				t.Errorf("step[3].ModeID = %q, want refactorer", tmpl.Steps[3].ModeID)
			}
		}
	}
	if !found {
		t.Error("review-refactor template not found in BuiltinTemplates()")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./internal/domain/pipeline/ -v -run TestBuiltinTemplatesContainReviewRefactor`
Expected: FAIL

- [ ] **Step 3: Add reviewRefactor() function and register in BuiltinTemplates()**

Add to `presets.go`:
```go
func reviewRefactor() Template {
	return Template{
		ID:          "review-refactor",
		Name:        "Contract-First Review & Refactor",
		Description: "Four-step pipeline: analyze boundaries, review contracts, review code, propose refactorings.",
		Builtin:     true,
		Protocol:    plan.ProtocolSequential,
		Steps: []Step{
			{Name: "Boundary Analysis", ModeID: "boundary-analyzer", DeliverMode: "append"},
			{Name: "Contract Review", ModeID: "contract-reviewer", DeliverMode: "append", DependsOn: []int{0}},
			{Name: "Code Review", ModeID: "reviewer", DeliverMode: "append", DependsOn: []int{1}},
			{Name: "Refactoring Proposals", ModeID: "refactorer", DeliverMode: "diff", DependsOn: []int{2}},
		},
	}
}
```

Add `reviewRefactor()` to the `BuiltinTemplates()` slice.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./internal/domain/pipeline/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && git add internal/domain/pipeline/ && git commit -m "feat(pipeline): add review-refactor template (4-step contract-first)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Chunk 2: Service Layer

### Task 7: DiffImpactScorer

**Files:**
- Create: `internal/service/diff_impact.go`
- Create: `internal/service/diff_impact_test.go`

- [ ] **Step 1: Write failing tests for all three impact tiers**

```go
// internal/service/diff_impact_test.go
package service

import "testing"

func TestDiffImpactScorer_LowImpact(t *testing.T) {
	scorer := NewDiffImpactScorer(DiffImpactConfig{
		AutoApplyThreshold:      50,
		ApprovalThreshold:       200,
		AlwaysApproveBoundary:   true,
		AlwaysApproveStructural: true,
	})
	stats := DiffStats{
		FilesChanged: 2,
		LinesAdded:   20,
		LinesRemoved: 5,
		CrossLayer:   false,
		Structural:   false,
	}
	level := scorer.Score(stats)
	if level != ImpactLow {
		t.Errorf("Score() = %q, want %q", level, ImpactLow)
	}
}

func TestDiffImpactScorer_MediumImpact(t *testing.T) {
	scorer := NewDiffImpactScorer(DiffImpactConfig{
		AutoApplyThreshold:      50,
		ApprovalThreshold:       200,
		AlwaysApproveBoundary:   true,
		AlwaysApproveStructural: true,
	})
	stats := DiffStats{
		FilesChanged: 5,
		LinesAdded:   60,
		LinesRemoved: 20,
		CrossLayer:   false,
		Structural:   false,
	}
	level := scorer.Score(stats)
	if level != ImpactMedium {
		t.Errorf("Score() = %q, want %q", level, ImpactMedium)
	}
}

func TestDiffImpactScorer_HighImpact(t *testing.T) {
	scorer := NewDiffImpactScorer(DiffImpactConfig{
		AutoApplyThreshold:      50,
		ApprovalThreshold:       200,
		AlwaysApproveBoundary:   true,
		AlwaysApproveStructural: true,
	})
	stats := DiffStats{
		FilesChanged: 10,
		LinesAdded:   150,
		LinesRemoved: 80,
		CrossLayer:   false,
		Structural:   false,
	}
	level := scorer.Score(stats)
	if level != ImpactHigh {
		t.Errorf("Score() = %q, want %q", level, ImpactHigh)
	}
}

func TestDiffImpactScorer_CrossLayerAlwaysHigh(t *testing.T) {
	scorer := NewDiffImpactScorer(DiffImpactConfig{
		AutoApplyThreshold:    50,
		ApprovalThreshold:     200,
		AlwaysApproveBoundary: true,
	})
	stats := DiffStats{
		FilesChanged: 1,
		LinesAdded:   5,
		LinesRemoved: 2,
		CrossLayer:   true,
	}
	level := scorer.Score(stats)
	if level != ImpactHigh {
		t.Errorf("Score() = %q, want %q for cross-layer change", level, ImpactHigh)
	}
}

func TestDiffImpactScorer_StructuralAlwaysHigh(t *testing.T) {
	scorer := NewDiffImpactScorer(DiffImpactConfig{
		AutoApplyThreshold:      50,
		ApprovalThreshold:       200,
		AlwaysApproveStructural: true,
	})
	stats := DiffStats{
		FilesChanged: 1,
		LinesAdded:   3,
		Structural:   true,
	}
	level := scorer.Score(stats)
	if level != ImpactHigh {
		t.Errorf("Score() = %q, want %q for structural change", level, ImpactHigh)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./internal/service/ -v -run TestDiffImpactScorer`
Expected: FAIL (undefined types)

- [ ] **Step 3: Implement DiffImpactScorer**

```go
// internal/service/diff_impact.go
package service

// ImpactLevel represents the severity of a diff's impact.
type ImpactLevel string

const (
	ImpactLow    ImpactLevel = "low"
	ImpactMedium ImpactLevel = "medium"
	ImpactHigh   ImpactLevel = "high"
)

// DiffStats contains metrics about a proposed diff.
type DiffStats struct {
	FilesChanged int  `json:"files_changed"`
	LinesAdded   int  `json:"lines_added"`
	LinesRemoved int  `json:"lines_removed"`
	CrossLayer   bool `json:"cross_layer"`
	Structural   bool `json:"structural"`
}

// TotalLines returns the total number of changed lines.
func (d DiffStats) TotalLines() int {
	return d.LinesAdded + d.LinesRemoved
}

// DiffImpactConfig holds configurable thresholds.
type DiffImpactConfig struct {
	AutoApplyThreshold      int  `json:"auto_apply_threshold" yaml:"auto_apply_threshold"`
	ApprovalThreshold       int  `json:"approval_threshold" yaml:"approval_threshold"`
	AlwaysApproveBoundary   bool `json:"always_approve_boundary" yaml:"always_approve_boundary"`
	AlwaysApproveStructural bool `json:"always_approve_structural" yaml:"always_approve_structural"`
}

// DiffImpactScorer evaluates the impact of a proposed diff.
type DiffImpactScorer struct {
	cfg DiffImpactConfig
}

// NewDiffImpactScorer creates a scorer with the given config.
func NewDiffImpactScorer(cfg DiffImpactConfig) *DiffImpactScorer {
	return &DiffImpactScorer{cfg: cfg}
}

// Score evaluates the diff and returns the impact level.
func (s *DiffImpactScorer) Score(stats DiffStats) ImpactLevel {
	if s.cfg.AlwaysApproveBoundary && stats.CrossLayer {
		return ImpactHigh
	}
	if s.cfg.AlwaysApproveStructural && stats.Structural {
		return ImpactHigh
	}
	total := stats.TotalLines()
	if total >= s.cfg.ApprovalThreshold {
		return ImpactHigh
	}
	if total >= s.cfg.AutoApplyThreshold {
		return ImpactMedium
	}
	return ImpactLow
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./internal/service/ -v -run TestDiffImpactScorer`
Expected: PASS (all 5 tests)

- [ ] **Step 5: Commit**

```bash
cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && git add internal/service/diff_impact.go internal/service/diff_impact_test.go && git commit -m "feat(service): add DiffImpactScorer with three-tier threshold evaluation

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 8: Phase-aware Context Budget

**Files:**
- Modify: `internal/service/context_budget.go`

- [ ] **Step 1: Write failing test**

```go
// Add to context_budget_test.go
func TestPhaseAwareContextBudget(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		base     int
		wantPct  int // approximate percentage of base
	}{
		{"boundary-analyzer gets full", "boundary-analyzer", 2000, 100},
		{"contract-reviewer gets 60%", "contract-reviewer", 2000, 60},
		{"reviewer gets 50%", "reviewer", 2000, 50},
		{"refactorer gets 70%", "refactorer", 2000, 70},
		{"unknown gets full", "coder", 2000, 100},
		{"zero base returns zero", "reviewer", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PhaseAwareContextBudget(tt.base, tt.mode)
			want := tt.base * tt.wantPct / 100
			if got != want {
				t.Errorf("PhaseAwareContextBudget(%d, %q) = %d, want %d", tt.base, tt.mode, got, want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./internal/service/ -v -run TestPhaseAwareContextBudget`
Expected: FAIL (undefined: PhaseAwareContextBudget)

- [ ] **Step 3: Implement PhaseAwareContextBudget**

Add to `internal/service/context_budget.go`:
```go
// phaseContextScale maps mode IDs to context budget scaling factors (percent).
var phaseContextScale = map[string]int{
	"boundary-analyzer": 100,
	"contract-reviewer": 60,
	"reviewer":          50,
	"refactorer":        70,
}

// PhaseAwareContextBudget scales the base context budget based on the
// pipeline phase (identified by mode ID). Review phases need less context
// since they work with focused diffs; boundary analysis needs full context.
func PhaseAwareContextBudget(baseBudget int, modeID string) int {
	if baseBudget <= 0 {
		return 0
	}
	pct, ok := phaseContextScale[modeID]
	if !ok {
		return baseBudget
	}
	return baseBudget * pct / 100
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./internal/service/ -v -run TestPhaseAwareContextBudget`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && git add internal/service/context_budget.go internal/service/context_budget_test.go && git commit -m "feat(service): add PhaseAwareContextBudget for review pipeline phases

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 9: Store Interface + PostgreSQL Implementation for Boundaries

**Files:**
- Modify: `internal/port/database/store.go` (add methods to interface)
- Create: `internal/adapter/postgres/store_boundary.go`

- [ ] **Step 0: Write interface-level test to verify store contract compiles**

Create a compile-time interface check (store integration tests are covered in Task 15):
```go
// internal/adapter/postgres/store_boundary_test.go
package postgres

// Compile-time check that Store satisfies boundary store methods
var _ interface {
	GetProjectBoundaries(ctx context.Context, projectID string) (*boundary.ProjectBoundaryConfig, error)
	UpsertProjectBoundaries(ctx context.Context, cfg *boundary.ProjectBoundaryConfig) error
	DeleteProjectBoundaries(ctx context.Context, projectID string) error
} = (*Store)(nil)
```

- [ ] **Step 1: Add boundary methods to Store interface**

Add to `internal/port/database/store.go` interface:
```go
// Boundaries
GetProjectBoundaries(ctx context.Context, projectID string) (*boundary.ProjectBoundaryConfig, error)
UpsertProjectBoundaries(ctx context.Context, cfg *boundary.ProjectBoundaryConfig) error
DeleteProjectBoundaries(ctx context.Context, projectID string) error

// Review Triggers
CreateReviewTrigger(ctx context.Context, projectID, commitSHA, source string) (string, error)
FindRecentReviewTrigger(ctx context.Context, projectID, commitSHA string, within time.Duration) (bool, error)
```

- [ ] **Step 2: Implement PostgreSQL store for boundaries**

```go
// internal/adapter/postgres/store_boundary.go
package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/boundary"
	"github.com/Strob0t/CodeForge/internal/middleware/tenantctx"
)

func (s *Store) GetProjectBoundaries(ctx context.Context, projectID string) (*boundary.ProjectBoundaryConfig, error) {
	tid := tenantctx.FromCtx(ctx)
	var cfg boundary.ProjectBoundaryConfig
	var boundariesJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT project_id, tenant_id, boundaries, last_analyzed, version
		 FROM project_boundaries
		 WHERE project_id = $1 AND tenant_id = $2`,
		projectID, tid,
	).Scan(&cfg.ProjectID, &cfg.TenantID, &boundariesJSON, &cfg.LastAnalyzed, &cfg.Version)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(boundariesJSON, &cfg.Boundaries); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (s *Store) UpsertProjectBoundaries(ctx context.Context, cfg *boundary.ProjectBoundaryConfig) error {
	tid := tenantctx.FromCtx(ctx)
	boundariesJSON, err := json.Marshal(cfg.Boundaries)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO project_boundaries (project_id, tenant_id, boundaries, last_analyzed, version)
		 VALUES ($1, $2, $3, $4, 1)
		 ON CONFLICT (project_id)
		 DO UPDATE SET boundaries = $3, last_analyzed = $4, version = project_boundaries.version + 1, updated_at = now()
		 WHERE project_boundaries.tenant_id = $2`,
		cfg.ProjectID, tid, boundariesJSON, time.Now(),
	)
	return err
}

func (s *Store) DeleteProjectBoundaries(ctx context.Context, projectID string) error {
	tid := tenantctx.FromCtx(ctx)
	_, err := s.pool.Exec(ctx,
		`DELETE FROM project_boundaries WHERE project_id = $1 AND tenant_id = $2`,
		projectID, tid,
	)
	return err
}
```

- [ ] **Step 3: Implement store for review triggers**

```go
// internal/adapter/postgres/store_review_trigger.go
package postgres

import (
	"context"
	"time"

	"github.com/Strob0t/CodeForge/internal/middleware/tenantctx"
)

func (s *Store) CreateReviewTrigger(ctx context.Context, projectID, commitSHA, source string) (string, error) {
	tid := tenantctx.FromCtx(ctx)
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO review_triggers (project_id, tenant_id, commit_sha, source)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id`,
		projectID, tid, commitSHA, source,
	).Scan(&id)
	return id, err
}

func (s *Store) FindRecentReviewTrigger(ctx context.Context, projectID, commitSHA string, within time.Duration) (bool, error) {
	tid := tenantctx.FromCtx(ctx)
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM review_triggers
			WHERE project_id = $1 AND tenant_id = $2 AND commit_sha = $3
			AND triggered_at > $4
		)`,
		projectID, tid, commitSHA, time.Now().Add(-within),
	).Scan(&exists)
	return exists, err
}
```

- [ ] **Step 4: Verify Go compiles**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go build ./...`
Expected: No errors (may need to add stub implementations in test mocks)

- [ ] **Step 5: Commit**

```bash
cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && git add internal/port/database/store.go internal/adapter/postgres/store_boundary.go internal/adapter/postgres/store_review_trigger.go && git commit -m "feat(store): add boundary and review trigger PostgreSQL store implementations

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 9b: BoundaryService

**Files:**
- Create: `internal/service/boundary.go`
- Create: `internal/service/boundary_test.go`

- [ ] **Step 1: Write failing tests for BoundaryService CRUD**

```go
// internal/service/boundary_test.go
package service

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/boundary"
)

type mockBoundaryStore struct {
	cfg *boundary.ProjectBoundaryConfig
}

func (m *mockBoundaryStore) GetProjectBoundaries(_ context.Context, _ string) (*boundary.ProjectBoundaryConfig, error) {
	if m.cfg == nil {
		return nil, fmt.Errorf("not found")
	}
	return m.cfg, nil
}

func (m *mockBoundaryStore) UpsertProjectBoundaries(_ context.Context, cfg *boundary.ProjectBoundaryConfig) error {
	m.cfg = cfg
	return nil
}

func (m *mockBoundaryStore) DeleteProjectBoundaries(_ context.Context, _ string) error {
	m.cfg = nil
	return nil
}

func TestBoundaryService_GetBoundaries(t *testing.T) {
	store := &mockBoundaryStore{cfg: &boundary.ProjectBoundaryConfig{
		ProjectID: "proj-1",
		Boundaries: []boundary.BoundaryFile{{Path: "a.proto", Type: boundary.BoundaryTypeAPI}},
	}}
	svc := NewBoundaryService(store)
	cfg, err := svc.GetBoundaries(context.Background(), "proj-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Boundaries) != 1 {
		t.Errorf("expected 1 boundary, got %d", len(cfg.Boundaries))
	}
}

func TestBoundaryService_UpdateBoundariesValidates(t *testing.T) {
	store := &mockBoundaryStore{}
	svc := NewBoundaryService(store)
	err := svc.UpdateBoundaries(context.Background(), &boundary.ProjectBoundaryConfig{
		ProjectID: "",
	})
	if err == nil {
		t.Error("expected validation error for empty project ID")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./internal/service/ -v -run TestBoundaryService`
Expected: FAIL (undefined)

- [ ] **Step 3: Implement BoundaryService**

```go
// internal/service/boundary.go
package service

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/boundary"
)

// BoundaryStore is the subset of the store needed by BoundaryService.
type BoundaryStore interface {
	GetProjectBoundaries(ctx context.Context, projectID string) (*boundary.ProjectBoundaryConfig, error)
	UpsertProjectBoundaries(ctx context.Context, cfg *boundary.ProjectBoundaryConfig) error
	DeleteProjectBoundaries(ctx context.Context, projectID string) error
}

// BoundaryService manages project boundary configurations.
type BoundaryService struct {
	store BoundaryStore
}

// NewBoundaryService creates a new BoundaryService.
func NewBoundaryService(store BoundaryStore) *BoundaryService {
	return &BoundaryService{store: store}
}

// GetBoundaries returns the boundary config for a project.
func (s *BoundaryService) GetBoundaries(ctx context.Context, projectID string) (*boundary.ProjectBoundaryConfig, error) {
	return s.store.GetProjectBoundaries(ctx, projectID)
}

// UpdateBoundaries validates and persists a boundary config.
func (s *BoundaryService) UpdateBoundaries(ctx context.Context, cfg *boundary.ProjectBoundaryConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	return s.store.UpsertProjectBoundaries(ctx, cfg)
}

// DeleteBoundaries removes the boundary config for a project.
func (s *BoundaryService) DeleteBoundaries(ctx context.Context, projectID string) error {
	return s.store.DeleteProjectBoundaries(ctx, projectID)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./internal/service/ -v -run TestBoundaryService`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && git add internal/service/boundary.go internal/service/boundary_test.go && git commit -m "feat(service): add BoundaryService with CRUD and validation

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 10: ReviewTriggerService

**NOTE:** The codebase has an existing `ReviewService` in `internal/service/review.go` with `ReviewPolicy` and `TriggerType`. The new `ReviewTriggerService` is intentionally separate: it handles the cascade trigger logic and SHA-based deduplication for the review-refactor pipeline specifically. The existing `ReviewService` handles per-project review policy configuration. They coexist — `ReviewTriggerService` could consult `ReviewService` for policy checks in a future iteration.

**Files:**
- Create: `internal/service/review_trigger.go`
- Create: `internal/service/review_trigger_test.go`

- [ ] **Step 1: Write failing tests for dedup logic**

```go
// internal/service/review_trigger_test.go
package service

import (
	"context"
	"testing"
	"time"
)

type mockReviewTriggerStore struct {
	recentExists bool
	createdIDs   []string
}

func (m *mockReviewTriggerStore) FindRecentReviewTrigger(_ context.Context, _, _ string, _ time.Duration) (bool, error) {
	return m.recentExists, nil
}

func (m *mockReviewTriggerStore) CreateReviewTrigger(_ context.Context, _, _, _ string) (string, error) {
	id := "trigger-1"
	m.createdIDs = append(m.createdIDs, id)
	return id, nil
}

func TestReviewTriggerService_DedupSkipsRecentSHA(t *testing.T) {
	store := &mockReviewTriggerStore{recentExists: true}
	svc := NewReviewTriggerService(store, nil, 30*time.Minute)

	triggered, err := svc.TriggerReview(context.Background(), "proj-1", "abc123", "pipeline-completion")
	if err != nil {
		t.Fatal(err)
	}
	if triggered {
		t.Error("expected dedup to skip, but review was triggered")
	}
}

func TestReviewTriggerService_ManualBypassesDedup(t *testing.T) {
	store := &mockReviewTriggerStore{recentExists: true}
	svc := NewReviewTriggerService(store, nil, 30*time.Minute)

	triggered, err := svc.TriggerReview(context.Background(), "proj-1", "abc123", "manual")
	if err != nil {
		t.Fatal(err)
	}
	if !triggered {
		t.Error("manual trigger should bypass dedup")
	}
}

func TestReviewTriggerService_NewSHATriggersReview(t *testing.T) {
	store := &mockReviewTriggerStore{recentExists: false}
	svc := NewReviewTriggerService(store, nil, 30*time.Minute)

	triggered, err := svc.TriggerReview(context.Background(), "proj-1", "newsha", "branch-merge")
	if err != nil {
		t.Fatal(err)
	}
	if !triggered {
		t.Error("new SHA should trigger review")
	}
	if len(store.createdIDs) != 1 {
		t.Errorf("expected 1 trigger created, got %d", len(store.createdIDs))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./internal/service/ -v -run TestReviewTriggerService`
Expected: FAIL

- [ ] **Step 3: Implement ReviewTriggerService**

```go
// internal/service/review_trigger.go
package service

import (
	"context"
	"time"
)

// ReviewTriggerStore is the subset of the store needed by ReviewTriggerService.
type ReviewTriggerStore interface {
	FindRecentReviewTrigger(ctx context.Context, projectID, commitSHA string, within time.Duration) (bool, error)
	CreateReviewTrigger(ctx context.Context, projectID, commitSHA, source string) (string, error)
}

// ReviewTriggerOrchestrator creates and starts review-refactor plans.
type ReviewTriggerOrchestrator interface {
	StartReviewPipeline(ctx context.Context, projectID string) error
}

// ReviewTriggerService manages cascade triggers with deduplication.
type ReviewTriggerService struct {
	store        ReviewTriggerStore
	orchestrator ReviewTriggerOrchestrator
	dedupWindow  time.Duration
}

// NewReviewTriggerService creates a new ReviewTriggerService.
func NewReviewTriggerService(store ReviewTriggerStore, orch ReviewTriggerOrchestrator, dedupWindow time.Duration) *ReviewTriggerService {
	return &ReviewTriggerService{
		store:        store,
		orchestrator: orch,
		dedupWindow:  dedupWindow,
	}
}

// TriggerReview attempts to start a review-refactor pipeline.
// Returns true if a review was triggered, false if deduplicated.
func (s *ReviewTriggerService) TriggerReview(ctx context.Context, projectID, commitSHA, source string) (bool, error) {
	// Manual triggers bypass dedup
	if source != "manual" {
		exists, err := s.store.FindRecentReviewTrigger(ctx, projectID, commitSHA, s.dedupWindow)
		if err != nil {
			return false, err
		}
		if exists {
			return false, nil
		}
	}

	if _, err := s.store.CreateReviewTrigger(ctx, projectID, commitSHA, source); err != nil {
		return false, err
	}

	if s.orchestrator != nil {
		if err := s.orchestrator.StartReviewPipeline(ctx, projectID); err != nil {
			return true, err
		}
	}

	return true, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./internal/service/ -v -run TestReviewTriggerService`
Expected: PASS (all 3 tests)

- [ ] **Step 5: Commit**

```bash
cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && git add internal/service/review_trigger.go internal/service/review_trigger_test.go && git commit -m "feat(service): add ReviewTriggerService with cascade dedup logic

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 11: Orchestrator — Handle waiting_approval

**Files:**
- Modify: `internal/service/orchestrator.go`

- [ ] **Step 1: Write failing test for waiting_approval blocking advancePlan**

The test should verify that when a step is in `waiting_approval` status, the orchestrator does NOT advance to the next step. Read the existing orchestrator test patterns first, then add a test.

- [ ] **Step 2: Modify protocol-specific advance functions to handle waiting_approval**

The check must go into `advanceSequential()` (and other protocol handlers like `advanceParallel()`) — NOT just the top-level `advancePlan()`. This is because `waiting_approval` is neither terminal nor running, so `RunningCount()` returns 0 and `ReadySteps()` skips it. Without this check, the sequential advance would loop trying to find a next step.

In `advanceSequential()`, before the `ReadySteps` call:
```go
// If any step is waiting for approval, do not advance
for _, step := range p.Steps {
    if step.Status == plan.StepStatusWaitingApproval {
        return nil // blocked, waiting for user decision
    }
}
```

Apply the same check in `advanceParallel()` if it exists.

- [ ] **Step 3: Add ApproveStep and RejectStep methods to OrchestratorService**

```go
func (s *OrchestratorService) ApproveStep(ctx context.Context, planID, stepID string) error {
    // Update step status from waiting_approval -> completed
    // Resume advancePlan()
}

func (s *OrchestratorService) RejectStep(ctx context.Context, planID, stepID string) error {
    // Update step status from waiting_approval -> failed
    // Cancel remaining steps
}
```

- [ ] **Step 4: Run orchestrator tests**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./internal/service/ -v -run TestOrchestrator`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && git add internal/service/orchestrator.go internal/service/orchestrator_test.go && git commit -m "feat(orchestrator): handle waiting_approval status with approve/reject

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Chunk 3: HTTP Layer + Frontend

### Task 12: HTTP Handlers for Boundaries and Review Triggers

**Files:**
- Create: `internal/adapter/http/handlers_review.go`
- Modify: `internal/adapter/http/routes.go`

- [ ] **Step 1: Implement boundary CRUD handlers**

```go
// internal/adapter/http/handlers_review.go
// GET /api/v1/projects/{id}/boundaries
// PUT /api/v1/projects/{id}/boundaries
// POST /api/v1/projects/{id}/boundaries/analyze
// POST /api/v1/projects/{id}/review-refactor (manual trigger)
// POST /api/v1/runs/{id}/approve
// POST /api/v1/runs/{id}/reject
// POST /api/v1/runs/{id}/approve-partial
```

Follow the existing handler pattern from other handlers in the codebase:
- Parse path params with `chi.URLParam(r, "id")`
- Read JSON body with `json.NewDecoder(r.Body).Decode(&req)`
- Write JSON response with `writeJSON(w, status, response)`
- Use `tenantctx.FromCtx(ctx)` for tenant isolation

- [ ] **Step 2: Register routes in routes.go**

Add to the `r.Route("/api/v1", ...)` block:
```go
// Boundaries
r.Route("/projects/{id}/boundaries", func(r chi.Router) {
    r.Get("/", h.GetProjectBoundaries)
    r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).
        Put("/", h.UpdateProjectBoundaries)
    r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).
        Post("/analyze", h.TriggerBoundaryAnalysis)
})

// Review/Refactor
r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).
    Post("/projects/{id}/review-refactor", h.TriggerReviewRefactor)

// Approval
r.Route("/runs/{id}", func(r chi.Router) {
    r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).
        Post("/approve", h.ApproveRun)
    r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).
        Post("/reject", h.RejectRun)
    r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).
        Post("/approve-partial", h.ApproveRunPartial)
})
```

- [ ] **Step 3: Write handler tests using httptest**

Create `internal/adapter/http/handlers_review_test.go` with tests using `httptest.NewRecorder()` and mock services. At minimum test:
- `GET /api/v1/projects/{id}/boundaries` returns 200 with valid JSON
- `PUT /api/v1/projects/{id}/boundaries` returns 400 on invalid body
- `POST /api/v1/runs/{id}/approve` returns 200 on valid run
- `POST /api/v1/runs/{id}/reject` returns 200 on valid run

Follow existing handler test patterns in the codebase.

- [ ] **Step 4: Run handler tests**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./internal/adapter/http/ -v -run TestHandlersReview`
Expected: PASS

- [ ] **Step 5: Verify Go compiles**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go build ./internal/adapter/http/...`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && git add internal/adapter/http/ && git commit -m "feat(http): add boundary CRUD + review trigger + approval endpoints

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 13: Python NATS Consumer for Review Events

**Files:**
- Create: `workers/codeforge/consumer/_review.py`
- Modify: `workers/codeforge/consumer/__init__.py`

- [ ] **Step 1: Implement ReviewHandlerMixin**

```python
# workers/codeforge/consumer/_review.py
from __future__ import annotations

import json
import logging

from codeforge.consumer._subjects import SUBJECT_REVIEW_TRIGGER_REQUEST
from codeforge.models import ReviewTriggerRequestPayload

logger = logging.getLogger(__name__)


class ReviewHandlerMixin:
    """Handles review.trigger.request NATS messages."""

    async def _handle_review_trigger(self, msg) -> None:
        try:
            payload = ReviewTriggerRequestPayload.model_validate_json(msg.data)
            logger.info(
                "review trigger received",
                extra={
                    "project_id": payload.project_id,
                    "commit_sha": payload.commit_sha,
                    "source": payload.source,
                },
            )
            # TODO: Dispatch boundary-analyzer run via agent loop
            await msg.ack()
        except Exception as exc:
            logger.error("review trigger failed", extra={"error": str(exc)})
            await msg.ack()  # ack to prevent infinite redelivery
```

- [ ] **Step 2: Register in TaskConsumer**

Add `ReviewHandlerMixin` to the `TaskConsumer` base classes in `__init__.py`.
Add subscription tuple: `(SUBJECT_REVIEW_TRIGGER_REQUEST, self._handle_review_trigger)`.

- [ ] **Step 3: Write Python test**

```python
# workers/tests/test_review_consumer.py
import pytest
from codeforge.consumer._review import ReviewHandlerMixin
from codeforge.models import ReviewTriggerRequestPayload


def test_review_trigger_payload_parsing():
    raw = '{"project_id": "p1", "tenant_id": "t1", "commit_sha": "abc", "source": "manual"}'
    payload = ReviewTriggerRequestPayload.model_validate_json(raw)
    assert payload.project_id == "p1"
    assert payload.source == "manual"


def test_review_trigger_payload_rejects_invalid():
    with pytest.raises(Exception):
        ReviewTriggerRequestPayload.model_validate_json('{"bad": "data"}')
```

- [ ] **Step 4: Run Python tests**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && cd workers && python -m pytest tests/test_review_consumer.py -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && git add workers/codeforge/consumer/_review.py workers/codeforge/consumer/__init__.py workers/tests/test_review_consumer.py workers/codeforge/models.py && git commit -m "feat(worker): add review trigger NATS consumer handler

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 14: Frontend — RefactorApproval Component

**Files:**
- Create: `frontend/src/features/project/RefactorApproval.tsx`
- Create: `frontend/src/features/project/BoundariesPanel.tsx`
- Modify: `frontend/src/features/project/ProjectView.tsx`

- [ ] **Step 1: Create RefactorApproval component**

SolidJS component that:
- Listens for `refactor.approval_required` WebSocket events
- Shows a modal/panel with diff stats (files changed, lines, cross-layer flag)
- Provides Approve / Reject buttons
- Sends POST to `/api/v1/runs/{id}/approve` or `/reject`

Follow existing component patterns: SolidJS signals, Tailwind CSS, native fetch API.

- [ ] **Step 2: Create BoundariesPanel component**

SolidJS component that:
- Fetches `GET /api/v1/projects/{id}/boundaries`
- Displays list of boundary files with type badges
- Allows editing (add/remove boundaries)
- Trigger re-analysis button

- [ ] **Step 3: Integrate into ProjectView**

Add a "Boundaries" tab or section to the project view that renders `BoundariesPanel`.
Add the `RefactorApproval` component as a global overlay that activates on WS events.

- [ ] **Step 4: Verify frontend builds**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor/frontend && npm run build`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && git add frontend/src/features/project/RefactorApproval.tsx frontend/src/features/project/BoundariesPanel.tsx frontend/src/features/project/ProjectView.tsx && git commit -m "feat(frontend): add RefactorApproval HITL UI and BoundariesPanel

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Chunk 4: Integration + Documentation

### Task 15: Integration — Wire Services Together

**Files:**
- Modify: `cmd/codeforge/main.go` or service initialization
- Modify: `internal/adapter/http/handlers.go` (autoIndexProject)

- [ ] **Step 1: Wire ReviewTriggerService in service initialization**

Instantiate `ReviewTriggerService` with store and orchestrator dependencies. Register in the dependency injection / handler struct.

- [ ] **Step 2: Add boundary analysis trigger to autoIndexProject()**

After the existing RepoMap + Retrieval Index triggers in `autoIndexProject()`, add a NATS publish for `review.trigger.request` with source `"manual"` to kick off initial boundary analysis.

- [ ] **Step 3: Add pipeline-completion trigger**

In `OrchestratorService.HandleRunCompleted()`, after checking plan completion, check if the project has review auto-trigger enabled and call `ReviewTriggerService.TriggerReview()`.

- [ ] **Step 4: Run full Go test suite**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./... -count=1`
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && git add cmd/codeforge/ internal/adapter/http/handlers.go internal/service/orchestrator.go && git commit -m "feat(integration): wire ReviewTriggerService and autoIndex boundary analysis

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 16: Documentation Updates

**Files:**
- Modify: `docs/todo.md`
- Modify: `docs/project-status.md`
- Modify: `docs/features/04-agent-orchestration.md`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update docs/todo.md**

Mark Phase 31 tasks as completed. Add any discovered follow-up tasks.

- [ ] **Step 2: Update docs/project-status.md**

Add Phase 31 entry with completion status.

- [ ] **Step 3: Update docs/features/04-agent-orchestration.md**

Add section on Contract-First Review/Refactor pipeline, HITL approval, cascade triggers.

- [ ] **Step 4: Update CLAUDE.md**

Add to the architecture section:
- Review/Refactor Pipeline description
- ReviewTriggerService description
- DiffImpactScorer description
- New NATS subjects under the existing list

- [ ] **Step 5: Commit**

```bash
cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && git add docs/ CLAUDE.md && git commit -m "docs: add Phase 31 Contract-First Review/Refactor documentation

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 17: Pre-commit + Final Verification

- [ ] **Step 1: Run pre-commit hooks**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && pre-commit run --all-files`
Expected: All checks pass. Fix any issues.

- [ ] **Step 2: Run full Go test suite**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && go test ./... -count=1 -race`
Expected: PASS

- [ ] **Step 3: Run Python test suite**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor/workers && python -m pytest -v`
Expected: PASS

- [ ] **Step 4: Run frontend build**

Run: `cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor/frontend && npm run build`
Expected: No errors

- [ ] **Step 5: Final commit if any fixes were needed**

```bash
cd /workspaces/CodeForge/.worktrees/phase-31-review-refactor && git add -A && git commit -m "fix: resolve pre-commit and test issues

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Execution Notes

- **Worktree:** All work happens in `/workspaces/CodeForge/.worktrees/phase-31-review-refactor/`
- **Branch:** `feature/phase-31-contract-first-review-refactor`
- **TDD:** Every task follows RED -> GREEN -> REFACTOR -> COMMIT
- **Cross-Language Sync:** After modifying Go NATS subjects, immediately update Python `_subjects.py` and vice versa
- **Tenant Isolation:** ALL SQL queries must include `AND tenant_id = $N` per CLAUDE.md rules
- **When done:** Use `superpowers:finishing-a-development-branch` skill to merge/PR
