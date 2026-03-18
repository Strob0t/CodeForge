# Claude Code Integration as Routing Target with Execution Branch

**Date:** 2026-03-18
**Status:** Draft (Rev 2 — post spec-review)
**Scope:** Python Worker (new executor + policy callback), Routing config

## Problem

CodeForge users with a Claude Code subscription (Pro/Max) cannot use Claude's capabilities through LiteLLM because the subscription and the Claude API are separate products with separate billing. LiteLLM requires API keys — a Claude Code subscription provides none.

Claude Code is a full autonomous agent (own tool loop, context management, MCP support), not a simple LLM completion endpoint. It cannot be wrapped as a LiteLLM Custom Provider without fundamental impedance mismatch: CodeForge's `AgentLoopExecutor` expects structured `tool_calls` responses that it executes itself, but Claude Code executes tools internally.

## Goals

1. Enable Claude Code subscription users to leverage Claude's capabilities within CodeForge
2. Integrate Claude Code into the existing Hybrid Routing cascade so MAB can learn when to prefer it
3. Maintain policy enforcement over Claude Code's tool execution via `can_use_tool` callback
4. Return `AgentLoopResult` so the rest of the pipeline (completion publishing, cost tracking, WebSocket events) works unchanged
5. Support fallback: if Claude Code is unavailable, fall back to LiteLLM models seamlessly

## Non-Goals

- Replacing LiteLLM — it remains the primary LLM gateway for all API-based models
- Wrapping Claude Code behind LiteLLM's `/v1/chat/completions` interface
- Supporting Claude Code for simple (non-agentic) chat — only agentic conversation runs
- Disabling Claude Code's built-in tools or re-implementing them via MCP

## Existing Infrastructure

| Component | Location | Relevance |
|---|---|---|
| Hybrid Router | `workers/codeforge/routing/router.py` | Selects model string, feeds into execution |
| `COMPLEXITY_DEFAULTS` | `router.py:28` | Tier-to-model preference lists |
| `_execute_conversation_run()` | `_conversation.py:234` | Branch point: simple chat vs agentic loop |
| `AgentLoopExecutor` | `agent_loop.py` | Current agentic executor (LiteLLM-based) |
| `AgentLoopResult` | `models.py` | Common return type for all executors |
| Tool Registry | `workers/codeforge/tools/` | 7 built-in tools (Read, Write, Edit, Bash, Search, Glob, ListDir) |
| Policy Layer | `internal/service/policy.go` | First-match-wins permission rules |
| RuntimeClient | `workers/codeforge/runtime.py` | Tool-call policy NATS round-trip, output streaming, cancellation |
| Routing Outcomes | `POST /api/v1/routing/outcomes` | MAB learning feedback loop |

## Design

### Part 1: Routing Integration

Claude Code appears as a model candidate in the routing system using the provider prefix `claudecode/`:

```
claudecode/default              -> Claude Code with its default model
```

Additional model aliases (e.g., `claudecode/claude-sonnet-4`) can be added later by stripping the prefix and passing the remainder to the SDK's `model` parameter. Start with `default` only.

#### COMPLEXITY_DEFAULTS Update

Add `claudecode/default` to COMPLEX and REASONING tiers:

```python
COMPLEXITY_DEFAULTS = {
    ComplexityTier.SIMPLE: [
        # unchanged — Claude Code is overkill for simple tasks
    ],
    ComplexityTier.MEDIUM: [
        # unchanged
    ],
    ComplexityTier.COMPLEX: [
        "claudecode/default",           # NEW — first choice if available
        "github_copilot/gpt-4o",
        "anthropic/claude-sonnet-4",
        "openai/gpt-4o",
        "gemini/gemini-2.5-pro",
    ],
    ComplexityTier.REASONING: [
        "claudecode/default",           # NEW — first choice if available
        "github_copilot/o3-mini",
        "anthropic/claude-opus-4.6",
        "openai/gpt-4o",
        "gemini/gemini-2.5-pro",
    ],
}
```

