# WT5: Python Cleanup — Stale TODO Removal & Stub Evaluation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove stale FIX-092 TODO and evaluate whether StubBackendExecutor should remain.

**Architecture:** Minimal cleanup — remove outdated comments, verify no production code depends on StubBackendExecutor.

**Tech Stack:** Python 3.12

---

### Task 1: Remove stale FIX-092 TODO

**Files:**
- Modify: `workers/codeforge/consumer/__init__.py:7-9`

**Context:** FIX-092 claims routing and evaluation modules still use `logging.getLogger()`. Research confirmed routing is 100% structlog (5/5 files) and evaluation is 100% structlog (5/5 root files). The TODO is stale.

- [ ] **Step 1: Remove the stale TODO**

Replace the module docstring (lines 1-9) in `workers/codeforge/consumer/__init__.py`:

```python
"""NATS JetStream consumer for receiving tasks from Go Core.

The TaskConsumer is composed from handler mixins — each mixin owns a
related group of NATS message handlers.  The ``main()`` entry point
at the bottom starts the consumer.
"""
```

(Remove lines 7-9 containing the FIX-092 TODO)

- [ ] **Step 2: Verify import works**

Run: `cd workers && python -c "import codeforge.consumer"`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add workers/codeforge/consumer/__init__.py
git commit -m "chore(FIX-092): remove stale logging standardization TODO (already done)"
```

---

### Task 2: Evaluate StubBackendExecutor

**Files:**
- Read: `workers/codeforge/backends/_base.py:90-120`

**Context:** StubBackendExecutor is an ABC providing default `execute()` (returns "not yet implemented") and `cancel()` (no-op). Research confirmed no production backend subclasses it — all 6 registered backends have full implementations.

- [ ] **Step 1: Check for subclasses**

Search for any class that inherits from `StubBackendExecutor`:

```bash
cd workers && grep -r "StubBackendExecutor" --include="*.py" -l
```

Expected: Only `_base.py` and possibly test files.

- [ ] **Step 2: Decision**

If no production subclasses exist:
- **Keep it.** It's a documented template for future backends. It costs nothing and provides a clear pattern for contributors.
- Add a clarifying comment:

```python
class StubBackendExecutor(ABC):
    """Template base class for backends not yet implemented in CodeForge.

    Not currently subclassed by any production backend. Provides sensible
    defaults for ``execute()`` (error message) and ``cancel()`` (no-op)
    so new backend stubs can be added with minimal boilerplate.
    """
```

- [ ] **Step 3: Commit (only if comment was updated)**

```bash
git add workers/codeforge/backends/_base.py
git commit -m "docs: clarify StubBackendExecutor purpose as template for future backends"
```

---

### Task 3: Update docs/todo.md

**Files:**
- Modify: `docs/todo.md`

- [ ] **Step 1: Mark FIX-092 as completed**

Find the FIX-092 entry and mark `[x]` with date `2026-03-24`.

- [ ] **Step 2: Commit**

```bash
git add docs/todo.md
git commit -m "docs: mark FIX-092 as completed"
```
