# Dead Code Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove orphaned classes (MemoryStore, legacy BenchmarkRunner) and relocate misplaced test files.

**Architecture:** Direct deletion of unused modules, update of __init__.py exports, relocation of test files, cleanup of stale comments/docstrings.

**Tech Stack:** Python, pytest

---

### Task 1: Remove MemoryStore class and associated dead code

**Files:**
- Delete: `workers/codeforge/memory/storage.py`
- Edit: `workers/codeforge/memory/__init__.py`
- Delete: `workers/tests/test_memory_tenant.py`
- Edit: `workers/tests/conftest.py`
- Edit: `workers/codeforge/memory/embedding.py`

- [ ] **Step 1: Delete MemoryStore module**

Delete `workers/codeforge/memory/storage.py` (143 lines, never instantiated).

- [ ] **Step 2: Update __init__.py exports**

Edit `workers/codeforge/memory/__init__.py`:
- Remove line 6: `from codeforge.memory.storage import MemoryStore`
- Remove `"MemoryStore",` from `__all__`

Result:
```python
"""Persistent agent memory with composite scoring (semantic + recency + importance)."""

from codeforge.memory.embedding import compute_embedding
from codeforge.memory.models import Memory, ScoredMemory, ScoreWeights
from codeforge.memory.scorer import CompositeScorer

__all__ = [
    "CompositeScorer",
    "Memory",
    "ScoreWeights",
    "ScoredMemory",
    "compute_embedding",
]
```

- [ ] **Step 3: Delete orphaned test file**

Delete `workers/tests/test_memory_tenant.py` (63 lines, only tests MemoryStore via inspect.getsource()).

- [ ] **Step 4: Clean up stale references**

Edit `workers/tests/conftest.py` line 10:
- Remove: `  - codeforge/memory/storage.py (MemoryStore: store, recall, embedding edge cases)`

Edit `workers/codeforge/memory/embedding.py`:
- Line 3: `Extracted from MemoryStore and ExperiencePool` -> `Extracted to a shared helper`
- Line 22: `Shared helper used by MemoryStore and ExperiencePool` -> `Shared helper used by ExperiencePool and memory consumer`

- [ ] **Step 5: Verify and commit**

```bash
grep -r "MemoryStore" workers/ --include="*.py" | grep -v "MemoryStoreRequest" | grep -v __pycache__
# Should return 0 results
.venv/bin/python -m pytest workers/tests/ -x -q 2>&1 | tail -20
```

```
refactor: remove dead MemoryStore class and associated tests
```

---

### Task 2: Remove legacy BenchmarkRunner and its test file

**Files:**
- Delete: `workers/codeforge/evaluation/runner.py`
- Delete: `workers/tests/test_evaluation_runner.py`

- [ ] **Step 1: Delete legacy runner**

Delete `workers/codeforge/evaluation/runner.py` (142 lines, superseded by runners/ hierarchy).

- [ ] **Step 2: Delete orphaned test file**

Delete `workers/tests/test_evaluation_runner.py` (184 lines, only tests legacy BenchmarkRunner).

- [ ] **Step 3: Verify and commit**

```bash
grep -r "from codeforge.evaluation.runner import" workers/ --include="*.py" | grep -v __pycache__
# Should return 0 results
.venv/bin/python -m pytest workers/tests/test_benchmark_runners.py workers/tests/test_base_benchmark_runner.py -x -q
```

```
refactor: remove legacy BenchmarkRunner superseded by runners/ hierarchy
```

---

### Task 3: Relocate misplaced test files from tools/ to tests/

**Files:**
- Move: `workers/codeforge/tools/test_diff_output.py` -> `workers/tests/test_tool_diff_output.py`
- Move: `workers/codeforge/tools/test_lint.py` -> `workers/tests/test_tool_lint.py`

- [ ] **Step 1: Move test files**

```bash
mv workers/codeforge/tools/test_diff_output.py workers/tests/test_tool_diff_output.py
mv workers/codeforge/tools/test_lint.py workers/tests/test_tool_lint.py
```

- [ ] **Step 2: Verify relocated tests pass**

```bash
.venv/bin/python -m pytest workers/tests/test_tool_diff_output.py workers/tests/test_tool_lint.py -v 2>&1 | tail -30
```

- [ ] **Step 3: Commit**

```
refactor: relocate misplaced test files from tools/ to tests/
```

---

### Task 4: Full verification

- [ ] **Step 1: Run full test suite**

```bash
.venv/bin/python -m pytest workers/tests/ -x -q 2>&1 | tail -30
```

- [ ] **Step 2: Run ruff linter**

```bash
.venv/bin/ruff check workers/codeforge/memory/ workers/codeforge/evaluation/
```

- [ ] **Step 3: Verify no dangling references**

```bash
grep -r "MemoryStore[^R]" workers/ --include="*.py" | grep -v __pycache__
grep -r "from codeforge.evaluation.runner " workers/ --include="*.py" | grep -v __pycache__
ls workers/codeforge/tools/test_diff_output.py workers/codeforge/tools/test_lint.py 2>&1
```
All should return empty or "not found".

---

### Summary

| Action | File | Reason |
|--------|------|--------|
| DELETE | `workers/codeforge/memory/storage.py` | Dead code: never instantiated |
| EDIT | `workers/codeforge/memory/__init__.py` | Remove MemoryStore export |
| DELETE | `workers/tests/test_memory_tenant.py` | Tests only dead MemoryStore |
| EDIT | `workers/tests/conftest.py` | Remove stale TODO reference |
| EDIT | `workers/codeforge/memory/embedding.py` | Update stale docstrings |
| DELETE | `workers/codeforge/evaluation/runner.py` | Dead code: superseded by runners/ |
| DELETE | `workers/tests/test_evaluation_runner.py` | Tests only dead BenchmarkRunner |
| MOVE | `tools/test_diff_output.py` -> `tests/test_tool_diff_output.py` | Never discovered by pytest |
| MOVE | `tools/test_lint.py` -> `tests/test_tool_lint.py` | Never discovered by pytest |

**Risk:** Low. All deleted code confirmed dead (zero production imports). 15 relocated tests now actually run.
