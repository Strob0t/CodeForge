"""The LLM is only offered the tools its agent mode may use (KI-69 a).

The Go policy denies a call to a tool the mode denies (``DeniedTools``) or, for
a built-in tool, does not list (``Tools``), compared by canonical name
(internal/domain/policy: CanonicalTool, WithModeTools). The worker offered
every tool anyway, so the LLM spent turns on calls that could only be denied.
"""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.consumer import TaskConsumer
from codeforge.executor import AgentExecutor
from codeforge.llm import ChatCompletionResponse
from codeforge.models import AgentLoopResult, ConversationRunStartMessage, ModeConfig, TaskMessage, TerminationConfig
from codeforge.policy_args import canonical_tool, mode_allows_tool
from codeforge.runtime import RuntimeClient
from codeforge.tools import ToolDefinition, ToolResult, build_default_registry
from tests.jetstream_fakes import RecordingJetStream, jetstream_msg

if TYPE_CHECKING:
    from codeforge.tools import ToolRegistry

REPO_ROOT = Path(__file__).resolve().parents[2]
READ_ONLY = ModeConfig(id="reviewer", tools=["Read", "Glob", "Grep", "ListDir"], denied_tools=["Write", "Edit", "Bash"])


def _go_tool_aliases() -> dict[str, str]:
    """toolAliases of internal/domain/policy/toolnames.go, with the constants resolved."""
    source = (REPO_ROOT / "internal/domain/policy/toolnames.go").read_text()
    constants = dict(re.findall(r"^\s*(Tool\w+)\s*=\s*\"(\w+)\"", source, re.MULTILINE))
    table = source.split("var toolAliases = map[string]string{", 1)[1].split("\n}", 1)[0]
    return {alias: constants[const] for alias, const in re.findall(r"\"([^\"]+)\":\s*(Tool\w+)", table)}


def test_canonical_names_mirror_the_go_policy() -> None:
    aliases = _go_tool_aliases()
    assert len(aliases) > 20
    for alias, canonical in aliases.items():
        assert canonical_tool(alias) == canonical, alias
        assert canonical_tool(alias.upper()) == canonical, "case is ignored like in Go"
    assert canonical_tool("mcp__fs__read") == "mcp__fs__read"
    assert canonical_tool("propose_goal") == "propose_goal"


@pytest.mark.parametrize(
    ("tool", "tools", "denied", "allowed"),
    [
        ("write_file", [], [], True),
        ("write_file", [], ["Write"], False),
        ("write_file", [], ["write_file"], False),
        ("Write", ["Read"], [], False),
        ("read_file", ["Read"], [], True),
        ("read_file", ["read_file"], [], True),
        ("list_directory", ["ListDir"], [], True),
        ("search_files", ["Read"], [], False),
        ("bash", ["Read", "Bash"], [], True),
        ("Monitor", ["Read"], [], False),
        # Tools that are not built in are only restricted by denied.
        ("propose_goal", ["Read"], [], True),
        ("mcp__fs__write", ["Read"], [], True),
        ("spawn_subagent", [], ["spawn_subagent"], False),
    ],
)
def test_mode_allows_tool(tool: str, tools: list[str], denied: list[str], allowed: bool) -> None:
    assert mode_allows_tool(tool, tools, denied) is allowed


class _Noop:
    async def execute(self, arguments: dict[str, object], workspace_path: str) -> ToolResult:
        return ToolResult(output="ok")


def _register(registry: ToolRegistry, name: str) -> None:
    registry.register(ToolDefinition(name=name, description=name, parameters={}), _Noop())


async def test_registry_restricted_to_mode_offers_only_allowed_tools() -> None:
    registry = build_default_registry()
    registry.restrict_to_mode(READ_ONLY.tools, READ_ONLY.denied_tools)

    names = set(registry.tool_names)
    assert {"write_file", "edit_file", "bash"}.isdisjoint(names)
    assert {"read_file", "search_files", "glob_files", "list_directory"} <= names
    assert {"search_conversations", "search_skills", "create_skill"} <= names, "not built in, not denied"
    assert {t["function"]["name"] for t in registry.get_openai_tools()} == names
    assert {d.name for d in registry.get_definitions()} == names
    result = await registry.execute("bash", {"command": "ls"}, "/tmp")
    assert not result.success
    assert "unknown tool" in (result.error or "")


def test_tools_registered_after_the_restriction_are_checked() -> None:
    registry = build_default_registry(skill_tools=False)
    registry.restrict_to_mode(["Read"], ["spawn_subagent"])

    _register(registry, "propose_goal")
    _register(registry, "spawn_subagent")
    _register(registry, "write_file")

    assert "propose_goal" in registry.tool_names
    assert "spawn_subagent" not in registry.tool_names
    assert "write_file" not in registry.tool_names


