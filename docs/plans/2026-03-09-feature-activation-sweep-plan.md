# Feature Activation Sweep — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Activate disabled-by-default features, fix Experience Pool gaps (eviction, multi-tenant, agentic loop), enable OTEL, and fix routing default inconsistency.

**Architecture:** Config-level changes (YAML + env-var bindings) are already done. Remaining work is Python-side Experience Pool fixes (3 gaps), a 1-line OTEL activation, and a 1-line routing default fix. The Experience Pool fix touches the Go/Python NATS boundary (tenant_id field).

**Tech Stack:** Python 3.12 (psycopg3, Pydantic), Go (pgx, NATS JetStream), PostgreSQL 18

---

## Task 1: Commit Existing Changes

Already implemented: Context Optimizer enabled, Copilot enabled, 13 env-var bindings, 4 tests, docs.

**Files (already modified):**
- `codeforge.yaml`
- `internal/config/loader.go`
- `internal/config/loader_test.go`
- `docs/dev-setup.md`

**Step 1: Run pre-commit and commit**

```bash
pre-commit run --all-files
git add codeforge.yaml internal/config/loader.go internal/config/loader_test.go docs/dev-setup.md
git commit -m "feat: activate context optimizer + copilot, add 13 missing env-var bindings"
git push
```

---

## Task 2: Experience Pool — Multi-Tenant Fix (Go + Python)

Add `tenant_id` to the NATS conversation payload so the Python worker can use it for tenant-scoped Experience Pool queries.

**Files:**
- Modify: `internal/port/messagequeue/schemas.go:430-451`
- Modify: `workers/codeforge/models.py:427-449`
- Modify: `workers/codeforge/memory/experience.py:26-145`
- Modify: `internal/service/conversation_agent.go:250-270` and `:430-445`
- Modify: `internal/service/conversation.go:250-265`
- Test: `workers/tests/test_memory_system.py`
- Test: `internal/port/messagequeue/contract_test.go`

**Step 1: Write failing test — Python Experience Pool tenant isolation**

In `workers/tests/test_memory_system.py`, add a test that verifies `lookup()` filters by tenant_id:

```python
class TestExperiencePoolTenantIsolation:
    """Verify tenant_id is used in queries."""

    @pytest.mark.asyncio
    async def test_lookup_filters_by_tenant(self, experience_pool_factory):
        pool = experience_pool_factory(tenant_id="tenant-A")
        # Store should use tenant-A, lookup for tenant-B should find nothing
        # (tested via mock cursor assertions on SQL parameters)
```

**Step 2: Run test to verify it fails**

```bash
cd workers && poetry run pytest tests/test_memory_system.py::TestExperiencePoolTenantIsolation -v
```

Expected: FAIL (tenant_id parameter not accepted yet)

**Step 3: Add TenantID to Go NATS payload**

In `internal/port/messagequeue/schemas.go:430`, add field after `ProviderAPIKey`:

```go
type ConversationRunStartPayload struct {
	// ... existing fields ...
	ProviderAPIKey    string                       `json:"provider_api_key,omitempty"`
	TenantID          string                       `json:"tenant_id,omitempty"`           // Tenant isolation for background jobs
	SessionMeta       *SessionMetaPayload          `json:"session_meta,omitempty"`
}
```

**Step 4: Add tenant_id to Python Pydantic model**

In `workers/codeforge/models.py:448`, add field after `provider_api_key`:

```python
class ConversationRunStartMessage(BaseModel):
    # ... existing fields ...
    provider_api_key: str = ""
    tenant_id: str = ""
    session_meta: SessionMetaPayload | None = None
```

**Step 5: Populate TenantID in Go dispatch methods**

In `internal/service/conversation_agent.go`, add `TenantID` to both `SendMessageAgentic` (line ~260) and `SendMessageAgenticWithMode` (line ~438) payload structs:

```go
TenantID:          tenantctx.FromCtx(ctx),
```

And in `internal/service/conversation.go:260` (simple chat path):

```go
TenantID:       tenantctx.FromCtx(ctx),
```

