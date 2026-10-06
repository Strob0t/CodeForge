"""Parsing text tool turns of pure-completion models (S9-C).

A reply holds one JSON object: {"thought", "tool", "args"} or {"thought",
"final"}. The parser finds it in fences, Hermes <tool_call> blocks or prose,
maps the key names other formats use, repairs trailing commas and raw
newlines, and turns everything it cannot use into an error the loop sends
back to the model once.
"""

from __future__ import annotations

import json
import re
import time

import pytest

from codeforge.tools.text_protocol import (
    TRANSITION_TOOL,
    TextFinal,
    TextProtocolError,
    TextToolCall,
    TextToolProtocol,
    _new_call_id,
    parse_tool_turn,
)

TOOLS = ("read_file", "write_file", "bash", "list_directory")


def _parse(text: str, *, truncated: bool = False) -> TextToolCall | TextFinal | TextProtocolError:
    return parse_tool_turn(text, TOOLS, truncated=truncated)


def _call(name: str, args: dict[str, object], thought: str = "", ignored: int = 0) -> TextToolCall:
    return TextToolCall(name=name, args=args, thought=thought, ignored_calls=ignored)


READ = {"file_path": "a.py"}


@pytest.mark.parametrize(
    ("text", "expected"),
    [
        pytest.param(
            '{"thought": "Read it.", "tool": "read_file", "args": {"file_path": "a.py"}}',
            _call("read_file", READ, "Read it."),
            id="plain-call",
        ),
        pytest.param(
            '{"thought": "Done.", "final": "The bug is fixed."}',
            TextFinal(content="The bug is fixed.", thought="Done."),
            id="plain-final",
        ),
        pytest.param(
            '```json\n{"thought": "t", "tool": "read_file", "args": {"file_path": "a.py"}}\n```',
            _call("read_file", READ, "t"),
            id="json-fence",
        ),
        pytest.param(
            '```\n{"thought": "t", "tool": "read_file", "args": {"file_path": "a.py"}}\n```',
            _call("read_file", READ, "t"),
            id="bare-fence",
        ),
        pytest.param(
            'I will read the file first.\n```json\n{"tool": "read_file", "args": {"file_path": "a.py"}}\n```',
            _call("read_file", READ, "I will read the file first."),
            id="prose-before-is-the-thought",
        ),
        pytest.param(
            '{"thought": "t", "tool": "read_file", "args": {"file_path": "a.py"}}\nThis reads the file.',
            _call("read_file", READ, "t"),
            id="text-after",
        ),
        pytest.param(
            '[{"thought": "t", "tool": "read_file", "args": {"file_path": "a.py"}},'
            ' {"thought": "u", "tool": "bash", "args": {"command": "ls"}}]',
            _call("read_file", READ, "t", ignored=1),
            id="array-first-used",
        ),
        pytest.param(
            '{"thought": "t", "tool": "read_file", "args": {"file_path": "a.py"}}\n'
            '{"thought": "u", "tool": "bash", "args": {"command": "ls"}}\n'
            '{"thought": "v", "final": "done"}',
            _call("read_file", READ, "t", ignored=2),
            id="three-objects",
        ),
        pytest.param(
            '{"thought": "t", "tool": "read_file", "args": {"file_path": "a.py"}}\n'
            '<tool_result tool="read_file">print(1)</tool_result>\nThe file prints 1.',
            _call("read_file", READ, "t", ignored=1),
            id="made-up-tool-result",
        ),
        pytest.param(
            '<tool_call>\n{"name": "read_file", "arguments": "{\\"file_path\\": \\"a.py\\"}"}\n</tool_call>',
            _call("read_file", READ),
            id="hermes-string-arguments",
        ),
        pytest.param(
            'Let me look.\n<tool_call>{"name": "read_file", "arguments": {"file_path": "a.py"}}</tool_call>',
            _call("read_file", READ, "Let me look."),
            id="hermes-object-arguments",
        ),
        pytest.param(
            '{"function": {"name": "read_file", "arguments": "{\\"file_path\\": \\"a.py\\"}"}}',
            _call("read_file", READ),
            id="openai-function-object",
        ),
        pytest.param(
            '{"thought": "t", "function": "read_file", "parameters": {"file_path": "a.py"}}',
            _call("read_file", READ, "t"),
            id="function-string-parameters",
        ),
        pytest.param(
            '{"thought": "t", "action": "bash", "action_input": {"command": "ls"}}',
            _call("bash", {"command": "ls"}, "t"),
            id="langchain-action",
        ),
        pytest.param(
            '{"thought": "t", "action": "Final Answer", "action_input": "All done."}',
            TextFinal(content="All done.", thought="t"),
            id="langchain-final-answer",
        ),
        pytest.param(
            '{"reasoning": "r", "final_answer": "ok"}',
            TextFinal(content="ok", thought="r"),
            id="final-answer-and-reasoning",
        ),
        pytest.param(
            '{"thought": "t", "answer": "ok"}',
            TextFinal(content="ok", thought="t"),
            id="answer",
        ),
        pytest.param(
            '{"thought": "t", "tool": "write_file", "args": {"file_path": "a.py", "content": "x = 1\ny = 2\n"}}',
            _call("write_file", {"file_path": "a.py", "content": "x = 1\ny = 2\n"}, "t"),
            id="raw-newlines",
        ),
        pytest.param(
            '{"thought": "t", "tool": "bash", "args": {"command": "ls, -la",},}',
            _call("bash", {"command": "ls, -la"}, "t"),
            id="trailing-commas",
        ),
        pytest.param(
            '{"thought": "t", "tool": "write_file", "args": {"file_path": "a.json", "content": "[1, 2, ]"},}',
            _call("write_file", {"file_path": "a.json", "content": "[1, 2, ]"}, "t"),
            id="trailing-comma-text-inside-strings-kept",
        ),
        pytest.param(
            '{"thought": "t", "tool": "Read_File", "args": {"file_path": "a.py"}}',
            _call("read_file", READ, "t"),
            id="case-insensitive-name",
        ),
        pytest.param(
            '{"thought": "t", "tool": "list_directory"}',
            _call("list_directory", {}, "t"),
            id="missing-args",
        ),
        pytest.param(
            '{"thought": "t", "tool": "bash", "args": "{\\"command\\": \\"ls\\"}"}',
            _call("bash", {"command": "ls"}, "t"),
            id="args-as-json-string",
        ),
        pytest.param(
            '{"thought": "t", "tool": "bash", "args": {"command": "ls"}, "final": ""}',
            _call("bash", {"command": "ls"}, "t"),
            id="tool-and-empty-final",
        ),
        pytest.param(
            "The tests pass and the bug is fixed.",
            TextFinal(content="The tests pass and the bug is fixed."),
            id="prose-only",
        ),
        pytest.param(
            'Use a set {x} for that. {"tool": "read_file", "args": {"file_path": "a.py"}}',
            _call("read_file", READ, "Use a set {x} for that."),
            id="braces-in-prose-before",
        ),
        pytest.param(
            'Prose. {"thought": "t", "tool": "read_file", "args": {"file_path": "a.py"}}',
            _call("read_file", READ, "t"),
            id="thought-key-wins-over-prose",
        ),
        pytest.param(
            'Here is the file:\n```json\n{"name": "my-app", "version": "1.0.0"}\n```',
            TextFinal(content='Here is the file:\n```json\n{"name": "my-app", "version": "1.0.0"}\n```'),
            id="non-protocol-json-in-prose",
        ),
        pytest.param(
            '<think>Which file? a.py.</think>{"thought": "t", "tool": "read_file", "args": {"file_path": "a.py"}}',
            _call("read_file", READ, "t"),
            id="think-block",
        ),
        pytest.param(
            'I should read a.py.</think>\n{"thought": "t", "tool": "read_file", "args": {"file_path": "a.py"}}',
            _call("read_file", READ, "t"),
            id="closing-think-only",
        ),
        pytest.param(
            '<think>I should read a.py.\n{"thought": "t", "tool": "read_file", "args": {"file_path": "a.py"}}',
            _call("read_file", READ, "t"),
            id="unterminated-think-with-a-call",
        ),
        pytest.param(
            '{"thought": "t", "tool": "write_file", "args": {"file_path": "a.py", "content": "s = \'<think>x</think>\'"}}',
            _call("write_file", {"file_path": "a.py", "content": "s = '<think>x</think>'"}, "t"),
            id="think-tags-inside-a-string-are-kept",
        ),
        pytest.param(
            '{"thought": "t", "tool": "write_file", "args": {"file_path": "a.md", "content": "close with </think> then"}}',
            _call("write_file", {"file_path": "a.md", "content": "close with </think> then"}, "t"),
            id="closing-tag-inside-a-string",
        ),
        pytest.param(
            '<think>plan</think>{"thought": "t", "tool": "write_file", "args": {"file_path": "a.md", "content": "a </think> b"}}',
            _call("write_file", {"file_path": "a.md", "content": "a </think> b"}, "t"),
            id="leading-block-and-a-closing-tag-in-a-string",
        ),
        pytest.param(
            '{"thought": "t", "final": "x"}\nmore </think> text',
            TextFinal(content="x", thought="t"),
            id="closing-tag-after-the-object",
        ),
    ],
)
def test_parse(text: str, expected: TextToolCall | TextFinal) -> None:
    assert _parse(text) == expected


