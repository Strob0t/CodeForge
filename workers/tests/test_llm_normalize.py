"""Tests for _normalize_tool_call and _parse_tool_calls helpers."""

from __future__ import annotations

from codeforge.llm import _normalize_tool_call, _parse_tool_calls

# ---------------------------------------------------------------------------
# _normalize_tool_call tests
# ---------------------------------------------------------------------------


class TestNormalizeToolCall:
    """Tests for _normalize_tool_call."""

    def test_normal_call_unchanged(self) -> None:
        """When arguments is non-empty, name and arguments are returned as-is."""
        name, args = _normalize_tool_call("read_file", '{"path": "main.py"}')
        assert name == "read_file"
        assert args == '{"path": "main.py"}'

    def test_groq_embedded_json(self) -> None:
        """Groq/Llama style: JSON embedded in name, empty arguments."""
        name, args = _normalize_tool_call('read_file{"path": "main.py"}', "")
        assert name == "read_file"
        assert args == '{"path": "main.py"}'

    def test_groq_embedded_with_whitespace_args(self) -> None:
        """Arguments that are only whitespace trigger normalization."""
        name, args = _normalize_tool_call('write_file{"content": "x"}', "   ")
        assert name == "write_file"
        assert args == '{"content": "x"}'

    def test_no_brace_empty_args(self) -> None:
        """Name without brace and empty arguments returns as-is."""
        name, args = _normalize_tool_call("echo", "")
        assert name == "echo"
        assert args == ""

    def test_brace_at_position_zero(self) -> None:
        """Brace at position 0 means no name prefix — returns as-is."""
        name, args = _normalize_tool_call('{"key": "val"}', "")
        assert name == '{"key": "val"}'
        assert args == ""

    def test_trailing_space_before_brace(self) -> None:
        """Trailing spaces before the brace in name are stripped."""
        name, args = _normalize_tool_call('my_tool  {"a": 1}', "")
        assert name == "my_tool"
        assert args == '{"a": 1}'


# ---------------------------------------------------------------------------
# _parse_tool_calls tests
# ---------------------------------------------------------------------------


class TestParseToolCalls:
    """Tests for _parse_tool_calls."""

    def test_empty_list(self) -> None:
        """Empty list produces empty result."""
        assert _parse_tool_calls([]) == []

    def test_non_list_input(self) -> None:
        """Non-list input returns empty."""
        assert _parse_tool_calls(None) == []
        assert _parse_tool_calls("string") == []
        assert _parse_tool_calls(42) == []

    def test_valid_tool_call(self) -> None:
        """Standard tool call is parsed correctly."""
        raw = [
            {
                "id": "call_1",
                "function": {"name": "read_file", "arguments": '{"path": "x.py"}'},
            }
        ]
        result = _parse_tool_calls(raw)
        assert len(result) == 1
        assert result[0].id == "call_1"
        assert result[0].name == "read_file"
        assert result[0].arguments == '{"path": "x.py"}'

    def test_missing_function_key_skipped(self) -> None:
        """Entry without 'function' key is skipped."""
        raw = [{"id": "call_1"}]
        assert _parse_tool_calls(raw) == []

    def test_non_dict_entries_skipped(self) -> None:
        """Non-dict entries in the list are skipped."""
        raw = ["not_a_dict", 42, None]
        assert _parse_tool_calls(raw) == []

    def test_normalization_applied(self) -> None:
        """Groq-style embedded JSON is normalized during parsing."""
        raw = [
            {
                "id": "call_2",
                "function": {"name": 'echo{"msg": "hi"}', "arguments": ""},
            }
        ]
        result = _parse_tool_calls(raw)
        assert len(result) == 1
        assert result[0].name == "echo"
        assert result[0].arguments == '{"msg": "hi"}'

    def test_missing_id_defaults_empty(self) -> None:
        """Missing 'id' defaults to empty string."""
        raw = [{"function": {"name": "test_tool", "arguments": "{}"}}]
        result = _parse_tool_calls(raw)
        assert result[0].id == ""

    def test_missing_arguments_defaults_empty(self) -> None:
        """Missing 'arguments' defaults to empty string."""
        raw = [{"id": "c1", "function": {"name": "test_tool"}}]
        result = _parse_tool_calls(raw)
        assert result[0].arguments == ""
