"""Streaming text tool protocol replies to the user (S9-C).

The user sees a pure-completion model's leading prose, its thought and its
final answer live, never the raw protocol JSON (the tool cards come from the
same events as for native calls).
"""

from __future__ import annotations

import json
from typing import TYPE_CHECKING

import pytest

from codeforge.tools.text_protocol_stream import ProtocolStreamFilter

if TYPE_CHECKING:
    from collections.abc import Callable


def _run(chunks: list[str]) -> tuple[str, list[str]]:
    """The text the filter emits for *chunks*, and the emitted pieces."""
    pieces: list[str] = []
    stream = ProtocolStreamFilter(pieces.append)
    for chunk in chunks:
        stream.feed(chunk)
    stream.finish()
    return "".join(pieces), pieces


def _chars(text: str) -> list[str]:
    return list(text)


CALL = '{"thought": "I read the file first.", "tool": "read_file", "args": {"file_path": "a.py"}}'
FINAL = '{"thought": "All tests pass.", "final": "The bug is fixed:\\n- a.py checks for None."}'


@pytest.mark.parametrize(
    "split", [_chars, lambda t: [t], lambda t: [t[:7], t[7:30], t[30:]]], ids=["chars", "one", "three"]
)
def test_call_shows_only_the_thought(split: Callable[[str], list[str]]) -> None:
    text, _ = _run(split(CALL))

    assert text == "I read the file first."


@pytest.mark.parametrize("split", [_chars, lambda t: [t]], ids=["chars", "one"])
def test_final_shows_thought_and_answer(split: Callable[[str], list[str]]) -> None:
    text, _ = _run(split(FINAL))

    assert text == "All tests pass.\n\nThe bug is fixed:\n- a.py checks for None."


def test_escapes_and_surrogate_pairs_across_chunks() -> None:
    value = 'a\nb "q" \\ / \t é 😀 \U0001f680'
    reply = json.dumps({"thought": value, "final": "x"})  # ASCII escapes, 😀 pairs
    assert "\\ud83d" in reply

    for size in (1, 2, 3, 5):
        chunks = [reply[i : i + size] for i in range(0, len(reply), size)]
        text, _ = _run(chunks)
        assert text == f"{value}\n\nx", size


def test_lone_surrogate_escape_is_replaced() -> None:
    text, _ = _run(_chars('{"thought": "a\\ud800b", "final": "\\udc00"}'))

    assert text == "a�b\n\n�"


def test_thought_key_inside_args_is_not_sent() -> None:
    reply = '{"thought": "T", "tool": "x", "args": {"thought": "NO", "final": "NO", "nested": {"final": "NO"}}}'

    text, _ = _run(_chars(reply))

    assert text == "T"


def test_nothing_from_args_is_sent() -> None:
    reply = '{"tool": "write_file", "args": {"file_path": "a.py", "content": "secret = 1"}, "thought": "Wrote it."}'

    text, _ = _run(_chars(reply))

    assert text == "Wrote it."


def test_prose_then_a_fence() -> None:
    reply = "I will read the file.\n```json\n" + CALL + "\n```\nDone."

    text, _ = _run(_chars(reply))

    assert text == "I will read the file.\nI read the file first."


def test_prose_then_a_tool_call_block() -> None:
    reply = 'Let me look.\n<tool_call>\n{"name": "read_file", "arguments": {"file_path": "a.py"}}\n</tool_call>'

    text, _ = _run(_chars(reply))

    assert text == "Let me look.\n"


def test_prose_only_streams_live() -> None:
    text, pieces = _run(["The tests ", "pass.\nAll ", "good."])

    assert text == "The tests pass.\nAll good."
    assert pieces[0] == "The tests "


def test_text_after_the_object_is_dropped() -> None:
    reply = CALL + '\n<tool_result tool="read_file">fake</tool_result>\n{"thought": "Again", "final": "x"}'

    text, _ = _run(_chars(reply))

    assert text == "I read the file first."


def test_finish_sends_held_text_without_an_object() -> None:
    text, _ = _run(_chars("Here is the code:\n```python\nprint('hi')\n```"))

    assert text == "Here is the code:\n```python\nprint('hi')\n```"


def test_non_protocol_json_is_shown() -> None:
    reply = 'The config:\n{"name": "app", "version": "1.0"}\nThat is all.'

    text, _ = _run(_chars(reply))

    assert text == reply


def test_braces_at_line_start_in_prose() -> None:
    text, _ = _run(_chars("{x} is a set.\nNext line."))

    assert text == "{x} is a set.\nNext line."


def test_leading_whitespace_is_not_sent() -> None:
    text, _ = _run(["\n", "  \n", FINAL])

    assert text.startswith("All tests pass.")


