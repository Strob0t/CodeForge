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


def test_feed_after_finish_is_ignored() -> None:
    pieces: list[str] = []
    stream = ProtocolStreamFilter(pieces.append)
    stream.feed("Hello")
    stream.finish()
    stream.feed(" again")
    stream.finish()

    assert "".join(pieces) == "Hello"
