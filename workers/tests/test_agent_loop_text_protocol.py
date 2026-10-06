"""Pure-completion models call tools through the text tool protocol (S9-C).

Before, the loop sent such models no tools and treated every reply as the
final answer: a JSON or ReAct call written as text ended the turn, and the
tool guide asked for "a function call in the format specified by the API".
Now the loop renders the offered tools into the system message of each
request, constrains the reply with a JSON-schema grammar, parses the reply
and turns a call into a normal ToolCallPart: the Go policy, approvals,
trajectory, stall detection and stored messages are the same as for native
calls.
"""

from __future__ import annotations

import copy
import json
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge.agent_loop import AgentLoopExecutor, LoopConfig
from codeforge.config import get_settings, load_yaml_config
from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._conversation import ConversationHandlerMixin
from codeforge.history import ConversationHistoryManager, HistoryConfig
from codeforge.llm import ChatCompletionResponse, LLMError, RoutingResult, ToolCallPart
from codeforge.loop_config import ModelCapability, build_loop_config
from codeforge.models import ConversationMessagePayload, ConversationRunStartMessage, ToolCallDecision
from codeforge.tools import ToolRegistry, build_default_registry
from codeforge.tools.capability import CapabilityLevel
from codeforge.tools.text_protocol import TURN_MAX_TOKENS
from codeforge.tools.tool_guide import build_tool_usage_guide

MODEL = "ollama/llama3"
SECTION = "## Tools\nYou work by calling tools."
REPAIR_NOTICE = "[The reply did not follow the tool format; asking the model again]"


def _reply(
    content: str, *, finish: str = "stop", tool_calls: list[ToolCallPart] | None = None
) -> ChatCompletionResponse:
    return ChatCompletionResponse(
        content=content,
        tool_calls=tool_calls or [],
        finish_reason=finish,
        tokens_in=100,
        tokens_out=20,
        model=MODEL,
        cost_usd=0.01,
    )


def _call(tool: str, args: dict[str, object], thought: str = "Next step.") -> ChatCompletionResponse:
    return _reply(json.dumps({"thought": thought, "tool": tool, "args": args}))


def _final(answer: str, thought: str = "Done.") -> ChatCompletionResponse:
    return _reply(json.dumps({"thought": thought, "final": answer}))


class ScriptedLLM:
    """Streams each scripted reply character by character; an LLMError is raised."""

    def __init__(self, replies: list[ChatCompletionResponse | LLMError]) -> None:
        self._replies = list(replies)
        self.calls: list[dict[str, object]] = []

    async def chat_completion_stream(self, **kwargs: object) -> ChatCompletionResponse:
        recorded = dict(kwargs)
        recorded["messages"] = copy.deepcopy(kwargs["messages"])
        self.calls.append(recorded)
        reply = self._replies.pop(0) if self._replies else _final("(exhausted)")
        if isinstance(reply, LLMError):
            raise reply
        on_chunk = kwargs.get("on_chunk")
        if callable(on_chunk):
            for ch in reply.content:
                on_chunk(ch)
        return reply

    def request(self, index: int) -> list[dict[str, object]]:
        return self.calls[index]["messages"]  # type: ignore[return-value]

    def text(self, index: int) -> str:
        return "\n".join(str(m.get("content")) for m in self.request(index))


def _runtime(deny: frozenset[str] = frozenset()) -> MagicMock:
    runtime = MagicMock()
    runtime.run_id = "run-1"
    runtime.project_id = "proj-1"
    runtime.is_cancelled = False
    runtime.send_output = AsyncMock()
    runtime.report_tool_result = AsyncMock()
    runtime.publish_trajectory_event = AsyncMock()

    async def decide(tool: str, command: str = "", path: str = "", arguments_preview: str = "") -> ToolCallDecision:
        if tool in deny:
            return ToolCallDecision(call_id=f"c-{tool}", decision="deny", reason="denied by policy")
        return ToolCallDecision(call_id=f"c-{tool}", decision="allow")

    runtime.request_tool_call = AsyncMock(side_effect=decide)
    return runtime


