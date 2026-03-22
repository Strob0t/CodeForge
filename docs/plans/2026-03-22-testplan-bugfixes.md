# Multi-Language Testplan Bug Fixes — Implementation Plan

> **For agentic workers:** Steps use checkbox (`- [ ]`) syntax for tracking. Execute task-by-task, commit after each task.

**Goal:** Fix 4 bugs discovered during the first autonomous multi-language testplan run (Run 1, Mode A, qwen3-30b).

**Spec:** `docs/testing/2026-03-22-multi-language-autonomous-report.md` (Bug List)

**Research basis:**
- Plan/Act tool filtering: Roo Code pattern (mode-specific tool groups), Cline pattern (hardcoded blocklist)
- Framework detection: Netlify `framework-info` pattern, Cline `.clinerules`, Cursor `.mdc` rules
- Post-write validation: SWE-agent `edit_linting.sh`, Aider tree-sitter lint, PostToolUse hook pattern

---

## File Structure

| File | Action | Purpose |
|---|---|---|
| `workers/codeforge/plan_act.py` | Modify | Fix #1: Accept mode-specific extra plan tools |
| `workers/codeforge/consumer/_conversation.py` | Modify | Fix #1: Pass mode tools to PlanActController |
| `internal/service/conversation_agent.go` | Modify | Fix #2: Include frameworks in stack summary |
| `workers/codeforge/tools/write_file.py` | Modify | Fix #3: Post-write syntax check |
| `workers/codeforge/tools/edit_file.py` | Modify | Fix #3: Post-write syntax check |
| `workers/codeforge/tools/_lint.py` | Create | Fix #3: Shared per-language lint/syntax runner |
| `workers/codeforge/plan_act_test.py` | Modify | Tests for Fix #1 |
| `internal/service/conversation_agent_test.go` | Modify | Tests for Fix #2 |
| `workers/codeforge/tools/test_lint.py` | Create | Tests for Fix #3 |

---

## Fix #1: Plan/Act respects mode-specific tools (Bug #2)

**Root cause:** `PLAN_TOOLS` in `plan_act.py` is a hardcoded frozenset. `goal_researcher` mode declares `propose_goal` in its `Tools` list, but the `PlanActController` ignores mode tool config. Result: `propose_goal` is blocked in PLAN phase.

**Pattern:** Roo Code — mode declares which tool groups are available; "always available" tools exist per mode. The simplest adaptation: `PlanActController.__init__` accepts `extra_plan_tools` from the mode's declared Tools list.

### Task 1.1: Add extra_plan_tools to PlanActController

**Files:**
- Modify: `workers/codeforge/plan_act.py`

- [ ] **Step 1: Write failing test**

```python
# In workers/codeforge/plan_act_test.py (or existing test file)
def test_mode_extra_plan_tools_allowed():
    """Mode-declared tools should be allowed in PLAN phase."""
    ctrl = PlanActController(enabled=True, extra_plan_tools=frozenset({"propose_goal", "write_file"}))
    assert ctrl.is_tool_allowed("read_file") is True    # base PLAN tool
    assert ctrl.is_tool_allowed("propose_goal") is True  # mode extra tool
    assert ctrl.is_tool_allowed("bash") is False          # still blocked

def test_no_extra_plan_tools_default():
    """Without extra tools, behavior is unchanged."""
    ctrl = PlanActController(enabled=True)
    assert ctrl.is_tool_allowed("propose_goal") is False
    assert ctrl.is_tool_allowed("read_file") is True
```