@pytest.mark.parametrize(
    ("text", "truncated", "fragment"),
    [
        pytest.param('{"thought": "t", "tool": "rm_rf", "args": {}}', False, "unknown tool 'rm_rf'", id="unknown-tool"),
        pytest.param(
            '{"thought": "t", "tool": "rm_rf", "args": {}}',
            False,
            "read_file, write_file, bash, list_directory",
            id="unknown-tool-lists-the-tools",
        ),
        pytest.param(
            '{"thought": "t", "tool": "bash", "args": ["ls"]}', False, "must be a JSON object", id="args-array"
        ),
        pytest.param(
            '{"thought": "t", "tool": "bash", "args": "ls -la"}', False, "must be a JSON object", id="args-text"
        ),
        pytest.param(
            '{"thought": "t", "tool": "bash", "args": {"command": "ls"}, "final": "done"}',
            False,
            "both a tool call and a final answer",
            id="tool-and-final",
        ),
        pytest.param("", False, "empty", id="empty"),
        pytest.param("  \n ", False, "empty", id="whitespace"),
        pytest.param(
            '{"thought": "t", "tool": "write_file", "args": {"file_path": "a.py" "content": "x"}}',
            False,
            "could not be read",
            id="broken-json-with-tool",
        ),
        pytest.param(
            '{"thought": "t", "tool": "write_file", "args": {"file_path": "a.py", "content": "def f():\n    retu',
            True,
            "cut off",
            id="cut-off",
        ),
        pytest.param("The answer is", True, "write large files in parts", id="cut-off-prose"),
        pytest.param("", True, "cut off", id="cut-off-empty"),
        pytest.param('{"thought": "I will think about it."}', False, "neither a tool nor a final", id="thought-only"),
        pytest.param('{"thought": "t", "args": {"command": "ls"}}', False, "names no tool", id="args-without-tool"),
        pytest.param("<think>Let me think about which file", False, "<think>", id="unterminated-think-only"),
        pytest.param(
            '<tool_call>{"name": "read_file", "arguments": {"file_path": </tool_call>',
            False,
            "could not be read",
            id="broken-tool-call-block",
        ),
        pytest.param(
            "{'thought': 'I read it.', 'tool': 'read_file', 'args': {'file_path': 'a.py'}}",
            False,
            "could not be read",
            id="single-quoted-call",
        ),
        pytest.param(
            'Thought: I read it.\nAction: read_file\nAction Input: {"file_path": "a.py"}',
            False,
            "could not be read",
            id="react-call",
        ),
    ],
)
def test_parse_errors(text: str, truncated: bool, fragment: str) -> None:
    result = _parse(text, truncated=truncated)

    assert isinstance(result, TextProtocolError), result
    assert fragment in result.message