#### Model Availability

Claude Code models are detected via CLI probe, not via LiteLLM `/v1/models`. The `_get_available_models()` method in `_conversation.py` is extended:

```python
async def _get_available_models(self) -> list[str]:
    models = await self._get_litellm_models()  # existing logic

    # Probe Claude Code CLI availability (respects enabled flag)
    if await is_claude_code_available():
        models.append("claudecode/default")

    return models
```

`is_claude_code_available()` checks `CODEFORGE_CLAUDECODE_ENABLED` first, then runs `claude --version` and caches the result (TTL: 5 minutes, protected by `asyncio.Lock`).

#### MAB Integration

No changes to MAB. It already works with arbitrary model strings:
- `model_name: "claudecode/default"` in `model_performance_stats`
- UCB1 scoring works identically
- Routing outcomes recorded via existing `POST /api/v1/routing/outcomes`

#### Fallback Behavior

If Claude Code is selected but fails (CLI not found, timeout, error), the existing fallback chain kicks in. The next model in the chain is a LiteLLM model — seamless degradation.

```
claudecode/default (fails) -> anthropic/claude-sonnet-4 (LiteLLM) -> openai/gpt-4o (LiteLLM)
```

### Part 2: Execution Branch

The branch point is `_execute_conversation_run()` in `_conversation.py:234`. Currently it dispatches to either `_run_simple_chat()` or `AgentLoopExecutor.run()`. A third path is added for Claude Code:

```python
async def _execute_conversation_run(self, run_msg, messages, primary_model, routing,
                                     runtime, registry, fallback_models) -> AgentLoopResult:
    if not run_msg.agentic:
        return await self._run_simple_chat(...)

    # NEW: Claude Code execution path
    if primary_model.startswith("claudecode/"):
        from codeforge.claude_code_executor import ClaudeCodeExecutor

        executor = ClaudeCodeExecutor(
            workspace_path=run_msg.workspace_path,
            runtime=runtime,
        )
        result = await executor.run(
            messages=messages,
            model=primary_model,
            max_turns=run_msg.termination.max_steps or 50,
            system_prompt=run_msg.system_prompt,
        )
        # If Claude Code failed and we have fallbacks, try LiteLLM path
        if result.error and fallback_models:
            next_model = fallback_models[0]
            remaining = fallback_models[1:]
            await runtime.send_output(
                f"\n[Claude Code unavailable. Switching to {next_model}]\n"
            )
            return await self._execute_litellm_loop(
                run_msg, messages, next_model, routing, runtime, registry, remaining
            )
        return result

    # Existing: LiteLLM agentic loop
    return await self._execute_litellm_loop(
        run_msg, messages, primary_model, routing, runtime, registry, fallback_models
    )
```

The existing agentic loop code moves into `_execute_litellm_loop()` (extract method, no behavior change).

### Part 3: ClaudeCodeExecutor

New file: `workers/codeforge/claude_code_executor.py`

#### Responsibilities

1. Invoke Claude Code via `claude-code-sdk` Python package (or CLI fallback)
2. Enforce CodeForge's policy layer via `can_use_tool` callback (Claude Code keeps its built-in tools)
3. Stream output to NATS via `RuntimeClient` and emit AG-UI events
4. Parse token usage and cost from `ResultMessage`
5. Support cancellation via `RuntimeClient.is_cancelled`
6. Return `AgentLoopResult` with the same shape as `AgentLoopExecutor`

#### Interface

```python
class ClaudeCodeExecutor:
    def __init__(
        self,
        workspace_path: str,
        runtime: RuntimeClient,
    ) -> None: ...

    async def run(
        self,
        messages: list[dict],
        model: str = "claudecode/default",
        max_turns: int = 50,
        system_prompt: str = "",
    ) -> AgentLoopResult: ...

    async def cancel(self) -> None: ...
```

#### Implementation Strategy: SDK-First with CLI Fallback

**Primary: `claude-code-sdk` Python package**