def _config(**overrides: object) -> LoopConfig:
    cfg = LoopConfig(model=MODEL, capability_level="pure_completion", max_iterations=10)
    for key, value in overrides.items():
        setattr(cfg, key, value)
    return cfg


def _messages() -> list[dict[str, object]]:
    return [
        {"role": "system", "content": "You are a coder."},
        {"role": "user", "content": "Fix the bug in app.py"},
    ]


async def _run(
    llm: ScriptedLLM, workspace: Path, runtime: MagicMock | None = None, **overrides: object
) -> tuple[object, MagicMock, list[dict[str, object]]]:
    runtime = runtime or _runtime()
    messages = _messages()
    executor = AgentLoopExecutor(
        llm=llm,  # type: ignore[arg-type]
        tool_registry=build_default_registry(skill_tools=False),
        runtime=runtime,
        workspace_path=str(workspace),
    )
    result = await executor.run(messages, _config(**overrides))
    return result, runtime, messages


def _tool_requests(runtime: MagicMock) -> list[tuple[str, str]]:
    return [(c.kwargs["tool"], c.kwargs.get("path", "")) for c in runtime.request_tool_call.await_args_list]


def _output(runtime: MagicMock) -> str:
    return "".join(str(c.args[0]) for c in runtime.send_output.await_args_list)


# --- request shape ---


async def test_request_has_no_tools_but_the_grammar_and_max_tokens(tmp_path: Path) -> None:
    llm = ScriptedLLM([_final("Nothing to do.")])

    await _run(llm, tmp_path)

    call = llm.calls[0]
    assert call["tools"] is None
    fmt = call["response_format"]
    assert isinstance(fmt, dict)
    assert fmt["type"] == "json_schema"
    assert fmt["json_schema"]["name"] == "codeforge_turn"  # type: ignore[index]
    assert call["max_tokens"] == TURN_MAX_TOKENS
    system = llm.request(0)[0]
    assert system["role"] == "system"
    assert str(system["content"]).startswith("You are a coder.\n\n" + SECTION)
    assert "- write_file(file_path: string, content: string)" in str(system["content"])


async def test_a_json_call_runs_the_tool_through_the_policy(tmp_path: Path) -> None:
    llm = ScriptedLLM(
        [_call("write_file", {"file_path": "hello.txt", "content": "hi\n"}, "I create the file."), _final("Created.")]
    )

    result, runtime, messages = await _run(llm, tmp_path)

    assert (tmp_path / "hello.txt").read_text() == "hi\n"
    assert _tool_requests(runtime) == [("LLM", ""), ("write_file", "hello.txt"), ("LLM", "")]
    assert result.final_content == "Created."  # type: ignore[attr-defined]
    assert not result.error  # type: ignore[attr-defined]
    stored = result.tool_messages  # type: ignore[attr-defined]
    assert stored[0].role == "assistant"
    assert stored[0].content == "I create the file."
    assert stored[0].tool_calls[0].function.name == "write_file"
    assert json.loads(stored[0].tool_calls[0].function.arguments) == {"file_path": "hello.txt", "content": "hi\n"}
    assert len(stored[0].tool_calls[0].id) == 9
    assert stored[1].role == "tool"
    assert stored[1].tool_call_id == stored[0].tool_calls[0].id
    assert any(m.get("tool_calls") for m in messages), "the loop's own history stays in OpenAI format"


async def test_the_result_comes_back_as_tool_result_text(tmp_path: Path) -> None:
    (tmp_path / "app.py").write_text("print(1)\n")
    llm = ScriptedLLM([_call("read_file", {"file_path": "app.py"}), _final("It prints 1.")])

    await _run(llm, tmp_path)

    second = llm.request(1)
    assert all(m["role"] in ("system", "user", "assistant") for m in second)
    assert all(set(m) == {"role", "content"} for m in second)
    assert second[2] == {
        "role": "assistant",
        "content": '{"thought": "Next step.", "tool": "read_file", "args": {"file_path": "app.py"}}',
    }
    assert str(second[3]["content"]).startswith('<tool_result tool="read_file">\n')
    assert "print(1)" in str(second[3]["content"])