# --- a call nested in a broken object never runs (S9-C review, finding 1) ---

_NESTED_TRUNCATED = (
    '{"thought":"t","tool":"write_file","args":{"path":"x.json","content":'
    '{"thought":"","tool":"bash","args":{"command":"rm -rf /tmp/x"}}'
)
_NESTED_BAD_ESCAPE = (
    '{"thought":"t","tool":"write_file","args":{"path":"a\\q","data":{"name":"bash","input":{"command":"id"}}}}'
)


@pytest.mark.parametrize(
    ("text", "truncated", "fragment"),
    [
        pytest.param(_NESTED_TRUNCATED, True, "cut off", id="truncated-outer-call"),
        pytest.param(_NESTED_TRUNCATED, False, "could not be read", id="unclosed-outer-call"),
        pytest.param(_NESTED_BAD_ESCAPE, False, "could not be read", id="invalid-escape-in-outer-call"),
        pytest.param(_NESTED_BAD_ESCAPE, True, "cut off", id="invalid-escape-cut-off"),
        pytest.param(
            '{"thought": "t", "tool": "write_file", "args": {"file_path": "a\\q"}}\n'
            '{"thought": "u", "tool": "bash", "args": {"command": "id"}}',
            False,
            "could not be read",
            id="broken-call-before-a-valid-call",
        ),
        pytest.param(
            'Use { to open a block. {"thought": "t", "tool": "bash", "args": {"command": "id"}}',
            False,
            "could not be read",
            id="unbalanced-brace-before-a-call",
        ),
        pytest.param(
            '{"result": {"thought": "t", "tool": "bash", "args": {"command": "id"}}}',
            False,
            "could not be read",
            id="call-nested-in-a-valid-object",
        ),
    ],
)
def test_a_call_inside_a_broken_or_other_object_never_runs(text: str, truncated: bool, fragment: str) -> None:
    result = _parse(text, truncated=truncated)

    assert isinstance(result, TextProtocolError), result
    assert fragment in result.message


