# S2 Report Recommendations — Design Spec

**Date:** 2026-03-20
**Source:** `docs/testing/2026-03-20-autonomous-s2-task-manager-report.md` Recommendations section
**Scope:** 5 independent fixes improving autonomous agent execution quality

---

## Overview

The S2 autonomous test run exposed 5 issues. This spec defines fixes for each.

---

## Rec 1: Add `Model` Field to `SendMessageRequest`

**Problem:** `POST /conversations/{id}/messages` with `"model": "groq/qwen3-32b"` is silently ignored. The model is resolved server-side via `resolveModel()` with no way for API callers to override.

**Fix:**

Add `Model string` to `SendMessageRequest` (conversation.go:50-56). In `SendMessageAgentic()` (conversation_agent.go:299), use request model first, fall back to `resolveModel()`:

```go
// conversation.go — SendMessageRequest struct
Model string `json:"model,omitempty"` // Optional model override

// conversation_agent.go — SendMessageAgentic, after line 277
model := req.Model
if model == "" {
    model = s.resolveModel()
}
```

**Files:**
- `internal/domain/conversation/conversation.go:50` — add field
- `internal/service/conversation_agent.go:299` — use `req.Model` with fallback

**Tests:**
- Existing `TestConversationToolCall_*` tests in `runtime_coverage_test.go` remain valid (they don't set Model)
- Add 1 test: send message with explicit model, verify NATS payload carries it

---

## Rec 2: Microagent — Python Package Patterns

**Problem:** Agent created a Python package without `__main__.py`, breaking `python -m package_name`. This is a common pattern that should be surfaced via RAG when the agent works on Python packages.

**Fix:** Create a microagent that triggers on Python package creation context.

**File:** `internal/service/prompts/microagents/python-package.md`

```yaml
name: python-package-patterns
type: knowledge
trigger: "*.py"
description: Python package structure patterns and best practices
---
## Python Package Structure

When creating a Python package (directory with `__init__.py`):

1. **Always create `__main__.py`** so `python -m package_name` works:
   ```python
   from .cli import main

   if __name__ == "__main__":
       main()
   ```

2. **Package layout:**
   ```
   package_name/
     __init__.py        # Can be empty or contain version
     __main__.py        # Entry point for python -m
     cli.py             # CLI logic (argparse)
     models.py          # Data structures
   ```

3. **setup.py / pyproject.toml entry points:**
   ```python
   entry_points={"console_scripts": ["cmd-name=package_name.cli:main"]}
   ```
```

**Trigger:** `*.py` — broad trigger, injected when agent works with Python files. The microagent system filters by relevance via BM25 scoring, so this only surfaces when contextually relevant (package creation, CLI setup).

**No Go code changes.** Microagent loader picks up new `.md` files from the directory automatically.

---

## Rec 3: Microagent — Argparse Testing Patterns

**Problem:** Agent wrote pytest tests that call `main()` directly without mocking `sys.argv`, causing `SystemExit: 2` on all 4 tests.

**Fix:** Create a microagent for Python testing patterns.

**File:** `internal/service/prompts/microagents/python-testing.md`

```yaml
name: python-testing-patterns
type: knowledge
trigger: "test_*.py"
description: Python testing patterns for CLI tools and argparse
---
## Testing Argparse CLIs with Pytest

**Never call argparse main() directly in tests** — it reads `sys.argv` and raises `SystemExit`.

### Correct patterns:

**Option A — subprocess (recommended for CLI integration tests):**
```python
import subprocess

def test_add_task(tmp_path):
    result = subprocess.run(
        ["python", "-m", "package_name", "add", "--title", "Test"],
        capture_output=True, text=True, cwd=str(tmp_path)
    )
    assert result.returncode == 0
    assert "Added" in result.stdout
```

**Option B — monkeypatch sys.argv (for unit tests):**
```python
def test_add_task(monkeypatch, tmp_path):
    monkeypatch.setattr("sys.argv", ["prog", "add", "--title", "Test"])
    main()  # Now argparse sees correct argv
```

**Option C — parse_args directly (for parser unit tests):**
```python
def test_parser():
    parser = create_parser()
    args = parser.parse_args(["add", "--title", "Test"])
    assert args.title == "Test"
```

### Common mistakes:
- Calling `main()` without setting `sys.argv` -> `SystemExit: 2`
- Not using `tmp_path` fixture for file-based tests -> test pollution
- Forgetting `capture_output=True` in subprocess -> no assertion data
```

**Trigger:** `test_*.py` — only surfaces when agent creates or modifies test files.

---

## Rec 4: NATS Startup Order in dev-setup.md

**Problem:** `docs/dev-setup.md` doesn't document the critical startup sequence. Stale Go consumers after NATS purge silently drop toolcall requests (30s timeout, no NATS error).

**Fix:** Add a "Critical Startup Order" section to `docs/dev-setup.md`.

**Content to add:**

```markdown
## Critical Startup Order

Services MUST be started in this order. Violating the order causes silent NATS
message drops (toolcall requests timeout after 30s with no error).

1. **Docker services:** `docker compose up -d postgres nats litellm`
2. **Purge NATS** (if fresh test run): Kill Go backend + Python worker first,
   then purge stream. Stale consumers from killed processes block new ones.
3. **Go backend:** `APP_ENV=development go run ./cmd/codeforge/`
   - MUST start AFTER NATS purge — it creates fresh JetStream consumers on startup
   - Verify: `curl http://localhost:8080/health` returns `{"status":"ok"}`
4. **Python worker:** Start with container IPs (see WSL2 section)
   - MUST start AFTER Go backend — both sides need active consumers
5. **Frontend:** `cd frontend && npm run dev`

**Common pitfall:** A stale Go process (e.g., VSCode debug binary) holds old NATS
consumers that don't match the purged stream. Kill ALL Go processes before purging.
Check with: `ps aux | grep codeforge | grep -v grep`
```

**File:** `docs/dev-setup.md` — insert after the Quick Start section

---

## Rec 5: Full-Auto Gate — Require Open Goals or Roadmap Todos

**Problem:** When user sends "build X" in full-auto mode without defining goals first, the agent skips goal discovery and produces lower quality results (no structured plan, no roadmap tracking).

**Fix:** In `SendMessageAgentic()`, before dispatching the agent loop, check if the project has open goals or open roadmap features. If neither exists, automatically redirect to `goal_researcher` mode so the agent collaborates with the user on goal definition first.

**Location:** `internal/service/conversation_agent.go`, insert after project fetch (line 277) and before session creation (line 282).

**Logic:**

```go
// After: proj, err := s.db.GetProject(ctx, conv.ProjectID)
// Before: session handling

// Full-auto gate: require goals or open roadmap items before autonomous execution.
if s.goalSvc != nil && s.isFullAutoProject(ctx, proj) {
    goals, _ := s.goalSvc.ListEnabled(ctx, proj.ID)
    hasOpenGoals := len(goals) > 0

    hasOpenFeatures := false
    if rm, rmErr := s.db.GetRoadmapByProject(ctx, proj.ID); rmErr == nil && rm != nil {
        for _, ms := range rm.Milestones {
            for _, f := range ms.Features {
                if f.Status != roadmap.FeatureDone && f.Status != roadmap.FeatureCancelled {
                    hasOpenFeatures = true
                    break
                }
            }
            if hasOpenFeatures { break }
        }
    }

    if !hasOpenGoals && !hasOpenFeatures {
        slog.Info("full-auto gate: no goals or open features, redirecting to goal_researcher",
            "project_id", proj.ID, "conversation_id", conversationID)
        return s.SendMessageAgenticWithMode(ctx, conversationID, req.Content, "goal_researcher")
    }
}
```

**Helper method:**

```go
func (s *ConversationService) isFullAutoProject(ctx context.Context, proj *project.Project) bool {
    preset := proj.PolicyProfile
    if preset == "" {
        preset = proj.Config["policy_preset"]
    }
    profile, ok := s.policy.GetProfile(preset)
    if !ok { return false }
    return profile.Mode == policy.ModeAcceptEdits || profile.Mode == policy.ModeDelegate
}
```

**Behavior:**
- User sends "build a task manager" in full-auto project with no goals
- Gate detects: 0 goals, 0 open features
- Redirects to `goal_researcher` mode
- Agent collaborates with user: asks questions, proposes goals via `propose_goal`
- Goals auto-persist (our earlier fix)
- User sends next message -> gate passes (goals exist now) -> normal execution

**Edge cases:**
- Non-full-auto projects: gate skipped (supervised mode always has human guidance)
- Goals exist but no roadmap: gate passes (goals sufficient)
- Roadmap exists with open features but no goals: gate passes (features sufficient)
- `goalSvc` is nil: gate skipped (graceful degradation)

**Files:**
- `internal/service/conversation_agent.go:277-282` — insert gate
- `internal/service/conversation_agent.go` — add `isFullAutoProject()` helper
- `internal/domain/roadmap/roadmap.go` — import for `FeatureDone`, `FeatureCancelled` constants

**Tests:**
- Test: full-auto project with 0 goals -> verify `goal_researcher` mode dispatched
- Test: full-auto project with 1+ goals -> verify normal execution
- Test: non-full-auto project with 0 goals -> verify no redirect (gate skipped)

---

## Task Dependency Graph

```
Rec 1 (Model field)     ──┐
Rec 2 (Python microagent) ──┤── all independent
Rec 3 (Testing microagent) ──┤
Rec 4 (NATS docs)         ──┘

Rec 5 (Full-auto gate)  ── depends on Rec 1 bugfix batch (goalSvc wired to RuntimeService)
                            but NOT on Recs 1-4 above
```

All 5 tasks are independent and can be implemented in parallel.
