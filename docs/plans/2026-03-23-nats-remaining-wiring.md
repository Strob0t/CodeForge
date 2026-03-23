# NATS Remaining Wiring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans.

**Goal:** Wire the 7 remaining broken NATS subjects so every declared subject has both a publisher and a subscriber.
**Architecture:** Each fix is isolated to one subject. Patterns follow existing code: Go subscribers use `queue.Subscribe()` with a handler method, Python subscribers are added to the `subscriptions` list in `TaskConsumer.start()`, cancel listeners use `RuntimeClient.start_cancel_listener()` with `extra_subjects`.
**Tech Stack:** Go (internal/service, internal/port/messagequeue), Python (workers/codeforge/consumer, workers/codeforge/runtime.py)

---

### Task 1: Wire `tasks.cancel` -- Python subscriber

The Go side publishes `tasks.cancel` (via `SubjectTaskCancel`), but the Python worker never listens on it. The cancel listener in `RuntimeClient.start_cancel_listener()` only subscribes to `runs.cancel` plus explicit `extra_subjects`. Legacy task runs (non-conversation) call `start_cancel_listener()` from `_runs.py:48` without extra subjects, so `tasks.cancel` is never heard.

**Files:**
- [ ] **Step 1: Add `tasks.cancel` to `_runs.py` cancel listener.** In `workers/codeforge/consumer/_runs.py`, change the `start_cancel_listener()` call (line 48) to include `extra_subjects=["tasks.cancel"]`. This mirrors the pattern used in `_conversation.py:132-133` for `conversation.run.cancel`.
- [ ] **Step 2: Add `SUBJECT_TASK_CANCEL` constant.** In `workers/codeforge/consumer/_subjects.py`, add `SUBJECT_TASK_CANCEL = "tasks.cancel"` under the Task subjects section (after line 29). Import and use this constant in `_runs.py` instead of a bare string.
- [ ] **Step 3: Update `RuntimeClient` cancel matching.** In `workers/codeforge/runtime.py`, the `_listen_sub` closure (line 88) matches on `data.get("run_id") == self.run_id`. The `TaskCancelPayload` uses `task_id` (Go schema: `schemas.go:23-26`). Add a fallback: `data.get("run_id") == self.run_id or data.get("task_id") == self.task_id`.

**Verification:**
- Unit test: mock JetStream, publish `{"task_id": "t1"}` on `tasks.cancel`, assert `RuntimeClient._cancelled` becomes `True`.
- Integration: start a legacy run, send `tasks.cancel` via NATS, confirm run terminates.

**Commit:** `fix: wire tasks.cancel Python subscriber via RuntimeClient extra_subjects`

---

### Task 2: Wire `context.shared.updated` -- Python subscriber

Go publishes this from `SharedContextService.AddItem()` (`shared_context.go:69`). No Python handler exists.

**Files:**
- [ ] **Step 1: Add subject constant.** In `workers/codeforge/consumer/_subjects.py`, add `SUBJECT_SHARED_UPDATED = "context.shared.updated"`.
- [ ] **Step 2: Create handler mixin.** Create `workers/codeforge/consumer/_context_events.py` with a `ContextEventsHandlerMixin` class containing `_handle_shared_context_updated(self, msg)`. Handler: ack message, parse `SharedContextUpdatedPayload` (fields: `team_id`, `key`, `author`, `version`), log at info level (`"shared context updated"`, include team_id and key). No further action needed -- this is an awareness event for Python workers.
- [ ] **Step 3: Register mixin and subscription.** In `workers/codeforge/consumer/__init__.py`: import `ContextEventsHandlerMixin` and `SUBJECT_SHARED_UPDATED`, add mixin to `TaskConsumer` bases, add `(SUBJECT_SHARED_UPDATED, self._handle_shared_context_updated)` to the subscriptions list.

**Verification:**
- Unit test: call handler with mock message containing valid JSON, assert ack called and no exception.
- Manual: add a shared context item via API, verify Python log line appears.

**Commit:** `fix: wire context.shared.updated Python subscriber`

---

### Task 3: Wire `prompt.evolution.promoted` -- Python subscriber

Go publishes this from `PromptEvolutionService` when a variant is promoted. Python should be aware so it can reload any cached prompt variant.