def test_a_broken_call_after_the_turn_is_an_ignored_call() -> None:
    text = (
        '{"thought": "t", "tool": "bash", "args": {"command": "ls"}}\n'
        '{"thought": "u", "tool": "write_file", "args": {"file_path": "a\\q"}}'
    )

    assert _parse(text) == _call("bash", {"command": "ls"}, "t", ignored=1)


@pytest.mark.parametrize(
    ("text", "expected"),
    [
        pytest.param(
            '{"thought": "t", "tool": "bash", "args": {"command": "ls", "env": [1, 2, ], "opts": {"a": [],},},}',
            _call("bash", {"command": "ls", "env": [1, 2], "opts": {"a": []}}, "t"),
            id="nested-trailing-commas",
        ),
        pytest.param(
            '{"thought": "a, }", "tool": "bash", "args": {"command": "echo \\"x,]\\", y",},}',
            _call("bash", {"command": 'echo "x,]", y'}, "a, }"),
            id="commas-and-brackets-inside-strings",
        ),
    ],
)
def test_trailing_commas_are_repaired_outside_strings(text: str, expected: TextToolCall) -> None:
    assert _parse(text) == expected


def _crafted_comma_reply() -> str:
    """64 nested starts, each decoding a long array again for every trailing-comma repair (5.4 s before)."""
    return '{"a":' * 64 + "[" + "1," * 95_000 + "1]" + ",}" * 64