The SDK's `query()` yields high-level `Message` objects (not raw streaming events):
- `AssistantMessage(content: list[TextBlock | ToolUseBlock | ToolResultBlock])` — agent output
- `ResultMessage(total_cost_usd, usage, num_turns, ...)` — final summary with cost/token data
- `SystemMessage(subtype, data)` — system events

```python
from claude_code_sdk import query, ClaudeCodeOptions, PermissionResultAllow, PermissionResultDeny

async def run(self, messages, model, max_turns, system_prompt) -> AgentLoopResult:
    prompt = self._format_messages_as_prompt(messages)

    tokens_in = 0
    tokens_out = 0
    cost_usd = 0.0
    content_parts: list[str] = []
    tool_messages: list[dict] = []
    step_count = 0
    start_time = time.monotonic()

    # Emit AG-UI run_started
    await self._runtime.send_ag_ui_event("run_started", {"model": model})

    try:
        async for message in query(
            prompt=prompt,
            options=ClaudeCodeOptions(
                system_prompt=system_prompt,
                max_turns=max_turns,
                permission_mode="bypassPermissions",
                can_use_tool=self._make_policy_callback(),
            ),
        ):
            # Check cancellation on each message
            if self._runtime.is_cancelled:
                self._cancelled = True
                break

            await self._handle_message(
                message, content_parts, tool_messages,
            )

    except Exception as exc:
        await self._runtime.send_ag_ui_event("run_finished", {"error": str(exc)})
        return AgentLoopResult(
            final_content="",
            error=f"Claude Code failed: {exc}",
            model=model,
            step_count=step_count,
        )

    # Extract cost/tokens from the final ResultMessage
    # (handled inside _handle_message when ResultMessage is received)

    elapsed_ms = int((time.monotonic() - start_time) * 1000)
    await self._runtime.send_ag_ui_event("run_finished", {})

    return AgentLoopResult(
        final_content="".join(content_parts),
        tool_messages=tool_messages,
        total_cost=self._cost_usd,
        total_tokens_in=self._tokens_in,
        total_tokens_out=self._tokens_out,
        step_count=self._step_count,
        model=model,
    )
```

**Fallback: CLI subprocess** (if SDK not installed)

```python
async def _run_via_cli(self, prompt, system_prompt, max_turns) -> AgentLoopResult:
    cmd = [
        "claude", "-p", prompt,
        "--output-format", "stream-json",
        "--max-turns", str(max_turns),
    ]
    if system_prompt:
        cmd.extend(["--system-prompt", system_prompt])

    proc = await asyncio.create_subprocess_exec(
        *cmd, stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE,
        cwd=self._workspace_path,
    )
    self._process = proc  # for cancel()

    # Parse stream-json lines from stdout
    # CLI emits raw Anthropic SSE events, unlike the SDK's high-level Messages
    # Parse content_block_start/delta/stop, message_delta for tokens
```

#### Policy Enforcement via `can_use_tool` Callback

Instead of disabling built-in tools and routing through MCP, Claude Code keeps its own tools. Policy enforcement happens via the SDK's native `can_use_tool` callback, which runs in-process in the Python worker with access to `RuntimeClient`:

```python
def _make_policy_callback(self):
    """Create a can_use_tool callback that enforces CodeForge's policy layer."""
    runtime = self._runtime

    async def policy_check(tool_name: str, tool_input: dict) -> PermissionResult:
        # Map Claude Code tool names to CodeForge policy categories
        tool_category = _MAP_TOOL_TO_POLICY.get(tool_name, f"claudecode:{tool_name}")

        # Extract context for policy check (e.g., file path, command)
        command = ""
        path = ""
        if tool_name == "Bash":
            command = tool_input.get("command", "")
        elif tool_name in ("Read", "Write", "Edit", "MultiEdit"):
            path = tool_input.get("file_path", "")

        # NATS round-trip: Python -> Go Core -> (maybe user approval) -> response
        decision = await runtime.request_tool_call(
            tool=tool_category,
            command=command,
            path=path,
        )

        if decision.decision == "approve":
            # Emit AG-UI tool_call event
            await runtime.send_ag_ui_event("tool_call_start", {
                "tool": tool_name,
                "input": tool_input,
            })
            return PermissionResultAllow()

        return PermissionResultDeny(message=decision.reason or "Denied by policy")

    return policy_check

# Tool name -> policy category mapping
_MAP_TOOL_TO_POLICY: dict[str, str] = {
    "Bash": "command:execute",
    "Read": "file:read",
    "Write": "file:write",
    "Edit": "file:edit",
    "MultiEdit": "file:edit",
    "Search": "file:read",
    "Glob": "file:read",
    "ListDir": "file:read",
}
```