async def test_the_protocol_parses_the_raw_reply(tmp_path: Path) -> None:
    """The client drops <think> blocks anywhere in content, also inside JSON strings (S9-C review)."""
    raw = json.dumps(
        {"thought": "t", "tool": "write_file", "args": {"file_path": "a.py", "content": "s = '<think>x</think>'\n"}}
    )
    stripped = raw.replace("<think>x</think>", "")
    reply = ChatCompletionResponse(
        content=stripped,
        raw_content=raw,
        tool_calls=[],
        finish_reason="stop",
        tokens_in=1,
        tokens_out=1,
        model=MODEL,
    )
    llm = ScriptedLLM([reply, _final("Done.")])

    await _run(llm, tmp_path)

    assert (tmp_path / "a.py").read_text() == "s = '<think>x</think>'\n"


async def test_final_answer_ends_the_turn(tmp_path: Path) -> None:
    llm = ScriptedLLM([_final("The code is fine.", "I checked it.")])

    result, _, _ = await _run(llm, tmp_path)

    assert result.final_content == "The code is fine."  # type: ignore[attr-defined]
    assert result.tool_messages == []  # type: ignore[attr-defined]


async def test_a_policy_deny_is_the_tool_result(tmp_path: Path) -> None:
    llm = ScriptedLLM([_call("write_file", {"file_path": "x.txt", "content": "x"}), _final("Could not write.")])

    result, _, _ = await _run(llm, tmp_path, runtime=_runtime(deny=frozenset({"write_file"})))

    assert not (tmp_path / "x.txt").exists()
    assert '<tool_result tool="write_file">\nPermission denied: denied by policy' in llm.text(1)
    assert result.tool_messages[1].content == "Permission denied: denied by policy"  # type: ignore[attr-defined]


async def test_no_protocol_json_reaches_the_output(tmp_path: Path) -> None:
    llm = ScriptedLLM([_call("bash", {"command": "ls"}, "I list the files."), _final("Listed.", "All seen.")])

    _, runtime, _ = await _run(llm, tmp_path)

    output = _output(runtime)
    assert '"tool":' not in output
    assert '"args"' not in output
    assert "I list the files." in output
    assert "All seen.\n\nListed." in output


# --- repair ---


async def test_one_repair_then_success(tmp_path: Path) -> None:
    llm = ScriptedLLM([_reply('{"thought": "x", "tool": "rm_rf", "args": {}}'), _final("Fixed.")])

    result, runtime, _ = await _run(llm, tmp_path)

    assert result.final_content == "Fixed."  # type: ignore[attr-defined]
    assert not result.error  # type: ignore[attr-defined]
    retry = llm.text(1)
    assert "[System] Your last reply could not be used: unknown tool 'rm_rf'" in retry
    assert 'rm_rf", "args"' not in retry, "the malformed reply is not sent again"
    assert REPAIR_NOTICE in _output(runtime)
    assert result.tool_messages == []  # type: ignore[attr-defined]
    assert result.total_cost == pytest.approx(0.02)  # type: ignore[attr-defined]


async def test_two_unusable_replies_in_a_row_end_the_run(tmp_path: Path) -> None:
    llm = ScriptedLLM([_reply('{"thought": "x", "tool": "rm_rf"}'), _reply('{"thought": "x", "tool": "rm_rf"}')])

    result, runtime, _ = await _run(llm, tmp_path)

    assert result.error.startswith("text tool protocol: unknown tool 'rm_rf'")  # type: ignore[attr-defined]
    assert len(llm.calls) == 2
    assert result.total_cost == pytest.approx(0.02)  # type: ignore[attr-defined]
    assert result.total_tokens_in == 200  # type: ignore[attr-defined]
    llm_reports = [c for c in runtime.report_tool_result.await_args_list if c.kwargs["tool"] == "LLM"]
    assert len(llm_reports) == 2


async def test_repairs_are_counted_in_a_row_only(tmp_path: Path) -> None:
    bad = _reply('{"thought": "x", "tool": "nope", "args": {}}')
    llm = ScriptedLLM([bad, _call("bash", {"command": "ls"}), bad, _final("Done.")])

    result, _, _ = await _run(llm, tmp_path)

    assert not result.error  # type: ignore[attr-defined]
    assert result.final_content == "Done."  # type: ignore[attr-defined]