**Step 6: Add tenant_id parameter to ExperiencePool**

In `workers/codeforge/memory/experience.py`, update `__init__`, `lookup`, and `store`:

```python
class ExperiencePool:
    def __init__(
        self,
        db_url: str,
        llm: LiteLLMClient,
        scorer: CompositeScorer | None = None,
        confidence_threshold: float = 0.85,
        max_entries: int = 1000,
        tenant_id: str = "00000000-0000-0000-0000-000000000000",
    ) -> None:
        self._db_url = db_url
        self._llm = llm
        self._scorer = scorer or CompositeScorer()
        self._threshold = confidence_threshold
        self._max_entries = max_entries
        self._tenant_id = tenant_id
```

Update `lookup()` query (line ~56-63) — add tenant filter:

```python
await cur.execute(
    """SELECT id, task_description, task_embedding, result_output,
              result_cost, result_status, confidence, created_at
       FROM experience_entries
       WHERE project_id = %s AND tenant_id = %s
       ORDER BY last_used_at DESC
       LIMIT 200""",
    (project_id, self._tenant_id),
)
```

Update `store()` (line ~129-139) — use instance tenant_id:

```python
(
    self._tenant_id,  # was hardcoded "00000000-..."
    project_id,
    task_desc,
    # ... rest unchanged
),
```

**Step 7: Run tests to verify**

```bash
cd workers && poetry run pytest tests/test_memory_system.py -v
go test ./internal/port/messagequeue/ -v
```

Expected: ALL PASS

**Step 8: Commit**

```bash
git add internal/port/messagequeue/schemas.go workers/codeforge/models.py \
        workers/codeforge/memory/experience.py \
        internal/service/conversation_agent.go internal/service/conversation.go \
        workers/tests/test_memory_system.py internal/port/messagequeue/contract_test.go
git commit -m "fix: add tenant_id to conversation NATS payload and experience pool queries"
git push
```

---

## Task 3: Experience Pool — max_entries Eviction

Add eviction logic to `ExperiencePool.store()` that removes oldest entries when the project exceeds `max_entries`.

**Files:**
- Modify: `workers/codeforge/memory/experience.py:107-145`
- Test: `workers/tests/test_memory_system.py`

**Step 1: Write failing test**

In `workers/tests/test_memory_system.py`, add:

```python
class TestExperiencePoolEviction:
    """Verify max_entries eviction after store."""

    @pytest.mark.asyncio
    async def test_store_evicts_oldest_when_over_limit(self):
        """After store, if entries > max_entries, oldest are deleted."""
        # Build pool with max_entries=2
        pool = _build_pool(max_entries=2)
        # Mock DB to return count=3 after INSERT
        # Assert DELETE query is executed for oldest entry
```

**Step 2: Run test to verify it fails**

```bash
cd workers && poetry run pytest tests/test_memory_system.py::TestExperiencePoolEviction -v
```

**Step 3: Add eviction logic to store()**

In `workers/codeforge/memory/experience.py`, after the INSERT + commit in `store()`, add:

```python
async def store(self, ...) -> str:
    # ... existing INSERT logic ...
    row = await cur.fetchone()
    await conn.commit()
    entry_id = str(row[0]) if row else ""

    # Evict oldest entries if over max_entries limit
    if self._max_entries > 0:
        await cur.execute(
            """DELETE FROM experience_entries
               WHERE id IN (
                   SELECT id FROM experience_entries
                   WHERE project_id = %s AND tenant_id = %s
                   ORDER BY last_used_at ASC
                   LIMIT GREATEST(
                       (SELECT COUNT(*) FROM experience_entries
                        WHERE project_id = %s AND tenant_id = %s) - %s,
                       0
                   )
               )""",
            (project_id, self._tenant_id, project_id, self._tenant_id, self._max_entries),
        )
        await conn.commit()

    logger.info("experience stored", entry_id=entry_id)
    return entry_id
```

**Step 4: Run tests**

```bash
cd workers && poetry run pytest tests/test_memory_system.py -v
```

**Step 5: Commit**