This approach:
- Keeps Claude Code's battle-tested built-in tool implementations
- Runs in the Python worker process (has access to RuntimeClient + NATS)
- Supports HITL: `runtime.request_tool_call()` can wait for user approval via WebSocket
- Uses `permission_mode="bypassPermissions"` so Claude Code delegates all decisions to CodeForge

#### Message Formatting

Claude Code expects a single prompt string, not a messages array. The executor preserves the full conversation history including all user messages:

```python
def _format_messages_as_prompt(self, messages: list[dict]) -> str:
    """Convert chat messages to a single prompt for Claude Code.

    System messages are excluded (passed via --system-prompt instead).
    All user and assistant messages are preserved in order to maintain
    multi-turn context.
    """
    non_system = [m for m in messages if m["role"] != "system"]
    if not non_system:
        return ""

    # If only one user message, use it directly
    user_messages = [m for m in non_system if m["role"] == "user"]
    if len(non_system) == 1:
        return non_system[0].get("content", "")

    # Multi-turn: preserve full conversation structure
    parts: list[str] = []
    for m in non_system[:-1]:
        role = m["role"].upper()
        parts.append(f"[{role}]: {m.get('content', '')}")

    # Last message is the current request (no role prefix)
    last = non_system[-1]
    context = "\n\n".join(parts)
    return (
        f"<conversation_history>\n{context}\n"
        f"</conversation_history>\n\n{last.get('content', '')}"
    )
```

#### Message Handling (SDK Path)

The SDK yields high-level `Message` objects, not raw SSE events:

```python
from claude_code_sdk import AssistantMessage, ResultMessage, SystemMessage

async def _handle_message(self, message, content_parts, tool_messages):
    if isinstance(message, AssistantMessage):
        for block in message.content:
            if hasattr(block, "text"):
                # TextBlock — stream to frontend
                content_parts.append(block.text)
                await self._runtime.send_output(block.text)

            elif hasattr(block, "name") and hasattr(block, "id"):
                # ToolUseBlock — record for conversation history
                self._step_count += 1
                tool_messages.append({
                    "role": "assistant",
                    "content": "",
                    "tool_calls": [{
                        "id": block.id,
                        "type": "function",
                        "function": {
                            "name": block.name,
                            "arguments": json.dumps(block.input) if block.input else "{}",
                        },
                    }],
                })
                await self._runtime.send_ag_ui_event("tool_call", {
                    "id": block.id,
                    "name": block.name,
                })

            elif hasattr(block, "tool_use_id"):
                # ToolResultBlock — record result
                content = block.content if isinstance(block.content, str) else str(block.content)
                tool_messages.append({
                    "role": "tool",
                    "tool_call_id": block.tool_use_id,
                    "content": content,
                })
                await self._runtime.send_ag_ui_event("tool_result", {
                    "id": block.tool_use_id,
                    "output": content[:500],  # truncate for WS
                })

    elif isinstance(message, ResultMessage):
        # Final message with cost and token data
        self._cost_usd = message.total_cost_usd or 0.0
        if message.usage:
            self._tokens_in = getattr(message.usage, "input_tokens", 0)
            self._tokens_out = getattr(message.usage, "output_tokens", 0)
        self._step_count = message.num_turns or self._step_count
```

#### Cancellation