@pytest.mark.parametrize(
    "text",
    [
        pytest.param(_crafted_comma_reply(), id="comma-retries"),
        pytest.param("{" * 200_000, id="open-braces"),
        pytest.param("{x} " * 50_000, id="prose-braces"),
        pytest.param('{"k": "' + "\\" * 199_990, id="backslashes"),
        pytest.param(('{"a": 1,}' + " " * 10) * 10_000, id="many-repairable-objects"),
    ],
)
def test_parsing_is_bounded(text: str) -> None:
    """S9-C review, finding 3c: 64 starts x 32 comma retries x a 200,000-character copy took 8.9 s."""
    elapsed, _ = _elapsed(text)

    assert elapsed < 0.5


# --- deep nesting is a repair, never a crash (S9-C review, finding 5) ---


def _nested(depth: int) -> str:
    return "[" * depth + "]" * depth


@pytest.mark.parametrize(
    "text",
    [
        pytest.param(
            '{"thought": "t", "tool": "bash", "args": {"command": "ls", "x": ' + _nested(40) + "}}",
            id="args-over-the-limit",
        ),
        pytest.param(
            '{"thought": "t", "tool": "bash", "args": {"x": ' + _nested(900) + "}}",
            id="args-near-the-recursion-limit",
        ),
        pytest.param('{"thought": "t", "final": ' + _nested(500) + "}", id="deep-final"),
        pytest.param('{"thought": "t", "tool": "bash", "args": ' + _nested(20_000)[:-1] + "}", id="decoder-recursion"),
        pytest.param(
            '{"thought": "t", "tool": "bash", "args": "{\\"x\\": ' + _nested(3000) + '}"}',
            id="deep-args-in-a-string",
        ),
    ],
)
def test_deep_nesting_is_an_error(text: str) -> None:
    result = _parse(text)

    assert isinstance(result, TextProtocolError), result
    assert "nested" in result.message


def test_nesting_up_to_the_limit_is_accepted() -> None:
    args = {"command": "ls", "x": json.loads(_nested(28))}

    assert _parse(json.dumps({"thought": "t", "tool": "bash", "args": args})) == _call("bash", args, "t")


@pytest.mark.parametrize(
    "text",
    [
        pytest.param('{"a": ' + "[" * 199_990, id="open-brackets"),
        pytest.param('{"thought": "t", "tool": "bash", "args": ' + _nested(90_000) + "}", id="closed-brackets"),
    ],
)
def test_deep_nesting_is_bounded(text: str) -> None:
    elapsed, _ = _elapsed(text)

    assert elapsed < 0.5


def test_ambiguous_case_insensitive_name_is_unknown() -> None:
    result = parse_tool_turn('{"thought": "t", "tool": "BASH", "args": {}}', ("bash", "Bash"))

    assert isinstance(result, TextProtocolError)
    assert "unknown tool 'BASH'" in result.message


def test_complete_call_survives_a_cut_off_tail() -> None:
    """With a grammar a complete object followed by a whitespace loop hits the token limit."""
    text = '{"thought": "t", "tool": "read_file", "args": {"file_path": "a.py"}}' + " " * 500

    assert _parse(text, truncated=True) == _call("read_file", READ, "t")


def test_lone_surrogates_are_replaced() -> None:
    text = (
        '{"thought": "a\\ud800b", "tool": "write_file", "args": {"file_path": "x\\udfff", "content": "\\ud83d\\ude00"}}'
    )

    result = _parse(text)

    assert result == _call("write_file", {"file_path": "x�", "content": "\U0001f600"}, "a�b")
    assert isinstance(result, TextToolCall)
    for value in (result.thought, *result.args.values()):
        str(value).encode("utf-8")


def test_nested_argument_strings_are_made_utf8_safe() -> None:
    text = '{"thought": "", "tool": "bash", "args": {"command": "ls", "env": {"A": ["\\ud800"]}}}'

    result = _parse(text)

    assert isinstance(result, TextToolCall)
    assert result.args == {"command": "ls", "env": {"A": ["�"]}}


