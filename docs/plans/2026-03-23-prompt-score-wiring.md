# Prompt Score Collector & SharedContext HTTP Wiring Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans.

**Goal:** Wire the discarded `PromptScoreCollector` into the run completion pipeline and expose `SharedContextService` via HTTP endpoints.
**Architecture:** `PromptScoreCollector` is already instantiated in `main.go:608` but discarded with `_ = scoreCollector`. It needs to be injected into `ConversationService` so `HandleConversationRunComplete` can record success/cost scores after each run. `SharedContextService` already exists with `InitForTeam`, `AddItem`, and `Get` methods, and the `Handlers` struct already has a `SharedContext` field (line 61). Only the HTTP handler functions and route registrations are missing.
**Tech Stack:** Go (internal/service, internal/adapter/http, cmd/codeforge/main.go), chi router

---

### Task 1: Wire PromptScoreCollector into ConversationService

**Files:**
- [ ] **Step 1: Add field to `ConversationService`.** In `internal/service/conversation.go`, add a `scoreCollector *PromptScoreCollector` field to the `ConversationService` struct (after line 81, alongside `events`).
- [ ] **Step 2: Add setter method.** In `internal/service/conversation.go`, add:
  ```go
  // SetPromptScoreCollector configures automatic score recording on run completion.
  func (s *ConversationService) SetPromptScoreCollector(sc *PromptScoreCollector) {
      s.scoreCollector = sc
  }
  ```
  Follow the existing pattern of `SetPromptAssembler`, `SetEventStore`, etc. (lines 144-148).
- [ ] **Step 3: Record scores in `HandleConversationRunComplete`.** In `internal/service/conversation_agent.go`, at the end of `HandleConversationRunComplete` (after the WS broadcast block, around line 842), add score recording:
  ```go
  // Record prompt scores for evolution tracking.
  if s.scoreCollector != nil && payload.Model != "" {
      tenantID := tenantctx.FromContext(ctx)
      // Determine prompt fingerprint from the conversation's mode.
      fingerprint := ""
      if s.promptAssembler != nil {
          conv, convErr := s.db.GetConversation(ctx, payload.ConversationID)
          if convErr == nil && conv.ModeID != "" {
              fingerprint = s.promptAssembler.FingerprintForMode(conv.ModeID)
          }
      }
      if fingerprint != "" {
          modelFamily := extractModelFamily(payload.Model)
          succeeded := payload.Status == "completed"
          if err := s.scoreCollector.RecordSuccessScore(ctx, tenantID, fingerprint,
              "", modelFamily, payload.RunID, succeeded); err != nil {
              logBestEffort("record success score", err, "run_id", payload.RunID)
          }
          if payload.CostUSD > 0 && payload.TokensOut > 0 {
              qualityPerDollar := float64(payload.TokensOut) / payload.CostUSD
              if err := s.scoreCollector.RecordCostScore(ctx, tenantID, fingerprint,
                  "", modelFamily, payload.RunID, qualityPerDollar); err != nil {
                  logBestEffort("record cost score", err, "run_id", payload.RunID)
              }
          }
      }
  }
  ```
- [ ] **Step 4: Add `extractModelFamily` helper.** In `internal/service/conversation_agent.go`, add a small helper that extracts the model family from a model string (e.g. `"openai/gpt-4o"` -> `"openai"`, `"anthropic/claude-3"` -> `"anthropic"`):
  ```go
  func extractModelFamily(model string) string {
      if idx := strings.Index(model, "/"); idx > 0 {
          return model[:idx]
      }
      return model
  }
  ```
- [ ] **Step 5: Add `FingerprintForMode` to `PromptAssembler`.** In `internal/service/prompt_assembler.go`, add a method that returns the current prompt fingerprint for a given mode ID. If the assembler has a selector with an active variant, use that fingerprint; otherwise compute from the base template. Return empty string if mode not found (caller skips scoring).
- [ ] **Step 6: Wire in `main.go`.** In `cmd/codeforge/main.go`, replace line 610 (`_ = scoreCollector`) with:
  ```go
  conversationSvc.SetPromptScoreCollector(scoreCollector)
  ```

**Verification:**
- Unit test: create `ConversationService` with a mock `PromptScoreCollector` and `PromptAssembler`. Call `HandleConversationRunComplete` with a completed payload. Assert `RecordSuccessScore` called with `succeeded=true` and `RecordCostScore` called with correct `qualityPerDollar`.
- Unit test: call with a failed payload. Assert `RecordSuccessScore` called with `succeeded=false`.
- Unit test: call with no score collector set (nil). Assert no panic, no score recorded.
- Unit test: call with empty model string. Assert no score recorded (guard clause).