def test_unrestricted_registry_keeps_every_tool() -> None:
    registry = build_default_registry()
    before = registry.tool_names
    registry.restrict_to_mode([], [])
    assert registry.tool_names == before


# ---------------------------------------------------------------------------
# Both loop paths apply the mode
# ---------------------------------------------------------------------------


class _AllowAll(RecordingJetStream):
    async def publish(self, subject: str, payload: bytes = b"", **kwargs: object) -> object:
        ack = await super().publish(subject, payload, **kwargs)  # type: ignore[arg-type]
        if subject == "runs.toolcall.request":
            request = json.loads(payload)
            response = {"run_id": request["run_id"], "call_id": request["call_id"], "decision": "allow"}
            listening = [s for s in self.subscriptions if s.subject == "runs.toolcall.response" and not s.unsubscribed]
            listening[-1].deliver(json.dumps(response).encode())
        return ack


class _OneAnswer:
    def __init__(self) -> None:
        self.calls: list[dict[str, object]] = []

    async def chat_completion_stream(self, **kwargs: object) -> ChatCompletionResponse:
        self.calls.append(kwargs)
        return ChatCompletionResponse(
            content="done", tool_calls=[], finish_reason="stop", tokens_in=1, tokens_out=1, model="openai/gpt-4o"
        )


async def test_run_offers_only_the_mode_tools(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr("codeforge.consumer._conversation_routing.get_hybrid_router", AsyncMock(return_value=None))
    monkeypatch.setattr("codeforge.consumer._conversation_routing.get_available_models", AsyncMock(return_value=[]))
    llm = _OneAnswer()
    js = _AllowAll()
    runtime = RuntimeClient(
        js=js,  # type: ignore[arg-type]
        notifications=js,  # type: ignore[arg-type]
        run_id="run-1",
        task_id="task-1",
        project_id="proj-1",
        termination=TerminationConfig(max_steps=5, max_cost=5.0),
        approval_timeout_seconds=1,
    )
    task = TaskMessage(
        id="task-1",
        project_id="proj-1",
        title="Review",
        prompt="Review the code",
        workspace_path=str(tmp_path),
        config={"model": "openai/gpt-4o"},
    )

    await AgentExecutor(llm=llm).execute_with_runtime(task, runtime, mode=READ_ONLY)  # type: ignore[arg-type]

    offered = {t["function"]["name"] for t in llm.calls[0]["tools"]}  # type: ignore[union-attr]
    assert "read_file" in offered
    assert {"write_file", "edit_file", "bash"}.isdisjoint(offered)


async def test_conversation_offers_only_the_mode_tools(monkeypatch: pytest.MonkeyPatch) -> None:
    worker = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    worker._js = RecordingJetStream()  # type: ignore[assignment]
    worker._notifications = worker._js
    seen: dict[str, list[str]] = {}

    async def build_messages(
        run_msg: object, runtime: object, registry: ToolRegistry, log: object, *, model: str
    ) -> list[object]:
        seen["prompt_tools"] = list(registry.tool_names)
        return []

    async def execute(*, registry: ToolRegistry, **_kwargs: object) -> AgentLoopResult:
        seen["loop_tools"] = list(registry.tool_names)
        return AgentLoopResult(final_content="done")

    monkeypatch.setattr(worker, "_maybe_prefetch_docs", AsyncMock())
    monkeypatch.setattr(worker, "_build_conversation_messages", build_messages)
    monkeypatch.setattr(worker, "_resolve_routing_and_fallbacks", AsyncMock(return_value=("m", MagicMock(), [])))
    monkeypatch.setattr(worker, "_execute_conversation_run", execute)
    payload = ConversationRunStartMessage(
        run_id="conv-1",
        conversation_id="conv-1",
        project_id="p1",
        tenant_id="00000000-0000-0000-0000-000000000001",
        messages=[],
        system_prompt="s",
        model="m",
        mode=READ_ONLY,
        # An existing directory: an agentic run needs one (KI-193); no tool runs here.
        workspace_path=str(Path(__file__).resolve().parent),
    )
    msg, _ = jetstream_msg(payload.model_dump_json().encode(), subject="conversation.run.start")

    await worker._handle_conversation_run(msg)

    for key in ("prompt_tools", "loop_tools"):
        assert "read_file" in seen[key], key
        assert {"write_file", "edit_file", "bash"}.isdisjoint(seen[key]), key