**Files:**
- [ ] **Step 1: Add subject constants.** In `workers/codeforge/consumer/_subjects.py`, add `SUBJECT_PROMPT_EVOLUTION_PROMOTED = "prompt.evolution.promoted"` and `SUBJECT_PROMPT_EVOLUTION_REVERTED = "prompt.evolution.reverted"`.
- [ ] **Step 2: Add handlers to existing mixin.** In `workers/codeforge/consumer/_prompt_evolution.py`, add two methods: `_handle_prompt_promoted(self, msg)` and `_handle_prompt_reverted(self, msg)`. Both: ack message, parse payload, log at info level with mode_id/variant_id. For promoted: log `"prompt variant promoted, clearing cached prompts"`. For reverted: log `"prompt variant reverted, clearing cached prompts"`.
- [ ] **Step 3: Register subscriptions.** In `workers/codeforge/consumer/__init__.py`, import the two new constants and add them to the subscriptions list, mapped to the handlers from `_prompt_evolution.py`.

**Verification:**
- Unit test: call each handler with mock messages, assert ack and log output.
- Manual: promote a variant via API, confirm Python log line.

**Commit:** `fix: wire prompt.evolution.promoted and prompt.evolution.reverted Python subscribers`

---

### Task 4: Wire `prompt.evolution.reverted` -- Python subscriber

Covered in Task 3 (same mixin, same commit). No separate task needed.

---

### Task 5: Wire `evaluation.gemmas.result` -- Go subscriber

Go publishes `evaluation.gemmas.request` from `EvaluationService.HandlePlanComplete()` (`evaluation.go:81`). Python computes GEMMAS and publishes back on `evaluation.gemmas.result`. But Go never subscribes to the result. The `GemmasEvalResultPayload` struct already exists (`schemas.go:497-503`).

**Files:**
- [ ] **Step 1: Add `HandleGemmasResult` method.** In `internal/service/evaluation.go`, add:
  ```go
  func (s *EvaluationService) HandleGemmasResult(ctx context.Context, _ string, data []byte) error {
      var payload messagequeue.GemmasEvalResultPayload
      if err := json.Unmarshal(data, &payload); err != nil {
          return fmt.Errorf("unmarshal gemmas result: %w", err)
      }
      if payload.Error != "" {
          slog.Error("gemmas evaluation failed", "plan_id", payload.PlanID, "error", payload.Error)
          return nil
      }
      slog.Info("gemmas evaluation result received",
          "plan_id", payload.PlanID,
          "diversity_score", payload.InformationDiversityScore,
          "unnecessary_path_ratio", payload.UnnecessaryPathRatio,
      )
      // Store GEMMAS scores on the plan record.
      if err := s.store.UpdatePlanGemmasScores(ctx, payload.PlanID,
          payload.InformationDiversityScore, payload.UnnecessaryPathRatio); err != nil {
          slog.Error("failed to store gemmas scores", "plan_id", payload.PlanID, "error", err)
      }
      return nil
  }
  ```
- [ ] **Step 2: Add `StartGemmasResultSubscriber`.** In `internal/service/evaluation.go`, add:
  ```go
  func (s *EvaluationService) StartGemmasResultSubscriber(ctx context.Context) (func(), error) {
      if s.queue == nil {
          return func() {}, nil
      }
      return s.queue.Subscribe(ctx, messagequeue.SubjectEvalGemmasResult, s.HandleGemmasResult)
  }
  ```
- [ ] **Step 3: Add store method stub.** Add `UpdatePlanGemmasScores(ctx context.Context, planID string, diversity, unnecessaryRatio float64) error` to the `database.Store` interface and implement it in the PostgreSQL adapter (update `plans` table JSONB metadata field). If the store method is complex, use `logBestEffort` to log and skip on error.
- [ ] **Step 4: Wire in `main.go`.** After the `evoSvc` initialization block (~line 619), add:
  ```go
  gemmasCancel, err := evalSvc.StartGemmasResultSubscriber(ctx)
  if err != nil {
      return fmt.Errorf("gemmas result subscriber: %w", err)
  }
  ```
  Add `gemmasCancel` to the deferred cleanup slice.

**Verification:**
- Unit test: call `HandleGemmasResult` with sample payload, assert store method called with correct values.
- Unit test: call with error payload, assert no store call and error logged.
- Integration: complete a plan, verify GEMMAS scores are stored on the plan record.

**Commit:** `fix: wire evaluation.gemmas.result Go subscriber with score persistence`

---

### Task 6: Wire `agents.output` -- Python publisher