```bash
git add workers/codeforge/memory/experience.py workers/tests/test_memory_system.py
git commit -m "feat: add max_entries eviction to experience pool store"
git push
```

---

## Task 4: Experience Pool — AgentLoopExecutor Integration

Add experience pool cache check (pre-loop) and store (post-loop) to the agentic conversation loop.

**Files:**
- Modify: `workers/codeforge/agent_loop.py:81-101` (constructor) and `:162-235` (run method)
- Modify: `workers/codeforge/consumer/_conversation.py:245-250`
- Modify: `workers/codeforge/consumer/__init__.py:117-120`
- Test: `workers/tests/test_agent_loop.py`

**Step 1: Write failing test**

In `workers/tests/test_agent_loop.py`, add a test that verifies cache lookup is called:

```python
class TestAgentLoopExperiencePool:
    """Experience pool integration in agentic loop."""

    @pytest.mark.asyncio
    async def test_cache_hit_skips_loop(self):
        """If experience pool returns a cached result, loop is skipped."""
        mock_pool = AsyncMock()
        mock_pool.lookup.return_value = {
            "id": "cached-1",
            "result_output": "cached answer",
            "similarity": 0.95,
        }
        executor = AgentLoopExecutor(
            llm=mock_llm,
            tool_registry=mock_registry,
            runtime=mock_runtime,
            workspace_path="/tmp",
            experience_pool=mock_pool,
        )
        # Should return cached result without calling LLM
```

**Step 2: Run test to verify it fails**

```bash
cd workers && poetry run pytest tests/test_agent_loop.py::TestAgentLoopExperiencePool -v
```

Expected: FAIL (experience_pool parameter not accepted)

**Step 3: Add experience_pool parameter to AgentLoopExecutor**

In `workers/codeforge/agent_loop.py:91-101`, add parameter:

```python
def __init__(
    self,
    llm: LiteLLMClient,
    tool_registry: ToolRegistry,
    runtime: RuntimeClient,
    workspace_path: str,
    experience_pool: ExperiencePool | None = None,
) -> None:
    self._llm = llm
    self._tools = tool_registry
    self._runtime = runtime
    self._workspace = workspace_path
    self._experience_pool = experience_pool
```

Add the TYPE_CHECKING import at the top of the file:

```python
if TYPE_CHECKING:
    from codeforge.memory.experience import ExperiencePool
```

**Step 4: Add cache check before loop in run()**

In `workers/codeforge/agent_loop.py`, at the start of `run()` (after line 177, before the for-loop at line 181), add:

```python
cfg = config or LoopConfig()
state = _LoopState(model=cfg.model)

# Check experience pool cache before starting the loop
if self._experience_pool and messages:
    user_prompt = ""
    for msg in reversed(messages):
        if msg.get("role") == "user":
            content = msg.get("content", "")
            user_prompt = content if isinstance(content, str) else str(content)
            break
    if user_prompt:
        try:
            cached = await self._experience_pool.lookup(user_prompt, self._runtime.project_id)
            if cached:
                logger.info(
                    "experience cache hit in agent loop",
                    entry_id=cached["id"],
                    similarity=cached["similarity"],
                )
                state.final_content = cached["result_output"]
                return AgentLoopResult(
                    final_content=state.final_content,
                    tool_messages=[],
                    total_cost=0.0,
                    total_tokens_in=0,
                    total_tokens_out=0,
                    step_count=0,
                    model=cfg.model,
                    error="",
                )
        except Exception as exc:
            logger.warning("experience pool lookup failed, continuing: %s", exc)

tools_array = self._tools.get_openai_tools()
# ... rest of loop ...
```

**Step 5: Add cache store after successful loop completion**

In `workers/codeforge/agent_loop.py`, before the `return AgentLoopResult(...)` at line 226, add:

```python
# Store successful result in experience pool
if self._experience_pool and not state.error and state.final_content:
    user_prompt = ""
    for msg in reversed(messages):
        if msg.get("role") == "user":
            content = msg.get("content", "")
            user_prompt = content if isinstance(content, str) else str(content)
            break
    if user_prompt and hasattr(self._runtime, "project_id"):
        try:
            await self._experience_pool.store(
                task_desc=user_prompt,
                project_id=self._runtime.project_id,
                result_output=state.final_content,
                result_cost=state.total_cost,
                result_status="completed",
                run_id=self._runtime.run_id,
            )
        except Exception as exc:
            logger.warning("experience pool store failed: %s", exc)
```

**Step 6: Pass experience_pool in conversation consumer**

In `workers/codeforge/consumer/_conversation.py:245-250`, add the parameter:

```python
executor = AgentLoopExecutor(
    llm=self._llm,
    tool_registry=registry,
    runtime=runtime,
    workspace_path=run_msg.workspace_path,
    experience_pool=self._experience_pool,
)
```

**Step 7: Run tests**

```bash
cd workers && poetry run pytest tests/test_agent_loop.py -v
```

**Step 8: Commit**

```bash
git add workers/codeforge/agent_loop.py workers/codeforge/consumer/_conversation.py \
        workers/tests/test_agent_loop.py
git commit -m "feat: integrate experience pool into agentic loop (pre-check + post-store)"
git push
```

---

## Task 5: Enable OTEL

Jaeger container already exists in `docker-compose.yml` (profile `dev`). Just activate in config.

**Files:**
- Modify: `codeforge.yaml:140-145`

**Step 1: Update codeforge.yaml**

Change `otel.enabled` from `false` to `true`, update endpoint to point at Jaeger container:

```yaml
otel:
  enabled: true
  endpoint: "jaeger:4317"
  service_name: "codeforge-core"
  insecure: true
  sample_rate: 1.0
```

**Step 2: Verify build**

```bash
go build ./...
```

**Step 3: Commit**

```bash
git add codeforge.yaml
git commit -m "feat: enable OpenTelemetry tracing with Jaeger collector"
git push
```

---

## Task 6: Fix Python Routing Default

Cosmetic fix — `config.py` default is `False` while `llm.py` default is `True`. Make consistent.

**Files:**
- Modify: `workers/codeforge/config.py:140`
- Test: `workers/tests/test_routing_integration.py`

**Step 1: Fix default**

In `workers/codeforge/config.py:140`, change:

```python
# Before:
self.routing_enabled = _resolve_bool("CODEFORGE_ROUTING_ENABLED", routing_cfg.get("enabled"), False)
# After:
self.routing_enabled = _resolve_bool("CODEFORGE_ROUTING_ENABLED", routing_cfg.get("enabled"), True)
```

**Step 2: Run routing tests**

```bash
cd workers && poetry run pytest tests/test_routing_integration.py -v
```

**Step 3: Commit**

```bash
git add workers/codeforge/config.py
git commit -m "fix: align routing default to true in worker config (consistency with llm.py)"
git push
```

---

## Task 7: Update Documentation

**Files:**
- Modify: `docs/todo.md` — mark tasks done, add new items
- Modify: `docs/features/04-agent-orchestration.md` — experience pool agentic integration
- Modify: `docs/dev-setup.md` — OTEL endpoint update, new env vars

**Step 1: Update docs**

- `docs/todo.md`: Mark context optimizer, copilot, env-var bindings as done
- `docs/features/04-agent-orchestration.md`: Add section about experience pool in agentic loop
- `docs/dev-setup.md`: Update OTEL endpoint from `localhost:4317` to `jaeger:4317`

**Step 2: Commit**

```bash
git add docs/
git commit -m "docs: update for feature activation sweep (OTEL, experience pool, env vars)"
git push
```

---

## Execution Order

```
Task 1 (commit existing) → independent
Task 2 (tenant fix)      → must be before Task 3 and 4
Task 3 (eviction)        → after Task 2 (needs tenant_id in queries)
Task 4 (agent loop)      → after Task 2 (needs tenant_id in pool)
Task 5 (OTEL)            → independent
Task 6 (routing default) → independent
Task 7 (docs)            → after all others
```

Parallelizable: Tasks 1, 5, 6 can run in parallel. Tasks 2→3→4 are sequential.