async def test_cut_off_reply_is_repaired(tmp_path: Path) -> None:
    llm = ScriptedLLM(
        [_reply('{"thought": "x", "tool": "write_file", "args": {"content": "def f(', finish="length"), _final("ok")]
    )

    await _run(llm, tmp_path)

    assert "write large files in parts" in llm.text(1)


# --- grammar ---


async def test_rejected_grammar_is_dropped_and_the_iteration_retried(tmp_path: Path) -> None:
    rejected = LLMError(400, MODEL, '{"error": {"message": "response_format json_schema is not supported"}}')
    llm = ScriptedLLM([rejected, _final("Done.")])

    result, _, _ = await _run(llm, tmp_path, fallback_models=["ollama/other"])

    assert not result.error  # type: ignore[attr-defined]
    assert llm.calls[0]["response_format"] is not None
    assert llm.calls[1]["response_format"] is None
    assert llm.calls[1]["model"] == MODEL, "a rejected grammar does not switch the model"


async def test_grammar_stays_off_for_the_rest_of_the_run(tmp_path: Path) -> None:
    rejected = LLMError(422, MODEL, "invalid grammar")
    llm = ScriptedLLM([rejected, _call("bash", {"command": "ls"}), _final("Done.")])

    await _run(llm, tmp_path)

    assert [c["response_format"] for c in llm.calls[1:]] == [None, None]


async def test_other_errors_keep_the_grammar(tmp_path: Path) -> None:
    llm = ScriptedLLM([LLMError(400, MODEL, "context length exceeded")])

    result, _, _ = await _run(llm, tmp_path)

    assert "LLM call failed" in result.error  # type: ignore[attr-defined]
    assert len(llm.calls) == 1


async def test_grammar_switch_off(tmp_path: Path) -> None:
    llm = ScriptedLLM([_final("Done.")])

    await _run(llm, tmp_path, text_tool_grammar=False)

    assert llm.calls[0]["response_format"] is None
    assert SECTION in llm.text(0), "the protocol works without the grammar"


# --- native calls, plan/act, extra calls, stall ---


async def test_a_native_tool_call_is_used_as_is(tmp_path: Path) -> None:
    native = ToolCallPart(id="native1", name="bash", arguments='{"command": "ls"}')
    llm = ScriptedLLM([_reply("", finish="tool_calls", tool_calls=[native]), _final("Done.")])

    result, runtime, _ = await _run(llm, tmp_path)

    assert ("bash", "") in [(t, "") for t, _ in _tool_requests(runtime)]
    assert result.tool_messages[0].tool_calls[0].id == "native1"  # type: ignore[attr-defined]


async def test_plan_act_transition_and_the_section_survive_the_phase_switch(tmp_path: Path) -> None:
    llm = ScriptedLLM(
        [
            _call("transition_to_act", {}, "The plan is ready."),
            _call("write_file", {"file_path": "a.txt", "content": "a"}),
            _final("Done."),
        ]
    )

    result, _, messages = await _run(llm, tmp_path, plan_act_enabled=True)

    assert not result.error  # type: ignore[attr-defined]
    first_system = str(llm.request(0)[0]["content"])
    assert "PLAN phase" in first_system
    assert "- transition_to_act(): " in first_system
    later_system = str(llm.request(1)[0]["content"])
    assert "ACT phase" in later_system
    assert "PLAN phase" not in later_system
    assert later_system.count(SECTION) == 1
    assert (tmp_path / "a.txt").read_text() == "a"
    assert SECTION not in str(messages[0]["content"]), "the section is never stored"


async def test_extra_calls_are_dropped_with_a_note(tmp_path: Path) -> None:
    two = json.dumps(
        [
            {"thought": "t", "tool": "write_file", "args": {"file_path": "one.txt", "content": "1"}},
            {"thought": "u", "tool": "write_file", "args": {"file_path": "two.txt", "content": "2"}},
        ]
    )
    llm = ScriptedLLM([_reply(two), _final("Done.")])

    await _run(llm, tmp_path)

    assert (tmp_path / "one.txt").exists()
    assert not (tmp_path / "two.txt").exists()
    assert "[System] Only the first tool call of your reply was run. Send one call per reply." in llm.text(1)


