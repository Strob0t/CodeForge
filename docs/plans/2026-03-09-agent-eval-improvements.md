# Agent-Eval Improvements Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix 10 concrete issues identified across two real agent-eval benchmark runs (Mistral Large 274/300, Gemini 2.5 Flash 193/300) spanning the eval skill, auto-agent infrastructure, and routing system.

**Architecture:** Changes span 3 layers — (1) the `/agent-eval` skill definition (Markdown), (2) Go Core auto-agent service, (3) Python worker routing and retry logic. Each fix is independent and can be implemented/committed separately.

**Tech Stack:** Go 1.25 (auto-agent service), Python 3.12 (worker routing/retry), Markdown (skill definition), pytest (tests)

---

## Task 1: Add pytest verification instruction to feature prompts

**Priority:** High | **Effort:** Trivial | **Layer:** Skill definition

The agent doesn't always run tests after writing code. In the Gemini run, the LRU Cache had a fatal `AttributeError` that one pytest run would have caught. The instruction to "run pytest" is buried in the goals constraint, not in each feature prompt.

**Files:**
- Modify: `.claude/commands/agent-eval.md:244-250`

**Step 1: Update the prompt template in processFeature-equivalent section**

In the agent-eval skill, each feature description already has a `File:` and `Tests:` line. Add an explicit verification instruction after the description in each of the 3 feature specs.