```python
async def cancel(self) -> None:
    """Cancel the running Claude Code execution."""
    self._cancelled = True
    # CLI fallback: terminate subprocess
    if self._process is not None and self._process.returncode is None:
        from codeforge.subprocess_utils import graceful_terminate
        await graceful_terminate(self._process)
```

The SDK path checks `self._runtime.is_cancelled` on each yielded message and breaks out of the async generator. The CLI fallback terminates the subprocess.

### Part 4: Cost and Token Tracking

#### From ResultMessage

The SDK's `ResultMessage` provides cost and token data directly:

```python
elif isinstance(message, ResultMessage):
    self._cost_usd = message.total_cost_usd or 0.0  # may be None for subscription
    self._tokens_in = message.usage.input_tokens if message.usage else 0
    self._tokens_out = message.usage.output_tokens if message.usage else 0
```

#### Cost Estimation for MAB

Since Claude Code calls go through the subscription (not pay-per-token), `total_cost_usd` may be `0.0` or `None`. For MAB comparison with paid models, we estimate the equivalent API cost:

```python
def _estimate_equivalent_cost(self, tokens_in: int, tokens_out: int) -> float:
    """Estimate what the equivalent API cost would be for MAB comparison."""
    from codeforge.pricing import resolve_cost
    # resolve_cost(litellm_cost, model, tokens_in, tokens_out)
    return resolve_cost(0.0, "anthropic/claude-sonnet-4", tokens_in, tokens_out)
```

This gives MAB a fair comparison: if Claude Code uses more tokens but produces better results, it can still win on quality. If it is slower, the latency penalty balances it.

#### Routing Outcome Recording

After execution, outcomes are recorded identically to LiteLLM runs:

```python
estimated_cost = self._estimate_equivalent_cost(result.total_tokens_in, result.total_tokens_out)

await _record_routing_outcome(
    model="claudecode/default",
    task_type=routing.task_type,
    complexity_tier=routing.complexity_tier,
    success=not result.error,
    cost_usd=estimated_cost,
    latency_ms=elapsed_ms,
    tokens_in=result.total_tokens_in,
    tokens_out=result.total_tokens_out,
    routing_layer=routing.routing_layer,
    run_id=runtime.run_id,
)
```

### Part 5: AG-UI Event Emission

The existing `AgentLoopExecutor` emits AG-UI events that the frontend renders (tool call cards, step indicators, cost breakdowns). `ClaudeCodeExecutor` emits the same events so the frontend experience is consistent:

| AG-UI Event | When | Data |
|---|---|---|
| `run_started` | Executor starts | `{model}` |
| `step_started` | New tool call begins | `{tool, step_number}` |
| `tool_call` | Tool use block received | `{id, name}` |
| `tool_result` | Tool result block received | `{id, output}` |
| `state_delta` | Cost/token update | `{cost_usd, tokens_in, tokens_out}` |
| `run_finished` | Executor completes | `{error?}` |

Events are emitted via `runtime.send_ag_ui_event()` which publishes to NATS -> Go Core -> WebSocket -> Frontend.

### Part 6: Configuration

#### Environment Variables

| Variable | Default | Purpose |
|---|---|---|
| `CODEFORGE_CLAUDECODE_ENABLED` | `false` | Enable Claude Code as routing target |
| `CODEFORGE_CLAUDECODE_PATH` | `claude` | Path to Claude Code CLI binary |
| `CODEFORGE_CLAUDECODE_MAX_TURNS` | `50` | Default max turns per run |
| `CODEFORGE_CLAUDECODE_TIMEOUT` | `300` | Timeout in seconds per run |
| `CODEFORGE_CLAUDECODE_TIERS` | `COMPLEX,REASONING` | Which complexity tiers include Claude Code |
| `CODEFORGE_CLAUDECODE_MAX_CONCURRENT` | `5` | Max parallel Claude Code runs per worker |

#### Routing Config Extension

```yaml
# routing.yaml
routing:
  enabled: true
  claude_code:
    enabled: true
    tiers: [COMPLEX, REASONING]
    model_alias: "claudecode/default"
    max_concurrent: 5
```

