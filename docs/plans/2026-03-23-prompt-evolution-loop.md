# PromptEvolution Loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete the automatic prompt evolution loop by wiring TriggerReflection to an HTTP endpoint, implementing prompt_scores store layer, and connecting HandleMutateComplete to persist variants.

**Architecture:** Add store methods for prompt_scores + prompt_variants, add HTTP endpoint for triggering reflection, wire the full loop: HTTP trigger -> NATS -> Python -> NATS -> Go handler -> DB persist.

**Tech Stack:** Go, PostgreSQL, NATS JetStream

**Depends on:** fix/nats-subscriber-wiring (for StartSubscribers and HandleReflectComplete)

---

### Task 1: Implement prompt variant store methods

**Files:**
- Create: `internal/adapter/postgres/store_prompt_variant.go`
- Modify: `internal/port/database/store.go` (add interface methods)

- [ ] **Step 1: Add interface methods to Store**

```go
InsertPromptVariant(ctx context.Context, v *prompt.PromptVariant) error
GetPromptVariant(ctx context.Context, id string) (*prompt.PromptVariant, error)
GetVariantsByModeAndModel(ctx context.Context, modeID, modelFamily string) ([]*prompt.PromptVariant, error)
UpdatePromptVariantStatus(ctx context.Context, id string, status prompt.PromotionStatus) error
```

- [ ] **Step 2: Implement store methods**

Follow patterns from `store_prompt_section.go` (same table area).

- [ ] **Step 3: Write tests**

- [ ] **Step 4: Verify**

```bash
go test ./internal/adapter/postgres/... -run TestPromptVariant -v
```

---

### Task 2: Implement prompt score store methods

**Files:**
- Create: `internal/adapter/postgres/store_prompt_score.go`
- Modify: `internal/port/database/store.go` (add interface methods)

- [ ] **Step 1: Add interface methods**

```go
InsertPromptScore(ctx context.Context, tenantID string, score *prompt.PromptScore) error
GetPromptScores(ctx context.Context, tenantID, fingerprint string) ([]*prompt.PromptScore, error)
```

- [ ] **Step 2: Implement using prompt_scores table from migration 078**

- [ ] **Step 3: Write tests and verify**

---

### Task 3: Add TriggerReflection HTTP endpoint

**Files:**
- Modify: `internal/adapter/http/handlers_prompt_evolution.go`
- Modify: `internal/adapter/http/routes.go`

- [ ] **Step 1: Add handler**

```go
func (h *Handlers) TriggerPromptEvolutionReflect(w http.ResponseWriter, r *http.Request) {
    // Parse request body: mode_id, model_family, current_prompt, failures[]
    // Call evoSvc.TriggerReflection(ctx, tenantID, modeID, modelFamily, currentPrompt, failures)
    // Return 202 Accepted
}
```

- [ ] **Step 2: Register route**

```go
r.Post("/reflect", h.TriggerPromptEvolutionReflect)
```

- [ ] **Step 3: Write handler tests**
- [ ] **Step 4: Verify**

```bash
go test ./internal/adapter/http/... -run TestTriggerPromptEvolution -v
```

---

### Task 4: Wire score collector in main.go

**Files:**
- Modify: `cmd/codeforge/main.go`

- [ ] **Step 1: Create PromptScoreCollector with real store**
- [ ] **Step 2: Build verification**

```bash
go build ./cmd/codeforge/
```

---

### Task 5: Integration tests for full loop

**Files:**
- Create: `internal/adapter/http/handlers_prompt_evolution_test.go`
- Modify: `internal/service/prompt_evolution_test.go`

- [ ] **Step 1: Add handler-level tests for reflect endpoint**

Test: 202 success, 400 missing fields, 503 nil service.

- [ ] **Step 2: Add end-to-end unit test**

Test full loop: TriggerReflection -> published to NATS -> simulate mutate complete -> variant stored -> promote variant.

- [ ] **Step 3: Verify all tests pass**

```bash
go test ./internal/... -v 2>&1 | tail -30
```

---

### Task 6: Final commit

```
feat: complete prompt evolution loop with store layer and HTTP trigger

- Add store methods for prompt_variants and prompt_scores tables
- Add POST /prompt-evolution/reflect endpoint to trigger reflection
- Wire PromptScoreCollector in main.go
- Add integration tests for the full reflect -> mutate -> store loop
```