Add this block to the END of each feature description (inside the ``` block), before the closing ```:

For **Feature 1 (LRU Cache)**, after line `Tests: test_lru_cache.py (25 tests)`:
```
IMPORTANT: After writing your implementation, you MUST run:
  python -m pytest test_lru_cache.py -v --tb=short
Fix ALL failures before considering this feature complete. Do NOT move on until all 25 tests pass.
```

For **Feature 2 (JSON Schema Validator)**, after line `Tests: test_json_schema_validator.py (39 tests)`:
```
IMPORTANT: After writing your implementation, you MUST run:
  python -m pytest test_json_schema_validator.py -v --tb=short
Fix ALL failures before considering this feature complete. Do NOT move on until all 39 tests pass.
```

For **Feature 3 (Diff Analyzer)**, after line `Tests: test_diff_analyzer.py (22 tests)`:
```
IMPORTANT: After writing your implementation, you MUST run:
  python -m pytest test_diff_analyzer.py -v --tb=short
Fix ALL failures before considering this feature complete. Do NOT move on until all 22 tests pass.
```

**Step 2: Run a sanity check**

Verify the skill file parses correctly:
```bash
wc -l .claude/commands/agent-eval.md
# Expected: ~313 lines (was 301, +12 for 3 blocks of 4 lines)
```

**Step 3: Commit**

```bash
git add .claude/commands/agent-eval.md
git commit -m "fix(agent-eval): add explicit pytest verification to each feature prompt"
```

---

## Task 2: Inline test signatures in feature descriptions

**Priority:** Low-Medium | **Effort:** Trivial | **Layer:** Skill definition

The agent burns LLM steps discovering tests via `read_file`. With rate-limited models, every step counts. Including test class/method names directly in the feature description saves 1-2 tool calls per feature.

**Files:**
- Modify: `.claude/commands/agent-eval.md` (feature description sections)

**Step 1: Add test overview to each feature description**

Add a `Key test classes:` section to each feature description, listing the test class names and what they test. Do NOT include full test code (too long), just the structure.

For **Feature 1 (LRU Cache)**, add after the `Tests:` line:
```
Key test classes and methods (test_lru_cache.py):
  TestLRUCacheBasic: put_and_get, get_missing, put_overwrite, delete, clear
  TestLRUCacheCapacity: capacity_property
  TestLRUCacheEviction: evicts_lru, get_refreshes_order, put_refresh_prevents_eviction
  TestLRUCacheTTL: expires_after_ttl, default_ttl, per_key_ttl, no_ttl_no_expiry, len_excludes_expired, background_cleanup
  TestLRUCacheContains: existing, missing, does_not_refresh_order
  TestLRUCacheStats: hits_and_misses, hit_rate, hit_rate_zero, get_stats
  TestLRUCacheContextManager: clears_on_exit
  TestLRUCacheThreadSafety: concurrent_puts
```

For **Feature 2 (JSON Schema Validator)**:
```
Key test classes and methods (test_json_schema_validator.py):
  TestTypeValidation: string, integer, integer_rejects_float, number, boolean_array_object, null
  TestObjectValidation: properties, invalid_type, required_missing, additional_properties_false, additional_properties_schema
  TestArrayValidation: items_all_valid, items_some_invalid
  TestNumericValidation: minimum_maximum, minimum_invalid, maximum_invalid
  TestStringValidation: min_length, max_length, pattern
  TestEnumAndConst: enum_valid/invalid, const_valid/invalid
  TestComposition: any_of, all_of, one_of, not
  TestRef: ref_resolves_definition
  TestErrorPaths: nested_error_path, empty_schema_accepts_anything
```

For **Feature 3 (Diff Analyzer)**:
```
Key test classes and methods (test_diff_analyzer.py):
  TestParseDiff: simple_diff, hunk_count, hunk_line_counts, change_types, multi_file, new_file, deleted_file, multi_hunk, no_newline_marker, empty_input, line_numbers_tracked
  TestAnalyzeSmells: debug_code, todo_fixme, long_line, trailing_whitespace, large_addition, magic_number, no_smells_clean
  TestGenerateReport: summary, smells_by_severity, files_list, empty_input
```

**Step 2: Commit**

```bash
git add .claude/commands/agent-eval.md
git commit -m "feat(agent-eval): inline test signatures in feature descriptions to save agent steps"
```

---

## Task 3: Document step budget expectations

**Priority:** Low | **Effort:** Trivial | **Layer:** Skill definition

**Files:**
- Modify: `.claude/commands/agent-eval.md` (Rules section at bottom)

**Step 1: Add step budget guidance to the Rules section**

Add to the `## Rules` section at the end of the file:

```markdown
- Expected agent steps per problem (based on benchmark data):
  - LRU Cache: ~9-15 steps (read spec, implement, test, fix)
  - JSON Schema Validator: ~15-50 steps (most complex problem, many keywords)
  - Diff Analyzer: ~10-27 steps (moderate complexity)
- Models with <10 req/min free tier may need 20-40 minutes total due to rate limiting
- The auto-agent default iteration limit is 50 steps per feature
```

**Step 2: Commit**

```bash
git add .claude/commands/agent-eval.md
git commit -m "docs(agent-eval): add step budget expectations to rules section"
```

---

## Task 4: Automate quality scoring with mypy and ruff

**Priority:** Medium | **Effort:** Medium | **Layer:** Skill definition (Phase 5)

Currently, Code Quality (20pts), Type Safety (10pts), Edge Cases (15pts), and Efficiency (15pts) are scored subjectively by the evaluator. This makes scores non-reproducible across runs.

**Files:**
- Modify: `.claude/commands/agent-eval.md` (Phase 5 scoring section)

**Step 1: Replace subjective scoring instructions with automated commands**

Replace the scoring rubric table in Phase 5 (lines 261-267) with:

```markdown
4. Score each problem using this rubric (100 points each, 300 total):

   | Category      | Points | Criteria                                              |
   |---------------|--------|-------------------------------------------------------|
   | Correctness   | 40     | `40 * (tests_passed / tests_total)`                   |
   | Code Quality  | 20     | Run `ruff check <file> --select E,W,F` → `20 - min(errors * 2, 20)` |
   | Type Safety   | 10     | Run `python -m mypy <file> --strict --no-error-summary 2>&1 | grep "error:" | wc -l` → `max(10 - errors, 0)` |
   | Edge Cases    | 15     | Count tests in TTL/boundary/thread-safety/empty-input categories that pass → `15 * (edge_passed / edge_total)` |
   | Efficiency    | 15     | Time the test suite: `time python -m pytest <test_file> -q` → 15 if <2s, 12 if <5s, 8 if <10s, 4 if <30s, 0 if >30s |

   Edge case test categories per problem:
   - LRU Cache: TTL tests (6) + ThreadSafety (1) + Contains (3) = 10 edge tests
   - JSON Schema: Composition (6) + Ref (1) + ErrorPaths (2) = 9 edge tests
   - Diff Analyzer: new_file (1) + deleted_file (1) + no_newline (1) + empty_input (2) + no_smells (1) = 6 edge tests
```

**Step 2: Verify tooling is available in the workspace**

```bash
cd /workspaces/CodeForge && python -m mypy --version && ruff --version
```

If mypy or ruff are not installed in the workspace venv, add a note to Phase 0:
```
Ensure mypy and ruff are available: `pip install mypy ruff` (if not already installed)
```

**Step 3: Commit**

```bash
git add .claude/commands/agent-eval.md
git commit -m "feat(agent-eval): automate quality scoring with mypy, ruff, and timing"
```

---

## Task 5: Add post-feature test verification in auto-agent

**Priority:** High | **Effort:** Medium | **Layer:** Go Core

The auto-agent marks features `done` when the conversation completes, regardless of whether the implementation actually works. In the Gemini run, the LRU Cache was marked done with 0/25 tests passing.

**Files:**
- Modify: `internal/service/autoagent.go:221-266` (processFeature method)
- Test: `internal/service/autoagent_test.go` (create new)

**Step 1: Write the failing test**

Create `internal/service/autoagent_test.go`:

```go
package service_test

import (
	"testing"
)

func TestProcessFeature_RetriesOnTestFailure(t *testing.T) {
	// This test verifies the concept: when a feature's conversation completes
	// but the associated test file fails, processFeature should send a follow-up
	// message with the test output and retry.
	//
	// Full integration test requires mocking ConversationService, DB, and workspace.
	// For now, verify the retry prompt is correctly formatted.
	t.Skip("TODO: implement after infrastructure mocks are available")
}
```

**Step 2: Modify processFeature to run post-completion verification**

In `internal/service/autoagent.go`, after `waitForCompletion` returns successfully (line 264), add a verification step. The key insight: the feature description contains `Tests: test_<name>.py` — we can extract the test file name and run pytest.

Replace lines 260-266 of `processFeature`:

```go
	// Wait for the conversation run to complete via NATS.
	err = s.waitForCompletion(ctx, conv.ID, aa)
	if err != nil {
		return fmt.Errorf("wait for completion: %w", err)
	}

	// Post-completion verification: run the test file if specified in the description.
	testFile := extractTestFile(feat.Description)
	if testFile != "" {
		testResult, testErr := s.runWorkspaceTest(ctx, projectID, testFile)
		if testErr != nil || !testResult.AllPassed {
			slog.Info("auto-agent post-verification failed, sending fix prompt",
				"project_id", projectID,
				"feature_id", feat.ID,
				"test_file", testFile,
				"passed", testResult.Passed,
				"total", testResult.Total,
			)

			// Send a follow-up message with the test output.
			fixPrompt := fmt.Sprintf(
				"The tests are failing. %d/%d tests passed.\n\nTest output:\n```\n%s\n```\n\n"+
					"Please fix the implementation to make all tests pass.",
				testResult.Passed, testResult.Total, testResult.Output,
			)
			err = s.conversations.SendMessageAgentic(ctx, conv.ID, conversation.SendMessageRequest{
				Content: fixPrompt,
			})
			if err != nil {
				return fmt.Errorf("send fix prompt: %w", err)
			}

			// Wait for the fix conversation to complete.
			err = s.waitForCompletion(ctx, conv.ID, aa)
			if err != nil {
				return fmt.Errorf("wait for fix completion: %w", err)
			}
		}
	}

	return nil
```

**Step 3: Add the helper functions**

Add to `internal/service/autoagent.go`:

```go
// extractTestFile extracts the test filename from a feature description.
// Looks for patterns like "Tests: test_lru_cache.py" or "Tests: test_lru_cache.py (25 tests)".
func extractTestFile(description string) string {
	re := regexp.MustCompile(`Tests:\s+(test_\w+\.py)`)
	matches := re.FindStringSubmatch(description)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// testResult holds parsed pytest output.
type testResult struct {
	Passed    int
	Failed    int
	Total     int
	AllPassed bool
	Output    string
}

// runWorkspaceTest runs pytest on a test file in the project workspace.
func (s *AutoAgentService) runWorkspaceTest(
	ctx context.Context,
	projectID string,
	testFile string,
) (testResult, error) {
	workspace, err := s.db.GetWorkspacePath(ctx, projectID)
	if err != nil {
		return testResult{}, err
	}

	cmd := exec.CommandContext(ctx, "python", "-m", "pytest", testFile, "-v", "--tb=short")
	cmd.Dir = workspace
	out, err := cmd.CombinedOutput()
	output := string(out)

	result := testResult{Output: output}

	// Parse "X passed, Y failed" from pytest output.
	re := regexp.MustCompile(`(\d+) passed`)
	if m := re.FindStringSubmatch(output); len(m) >= 2 {
		result.Passed, _ = strconv.Atoi(m[1])
	}
	re2 := regexp.MustCompile(`(\d+) failed`)
	if m := re2.FindStringSubmatch(output); len(m) >= 2 {
		result.Failed, _ = strconv.Atoi(m[1])
	}
	result.Total = result.Passed + result.Failed
	result.AllPassed = result.Failed == 0 && result.Passed > 0

	return result, nil
}
```

Add the required imports at the top of the file:
```go
import (
	"os/exec"
	"regexp"
	"strconv"
)
```

**Step 4: Run existing tests**

```bash
cd /workspaces/CodeForge && go test ./internal/service/... -run TestAutoAgent -v
```

**Step 5: Test extractTestFile manually**

```bash
cd /workspaces/CodeForge && go test -run TestExtractTestFile ./internal/service/ -v
```

Add a unit test for `extractTestFile`:

```go
func TestExtractTestFile(t *testing.T) {
	tests := []struct {
		desc string
		want string
	}{
		{"Tests: test_lru_cache.py (25 tests)", "test_lru_cache.py"},
		{"Tests: test_diff_analyzer.py", "test_diff_analyzer.py"},
		{"No tests mentioned here", ""},
		{"File: foo.py\nTests: test_foo.py\nMore text", "test_foo.py"},
	}
	for _, tt := range tests {
		got := extractTestFile(tt.desc)
		if got != tt.want {
			t.Errorf("extractTestFile(%q) = %q, want %q", tt.desc, got, tt.want)
		}
	}
}
```

**Step 6: Commit**

```bash
git add internal/service/autoagent.go internal/service/autoagent_test.go
git commit -m "feat(auto-agent): add post-feature test verification with retry on failure"
```

---

## Task 6: Reset in_progress features on auto-agent start

**Priority:** Medium | **Effort:** Trivial | **Layer:** Go Core

When the auto-agent stops mid-feature, the feature stays `in_progress` and won't be picked up on restart. Users must manually reset it via API.

**Files:**
- Modify: `internal/service/autoagent.go:296-315` (pendingFeatures method)

**Step 1: Write the failing test**

Add to `internal/service/autoagent_test.go`:

```go
func TestPendingFeatures_IncludesInProgress(t *testing.T) {
	// Verify that pendingFeatures also returns features with "in_progress" status.
	// This ensures features interrupted by a previous stop are retried.
	t.Skip("TODO: implement after infrastructure mocks are available")
}
```

**Step 2: Modify pendingFeatures to include in_progress features**

In `internal/service/autoagent.go`, update the `pendingFeatures` method (line 309-311):

Replace:
```go
	for i := range allFeatures {
		if allFeatures[i].Status == roadmap.FeatureBacklog || allFeatures[i].Status == roadmap.FeaturePlanned {
			pending = append(pending, allFeatures[i])
		}
	}
```

With:
```go
	for i := range allFeatures {
		switch allFeatures[i].Status {
		case roadmap.FeatureBacklog, roadmap.FeaturePlanned, roadmap.FeatureInProgress:
			pending = append(pending, allFeatures[i])
		}
	}
```

**Step 3: Add a log message for clarity**

Add a log line after the loop to show how many in_progress features were recovered:

```go
	var recovered int
	for _, f := range pending {
		if f.Status == roadmap.FeatureInProgress {
			recovered++
		}
	}
	if recovered > 0 {
		slog.Info("auto-agent recovering interrupted features",
			"project_id", rm.ProjectID,
			"recovered_count", recovered,
		)
	}
```

**Step 4: Verify compilation**

```bash
cd /workspaces/CodeForge && go build ./internal/service/...
```

**Step 5: Commit**

```bash
git add internal/service/autoagent.go
git commit -m "fix(auto-agent): include in_progress features on restart to recover interrupted work"
```

---

## Task 7: Persist conversation history across NATS retries

**Priority:** Low | **Effort:** High | **Layer:** Go Core + Python Worker

When the auto-agent sends a follow-up message to the same conversation, the worker doesn't see previous tool results because conversation history isn't persisted across NATS roundtrips. This is a larger architectural change.

**Files:**
- Modify: `internal/service/conversation_agent.go` (include history in payload)
- Modify: `internal/port/messagequeue/schemas.go` (extend payload schema)
- Modify: `workers/codeforge/consumer/_conversation.py` (use persisted history)
- Modify: `workers/codeforge/models.py` (extend Pydantic model)

**Step 1: Extend the NATS payload schema (Go side)**

In `internal/port/messagequeue/schemas.go`, the `ConversationRunStartPayload` already has a `Messages` field. The issue is that on follow-up messages, only the NEW message is sent, not the full history.

In `internal/service/conversation_agent.go`, find `SendMessageAgentic`. Modify it to load the full conversation history from the database before publishing:

```go
// Load existing messages for this conversation.
existingMsgs, err := s.db.ListMessages(ctx, conversationID)
if err != nil {
    return fmt.Errorf("load conversation history: %w", err)
}

// Build messages list: existing history + new user message.
var allMessages []messagequeue.ConversationMessagePayload
for _, msg := range existingMsgs {
    allMessages = append(allMessages, messagequeue.ConversationMessagePayload{
        Role:    msg.Role,
        Content: msg.Content,
    })
}
allMessages = append(allMessages, messagequeue.ConversationMessagePayload{
    Role:    "user",
    Content: req.Content,
})
```

Then use `allMessages` in the NATS payload instead of just the single new message.

**Step 2: Verify on Python side**

The Python consumer already handles a `messages` list in the payload. Verify that `_run_agentic_loop` passes the full messages list to the `AgentLoopExecutor`.

**Step 3: Test round-trip**

This requires a full integration test with NATS running. Create a manual test:
1. Start auto-agent on a project with one feature
2. Observe first conversation run
3. Trigger a follow-up (e.g., via test verification failure from Task 5)
4. Verify the follow-up conversation includes prior tool call results

**Step 4: Commit**

```bash
git add internal/service/conversation_agent.go internal/port/messagequeue/schemas.go
git commit -m "feat(conversation): persist full history in NATS payload for follow-up messages"
```

**Note:** This task has the highest blast radius. Consider implementing it last, after Tasks 5 and 6 are proven stable.

---

## Task 8: Add gemini/gemini-2.5-flash to COMPLEXITY_DEFAULTS

**Priority:** High | **Effort:** Trivial | **Layer:** Python Worker (Routing)

`gemini/gemini-2.5-flash` is the most capable free-tier model but is NOT in any routing tier. The router never selects it as primary or fallback.

**Files:**
- Modify: `workers/codeforge/routing/router.py:27-48`
- Test: `workers/tests/test_routing_router.py`

**Step 1: Write the failing test**

Add to `workers/tests/test_routing_router.py`:

```python
def test_gemini_flash_in_medium_tier():
    """gemini-2.5-flash should be in the MEDIUM complexity tier defaults."""
    from codeforge.routing.router import COMPLEXITY_DEFAULTS
    from codeforge.routing.models import ComplexityTier

    medium_models = COMPLEXITY_DEFAULTS[ComplexityTier.MEDIUM]
    assert "gemini/gemini-2.5-flash" in medium_models
```

**Step 2: Run test to verify it fails**

```bash
cd /workspaces/CodeForge && python -m pytest workers/tests/test_routing_router.py::test_gemini_flash_in_medium_tier -v
```
Expected: FAIL — `gemini/gemini-2.5-flash` not in list.

**Step 3: Add gemini-2.5-flash to the MEDIUM tier**

In `workers/codeforge/routing/router.py`, update the MEDIUM tier (lines 33-37):

Replace:
```python
    ComplexityTier.MEDIUM: [
        "groq/llama-3.3-70b-versatile",
        "openai/gpt-4o-mini",
        "gemini/gemini-2.0-flash",
    ],
```

With:
```python
    ComplexityTier.MEDIUM: [
        "groq/llama-3.3-70b-versatile",
        "gemini/gemini-2.5-flash",
        "openai/gpt-4o-mini",
        "gemini/gemini-2.0-flash",
    ],
```

**Step 4: Run test to verify it passes**

```bash
cd /workspaces/CodeForge && python -m pytest workers/tests/test_routing_router.py::test_gemini_flash_in_medium_tier -v
```
Expected: PASS

**Step 5: Run all routing tests to verify no regressions**

```bash
cd /workspaces/CodeForge && python -m pytest workers/tests/test_routing_router.py -v
```

**Step 6: Commit**

```bash
git add workers/codeforge/routing/router.py workers/tests/test_routing_router.py
git commit -m "feat(routing): add gemini/gemini-2.5-flash to MEDIUM complexity tier"
```

---

## Task 9: Skip models with known TPM limits too small for context

**Priority:** Medium | **Effort:** Low | **Layer:** Python Worker (Routing)

Groq's `llama-3.1-8b-instant` has a 6000 TPM limit. Every agentic conversation exceeds this. The fallback chain wastes 5 retries (~62 seconds) before falling to the next model every time.

**Files:**
- Modify: `workers/codeforge/routing/rate_tracker.py`
- Modify: `workers/codeforge/agent_loop.py:111-136`
- Test: `workers/tests/test_routing_router.py`

**Step 1: Write the failing test**

Add to `workers/tests/test_routing_router.py`:

```python
def test_tpm_blocked_model_skipped_in_fallback():
    """Models that failed with TPM exceeded should be skipped in subsequent fallback picks."""
    from codeforge.routing.rate_tracker import get_tracker

    tracker = get_tracker()
    tracker.record_error("groq", error_type="tpm_exceeded")
    assert tracker.is_exhausted("groq")
```

**Step 2: Run test to verify it fails**

```bash
cd /workspaces/CodeForge && python -m pytest workers/tests/test_routing_router.py::test_tpm_blocked_model_skipped_in_fallback -v
```
Expected: FAIL — `tpm_exceeded` not in `_ERROR_COOLDOWNS`.

**Step 3: Add `tpm_exceeded` error type to RateLimitTracker**

In `workers/codeforge/routing/rate_tracker.py`, add to `_ERROR_COOLDOWNS` (line 46-50):

Replace:
```python
    _ERROR_COOLDOWNS: ClassVar[dict[str, float]] = {
        "billing": 3600.0,  # 1 hour
        "auth": 300.0,  # 5 minutes
        "rate_limit": 60.0,  # 1 minute
    }
```

With:
```python
    _ERROR_COOLDOWNS: ClassVar[dict[str, float]] = {
        "billing": 3600.0,  # 1 hour
        "auth": 300.0,  # 5 minutes
        "rate_limit": 60.0,  # 1 minute
        "tpm_exceeded": 300.0,  # 5 minutes — tokens-per-minute limit won't shrink
    }
```

**Step 4: Classify TPM errors in `classify_error_type`**

In `workers/codeforge/llm.py`, update `classify_error_type` (around line 82-93):

Add before the `return None` at line 93:
```python
    if exc.status_code == 429 and "tokens per minute" in body:
        return "tpm_exceeded"
```

The full function becomes:
```python
def classify_error_type(exc: LLMError) -> str | None:
    """Classify an LLM error as billing, auth, rate_limit, tpm_exceeded, or None."""
    if exc.status_code == 429:
        body = exc.body.lower()
        if "tokens per minute" in body or "tpm" in body:
            return "tpm_exceeded"
        return "rate_limit"
    if exc.status_code == 402:
        return "billing"
    body = exc.body.lower()
    if any(kw in body for kw in _BILLING_KEYWORDS):
        return "billing"
    if exc.status_code in (401, 403) or any(kw in body for kw in _AUTH_KEYWORDS):
        return "auth"
    return None
```

**Step 5: Run the test**

```bash
cd /workspaces/CodeForge && python -m pytest workers/tests/test_routing_router.py::test_tpm_blocked_model_skipped_in_fallback -v
```
Expected: PASS

**Step 6: Run all related tests**

```bash
cd /workspaces/CodeForge && python -m pytest workers/tests/test_routing_error_classification.py workers/tests/test_routing_router.py -v
```

**Step 7: Commit**

```bash
git add workers/codeforge/routing/rate_tracker.py workers/codeforge/llm.py workers/tests/test_routing_router.py
git commit -m "feat(routing): classify TPM errors separately, skip TPM-limited models for 5 minutes"
```

---

## Task 10: Parse retry-after hints from Gemini error body

**Priority:** Medium | **Effort:** Low | **Layer:** Python Worker (Retry logic)

**NOTE:** This is already partially implemented! The `_parse_retry_after` method in `llm.py:340-369` already parses `retry in <N>s` from the error body. However, the `_compute_backoff` at line 371-376 adds a fixed +5s buffer. Let me verify the actual behavior and improve it.

**Files:**
- Modify: `workers/codeforge/llm.py:371-376` (_compute_backoff)
- Test: `workers/tests/test_llm.py`

**Step 1: Write the failing test**

Add to `workers/tests/test_llm.py`:

```python
def test_compute_backoff_uses_retry_hint_without_excessive_buffer():
    """When Gemini says 'retry in 9.27s', backoff should be ~10-14s, not 32s."""
    from codeforge.llm import LiteLLMClient, LLMError, LLMConfig

    config = LLMConfig(max_retries=5, backoff_base=2.0, backoff_max=90.0)
    client = LiteLLMClient(config=config)

    # Simulate a Gemini 429 with retry hint at attempt 4 (would normally be 2^5=32s).
    exc = LLMError(
        429,
        "gemini/gemini-2.5-flash",
        'retry in 9.273757488s.',
    )
    backoff = client._compute_backoff(exc, attempt=4)
    # Should use the hint (9.27s) + small buffer, NOT the exponential 32s.
    assert backoff < 20.0, f"Backoff {backoff}s is too high, hint was 9.27s"
    assert backoff >= 9.27, f"Backoff {backoff}s is less than the hint"
```

**Step 2: Run test to verify current behavior**

```bash
cd /workspaces/CodeForge && python -m pytest workers/tests/test_llm.py::test_compute_backoff_uses_retry_hint_without_excessive_buffer -v
```

This test may already PASS since `_compute_backoff` does `min(hint + 5.0, backoff_max)` = 14.27s which is <20. If it passes, the existing implementation is fine and we just need to validate.

If it passes, update the test to assert more precisely:
```python
    assert 14.0 <= backoff <= 15.0, f"Expected ~14.27s, got {backoff}s"
```

**Step 3: Run all LLM tests**

```bash
cd /workspaces/CodeForge && python -m pytest workers/tests/test_llm.py -v
```

**Step 4: Commit (test-only if no code change needed)**

```bash
git add workers/tests/test_llm.py
git commit -m "test(llm): verify retry-after hint parsing for Gemini 429 responses"
```

---

## Implementation Order

Execute tasks in this order for maximum safety:

| Phase | Tasks | Risk |
|-------|-------|------|
| 1 | Tasks 1, 2, 3 (skill text changes) | Zero — only Markdown |
| 2 | Task 8 (add gemini to tiers) | Minimal — additive change |
| 3 | Tasks 9, 10 (retry/backoff improvements) | Low — well-tested subsystem |
| 4 | Task 6 (reset in_progress features) | Low — single condition change |
| 5 | Task 4 (automated scoring) | Low — skill text + tooling |
| 6 | Task 5 (post-feature verification) | Medium — new Go code in auto-agent |
| 7 | Task 7 (conversation history persistence) | High — cross-boundary NATS change |

---

## Verification After All Tasks

Run the full agent-eval benchmark again to compare:

```bash
/agent-eval gemini/gemini-2.5-flash
```

Expected improvements:
- LRU Cache should pass >20/25 tests (post-verification retry catches bugs)
- Routing should select gemini-2.5-flash when available
- Fallback chain should skip groq/llama-3.1-8b-instant after first TPM failure
- Total runtime should decrease by ~5-10 minutes (fewer wasted retries)
- Scoring should be reproducible across evaluators