### Part 7: Availability Detection

Claude Code availability is checked at worker startup and cached. Protected by `asyncio.Lock` to prevent concurrent modification, and gated by `CODEFORGE_CLAUDECODE_ENABLED`:

```python
import asyncio
import os
import time

_cache_lock = asyncio.Lock()
_claude_code_available: bool | None = None
_claude_code_check_time: float = 0.0
_CACHE_TTL = 300.0  # 5 minutes


async def is_claude_code_available() -> bool:
    global _claude_code_available, _claude_code_check_time

    # Feature gate: skip probe if disabled
    if os.environ.get("CODEFORGE_CLAUDECODE_ENABLED", "false").lower() != "true":
        return False

    async with _cache_lock:
        now = time.monotonic()
        if _claude_code_available is not None and (now - _claude_code_check_time) < _CACHE_TTL:
            return _claude_code_available

        cli_path = os.environ.get("CODEFORGE_CLAUDECODE_PATH", "claude")
        try:
            proc = await asyncio.create_subprocess_exec(
                cli_path, "--version",
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
            )
            await asyncio.wait_for(proc.wait(), timeout=5.0)
            _claude_code_available = proc.returncode == 0
        except (OSError, TimeoutError):
            _claude_code_available = False

        _claude_code_check_time = now
        return _claude_code_available
```

## Data Flow Summary

```
User Message
    |
    v
Go Core: SendMessageAgentic()
    |  builds payload, publishes to NATS
    v
Python Worker: _handle_conversation_run()
    |
    v
resolve_model_with_routing()
    |  ComplexityAnalyzer -> MAB -> MetaRouter -> fallback
    |  result: "claudecode/default"
    v
_execute_conversation_run()
    |  primary_model.startswith("claudecode/") == True
    v
ClaudeCodeExecutor.run()
    |  1. Format messages as prompt
    |  2. Start claude-code-sdk query() with:
    |     - permission_mode="bypassPermissions"
    |     - can_use_tool callback (policy enforcement)
    |     - max_turns, system_prompt
    |  3. Emit AG-UI run_started
    v
Claude Code Agent Loop (uses subscription)
    |  Claude Code wants to use a tool (e.g., Edit file):
    |    -> can_use_tool callback fires
    |      -> runtime.request_tool_call() (NATS round-trip to Go Core)
    |        -> Go Core checks policy
    |        -> If HITL needed: WebSocket -> User approves/denies
    |      -> PermissionResultAllow or PermissionResultDeny
    |    -> Claude Code executes tool (if allowed) or adapts (if denied)
    |  Repeats until done or max_turns
    v
Message objects yielded by query()
    |  AssistantMessage -> text forwarded via runtime.send_output()
    |                   -> tool calls recorded + AG-UI events emitted
    |  ResultMessage    -> cost_usd, tokens_in, tokens_out extracted
    v
AgentLoopResult
    |  same shape as AgentLoopExecutor output
    v
_publish_completion() -> NATS -> Go Core -> DB + WebSocket -> Frontend
    |
    v
_record_routing_outcome() -> POST /routing/outcomes -> MAB learns
```

## File Changes

| File | Change | LOC (est.) |
|---|---|---|
| `workers/codeforge/claude_code_executor.py` | **New** — ClaudeCodeExecutor with policy callback + AG-UI | ~250 |
| `workers/codeforge/claude_code_availability.py` | **New** — CLI probe + cache with Lock | ~45 |
| `workers/codeforge/consumer/_conversation.py` | Modify `_execute_conversation_run()` branch + `_get_available_models()` | ~30 |
| `workers/codeforge/routing/router.py` | Add `claudecode/default` to COMPLEXITY_DEFAULTS | ~4 |
| `pyproject.toml` | Add `claude-code-sdk` as optional dependency | ~2 |

**Total estimated: ~331 LOC across 5 files**

