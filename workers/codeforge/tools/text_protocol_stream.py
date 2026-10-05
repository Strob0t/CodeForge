"""Live output of text tool protocol replies (S9-C).

A pure-completion model streams one JSON object per reply. The user sees
its leading prose, the decoded ``thought`` and ``final`` strings of that
object as they arrive, and never the raw protocol JSON: tool cards come from
the same events as native calls. JSON that is no protocol object (a code
block, a config file in a prose answer) is shown as written once it ends.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

from codeforge.tools.text_protocol import TURN_KEYS

if TYPE_CHECKING:
    from collections.abc import Callable

# A prose line starting with one of these may start the protocol object
# ("[{": several calls as an array, of which the parser runs the first).
_MARKERS = ("```", "<tool_call>", "{", "[{")
_STREAMED_KEYS = frozenset({"thought", "final"})
_ESCAPES = {'"': '"', "\\": "\\", "/": "/", "b": "\b", "f": "\f", "n": "\n", "r": "\r", "t": "\t"}

_PROSE = "prose"  # text before the object streams line by line
_WAIT = "wait"  # after a fence or <tool_call>: held until the object starts
_OBJECT = "object"  # inside the first object
_DONE = "done"  # after the object: dropped


class ProtocolStreamFilter:
    """Wraps a stream's chunk callback: ``feed`` each chunk, ``finish`` at the end.

    *emit* receives the visible text, at most once per ``feed``.
    """

    def __init__(self, emit: Callable[[str], None]) -> None:
        self._emit = emit
        self._mode = _PROSE
        self._out: list[str] = []
        self._emitted = False
        # Prose: the start of the current line while it may still be a marker.
        self._line = ""
        self._at_line_start = True
        # Raw text since a marker, shown if it turns out to be no protocol object.
        self._held: list[str] = []
        self._protocol = False
        # JSON tokenizer of the object.
        self._depth = 0
        self._in_string = False
        self._escape = ""
        self._expect_key = False
        self._key: list[str] | None = None
        self._last_key = ""
        self._value_key = ""
        self._high_surrogate = 0
        self._separate = False

    def feed(self, chunk: str) -> None:
        """Process a chunk of the reply and emit what became visible."""
        for ch in chunk:
            if self._mode == _PROSE:
                self._prose(ch)
            elif self._mode == _WAIT:
                self._held.append(ch)
                if ch == "{":
                    self._start_object()
            elif self._mode == _OBJECT:
                self._object(ch)
        self._flush()

    def finish(self) -> None:
        """Emit text still held because no protocol object followed it."""
        if self._mode == _PROSE:
            self._send(self._line)
        elif self._mode in (_WAIT, _OBJECT) and not self._protocol:
            self._send("".join(self._held))
        self._line = ""
        self._held.clear()
        self._mode = _DONE
        self._flush()

    # --- prose ---

    def _prose(self, ch: str) -> None:
        if not self._at_line_start:
            self._send(ch)
            self._at_line_start = ch == "\n"
            return
        self._line += ch
        head = self._line.lstrip(" \t")
        if not head:
            return
        marker = next((m for m in _MARKERS if head.startswith(m)), None)
        if marker is not None:
            self._held = [self._line]
            self._line = ""
            if marker.endswith("{"):
                self._start_object()
            else:
                self._mode = _WAIT
            return
        if any(m.startswith(head) for m in _MARKERS):
            return
        self._send(self._line)
        self._line = ""
        self._at_line_start = ch == "\n"

    # --- object ---

    def _start_object(self) -> None:
        self._mode = _OBJECT
        self._depth = 1
        self._in_string = False
        self._escape = ""
        self._expect_key = True
        self._key = None
        self._last_key = ""
        self._value_key = ""

    def _object(self, ch: str) -> None:
        if not self._protocol:
            self._held.append(ch)
        if self._in_string:
            self._string_char(ch)
        elif ch == '"':
            self._open_string()
        elif ch in "{[":
            self._depth += 1
        elif ch in "}]":
            self._depth -= 1
            if self._depth == 0:
                self._end_object()
        elif self._depth == 1 and ch == ",":
            self._expect_key = True
        elif self._depth == 1 and ch == ":":
            self._expect_key = False

    def _open_string(self) -> None:
        self._in_string = True
        if self._depth != 1:
            return
        if self._expect_key:
            self._key = []
        elif self._protocol and self._last_key in _STREAMED_KEYS:
            self._value_key = self._last_key
            self._separate = self._value_key == "final" and self._emitted

    def _string_char(self, ch: str) -> None:
        if self._escape:
            self._escape += ch
            if self._escape[1] != "u":
                self._escape = ""
                self._put(_ESCAPES.get(ch, ch))
            elif len(self._escape) == 6:
                digits = self._escape[2:]
                self._escape = ""
                self._unicode(digits)
        elif ch == "\\":
            self._escape = ch
        elif ch == '"':
            self._close_string()
        else:
            self._put(ch)

    def _unicode(self, digits: str) -> None:
        try:
            code = int(digits, 16)
        except ValueError:
            self._put("�")
            return
        if 0xD800 <= code <= 0xDBFF:
            self._put_pending_surrogate()
            self._high_surrogate = code
        elif 0xDC00 <= code <= 0xDFFF and self._high_surrogate:
            pair = 0x10000 + ((self._high_surrogate - 0xD800) << 10) + (code - 0xDC00)
            self._high_surrogate = 0
            self._put(chr(pair))
        elif 0xDC00 <= code <= 0xDFFF:
            self._put("�")
        else:
            self._put(chr(code))

    def _put_pending_surrogate(self) -> None:
        if self._high_surrogate:
            self._high_surrogate = 0
            self._put("�")

    def _put(self, text: str) -> None:
        """A decoded string character: part of a key, of a streamed value, or dropped."""
        self._put_pending_surrogate()
        if self._key is not None:
            self._key.append(text)
        elif self._value_key:
            if self._separate:
                self._separate = False
                self._send("\n\n")
            self._send(text)

    def _close_string(self) -> None:
        self._put_pending_surrogate()
        self._in_string = False
        if self._key is not None:
            self._last_key = "".join(self._key)
            self._key = None
            if self._last_key in TURN_KEYS and not self._protocol:
                self._protocol = True
                self._held.clear()
        self._value_key = ""

    def _end_object(self) -> None:
        if self._protocol:
            self._mode = _DONE
            return
        self._send("".join(self._held))
        self._held.clear()
        self._mode = _PROSE
        self._at_line_start = False

    # --- output ---

    def _send(self, text: str) -> None:
        if not self._emitted:
            text = text.lstrip()
        if text:
            self._out.append(text)
            self._emitted = True

    def _flush(self) -> None:
        if self._out:
            text = "".join(self._out)
            self._out.clear()
            self._emit(text)