def test_a_1mb_reply_is_parsed_in_bounded_time() -> None:
    noise = "{x} {" * 200_000 + "a" * 200_000
    text = noise + '{"thought": "t", "tool": "read_file", "args": {"file_path": "a.py"}}'

    start = time.monotonic()
    result = _parse(text)
    elapsed = time.monotonic() - start

    assert elapsed < 2.0
    assert isinstance(result, (TextFinal, TextProtocolError))


def _elapsed(text: str, *, truncated: bool = False) -> tuple[float, TextToolCall | TextFinal | TextProtocolError]:
    start = time.monotonic()
    result = _parse(text, truncated=truncated)
    return time.monotonic() - start, result


@pytest.mark.parametrize(
    "text",
    [
        pytest.param("<think>" * 8000, id="unterminated-think-tags"),
        pytest.param("<think>" * 8000 + "</think>", id="think-tags-closed-once"),
        pytest.param("x" + "<think>" * 40_000 + '{"thought": "t", "final": "x"}', id="think-tags-in-prose"),
    ],
)
def test_think_handling_is_linear(text: str) -> None:
    elapsed, _ = _elapsed(text)

    assert elapsed < 0.5


@pytest.mark.parametrize(
    "text",
    [
        pytest.param("x" + "\n" * 40_000, id="newlines"),
        pytest.param("x" + " \n\t" * 40_000 + "Action", id="whitespace-lines"),
    ],
)
def test_the_call_check_is_linear(text: str) -> None:
    """S9-C review: ^\\s*Action Input\\s*: in MULTILINE mode was quadratic over blank lines."""
    elapsed, result = _elapsed(text)

    assert elapsed < 0.5
    assert isinstance(result, TextFinal)


def test_an_indented_react_call_is_still_an_error() -> None:
    result = _parse('Thought: x\n  Action: bash\n  Action Input: {"command": "ls"}')

    assert isinstance(result, TextProtocolError)


def test_a_huge_valid_call_is_parsed() -> None:
    content = "x" * 150_000
    text = '{"thought": "t", "tool": "write_file", "args": {"file_path": "a.txt", "content": "' + content + '"}}'

    assert _parse(text) == _call("write_file", {"file_path": "a.txt", "content": content}, "t")


def test_transition_to_act_only_with_plan_act() -> None:
    reply = f'{{"thought": "Plan ready.", "tool": "{TRANSITION_TOOL}", "args": {{}}}}'
    tools: list[dict[str, object]] = [
        {"type": "function", "function": {"name": "read_file", "description": "Read.", "parameters": {}}}
    ]

    without = TextToolProtocol(tools).parse(reply)
    with_plan = TextToolProtocol(tools, plan_act=True).parse(reply)

    assert isinstance(without, TextProtocolError)
    assert with_plan == _call(TRANSITION_TOOL, {}, "Plan ready.")


def test_protocol_parse_uses_its_tools() -> None:
    tools: list[dict[str, object]] = [
        {"type": "function", "function": {"name": "bash", "description": "Run.", "parameters": {}}}
    ]
    protocol = TextToolProtocol(tools)

    assert protocol.parse('{"thought": "", "tool": "bash", "args": {"command": "ls"}}') == _call(
        "bash", {"command": "ls"}
    )
    assert isinstance(protocol.parse('{"thought": "", "tool": "read_file", "args": {}}'), TextProtocolError)
    assert isinstance(protocol.parse('{"thought": "", "tool": "bash"', truncated=True), TextProtocolError)


def test_repair_message() -> None:
    message = TextToolProtocol([]).repair_message(TextProtocolError("unknown tool 'x'; the tools are: bash"))

    assert message.startswith("[System] Your last reply could not be used: unknown tool 'x'; the tools are: bash.")
    assert "Reply with exactly one JSON object" in message
    assert '"tool"' in message
    assert '"final"' in message


def test_call_ids_are_unique_and_9_alphanumeric_characters() -> None:
    ids = [_new_call_id() for _ in range(2000)]

    assert len(set(ids)) == len(ids)
    assert all(re.fullmatch(r"[A-Za-z0-9]{9}", i) for i in ids)
