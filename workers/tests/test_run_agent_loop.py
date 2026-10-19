"""runs.start executes in the agent loop, with a policy decision per tool call (KI-21).

Before, a run made one policy request and one LLM completion: no tools, no file
changes, and quality gates and delivery ran against an unchanged workspace.
The tests drive AgentExecutor.execute_with_runtime with the real RuntimeClient
against a JetStream fake that answers every runs.toolcall.request like the Go
policy layer, and a scripted LLM.
"""

from __future__ import annotations

import json
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock

import pytest

from codeforge.config import get_settings
from codeforge.executor import AgentExecutor
from codeforge.llm import ChatCompletionResponse, RoutingResult, ToolCallPart
from codeforge.models import ModeConfig, TaskMessage, TerminationConfig
from codeforge.runtime import RuntimeClient
from tests.jetstream_fakes import RecordingJetStream

if TYPE_CHECKING:
    from collections.abc import Callable
    from pathlib import Path

    from codeforge.agent_loop import LoopConfig

MODEL = "openai/gpt-4o"


@pytest.fixture(autouse=True)
def _no_router(monkeypatch: pytest.MonkeyPatch) -> None:
    """Routing disabled and no model list: the run uses the model of the task config."""
    monkeypatch.setattr("codeforge.consumer._conversation_routing.get_hybrid_router", AsyncMock(return_value=None))
    monkeypatch.setattr("codeforge.consumer._conversation_routing.get_available_models", AsyncMock(return_value=[]))


class PolicyJetStream(RecordingJetStream):
    """Answers each tool call request like the Go policy layer: allow, or deny for *deny* tools."""

    def __init__(self, deny: frozenset[str] = frozenset()) -> None:
        super().__init__()
        self._deny = deny

    async def publish(self, subject: str, payload: bytes = b"", **kwargs: object) -> object:
        ack = await super().publish(subject, payload, **kwargs)  # type: ignore[arg-type]
        if subject == "runs.toolcall.request":
            request = json.loads(payload)
            decision = "deny" if request["tool"] in self._deny else "allow"
            response = {"run_id": request["run_id"], "call_id": request["call_id"], "decision": decision}
            if decision == "deny":
                response["reason"] = "denied by policy"
            listening = [s for s in self.subscriptions if s.subject == "runs.toolcall.response" and not s.unsubscribed]
            listening[-1].deliver(json.dumps(response).encode())
        return ack

    def payloads(self, subject: str) -> list[dict[str, object]]:
        return [json.loads(data) for published, data in self.published if published == subject]


class ScriptedLLM:
    """Returns the scripted responses in order; *on_call* runs before each one."""

    def __init__(self, responses: list[ChatCompletionResponse], on_call: Callable[[int], None] | None = None) -> None:
        self._responses = list(responses)
        self._on_call = on_call
        self.calls: list[dict[str, object]] = []

    async def chat_completion_stream(self, **kwargs: object) -> ChatCompletionResponse:
        self.calls.append(kwargs)
        if self._on_call is not None:
            self._on_call(len(self.calls))
        return self._responses.pop(0)


def _answer(content: str = "", tool_calls: list[ToolCallPart] | None = None) -> ChatCompletionResponse:
    return ChatCompletionResponse(
        content=content,
        tool_calls=tool_calls or [],
        finish_reason="tool_calls" if tool_calls else "stop",
        tokens_in=100,
        tokens_out=20,
        model=MODEL,
        cost_usd=0.01,
    )


def _write(path: str, content: str) -> ToolCallPart:
    return ToolCallPart(id="call-1", name="write_file", arguments=json.dumps({"file_path": path, "content": content}))


def _task(workspace: str) -> TaskMessage:
    return TaskMessage(
        id="task-1",
        project_id="proj-1",
        title="Create hello.txt",
        prompt="Create hello.txt with a greeting",
        workspace_path=workspace,
        config={"model": MODEL},
    )


def _runtime(js: PolicyJetStream) -> RuntimeClient:
    return RuntimeClient(
        js=js,  # type: ignore[arg-type]
        notifications=js,  # type: ignore[arg-type]
        run_id="run-1",
        task_id="task-1",
        project_id="proj-1",
        termination=TerminationConfig(max_steps=10, max_cost=5.0),
        approval_timeout_seconds=1,
    )


def _completion(js: PolicyJetStream) -> dict[str, object]:
    completions = js.payloads("runs.complete")
    assert len(completions) == 1, completions
    return completions[0]