Run: `cd workers && python -m pytest codeforge/plan_act_test.py -v`
Expected: FAIL (extra_plan_tools parameter doesn't exist yet)

- [ ] **Step 2: Implement extra_plan_tools in PlanActController**

Modify `workers/codeforge/plan_act.py`:
- Add `extra_plan_tools` parameter to `__init__` (default: `frozenset()`)
- Store as `self.extra_plan_tools`
- Update `is_tool_allowed()`: `return tool_name.lower() in PLAN_TOOLS or tool_name in self.extra_plan_tools`
- Update `__slots__` to include `extra_plan_tools`
- Update `get_system_suffix()` PLAN message to mention extra tools if present

- [ ] **Step 3: Run tests — verify PASS**

Run: `cd workers && python -m pytest codeforge/plan_act_test.py -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add workers/codeforge/plan_act.py workers/codeforge/plan_act_test.py
git commit -m "fix: plan/act controller accepts mode-specific extra plan tools (Bug #2)"
```

### Task 1.2: Pass mode tools from consumer to PlanActController

**Files:**
- Modify: `workers/codeforge/consumer/_conversation.py`

- [ ] **Step 1: Find where PlanActController is created**

Search for `PlanActController` in `_conversation.py` — it's created when building the `AgentLoopExecutor`.

- [ ] **Step 2: Extract mode tools and pass as extra_plan_tools**

```python
# When creating PlanActController, extract mode's allowed tools:
mode_tools = frozenset(run_msg.mode.tools) if run_msg.mode and run_msg.mode.tools else frozenset()
plan_act = PlanActController(
    enabled=run_msg.plan_act_enabled,
    extra_plan_tools=mode_tools,
)
```

- [ ] **Step 3: Verify mode.tools field exists in ModeConfig model**

Check `workers/codeforge/models.py` — `ModeConfig` should have `tools: list[str]`.

- [ ] **Step 4: Run integration test — verify propose_goal works in goal_researcher PLAN phase**

Manual verification: start services, set mode to goal_researcher, send a message, check that propose_goal is NOT blocked.

- [ ] **Step 5: Commit**

```bash
git add workers/codeforge/consumer/_conversation.py
git commit -m "fix: pass mode tools to PlanActController as extra plan tools"
```

---

## Fix #2: Include frameworks in stack summary (Bug #4+6)

**Root cause:** `detectStackSummary()` in `conversation_agent.go:1098-1111` only returns `lang.Name` (e.g. "typescript"), discarding `lang.Frameworks` (e.g. `["solidjs"]`). The agent never sees the framework in its context.

**Pattern:** Netlify `framework-info` — detect framework from package.json deps, expose as structured data. CodeForge already does the detection (in `stackmap.go`), it just discards the result.

### Task 2.1: Include frameworks in detectStackSummary

**Files:**
- Modify: `internal/service/conversation_agent.go`
- Test: `internal/service/conversation_agent_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestDetectStackSummary_IncludesFrameworks(t *testing.T) {
    // Create temp workspace with package.json containing solid-js
    dir := t.TempDir()
    os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{
        "dependencies": {"solid-js": "^1.9.0"}
    }`), 0644)

    summary, err := detectStackSummary(dir)
    require.NoError(t, err)
    assert.Contains(t, summary, "typescript")
    assert.Contains(t, summary, "solidjs")
}
```

Run: `go test ./internal/service/ -run TestDetectStackSummary_IncludesFrameworks -v`
Expected: FAIL (summary only contains "typescript")

- [ ] **Step 2: Modify detectStackSummary to include frameworks**

```go
func detectStackSummary(workspacePath string) (string, error) {
    result, err := project.ScanWorkspace(workspacePath)
    if err != nil {
        return "", err
    }
    if len(result.Languages) == 0 {
        return "", nil
    }
    var parts []string
    for _, lang := range result.Languages {
        if len(lang.Frameworks) > 0 {
            parts = append(parts, fmt.Sprintf("%s (%s)", lang.Name, strings.Join(lang.Frameworks, ", ")))
        } else {
            parts = append(parts, lang.Name)
        }
    }
    return strings.Join(parts, ", "), nil
}
```

Result: `"typescript (solidjs)"` instead of `"typescript"`.

- [ ] **Step 3: Run test — verify PASS**

Run: `go test ./internal/service/ -run TestDetectStackSummary -v`
Expected: PASS

- [ ] **Step 4: Run linters**

```bash
gofmt -w internal/service/conversation_agent.go
goimports -w internal/service/conversation_agent.go
go vet ./internal/service/
```

- [ ] **Step 5: Commit**

```bash
git add internal/service/conversation_agent.go internal/service/conversation_agent_test.go
git commit -m "fix: include detected frameworks in agent stack context (Bug #4+6)"
```

---

## Fix #3: Post-write syntax check (Bug #5 + new feature)

**Root cause (Bug #5):** Local model generated `</n` instead of `\n` — the write_file tool wrote it verbatim. No validation caught it.

**Pattern:** SWE-agent `edit_linting.sh` — run syntax check after every edit, reject + restore if it fails. Aider — tree-sitter lint after every edit, report errors to LLM. The emerging PostToolUse hook pattern: lint after file write, feed errors back as tool result.

**Approach:** Add a lightweight `post_write_check()` that runs the appropriate syntax checker based on file extension. Append warnings to the tool result so the LLM sees them and can self-correct. Do NOT reject the write (files may be incomplete during multi-step creation).

### Task 3.1: Create shared lint runner module

**Files:**
- Create: `workers/codeforge/tools/_lint.py`
- Create: `workers/codeforge/tools/test_lint.py`

- [ ] **Step 1: Write failing test**

```python
# workers/codeforge/tools/test_lint.py
import pytest
from codeforge.tools._lint import post_write_check