Compared to Rev 1: eliminated `internal/adapter/mcp/agent_tools.go` (~150 LOC) and `internal/adapter/mcp/server.go` changes (~10 LOC) by using `can_use_tool` callback instead of MCP bridge.

## Testing Strategy

### Unit Tests

- `ClaudeCodeExecutor`: mock `claude-code-sdk` query with fake `AssistantMessage`/`ResultMessage` objects, verify `AgentLoopResult` shape
- `_make_policy_callback()`: mock `runtime.request_tool_call()`, verify approve returns `PermissionResultAllow`, deny returns `PermissionResultDeny`
- Message formatting: verify `_format_messages_as_prompt()` with single-turn, multi-turn, and system-message-only inputs
- Availability detection: mock subprocess, test caching and Lock behavior, test `ENABLED=false` gate
- Cost estimation: verify `_estimate_equivalent_cost()` with correct `resolve_cost()` signature

### Integration Tests

- Routing selects `claudecode/default` for COMPLEX tier when available
- Routing skips `claudecode/default` when `CODEFORGE_CLAUDECODE_ENABLED=false`
- Fallback chain: Claude Code fails -> next LiteLLM model succeeds
- Policy enforcement: `can_use_tool` callback correctly blocks denied tools
- Routing outcome recording: verify `model_name="claudecode/default"` in stats
- AG-UI events: verify `run_started`, `tool_call`, `tool_result`, `run_finished` are emitted

### Manual Testing

- End-to-end: send a complex coding task, verify Claude Code handles it with live streaming
- Policy denial: send task with restrictive policy, verify Claude Code receives denial and adapts
- HITL approval: send task with `supervised` autonomy, verify WebSocket approval prompt appears
- Unavailable CLI: unset Claude Code binary, verify graceful fallback to LiteLLM models
- Cancellation: start a long task, cancel via UI, verify Claude Code run terminates

## Dependencies

| Dependency | Type | Purpose |
|---|---|---|
| `claude-code-sdk` | Python (optional) | Programmatic Claude Code access |
| `claude` CLI | System binary | Fallback execution path + availability probe |
| Node.js runtime | System | Required by `claude-code-sdk` internally |

The `claude-code-sdk` dependency is **optional**. If not installed, the executor falls back to CLI subprocess mode. If neither is available, `claudecode/*` models are excluded from the routing pool.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| `claude-code-sdk` API changes | Executor breaks | Pin SDK version, CLI fallback |
| Claude Code CLI not installed | No Claude Code routing | Availability probe excludes from pool |
| `can_use_tool` HITL timeout | Claude Code stalls | Match `RuntimeClient` timeout to `ApprovalTimeoutSeconds`, deny on timeout |
| Higher latency than API calls | MAB deprioritizes | MAB naturally learns; latency-sensitive tasks route elsewhere |
| Node.js requirement in container | Deployment complexity | Document in dev-setup, add to Dockerfile |
| Subscription rate limits | Throttling under load | `asyncio.Semaphore(max_concurrent)` + RateLimitTracker with `claudecode` provider |
| `ResultMessage.total_cost_usd` is None | Cost tracking fails | Default to `0.0`, use `_estimate_equivalent_cost()` for MAB |
| `can_use_tool` not available in older SDK | Policy bypass | Version-check SDK at import; fall back to CLI with `--allowedTools` restrictions |

## Resolved Questions

1. **Cost reporting:** Show `$0.00 (subscription)` in dashboard with estimated API-equivalent cost in tooltip for transparency.
2. **Model selection:** Start with `claudecode/default` only. Model override via `claudecode/claude-sonnet-4` is a future extension (strip prefix, pass to SDK's `model` parameter).
3. **Concurrent runs:** Guard with `asyncio.Semaphore(CODEFORGE_CLAUDECODE_MAX_CONCURRENT)` (default 5). Configurable via env var.
4. **MCP bridge vs `can_use_tool`:** Resolved in favor of `can_use_tool` callback. Simpler, no Go changes needed, HITL works natively, Claude Code keeps its battle-tested built-in tools.
