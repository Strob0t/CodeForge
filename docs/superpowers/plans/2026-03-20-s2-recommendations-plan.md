# S2 Report Recommendations — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 5 issues from the S2 autonomous test report: model override in API, Python pattern microagents (RAG), NATS docs, and full-auto goal gate.

**Architecture:** 3 Go code changes, 2 microagent files, 1 docs update. All independent.

**Tech Stack:** Go 1.25, YAML+Markdown microagents, Markdown docs

**Spec:** `docs/specs/2026-03-20-s2-report-recommendations-design.md`

---

## File Map

| Task | File | Action | Purpose |
|------|------|--------|---------|
| 1 | `internal/domain/conversation/conversation.go` | Modify (line 50-56) | Add `Model` field to SendMessageRequest |
| 1 | `internal/service/conversation_agent.go` | Modify (line 299) | Use `req.Model` with fallback |
| 1 | `internal/service/runtime_coverage_test.go` | Modify | Test model override in NATS payload |
| 2 | `.codeforge/microagents/python_package.md` | Create | Python package structure patterns |
| 3 | `.codeforge/microagents/python_testing.md` | Create | Argparse/pytest testing patterns |
| 4 | `docs/dev-setup.md` | Modify (after line 31) | NATS startup order section |
| 5 | `internal/service/conversation_agent.go` | Modify (line 277-279) | Full-auto goal gate |
| 5 | `internal/service/conversation_agent.go` | Add method | `isFullAutoProject()` helper |
| 5 | `internal/service/conversation_agent_test.go` | Modify | Test goal gate |

---

## Task 1: Add `Model` Field to `SendMessageRequest`

**Problem:** `POST /conversations/{id}/messages` with `"model": "groq/qwen3-32b"` is silently ignored.

**Files:**
- Modify: `internal/domain/conversation/conversation.go:50-56`
- Modify: `internal/service/conversation_agent.go:299`
- Test: `internal/service/runtime_coverage_test.go`

- [ ] **Step 1: Add `Model` field to struct**

In `internal/domain/conversation/conversation.go`, add after line 53 (Mode field):

```go
type SendMessageRequest struct {
	Content string         `json:"content"`
	Agentic *bool          `json:"agentic,omitempty"`
	Mode    string         `json:"mode,omitempty"`
	Model   string         `json:"model,omitempty"` // Optional LLM model override.
	Images  []MessageImage `json:"images,omitempty"`
	UserID  string         `json:"-"`
}
```

- [ ] **Step 2: Use `req.Model` in `SendMessageAgentic`**

In `internal/service/conversation_agent.go`, replace line 299:

```go
// Before:
model := s.resolveModel()

// After:
model := req.Model
if model == "" {
	model = s.resolveModel()
}
```

- [ ] **Step 3: Write test**

Add to `internal/service/runtime_coverage_test.go`. Use the `extRuntimeMockStore` pattern. Send a message with `Model: "test-model-override"` via API, verify the NATS `ConversationRunStartPayload.Model` carries the override.

Since testing the full dispatch requires NATS, write a simpler test: call `SendMessageAgentic` with a mocked queue and verify the published NATS payload contains the override model. Follow the existing patterns in the test file for capturing NATS payloads via `runtimeMockQueue.lastMessage()`.

- [ ] **Step 4: Verify**

```bash
go test ./internal/service/ -run TestSendMessage.*Model -v
go test ./internal/service/ -count=1 -timeout 120s
go build ./cmd/codeforge/
```

- [ ] **Step 5: Commit**

```bash
git add internal/domain/conversation/conversation.go internal/service/conversation_agent.go internal/service/runtime_coverage_test.go
git commit -m "feat(api): add Model field to SendMessageRequest for explicit model override"
```

---

## Task 2: Microagent — Python Package Patterns

**Problem:** Agent didn't create `__main__.py`, breaking `python -m package_name`.

**Files:**
- Create: `.codeforge/microagents/python_package.md`

- [ ] **Step 1: Create microagent file**

Create `.codeforge/microagents/python_package.md`:

```
name: python-package-patterns
type: knowledge
trigger: "__init__.py|setup.py|pyproject.toml"
description: Python package structure and entry point patterns
---
## Python Package Structure

When creating a Python package (directory with `__init__.py`):

1. **Always create `__main__.py`** so `python -m package_name` works:
   ```python
   from .cli import main

   if __name__ == "__main__":
       main()
   ```

2. **Standard package layout:**
   ```
   package_name/
     __init__.py        # Can be empty or contain __version__
     __main__.py        # Entry point for python -m
     cli.py             # CLI logic (argparse)
     models.py          # Data structures
   tests/
     __init__.py
     conftest.py        # Shared fixtures
     test_cli.py
     test_models.py
   ```

3. **Entry points in setup.py / pyproject.toml:**
   ```python
   entry_points={"console_scripts": ["cmd-name=package_name.cli:main"]}
   ```

4. **Common mistake:** Forgetting `__main__.py` means `python -m package_name` fails with `No module named package_name.__main__`.
```