async def test_three_identical_calls_hit_the_stall_escape(tmp_path: Path) -> None:
    (tmp_path / "app.py").write_text("x = 1\n")
    same = _call("read_file", {"file_path": "app.py"})
    llm = ScriptedLLM([same, same, same, _final("Done.")])

    await _run(llm, tmp_path)

    assert "You have been exploring the codebase" in llm.text(3)


async def test_a_repair_does_not_count_the_previous_call_again(tmp_path: Path) -> None:
    """Only iterations that call a tool feed the stall detector: two reads and a repair are no stall."""
    (tmp_path / "app.py").write_text("x = 1\n")
    same = _call("read_file", {"file_path": "app.py"})
    llm = ScriptedLLM([same, same, _reply('{"thought": "x", "tool": "nope", "args": {}}'), _final("Done.")])

    await _run(llm, tmp_path)

    assert "You have been exploring the codebase" not in llm.text(3)


# --- rollouts and runs without tools ---


async def test_rollout_copies_do_not_repeat_the_section(tmp_path: Path) -> None:
    """ConversationRolloutExecutor runs the loop on list(messages) copies that share the message dicts."""
    llm = ScriptedLLM([_final("one"), _final("two")])
    messages = _messages()
    executor = AgentLoopExecutor(
        llm=llm,  # type: ignore[arg-type]
        tool_registry=build_default_registry(skill_tools=False),
        runtime=_runtime(),
        workspace_path=str(tmp_path),
    )

    await executor.run(list(messages), _config())
    await executor.run(list(messages), _config())

    for index in (0, 1):
        assert llm.text(index).count(SECTION) == 1
    assert messages[0]["content"] == "You are a coder."


async def test_no_tools_no_protocol(tmp_path: Path) -> None:
    raw = '{"thought": "x", "final": "y"}'
    llm = ScriptedLLM([_reply(raw)])
    executor = AgentLoopExecutor(
        llm=llm,  # type: ignore[arg-type]
        tool_registry=ToolRegistry(),
        runtime=_runtime(),
        workspace_path=str(tmp_path),
    )

    result = await executor.run(_messages(), _config())

    assert llm.calls[0]["response_format"] is None
    assert llm.calls[0]["tools"] is None
    assert SECTION not in llm.text(0)
    assert result.final_content == raw


async def test_native_models_are_unchanged(tmp_path: Path) -> None:
    llm = ScriptedLLM([_reply("All done.")])

    await _run(llm, tmp_path, capability_level="full", model="openai/gpt-4o")

    call = llm.calls[0]
    assert call["tools"]
    assert call["response_format"] is None
    assert call["max_tokens"] is None
    assert llm.request(0) == _messages()


# --- a server that refuses native tools ---

REFUSALS = [
    pytest.param(400, "registry.ollama.ai/library/gemma3:1b does not support tools", id="ollama"),
    pytest.param(400, '"auto" tool choice requires --enable-auto-tool-choice', id="vllm"),
    pytest.param(500, "tools param requires --jinja flag", id="llama-cpp"),
    pytest.param(400, "Tools are not supported for this model", id="tools-not-supported"),
    pytest.param(400, "Function calling is not supported by this model", id="fc-not-supported"),
]


@pytest.mark.parametrize(("status", "body"), REFUSALS)
async def test_a_refusal_of_native_tools_switches_to_the_text_protocol(tmp_path: Path, status: int, body: str) -> None:
    """A wrong classification (ollama/*=api_with_tools with gemma3) becomes harmless."""
    llm = ScriptedLLM(
        [
            LLMError(status, "ollama/gemma3:1b", f'{{"error": {{"message": "litellm.BadRequestError: {body}"}}}}'),
            _call("write_file", {"file_path": "a.txt", "content": "a"}),
            _final("Done."),
        ]
    )

    result, runtime, _ = await _run(
        llm, tmp_path, capability_level="api_with_tools", model="ollama/gemma3:1b", fallback_models=["ollama/x"]
    )

    assert not result.error  # type: ignore[attr-defined]
    assert llm.calls[0]["tools"]
    assert [c["tools"] for c in llm.calls[1:]] == [None, None]
    assert all(c["response_format"] is not None for c in llm.calls[1:])
    assert all(c["model"] == "ollama/gemma3:1b" for c in llm.calls), "no model fallback"
    assert SECTION in llm.text(1)
    assert (tmp_path / "a.txt").read_text() == "a"
    assert "does not support native tool calls" in _output(runtime)
    assert "text tool protocol" in _output(runtime)