async def test_run_executes_tool_calls_in_the_workspace(tmp_path: Path) -> None:
    llm = ScriptedLLM([_answer(tool_calls=[_write("hello.txt", "hi\n")]), _answer("Created hello.txt")])
    js = PolicyJetStream()

    await AgentExecutor(llm=llm).execute_with_runtime(_task(str(tmp_path)), _runtime(js))  # type: ignore[arg-type]

    assert (tmp_path / "hello.txt").read_text() == "hi\n"
    requests = js.payloads("runs.toolcall.request")
    assert [r["tool"] for r in requests] == ["LLM", "write_file", "LLM"], "every call is approved by the policy"
    assert requests[1]["path"] == "hello.txt"
    done = _completion(js)
    assert done["status"] == "completed"
    assert done["output"] == "Created hello.txt"
    assert done["step_count"] == 3
    assert done["cost_usd"] == pytest.approx(0.02)


async def test_denied_tool_call_is_not_executed(tmp_path: Path) -> None:
    llm = ScriptedLLM([_answer(tool_calls=[_write("hello.txt", "hi\n")]), _answer("Could not write the file")])
    js = PolicyJetStream(deny=frozenset({"write_file"}))

    await AgentExecutor(llm=llm).execute_with_runtime(_task(str(tmp_path)), _runtime(js))  # type: ignore[arg-type]

    assert not (tmp_path / "hello.txt").exists()
    results = [r for r in js.payloads("runs.toolcall.result") if r["tool"] == "write_file"]
    assert [r["success"] for r in results] == [False]
    assert "denied by policy" in str(llm.calls[1]["messages"])


async def test_denied_llm_call_fails_the_run(tmp_path: Path) -> None:
    llm = ScriptedLLM([])
    js = PolicyJetStream(deny=frozenset({"LLM"}))

    await AgentExecutor(llm=llm).execute_with_runtime(_task(str(tmp_path)), _runtime(js))  # type: ignore[arg-type]

    done = _completion(js)
    assert done["status"] == "failed"
    assert "LLM call denied" in str(done["error"])
    assert llm.calls == []


@pytest.mark.parametrize("workspace", ["", "   ", "missing-dir"])
async def test_run_without_workspace_fails_visibly(tmp_path: Path, workspace: str) -> None:
    """Tools must not run in the worker's own directory: no workspace, no run."""
    if workspace == "missing-dir":
        workspace = str(tmp_path / "missing-dir")
    llm = ScriptedLLM([])
    js = PolicyJetStream()

    await AgentExecutor(llm=llm).execute_with_runtime(_task(workspace), _runtime(js))  # type: ignore[arg-type]

    done = _completion(js)
    assert done["status"] == "failed"
    assert "workspace" in str(done["error"])
    assert llm.calls == []
    assert js.payloads("runs.toolcall.request") == []


async def test_cancelled_run_reports_cancelled(tmp_path: Path) -> None:
    js = PolicyJetStream()
    runtime = _runtime(js)

    def cancel_after_first_call(call: int) -> None:
        if call == 1:
            runtime._cancelled = True  # what the cancel listener does on runs.cancel

    llm = ScriptedLLM([_answer(tool_calls=[_write("hello.txt", "hi\n")])], on_call=cancel_after_first_call)

    await AgentExecutor(llm=llm).execute_with_runtime(_task(str(tmp_path)), runtime)  # type: ignore[arg-type]

    assert not (tmp_path / "hello.txt").exists(), "a cancelled run's pending tool call is denied"
    assert _completion(js)["status"] == "cancelled"


async def test_mode_prompt_and_tools_reach_the_loop(tmp_path: Path) -> None:
    llm = ScriptedLLM([_answer("nothing to do")])
    js = PolicyJetStream()
    mode = ModeConfig(id="reviewer", prompt_prefix="You are a strict reviewer.", tools=["read_file"])

    await AgentExecutor(llm=llm).execute_with_runtime(_task(str(tmp_path)), _runtime(js), mode=mode)  # type: ignore[arg-type]

    messages = llm.calls[0]["messages"]
    assert messages[0] == {"role": "system", "content": "You are a strict reviewer."}  # type: ignore[index]
    assert messages[1]["role"] == "user"  # type: ignore[index]
    offered = {t["function"]["name"] for t in llm.calls[0]["tools"]}  # type: ignore[union-attr]
    assert "read_file" in offered
    assert _completion(js)["status"] == "completed"


# ---------------------------------------------------------------------------
# Review of KI-21: runs get the conversation path's loop setup
# ---------------------------------------------------------------------------