**Commit:** `feat: wire PromptScoreCollector into HandleConversationRunComplete`

---

### Task 2: Add SharedContext HTTP endpoints

The `Handlers` struct already has `SharedContext *service.SharedContextService` (line 61 in `handlers.go`). The service has three methods: `InitForTeam(ctx, teamID, projectID)`, `AddItem(ctx, AddSharedItemRequest)`, `Get(ctx, teamID)`. Only the handler functions and route registrations are missing.

**Files:**
- [ ] **Step 1: Create handler file.** Create `internal/adapter/http/handlers_shared_context.go` with three handler methods on `*Handlers`:

  **`InitSharedContext`** -- `POST /api/v1/teams/{teamId}/shared-context`
  ```go
  func (h *Handlers) InitSharedContext(w http.ResponseWriter, r *http.Request) {
      teamID := chi.URLParam(r, "teamId")
      if teamID == "" {
          writeBadRequest(w, "missing teamId")
          return
      }
      var body struct {
          ProjectID string `json:"project_id"`
      }
      if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
          writeBadRequest(w, "invalid JSON body")
          return
      }
      sc, err := h.SharedContext.InitForTeam(r.Context(), teamID, body.ProjectID)
      if err != nil {
          writeInternalError(w, err)
          return
      }
      writeJSON(w, http.StatusCreated, sc)
  }
  ```

  **`GetSharedContext`** -- `GET /api/v1/teams/{teamId}/shared-context`
  ```go
  func (h *Handlers) GetSharedContext(w http.ResponseWriter, r *http.Request) {
      teamID := chi.URLParam(r, "teamId")
      if teamID == "" {
          writeBadRequest(w, "missing teamId")
          return
      }
      sc, err := h.SharedContext.Get(r.Context(), teamID)
      if err != nil {
          writeInternalError(w, err)
          return
      }
      writeJSON(w, http.StatusOK, sc)
  }
  ```

  **`AddSharedContextItem`** -- `POST /api/v1/teams/{teamId}/shared-context/items`
  ```go
  func (h *Handlers) AddSharedContextItem(w http.ResponseWriter, r *http.Request) {
      teamID := chi.URLParam(r, "teamId")
      if teamID == "" {
          writeBadRequest(w, "missing teamId")
          return
      }
      var body cfcontext.AddSharedItemRequest
      if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
          writeBadRequest(w, "invalid JSON body")
          return
      }
      body.TeamID = teamID
      item, err := h.SharedContext.AddItem(r.Context(), body)
      if err != nil {
          writeInternalError(w, err)
          return
      }
      writeJSON(w, http.StatusCreated, item)
  }
  ```

- [ ] **Step 2: Register routes.** In `internal/adapter/http/routes.go`, add inside the authenticated `/api/v1` router group (after the existing team/scope routes, around line 265):
  ```go
  // Shared Context (Phase 5D)
  r.Post("/teams/{teamId}/shared-context", h.InitSharedContext)
  r.Get("/teams/{teamId}/shared-context", h.GetSharedContext)
  r.Post("/teams/{teamId}/shared-context/items", h.AddSharedContextItem)
  ```

- [ ] **Step 3: Add nil guard.** Wrap the route registration in a nil check: `if h.SharedContext != nil { ... }`. This prevents panics if the service is not wired in a test or minimal config.

**Verification:**
- Unit test: call `InitSharedContext` with valid teamId and project_id body. Assert 201 response with shared context JSON.
- Unit test: call `GetSharedContext` with valid teamId. Assert 200 response.
- Unit test: call `AddSharedContextItem` with valid body. Assert 201 response with item JSON.
- Unit test: call each handler with missing teamId. Assert 400 response.
- Unit test: call `InitSharedContext` with empty body. Assert service validation error is returned.
- Integration: POST to create shared context, GET to retrieve it, POST an item, GET again to confirm item present.

**Commit:** `feat: add SharedContext HTTP endpoints (POST/GET teams/{teamId}/shared-context)`

---

### Summary

| # | Component | Issue | Fix | Effort |
|---|-----------|-------|-----|--------|
| 1 | PromptScoreCollector | Created but discarded (`_ = scoreCollector`) | Inject into ConversationService, record scores on run completion | M |
| 2 | SharedContext HTTP | Service exists, Handlers field exists, no routes | Create handler file + register 3 routes | S |

**Total commits:** 2
**Estimated effort:** 0.5-1 day