- [ ] **Step 2: Verify loader recognizes the file**

```bash
# Check that the file parses correctly (YAML front matter + markdown body)
python3 -c "
with open('.codeforge/microagents/python_package.md') as f:
    content = f.read()
    parts = content.split('---', 1)
    print('Front matter lines:', len(parts[0].strip().split('\n')))
    print('Body length:', len(parts[1]) if len(parts) > 1 else 0)
    print('Has trigger:', 'trigger:' in parts[0])
"
```

- [ ] **Step 3: Commit**

```bash
git add .codeforge/microagents/python_package.md
git commit -m "feat(microagent): add Python package structure patterns for RAG"
```

---

## Task 3: Microagent — Argparse Testing Patterns

**Problem:** Agent wrote tests calling `main()` without mocking `sys.argv`, causing `SystemExit: 2`.

**Files:**
- Create: `.codeforge/microagents/python_testing.md`

- [ ] **Step 1: Create microagent file**

Create `.codeforge/microagents/python_testing.md`:

```
name: python-testing-patterns
type: knowledge
trigger: "test_*.py|pytest|argparse"
description: Testing patterns for Python CLI tools with argparse
---
## Testing Argparse CLIs with Pytest

**Never call an argparse `main()` directly in tests** without handling `sys.argv`. Argparse reads `sys.argv` and raises `SystemExit` on parse errors.

### Correct patterns:

**Option A -- subprocess (integration tests, recommended):**
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

**Option B -- monkeypatch sys.argv (unit tests):**
```python
def test_add_task(monkeypatch, tmp_path):
    monkeypatch.setattr("sys.argv", ["prog", "add", "--title", "Test"])
    main()  # Now argparse sees the correct argv
```

**Option C -- parse_args directly (parser unit tests):**
```python
def test_parser():
    parser = create_parser()
    args = parser.parse_args(["add", "--title", "Test"])
    assert args.title == "Test"
```

### Common mistakes:
- Calling `main()` without setting `sys.argv` causes `SystemExit: 2`
- Not using `tmp_path` fixture for file I/O causes test pollution
- Forgetting `capture_output=True` in subprocess means no assertion data
- Not isolating JSON storage path causes tests to share state
```

- [ ] **Step 2: Verify loader**

```bash
python3 -c "
with open('.codeforge/microagents/python_testing.md') as f:
    content = f.read()
    parts = content.split('---', 1)
    print('Front matter lines:', len(parts[0].strip().split('\n')))
    print('Body length:', len(parts[1]) if len(parts) > 1 else 0)
    print('Has trigger:', 'trigger:' in parts[0])
"
```

- [ ] **Step 3: Commit**

```bash
git add .codeforge/microagents/python_testing.md
git commit -m "feat(microagent): add argparse/pytest testing patterns for RAG"
```

---

## Task 4: Document NATS Startup Order in dev-setup.md

**Problem:** Stale Go consumers after NATS purge silently drop toolcall requests. This is undocumented in `docs/dev-setup.md`.

**Files:**
- Modify: `docs/dev-setup.md` (after line 31, after the Quick Start paragraph)

- [ ] **Step 1: Add startup order section**

Insert after line 31 (after "pre-configured in `devcontainer.json` with no manual setup needed."):

```markdown

### Critical Startup Order (Manual / Outside Devcontainer)

When starting services manually (not via `setup.sh`), follow this **strict order**.
Violating the order causes silent NATS message drops (toolcall requests timeout
after 30s with no error in the logs).

1. **Docker services:** `docker compose up -d postgres nats litellm`
2. **Purge NATS** (fresh test runs only): Kill Go backend + Python worker **first**,
   then purge the JetStream stream. Stale consumers from killed processes block new ones.
3. **Go backend:** `APP_ENV=development go run ./cmd/codeforge/`
   - MUST start **after** NATS purge -- creates fresh JetStream consumers on startup
   - Verify: `curl http://localhost:8080/health` returns `{"status":"ok"}`
4. **Python worker:** Start with container IPs (see WSL2 section in CLAUDE.md)
   - MUST start **after** Go backend -- both sides need active consumers
5. **Frontend:** `cd frontend && npm run dev`