class _LoopSpy:
    """Records the messages and LoopConfig the run hands to the agent loop."""

    def __init__(self, monkeypatch: pytest.MonkeyPatch) -> None:
        from codeforge.agent_loop import AgentLoopExecutor

        self.messages: list[dict[str, object]] = []
        self.config: LoopConfig | None = None
        self.tool_names: list[str] = []
        real_run = AgentLoopExecutor.run
        spy = self

        async def run(loop: AgentLoopExecutor, messages: list[dict[str, object]], config: LoopConfig) -> object:
            spy.messages = list(messages)
            spy.config = config
            spy.tool_names = list(loop._tools.tool_names)
            return await real_run(loop, messages, config)

        monkeypatch.setattr(AgentLoopExecutor, "run", run)


def _route(monkeypatch: pytest.MonkeyPatch, routing: RoutingResult, available: list[str]) -> None:
    """Routing as the HybridRouter would decide it, and the models LiteLLM offers."""
    monkeypatch.setattr("codeforge.llm.resolve_model_with_routing", lambda **_kwargs: routing)
    monkeypatch.setattr(
        "codeforge.consumer._conversation_routing.get_available_models", AsyncMock(return_value=available)
    )


def _task_with_model(workspace: str, model: str) -> TaskMessage:
    task = _task(workspace)
    task.config = {"model": model} if model else {}
    return task