Go subscribes in `AgentService.StartAgentOutputSubscriber()` (`agent.go:238`), expecting `AgentOutputEvent` payloads (`event/broadcast_payloads.go:24-29`: `task_id`, `line`, `stream`, `timestamp`). But Python never publishes to this subject.

**Files:**
- [ ] **Step 1: Add `SUBJECT_AGENT_OUTPUT` constant.** In `workers/codeforge/consumer/_subjects.py`, add `SUBJECT_AGENT_OUTPUT = "agents.output"`.
- [ ] **Step 2: Add publish helper to `RuntimeClient`.** In `workers/codeforge/runtime.py`, add:
  ```python
  async def publish_agent_output(self, line: str, stream: str = "stdout") -> None:
      payload = {
          "task_id": self.task_id,
          "line": line,
          "stream": stream,
          "timestamp": datetime.now(tz=timezone.utc).isoformat(),
      }
      await self._js.publish("agents.output", json.dumps(payload).encode())
  ```
- [ ] **Step 3: Call from backend executors.** In `workers/codeforge/executor.py` (or the appropriate executor file), when processing agent output lines, call `runtime.publish_agent_output(line)` to forward output to Go for WebSocket broadcast. Add calls at points where stdout/stderr lines are captured.

**Verification:**
- Unit test: mock JetStream, call `publish_agent_output("hello")`, assert publish called with correct subject and payload structure.
- Integration: run a task, verify WebSocket receives `agent_output` events.

**Commit:** `fix: wire agents.output Python publisher in RuntimeClient`

---

### Task 7: Wire `review.approval.required` -- Go publisher

The subject is declared (`queue.go:126`) and the `ReviewApprovalRequiredPayload` schema exists (`schemas.go:640-647`), but neither Go nor Python ever publishes to it. The comment says "Python -> Go: human approval needed" but this is a Go->Frontend notification. The review/refactor flow in `DiffImpactScorer` (`diff_impact.go`) determines when HITL is needed but never sends the NATS event.

**Files:**
- [ ] **Step 1: Publish from `DiffImpactScorer`.** In `internal/service/diff_impact.go`, at the point where the impact level triggers HITL (`waiting_approval` status), add a NATS publish:
  ```go
  approvalPayload := messagequeue.ReviewApprovalRequiredPayload{
      RunID:       runID,
      ProjectID:   projectID,
      TenantID:    tenantID,
      ImpactLevel: impactLevel,
  }
  data, err := json.Marshal(approvalPayload)
  if err == nil {
      if pubErr := s.queue.Publish(ctx, messagequeue.SubjectReviewApprovalRequired, data); pubErr != nil {
          slog.Warn("failed to publish review approval required", "run_id", runID, "error", pubErr)
      }
  }
  ```
- [ ] **Step 2: Add Go subscriber for WS broadcast.** In `internal/service/review_trigger.go` (or a new method on `ReviewService`), add a subscriber that receives the message and broadcasts it via WebSocket to the frontend so the approval UI can render. Pattern: like `StartAgentOutputSubscriber`.
- [ ] **Step 3: Wire subscriber in `main.go`.** Subscribe to `SubjectReviewApprovalRequired` in the startup sequence, add cancel to cleanup.
- [ ] **Step 4: No Python subscriber needed.** This is Go->Frontend only. Python does not need to listen.

**Verification:**
- Unit test: trigger a high-impact diff, assert `review.approval.required` is published.
- Unit test: subscriber receives payload and broadcasts WS event.
- Integration: trigger review-refactor on a project, verify frontend receives approval notification.

**Commit:** `fix: wire review.approval.required Go publisher and WS broadcast`

---

### Summary

| # | Subject | Side Broken | Fix Location | Effort |
|---|---------|-------------|--------------|--------|
| 1 | `tasks.cancel` | Python no listener | `_runs.py`, `runtime.py` | S |
| 2 | `context.shared.updated` | Python no listener | new `_context_events.py` | S |
| 3 | `prompt.evolution.promoted` | Python no listener | `_prompt_evolution.py` | S |
| 4 | `prompt.evolution.reverted` | Python no listener | (covered by Task 3) | - |
| 5 | `evaluation.gemmas.result` | Go no subscriber | `evaluation.go`, `main.go` | M |
| 6 | `agents.output` | Python no publisher | `runtime.py`, executors | M |
| 7 | `review.approval.required` | Neither side wired | `diff_impact.go`, `main.go` | M |

**Total commits:** 6 (Tasks 3+4 combined)
**Estimated effort:** 1-2 days