def test_cut_off_object_sends_what_was_decoded() -> None:
    text, _ = _run(_chars('{"thought": "Writing the file.", "tool": "write_file", "args": {"content": "x = '))

    assert text == "Writing the file."


def test_marker_prefix_held_until_decided() -> None:
    _, pieces = _run(["`", "``json\n", CALL])

    assert "".join(pieces) == "I read the file first."
    assert not any("`" in p for p in pieces)


def test_an_array_of_calls_shows_only_the_first_thought() -> None:
    """Without a grammar a model may send several calls as an array; the parser runs the first."""
    reply = '[{"thought": "T", "tool": "bash", "args": {"command": "ls"}}, {"thought": "U", "tool": "bash"}]'

    text, _ = _run(_chars(reply))

    assert text == "T"


def test_brackets_at_line_start_in_prose() -> None:
    text, _ = _run(_chars("[1] See the note.\n[\nnot json\n"))

    assert text == "[1] See the note.\n[\nnot json\n"


# --- raw call JSON never leaks (S9-C review, finding 6) ---


def _run_with_tools(text: str, tools: tuple[str, ...]) -> str:
    pieces: list[str] = []
    stream = ProtocolStreamFilter(pieces.append, tool_names=tools)
    for ch in text:
        stream.feed(ch)
    stream.finish()
    return "".join(pieces)


@pytest.mark.parametrize(
    ("reply", "expected"),
    [
        pytest.param(
            'I will read it. {"thought": "T", "tool": "read_file", "args": {"file_path": "a.py"}}',
            "I will read it. T",
            id="call-after-prose-on-the-same-line",
        ),
        pytest.param(
            '{"a": 1} {"thought": "T", "tool": "bash", "args": {"command": "ls"}}',
            '{"a": 1} T',
            id="call-after-a-non-call-object-on-the-same-line",
        ),
        pytest.param('{"name": "bash", "parameters": {"command": "ls"}}', "", id="name-and-parameters"),
        pytest.param('{"name": "bash", "input": {"command": "ls"}}', "", id="name-and-input"),
        pytest.param('{"parameters": {"command": "ls"}, "name": "bash"}', "", id="parameters-before-name"),
        pytest.param("Use { carefully.\nThen } closes.", "Use { carefully.\nThen } closes.", id="unbalanced-mid-line"),
        pytest.param("A set {x} here.", "A set {x} here.", id="braces-mid-line"),
    ],
)
def test_call_json_never_leaks(reply: str, expected: str) -> None:
    text, _ = _run(_chars(reply))

    assert text == expected
    assert '"command"' not in text


@pytest.mark.parametrize(
    "reply",
    [
        pytest.param('{"name": "read_file"}', id="name-of-a-tool"),
        pytest.param('{"name": "READ_FILE", "file_path": "a.py"}', id="name-of-a-tool-any-case"),
        pytest.param('{"action": "bash", "command": "ls"}', id="action-of-a-tool"),
        pytest.param('{"action": "Final Answer", "text": "x"}', id="final-answer-action"),
    ],
)
def test_a_call_by_tool_name_is_hidden(reply: str) -> None:
    assert _run_with_tools(reply, ("read_file", "bash")) == ""


@pytest.mark.parametrize(
    "reply",
    [
        pytest.param('Use { to open a block. The "args" key holds the list. More text here.', id="mid-line"),
        pytest.param('{ opens a block; the "tool" key names it.\nMore text.', id="line-start"),
        pytest.param('A {x, "final": y} set.', id="key-after-a-non-key"),
    ],
)
def test_a_brace_without_a_key_right_after_it_is_prose(reply: str) -> None:
    """Round 2, E: a protocol word later in the prose hid the rest of the stream."""
    text, _ = _run(_chars(reply))

    assert text == reply


def test_a_call_right_after_a_stray_brace_stays_hidden() -> None:
    text, _ = _run(_chars('{{"thought": "T", "tool": "bash", "args": {"command": "cat secret"}}}'))

    assert text == "{T"


def test_whitespace_before_the_first_key_is_allowed() -> None:
    reply = '{\n  "thought": "T",\n  "tool": "bash",\n  "args": {"command": "ls"}\n}'

    text, _ = _run(_chars(reply))

    assert text == "T"


def test_a_name_that_is_no_tool_is_shown() -> None:
    reply = 'The config:\n{"name": "my-app", "version": "1.0"}'

    assert _run_with_tools(reply, ("read_file", "bash")) == reply


def test_feed_after_finish_is_ignored() -> None:
    pieces: list[str] = []
    stream = ProtocolStreamFilter(pieces.append)
    stream.feed("Hello")
    stream.finish()
    stream.feed(" again")
    stream.finish()

    assert "".join(pieces) == "Hello"