@pytest.mark.parametrize(
    ("status", "body"),
    [
        pytest.param(400, "context length exceeded", id="other-400"),
        pytest.param(404, "model does not support tools", id="other-status"),
    ],
)
async def test_other_errors_do_not_switch(tmp_path: Path, status: int, body: str) -> None:
    llm = ScriptedLLM([LLMError(status, MODEL, body)])

    result, _, _ = await _run(llm, tmp_path, capability_level="api_with_tools")

    assert "LLM call failed" in result.error  # type: ignore[attr-defined]
    assert len(llm.calls) == 1


async def test_a_refusal_with_the_protocol_on_is_an_error(tmp_path: Path) -> None:
    llm = ScriptedLLM([LLMError(400, MODEL, "does not support tools")])

    result, _, _ = await _run(llm, tmp_path)

    assert "LLM call failed" in result.error  # type: ignore[attr-defined]
    assert len(llm.calls) == 1


# --- config: litellm.text_tool_grammar / CODEFORGE_TEXT_TOOL_GRAMMAR ---


def test_text_tool_grammar_is_on_by_default() -> None:
    assert get_settings().text_tool_grammar is True
    assert LoopConfig().text_tool_grammar is True


@pytest.mark.parametrize(("value", "expected"), [("false", False), ("0", False), ("no", False), ("true", True)])
def test_text_tool_grammar_from_env(value: str, expected: bool, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("CODEFORGE_TEXT_TOOL_GRAMMAR", value)

    assert get_settings().text_tool_grammar is expected


def test_text_tool_grammar_from_yaml_and_env_wins(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    config = tmp_path / "codeforge.yaml"
    config.write_text("litellm:\n  text_tool_grammar: false\n")
    monkeypatch.setenv("CODEFORGE_CONFIG_FILE", str(config))
    load_yaml_config.cache_clear()
    try:
        assert get_settings().text_tool_grammar is False
        monkeypatch.setenv("CODEFORGE_TEXT_TOOL_GRAMMAR", "true")
        get_settings.cache_clear()
        assert get_settings().text_tool_grammar is True
    finally:
        load_yaml_config.cache_clear()


@pytest.mark.parametrize(("value", "expected"), [("", True), ("false", False)])
def test_loop_config_carries_the_switch(value: str, expected: bool, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("CODEFORGE_TEXT_TOOL_GRAMMAR", value)

    cfg, _ = build_loop_config(
        primary_model=MODEL,
        capability_level=CapabilityLevel.PURE_COMPLETION,
        routing=RoutingResult(),
        tool_names=["read_file"],
        fallback_models=[],
        user_prompt="",
        max_steps=5,
        max_cost=0,
        mode_tools=frozenset(),
    )

    assert cfg.text_tool_grammar is expected


# --- conversation path, tool guide, history reserve ---


def _conversation_run(workspace: Path, history: list[ConversationMessagePayload] | None = None) -> object:
    return ConversationRunStartMessage(
        run_id="run-1",
        conversation_id="c1",
        project_id="proj-1",
        messages=history or [],
        system_prompt="You are a coder.",
        model=MODEL,
        workspace_path=str(workspace),
        agentic=True,
    )


async def test_conversation_completion_is_canonical_and_the_next_turn_round_trips(tmp_path: Path) -> None:
    from codeforge.consumer import TaskConsumer

    worker = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://litellm.test")
    first = ScriptedLLM(
        [_call("write_file", {"file_path": "a.py", "content": "x = 1\n"}, "I write a.py."), _final("Wrote a.py.")]
    )
    worker._llm = first  # type: ignore[assignment]

    result = await worker._execute_litellm_loop(
        _conversation_run(tmp_path),  # type: ignore[arg-type]
        _messages(),
        MODEL,
        RoutingResult(),
        _runtime(),
        build_default_registry(skill_tools=False),
        [],
    )

    assistant, tool = result.tool_messages
    assert assistant.tool_calls[0].type == "function"
    assert assistant.tool_calls[0].function.name == "write_file"
    assert json.loads(assistant.tool_calls[0].function.arguments) == {"file_path": "a.py", "content": "x = 1\n"}
    assert tool.role == "tool"
    assert tool.name == "write_file"
    assert tool.tool_call_id == assistant.tool_calls[0].id

    # The next turn: Go stores the completion's messages and sends them back as history.
    history = [
        ConversationMessagePayload(role="user", content="Fix the bug in app.py"),
        *result.tool_messages,
        ConversationMessagePayload(role="assistant", content=result.final_content),
        ConversationMessagePayload(role="user", content="Now add a test."),
    ]
    messages = ConversationHistoryManager().build_messages(system_prompt="You are a coder.", history=history)
    second = ScriptedLLM([_final("Added.")])
    worker._llm = second  # type: ignore[assignment]

    await worker._execute_litellm_loop(
        _conversation_run(tmp_path, history),  # type: ignore[arg-type]
        messages,
        MODEL,
        RoutingResult(),
        _runtime(),
        build_default_registry(skill_tools=False),
        [],
    )

    wire = second.request(0)
    assert [m["role"] for m in wire] == ["system", "user", "assistant", "user", "assistant", "user"]
    assert wire[2]["content"] == (
        '{"thought": "I write a.py.", "tool": "write_file", "args": {"file_path": "a.py", "content": "x = 1\\n"}}'
    )
    assert str(wire[3]["content"]).startswith('<tool_result tool="write_file">\n')
    assert wire[4]["content"] == '{"thought": "", "final": "Wrote a.py."}'
    assert wire[5]["content"] == "Now add a test."


@pytest.mark.parametrize("compact", [False, True])
def test_no_tool_guide_for_pure_completion(compact: bool) -> None:
    registry = build_default_registry()

    assert build_tool_usage_guide(registry, CapabilityLevel.PURE_COMPLETION, compact=compact) == ""
    assert build_tool_usage_guide(registry, CapabilityLevel.API_WITH_TOOLS, compact=compact) != ""


class _Handler(ConversationHandlerMixin, ConsumerBaseMixin):
    pass


@pytest.mark.parametrize(("level", "limit"), [("pure_completion", 15_000), ("api_with_tools", 16_000)])
async def test_history_reserves_room_for_the_protocol_section(level: str, limit: int) -> None:
    captured: list[int] = []

    class SpyHistory:
        def __init__(self, config: HistoryConfig) -> None:
            captured.append(config.max_context_tokens)

        def build_messages(self, **_kwargs: object) -> list[dict[str, object]]:
            return [{"role": "system", "content": "s"}]

    handler = _Handler()
    handler._llm = MagicMock()  # type: ignore[assignment]
    handler._db_url = ""
    handler._js = None
    with (
        patch("codeforge.consumer._conversation.build_system_prompt", AsyncMock(return_value=("prompt", []))),
        patch(
            "codeforge.consumer._conversation.resolve_model_capability",
            AsyncMock(return_value=ModelCapability(CapabilityLevel(level), 16_000)),
        ),
        patch("codeforge.consumer._conversation.wire_skill_tools"),
        patch("codeforge.consumer._conversation.register_handoff_tool"),
        patch("codeforge.consumer._conversation.register_propose_goal_tool"),
        patch("codeforge.consumer._conversation.register_propose_roadmap_tool"),
        patch("codeforge.history.ConversationHistoryManager", SpyHistory),
    ):
        await handler._build_conversation_messages(
            _conversation_run(Path("/nonexistent")),  # type: ignore[arg-type]
            MagicMock(),
            MagicMock(),
            MagicMock(),
            model=MODEL,
        )

    assert captured == [limit]