async def test_run_gets_fallbacks_limits_and_the_routing_decision(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """Fallback models and the routing decision reach the loop, so _record_routing_outcome feeds the MAB."""
    spy = _LoopSpy(monkeypatch)
    _route(
        monkeypatch,
        RoutingResult(model=MODEL, temperature=0.3, routing_layer="mab", complexity_tier="complex", task_type="code"),
        [MODEL, "openai/gpt-4o-mini"],
    )

    await AgentExecutor(llm=ScriptedLLM([_answer("done")])).execute_with_runtime(  # type: ignore[arg-type]
        _task_with_model(str(tmp_path), ""), _runtime(PolicyJetStream())
    )

    cfg = spy.config
    assert cfg is not None
    assert cfg.model == MODEL
    assert cfg.fallback_models == ["openai/gpt-4o-mini"]
    assert (cfg.routing_layer, cfg.complexity_tier, cfg.task_type) == ("mab", "complex", "code")
    assert (cfg.max_iterations, cfg.max_cost) == (10, 5.0)


async def test_local_model_gets_sampling_parameters_and_the_tool_guide(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    spy = _LoopSpy(monkeypatch)
    _route(monkeypatch, RoutingResult(), [])

    await AgentExecutor(llm=ScriptedLLM([_answer("done")])).execute_with_runtime(  # type: ignore[arg-type]
        _task_with_model(str(tmp_path), "ollama/llama3"), _runtime(PolicyJetStream())
    )

    cfg = spy.config
    assert cfg is not None
    assert (cfg.temperature, cfg.top_p) == (0.7, 0.8)
    assert cfg.extra_body == {"top_k": 20, "repetition_penalty": 1.05, "reasoning_effort": "none"}
    system_prompt = str(spy.messages[0]["content"])
    assert "--- Tool Usage Guide ---" in system_prompt or "--- Workflow Rules ---" in system_prompt


async def test_run_never_uses_the_claude_code_model(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """Claude Code bypasses the per-call policy (KI-72): runs go through LiteLLM models only."""
    spy = _LoopSpy(monkeypatch)
    _route(monkeypatch, RoutingResult(model="claudecode/default"), ["claudecode/default", MODEL])

    await AgentExecutor(llm=ScriptedLLM([_answer("done")])).execute_with_runtime(  # type: ignore[arg-type]
        _task_with_model(str(tmp_path), ""), _runtime(PolicyJetStream())
    )

    cfg = spy.config
    assert cfg is not None
    assert cfg.model == MODEL
    assert not any(m.startswith("claudecode/") for m in cfg.fallback_models)


async def test_run_has_no_skill_tools(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """Skill search and creation are conversation features: in a run they would find nothing or not save."""
    spy = _LoopSpy(monkeypatch)

    await AgentExecutor(llm=ScriptedLLM([_answer("done")])).execute_with_runtime(  # type: ignore[arg-type]
        _task(str(tmp_path)), _runtime(PolicyJetStream())
    )

    assert "write_file" in spy.tool_names
    assert not set(spy.tool_names) & {"search_skills", "create_skill"}


# ---------------------------------------------------------------------------
# S9-C: pure-completion models call tools through the text tool protocol
# ---------------------------------------------------------------------------

_WRITE_CALL = json.dumps(
    {"thought": "I create the file.", "tool": "write_file", "args": {"file_path": "hello.txt", "content": "hi\n"}}
)


async def test_pure_completion_run_writes_through_a_json_call(tmp_path: Path) -> None:
    llm = ScriptedLLM([_answer(_WRITE_CALL), _answer(json.dumps({"thought": "", "final": "Created hello.txt"}))])
    js = PolicyJetStream()

    await AgentExecutor(llm=llm).execute_with_runtime(  # type: ignore[arg-type]
        _task_with_model(str(tmp_path), "ollama/llama3"), _runtime(js)
    )

    assert (tmp_path / "hello.txt").read_text() == "hi\n"
    requests = js.payloads("runs.toolcall.request")
    assert [r["tool"] for r in requests] == ["LLM", "write_file", "LLM"], "the policy saw the call"
    assert requests[1]["path"] == "hello.txt"
    assert llm.calls[0]["tools"] is None
    assert llm.calls[0]["response_format"] is not None
    done = _completion(js)
    assert done["status"] == "completed"
    assert done["output"] == "Created hello.txt"


async def test_pure_completion_run_deny_blocks_the_json_call(tmp_path: Path) -> None:
    llm = ScriptedLLM([_answer(_WRITE_CALL), _answer(json.dumps({"thought": "", "final": "Could not write."}))])
    js = PolicyJetStream(deny=frozenset({"write_file"}))

    await AgentExecutor(llm=llm).execute_with_runtime(  # type: ignore[arg-type]
        _task_with_model(str(tmp_path), "ollama/llama3"), _runtime(js)
    )

    assert not (tmp_path / "hello.txt").exists()
    results = [r for r in js.payloads("runs.toolcall.result") if r["tool"] == "write_file"]
    assert [r["success"] for r in results] == [False]
    assert "Permission denied: denied by policy" in str(llm.calls[1]["messages"])


async def test_pure_completion_run_without_the_grammar(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """CODEFORGE_TEXT_TOOL_GRAMMAR=false: the protocol without the JSON-schema grammar."""
    monkeypatch.setenv("CODEFORGE_TEXT_TOOL_GRAMMAR", "false")
    llm = ScriptedLLM([_answer(_WRITE_CALL), _answer(json.dumps({"thought": "", "final": "Created hello.txt"}))])

    await AgentExecutor(llm=llm).execute_with_runtime(  # type: ignore[arg-type]
        _task_with_model(str(tmp_path), "ollama/llama3"), _runtime(PolicyJetStream())
    )

    assert (tmp_path / "hello.txt").read_text() == "hi\n"
    assert [call["response_format"] for call in llm.calls] == [None, None]
    assert llm.calls[0]["tools"] is None


@pytest.mark.parametrize(
    ("value", "expected"),
    [
        ("", {"top_k": 20, "repetition_penalty": 1.05, "reasoning_effort": "none"}),
        ("low", {"top_k": 20, "repetition_penalty": 1.05, "reasoning_effort": "low"}),
        ("off", {"top_k": 20, "repetition_penalty": 1.05}),
    ],
)
async def test_local_reasoning_effort_setting(
    value: str, expected: dict[str, object], tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """CODEFORGE_LOCAL_REASONING_EFFORT: thinking of local hybrid models (Qwen3.5) is off by default."""
    monkeypatch.setenv("CODEFORGE_LOCAL_REASONING_EFFORT", value)
    get_settings.cache_clear()
    try:
        spy = _LoopSpy(monkeypatch)
        _route(monkeypatch, RoutingResult(), [])
        await AgentExecutor(llm=ScriptedLLM([_answer("done")])).execute_with_runtime(  # type: ignore[arg-type]
            _task_with_model(str(tmp_path), "ollama/qwen3.5:4b"), _runtime(PolicyJetStream())
        )
    finally:
        get_settings.cache_clear()

    assert spy.config is not None
    assert spy.config.extra_body == expected


async def test_cloud_models_get_no_reasoning_effort(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    spy = _LoopSpy(monkeypatch)
    _route(monkeypatch, RoutingResult(), [])
    await AgentExecutor(llm=ScriptedLLM([_answer("done")])).execute_with_runtime(  # type: ignore[arg-type]
        _task_with_model(str(tmp_path), "openai/gpt-4o-mini"), _runtime(PolicyJetStream())
    )
    assert spy.config is not None
    assert spy.config.extra_body is None


def test_invalid_local_reasoning_effort_is_rejected(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("CODEFORGE_LOCAL_REASONING_EFFORT", "maximum")
    get_settings.cache_clear()
    try:
        with pytest.raises(ValueError, match="CODEFORGE_LOCAL_REASONING_EFFORT"):
            get_settings()
    finally:
        get_settings.cache_clear()