def test_valid_python():
    result = post_write_check("test.py", "def hello():\n    return 42\n")
    assert result is None  # no errors

def test_invalid_python_syntax():
    result = post_write_check("test.py", "def hello()\n    return 42\n")
    assert result is not None
    assert "SyntaxError" in result or "syntax" in result.lower()

def test_unknown_extension_skips():
    result = post_write_check("test.xyz", "gibberish{{{")
    assert result is None  # unknown extensions are skipped

def test_html_escape_in_python():
    """The </n artifact from local models should trigger syntax error."""
    result = post_write_check("test.py", "x = 1</n    y = 2\n")
    assert result is not None
```

Run: `cd workers && python -m pytest codeforge/tools/test_lint.py -v`
Expected: FAIL (module doesn't exist)

- [ ] **Step 2: Implement _lint.py**

```python
"""Post-write syntax checking for agent-generated files.

Runs a lightweight, language-specific syntax check after write_file/edit_file.
Returns None if OK, or an error string if syntax issues are detected.
Does NOT block the write — appends warnings to tool result for LLM self-correction.

Pattern: SWE-agent edit_linting.sh + Aider tree-sitter lint.
"""
from __future__ import annotations

import ast
import logging
import shutil
import subprocess
from pathlib import Path

logger = logging.getLogger(__name__)

# Map file extensions to checker functions.
# Each checker returns None (OK) or an error string.
_CHECKERS: dict[str, callable] = {}


def post_write_check(file_path: str, content: str) -> str | None:
    """Run syntax check for the given file. Returns None or error string."""
    ext = Path(file_path).suffix.lower()
    checker = _CHECKERS.get(ext)
    if checker is None:
        return None
    try:
        return checker(content, file_path)
    except Exception as exc:
        logger.debug("post_write_check failed for %s: %s", file_path, exc)
        return None


def _check_python(content: str, file_path: str) -> str | None:
    """Python syntax check via ast.parse (stdlib, zero deps)."""
    try:
        ast.parse(content, filename=file_path)
        return None
    except SyntaxError as e:
        return f"SyntaxError at line {e.lineno}: {e.msg}"


def _check_typescript(content: str, file_path: str) -> str | None:
    """TypeScript/JavaScript syntax check via tsc --noEmit if available."""
    # Only check if tsc is available (don't fail if not installed)
    if not shutil.which("npx"):
        return None
    # For TS/JS, we can't easily check a single file's syntax without a project.
    # Instead, check for obvious issues: mismatched braces, unclosed strings.
    # Full tsc check happens in Phase 7 (quality verification).
    return None


def _check_go(content: str, file_path: str) -> str | None:
    """Go syntax check — only if go is available."""
    if not shutil.which("go"):
        return None
    # go vet requires a full module; skip for individual file check.
    return None


