# WT-13: Tech Debt Cleanup — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Resolve high-priority FIX-* items that can be fixed without API breaking changes, standardize Python logging, and decompose the 3 largest Python files.

**Architecture:** Python module decomposition follows single-responsibility. Logging standardization replaces stdlib `logging` with `structlog`. FIX items addressed per severity.

**Tech Stack:** Python 3.12, structlog, Go 1.25

**Best Practice:**
- Python modules: Split by responsibility, not by size. Each module should have one reason to change.
- structlog: Use bound loggers with context (`structlog.get_logger().bind(component="routing")`).
- NATS consumer pattern: Keep handler mixin entry points in existing files, extract internal logic to sub-modules.

---

### Task 1: Standardize Python Logging (FIX-092)

**Files:**
- Modify: `workers/codeforge/routing/complexity.py`
- Modify: `workers/codeforge/routing/hybrid.py`
- Modify: `workers/codeforge/routing/mab.py`
- Modify: `workers/codeforge/routing/meta_router.py`
- Modify: `workers/codeforge/evaluation/deepeval_runner.py`
- Modify: `workers/codeforge/evaluation/functional_runner.py`

- [ ] **Step 1: Replace `logging.getLogger()` with `structlog.get_logger()` in routing/**

In each file, change:
```python
import logging
logger = logging.getLogger(__name__)
```
to:
```python
import structlog
logger = structlog.get_logger(component="routing")
```

Update all `logger.info/warning/error/debug` calls — structlog uses the same API but with keyword arguments:
```python
# Before:
logger.info(f"Selected model {model} for task")
# After:
logger.info("selected model for task", model=model)
```

- [ ] **Step 2: Same for evaluation/**

- [ ] **Step 3: Run tests**

```bash
cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/test_routing*.py workers/tests/test_evaluation*.py -v
```

- [ ] **Step 4: Commit**

```bash
git add workers/codeforge/routing/ workers/codeforge/evaluation/
git commit -m "fix: standardize Python logging to structlog (FIX-092)"
```

---

### Task 2: Load Missing RoutingConfig Fields (FIX-039)

**Files:**
- Modify: `workers/codeforge/llm.py` (~line 303)

- [ ] **Step 1: Add env var bindings for 9 missing RoutingConfig fields**

Find the RoutingConfig class/dataclass and ensure all fields are loaded from environment variables with sensible defaults.

- [ ] **Step 2: Run tests + commit**

```bash
git add workers/codeforge/llm.py
git commit -m "fix: load all 9 RoutingConfig fields from environment (FIX-039)"
```

---

### Task 3: Replace Broad `Any` Types (FIX-089)

**Files:**
- Modify: `workers/codeforge/tools/_error_handler.py:10`

- [ ] **Step 1: Replace `Callable[..., Any]` with specific async callable type**

```python
from collections.abc import Callable, Awaitable

ToolHandler = Callable[..., Awaitable[str]]
```

- [ ] **Step 2: Commit**

```bash
git add workers/codeforge/tools/_error_handler.py
git commit -m "fix: replace broad Any with specific callable type (FIX-089)"
```

---

### Task 4: Decompose agent_loop.py (1580 LOC -> 4 files)

**Files:**
- Create: `workers/codeforge/stall_detection.py`
- Create: `workers/codeforge/quality_tracking.py`
- Create: `workers/codeforge/loop_helpers.py`
- Modify: `workers/codeforge/agent_loop.py`

- [ ] **Step 1: Extract StallDetector to stall_detection.py**

Move `StallDetector` class (~55 LOC):
```python
# workers/codeforge/stall_detection.py
class StallDetector:
    """Detects repetitive tool call patterns indicating agent stall."""
    # ... move from agent_loop.py
```

- [ ] **Step 2: Extract IterationQualityTracker to quality_tracking.py**

Move `IterationQualityTracker` class + scoring helpers (~180 LOC):
```python
# workers/codeforge/quality_tracking.py
class IterationQualityTracker:
    """Tracks iteration quality scores for rollout selection."""
    # ... move from agent_loop.py

def compute_rollout_score(...): ...
def should_early_stop(...): ...
def select_best_rollout(...): ...
```

- [ ] **Step 3: Extract helper functions to loop_helpers.py**

Move ~300 LOC of helper functions:
```python
# workers/codeforge/loop_helpers.py
def build_tool_result_text(...): ...
def build_correction_hint(...): ...
def build_assistant_message(...): ...
def sanitize_tool_messages(...): ...
def init_plan_act(...): ...
def check_model_switch(...): ...
def check_plan_act_transition(...): ...
def append_system_suffix(...): ...
```

- [ ] **Step 4: Update imports in agent_loop.py**

```python
from codeforge.stall_detection import StallDetector
from codeforge.quality_tracking import IterationQualityTracker, compute_rollout_score
from codeforge.loop_helpers import (
    build_tool_result_text, build_correction_hint, sanitize_tool_messages, ...
)
```

- [ ] **Step 5: Verify agent_loop.py is now ~500 LOC**

- [ ] **Step 6: Run tests**

```bash
.venv/bin/python -m pytest workers/tests/ -v --tb=short
```

- [ ] **Step 7: Commit**

```bash
git add workers/codeforge/
git commit -m "refactor: decompose agent_loop.py into 4 focused modules (1580->~500 LOC)"
```

---

### Task 5: Decompose _conversation.py (1216 LOC -> 4 files)

**Files:**
- Create: `workers/codeforge/consumer/conversation_routing.py`
- Create: `workers/codeforge/consumer/conversation_prompt_builder.py`
- Create: `workers/codeforge/consumer/conversation_skill_integration.py`
- Modify: `workers/codeforge/consumer/_conversation.py`

- [ ] **Step 1: Extract routing logic to conversation_routing.py**

Move: `_get_hybrid_router`, `_get_available_models`, `_build_fallback_chain` (~240 LOC)

- [ ] **Step 2: Extract prompt building to conversation_prompt_builder.py**

Move: `_build_system_prompt`, `_inject_skills`, `_inject_tool_guide`, framework detection maps (~300 LOC)

- [ ] **Step 3: Extract skill integration to conversation_skill_integration.py**

Move: `_wire_skill_tools`, `_make_skill_save_fn`, `_register_handoff_tool`, `_register_propose_goal_tool` (~200 LOC)

- [ ] **Step 4: Update _conversation.py imports**

ConversationHandlerMixin keeps entry points, delegates to extracted modules.

- [ ] **Step 5: Verify _conversation.py is now ~400 LOC**

- [ ] **Step 6: Run tests + commit**

```bash
.venv/bin/python -m pytest workers/tests/consumer/ -v
git add workers/codeforge/consumer/
git commit -m "refactor: decompose _conversation.py into 4 focused modules (1216->~400 LOC)"
```

---

### Task 6: Decompose _benchmark.py (1040 LOC -> 3 files)

**Files:**
- Create: `workers/codeforge/consumer/benchmark_runners.py`
- Create: `workers/codeforge/consumer/benchmark_gemmas.py`
- Modify: `workers/codeforge/consumer/_benchmark.py`

- [ ] **Step 1: Extract benchmark runners to benchmark_runners.py**

Move: `_run_simple`, `_run_tool_use`, `_run_agent`, task callbacks (~300 LOC)

- [ ] **Step 2: Extract GEMMAS evaluation to benchmark_gemmas.py**

Move: `_handle_gemmas_eval`, `_do_gemmas_scoring`, `_build_embed_fn` (~150 LOC)

- [ ] **Step 3: Update _benchmark.py imports**

- [ ] **Step 4: Verify _benchmark.py is now ~250 LOC**

- [ ] **Step 5: Run tests + commit**

```bash
.venv/bin/python -m pytest workers/tests/consumer/test_benchmark*.py -v
git add workers/codeforge/consumer/
git commit -m "refactor: decompose _benchmark.py into 3 focused modules (1040->~250 LOC)"
```
