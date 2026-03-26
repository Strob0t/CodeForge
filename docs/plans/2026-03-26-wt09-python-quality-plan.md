# Worktree 9: refactor/python-quality — Python Type Safety + DRY + Tests

**Branch:** `refactor/python-quality`
**Priority:** Hoch
**Scope:** 8 findings (F-QUA-002, F-QUA-003, F-QUA-004, F-QUA-015, F-QUA-005, F-QUA-006, F-QUA-012, F-QUA-013)
**Estimated effort:** Large (1-2 weeks)

## Research Summary

- Kraken Engineering: 2.5-year typing journey on 10M+ LOC monorepo — gradual strictness ratchet
- Wolt: professional-grade mypy config with per-module overrides
- Template Method pattern (ABC) for CLI backend executors — 65% code reduction
- TypedDict for known-shape dicts, Pydantic for external API data
- `typing_copilot` for automated baseline + periodic tightening

## Steps (in dependency order)

### Phase 1: Base class extraction (biggest DRY win)

**F-QUA-002: CLIBackendExecutor base class**

Create `workers/codeforge/backends/_cli_base.py`:
```python
class CLIBackendExecutor(ABC):
    def __init__(self, cli_path: str | None, env_var: str, default_cmd: str) -> None: ...

    @property
    @abstractmethod
    def info(self) -> BackendInfo: ...

    @abstractmethod
    def _build_command(self, prompt: str, config: ExecutorConfig) -> list[str]: ...

    async def check_available(self) -> bool: ...      # shared
    async def execute(self, ...) -> TaskResult: ...    # shared (~50 lines)
    async def cancel(self, task_id: str) -> None: ...  # shared
```

Each backend becomes ~30 lines (info + _build_command only).

Also define `ExecutorConfig(TypedDict)` to replace `dict[str, Any]` in all backends:
```python
class ExecutorConfig(TypedDict, total=False):
    timeout: int
    model: str
    extra_args: list[str]
    extra_env: dict[str, str]
    working_dir_override: str
```

### Phase 2: Type annotations on agent loop

**F-QUA-003: Annotate critical methods**

```python
async def _do_llm_iteration(
    self, cfg: LoopConfig, tools_array: list[dict[str, object]],
    messages: list[dict[str, object]], state: _LoopState,
    iteration: int, plan_act: PlanActManager | None = None,
    error_tracker: ToolErrorTracker | None = None,
) -> str | None: ...
```

**F-QUA-005: Split `_execute_tool_call` (130 LOC)**

Extract:
- `_publish_tool_trajectory_event(tc, result_text, success, elapsed_ms, step)` — eliminates F-QUA-006 (3x duplication)
- Permission-denied early return into `_handle_permission_denied(tc, state)`

### Phase 3: Systematic Any removal

**F-QUA-004: Replace dict[str, Any] across 26 files**

Priority order:
1. `ExecutorConfig` TypedDict (done in Phase 1)
2. Per-tool `XxxArgs(TypedDict)` for tool arguments
3. `ToolResult.diff: DiffInfo` dataclass instead of `dict[str, Any]`
4. `RateLimitInfo(TypedDict)` in `llm.py` (eliminates 4 type-ignores)
5. OpenHands HTTP payloads → Pydantic models

### Phase 4: Test coverage + cleanup

**F-QUA-015: Agent loop unit tests**

Create `workers/tests/test_agent_loop_execution.py`:
- Test `AgentLoopExecutor.run()` with mock `LiteLLMClient`
- Test cases: stop on text-only, tool execution loop, cost limit, max iterations, cancellation, fallback model, stall detection

**F-QUA-012:** Add `ToolRegistry.iter_executors()` public method
**F-QUA-013:** Update test imports to `from codeforge.loop_helpers import ...`, remove 14 aliases

### Phase 5: Tooling setup

Add to `pyproject.toml`:
```toml
[tool.mypy]
python_version = "3.12"
warn_return_any = true
check_untyped_defs = true

[[tool.mypy.overrides]]
module = ["codeforge.agent_loop", "codeforge.llm", "codeforge.tools.*", "codeforge.backends.*"]
disallow_untyped_defs = true
disallow_any_explicit = true
```

Add `"ANN"` to Ruff select (ignore `ANN101`, `ANN102`).

## Verification

- `mypy --strict` passes on core modules
- `ruff check` passes with ANN rules
- Backend executors: all 5 backends pass existing test suite
- Agent loop: new test file with >80% coverage on execution paths
- Zero new `dict[str, Any]` in modified files

## Sources

- [Kraken: Static Typing Python at Scale](https://engineering.kraken.tech/news/2026/02/16/static-typing-python-at-scale.html)
- [Wolt: Professional-Grade mypy Config](https://careers.wolt.com/en/blog/tech/professional-grade-mypy-configuration)
- [Refactoring.guru: Template Method in Python](https://refactoring.guru/design-patterns/template-method/python/example)
- [TypedDicts Are Better Than You Think](https://blog.changs.co.uk/typeddicts-are-better-than-you-think.html)
- [PEP 589: TypedDict](https://peps.python.org/pep-0589/)