# Register checkers
_CHECKERS[".py"] = _check_python
_CHECKERS[".pyw"] = _check_python
```

- [ ] **Step 3: Run tests — verify PASS**

Run: `cd workers && python -m pytest codeforge/tools/test_lint.py -v`
Expected: PASS

- [ ] **Step 4: Run ruff on new file**

```bash
cd workers && ruff check codeforge/tools/_lint.py && ruff format codeforge/tools/_lint.py
```

- [ ] **Step 5: Commit**

```bash
git add workers/codeforge/tools/_lint.py workers/codeforge/tools/test_lint.py
git commit -m "feat: post-write syntax check module for agent file operations"
```

### Task 3.2: Integrate post_write_check into write_file and edit_file

**Files:**
- Modify: `workers/codeforge/tools/write_file.py`
- Modify: `workers/codeforge/tools/edit_file.py`

- [ ] **Step 1: Add post_write_check to write_file.py**

After `target.write_text(content, encoding="utf-8")` (line 65), add:

```python
from codeforge.tools._lint import post_write_check

# ... inside execute(), after write_text:
lint_warning = post_write_check(rel, content)
output_msg = f"wrote {len(content)} bytes to {rel}"
if lint_warning:
    output_msg += f"\n\n⚠ Syntax warning: {lint_warning}\nPlease review and fix the syntax error."
```

- [ ] **Step 2: Add post_write_check to edit_file.py**

After the replacement is written, add the same check on the new file content.

- [ ] **Step 3: Test manually — write a Python file with syntax error via agent**

Verify the tool result includes the syntax warning.

- [ ] **Step 4: Run ruff on modified files**

```bash
cd workers && ruff check codeforge/tools/write_file.py codeforge/tools/edit_file.py
ruff format codeforge/tools/write_file.py codeforge/tools/edit_file.py
```

- [ ] **Step 5: Commit**

```bash
git add workers/codeforge/tools/write_file.py workers/codeforge/tools/edit_file.py
git commit -m "feat: integrate post-write syntax check into write_file and edit_file tools"
```

---

## Fix #4: Prompt improvements (Bug #7, #8)

**Root cause:** The coder mode prompt doesn't explicitly instruct the agent to write tests for all languages or commit to git. Local models need more explicit instructions.

### Task 4.1: Enhance coder mode prompt

**Files:**
- Modify: `internal/service/prompts/modes/coder.yaml`

- [ ] **Step 1: Add explicit testing and git instructions**

Append to the coder mode prompt:

```yaml
## Completion Checklist
- Write tests for EVERY language in the project (pytest for Python, vitest/jest for TypeScript, go test for Go)
- Run all tests before considering the task complete
- Commit all changes to git with a descriptive message
- If you detect a framework (e.g. SolidJS, Vue, Angular), use that framework's patterns — NOT React patterns unless React is actually used
```

- [ ] **Step 2: Run gofmt and linters**

The prompt file is YAML, no Go linting needed. But verify YAML is valid:
```bash
python -c "import yaml; yaml.safe_load(open('internal/service/prompts/modes/coder.yaml'))"
```

- [ ] **Step 3: Commit**

```bash
git add internal/service/prompts/modes/coder.yaml
git commit -m "fix: add testing/git/framework checklist to coder mode prompt (Bug #7, #8)"
```

---

## Task Summary

| Task | Bug | Description | Language | Files |
|---|---|---|---|---|
| 1.1 | #2 | PlanActController extra_plan_tools | Python | plan_act.py |
| 1.2 | #2 | Pass mode tools from consumer | Python | _conversation.py |
| 2.1 | #4+6 | detectStackSummary includes frameworks | Go | conversation_agent.go |
| 3.1 | #5+new | Post-write syntax check module | Python | _lint.py |
| 3.2 | #5+new | Integrate into write_file/edit_file | Python | write_file.py, edit_file.py |
| 4.1 | #7+8 | Coder prompt enhancements | YAML | coder.yaml |
| **Total** | | **6 tasks, ~24 steps** | | |