**Common pitfall:** A stale Go process (e.g. VSCode debug binary) holds old NATS
consumers that silently fail after a stream purge. Kill ALL Go processes before
purging: `ps aux | grep codeforge | grep -v grep`
```

- [ ] **Step 2: Verify markdown renders**

```bash
head -60 docs/dev-setup.md
```

- [ ] **Step 3: Commit**

```bash
git add docs/dev-setup.md
git commit -m "docs(dev-setup): add critical NATS startup order section"
```

---

## Task 5: Full-Auto Gate — Require Open Goals or Roadmap Todos

**Problem:** In full-auto mode, agent skips goal discovery and produces lower quality results. When no goals or roadmap items exist, it should redirect to `goal_researcher` mode to collaborate with the user on goal definition first.

**Files:**
- Modify: `internal/service/conversation_agent.go:277-279` (insert gate)
- Modify: `internal/service/conversation_agent.go` (add `isFullAutoProject` helper)
- Test: `internal/service/conversation_agent_test.go` or `runtime_coverage_test.go`

- [ ] **Step 1: Write failing test**

Add to the conversation/runtime test file. Test that when:
- Project has `policy_preset: "trusted-mount-autonomous"` (full-auto)
- Project has 0 goals and no roadmap
- User sends a message via `SendMessageAgentic`

Then: the NATS payload is dispatched with mode `"goal_researcher"` (not default `"coder"`).

Use the existing `extRuntimeMockStore` + `runtimeMockQueue` patterns. Capture the NATS published payload and verify `Mode.ID == "goal_researcher"`.

- [ ] **Step 2: Run test, verify it fails**

```bash
go test ./internal/service/ -run TestFullAutoGate -v -timeout 30s
```

Expected: FAIL (no gate exists yet)

- [ ] **Step 3: Add `isFullAutoProject` helper**

Add to `internal/service/conversation_agent.go` (before `SendMessageAgentic`):

```go
// isFullAutoProject checks if the project's policy profile uses an auto-allow mode
// (ModeAcceptEdits or ModeDelegate), meaning HITL is bypassed and the agent runs autonomously.
func (s *ConversationService) isFullAutoProject(ctx context.Context, proj *project.Project) bool {
	if s.policySvc == nil {
		return false
	}
	preset := proj.PolicyProfile
	if preset == "" {
		if p, ok := proj.Config["policy_preset"]; ok {
			preset = p
		}
	}
	if preset == "" {
		return false
	}
	profile, ok := s.policySvc.GetProfile(preset)
	if !ok {
		return false
	}
	return profile.Mode == policy.ModeAcceptEdits || profile.Mode == policy.ModeDelegate
}
```

Add imports for `policy` package if not already present.

- [ ] **Step 4: Insert goal gate in `SendMessageAgentic`**

In `internal/service/conversation_agent.go`, insert after line 277 (after `proj, err := ...`) and before line 279 (session handling):

```go
	// Full-auto gate: if no goals or open roadmap features exist, redirect to
	// goal_researcher mode so the agent collaborates with the user on goals first.
	if s.isFullAutoProject(ctx, proj) && s.goalSvc != nil {
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
				if hasOpenFeatures {
					break
				}
			}
		}

		if !hasOpenGoals && !hasOpenFeatures {
			slog.Info("full-auto gate: no goals or open features, redirecting to goal_researcher",
				"project_id", proj.ID,
				"conversation_id", conversationID,
			)
			return s.SendMessageAgenticWithMode(ctx, conversationID, req.Content, "goal_researcher")
		}
	}
```

Add import: `"github.com/Strob0t/CodeForge/internal/domain/roadmap"`

- [ ] **Step 5: Run test, verify it passes**

```bash
go test ./internal/service/ -run TestFullAutoGate -v -timeout 30s
```

- [ ] **Step 6: Add edge case tests**

Add tests for:
- Full-auto project WITH goals -> normal execution (no redirect)
- Non-full-auto project with 0 goals -> no redirect (gate skipped)
- Full-auto project with open roadmap features but 0 goals -> no redirect (features sufficient)

- [ ] **Step 7: Full regression**

```bash
go test ./internal/service/ -count=1 -timeout 120s
go build ./cmd/codeforge/
```

- [ ] **Step 8: Commit**

```bash
git add internal/service/conversation_agent.go internal/service/conversation_agent_test.go
git commit -m "feat(agent): full-auto gate redirects to goal_researcher when no goals/features exist"
```

---

## Verification Checklist

After all 5 tasks:

- [ ] `go test ./internal/service/ -count=1 -timeout 120s` — all pass
- [ ] `go build ./cmd/codeforge/` — compiles
- [ ] `ls .codeforge/microagents/` — shows python_package.md + python_testing.md
- [ ] `head -60 docs/dev-setup.md` — shows startup order section
- [ ] Manual: POST message with `"model": "test"` -> NATS payload has model `"test"`
- [ ] Manual: full-auto project + 0 goals -> agent enters goal_researcher mode

---

## Task Dependency Graph

```
Task 1 (Model field)       ──┐
Task 2 (Package microagent) ──┤── all independent
Task 3 (Testing microagent) ──┤
Task 4 (NATS docs)          ──┘

Task 5 (Full-auto gate)    ── independent (uses policySvc + goalSvc already wired)
```
