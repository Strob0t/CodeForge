"""Edge-case tests for LLM streaming (_StreamAccumulator) and classify_error_type."""

from __future__ import annotations

import json

from codeforge.llm import LLMError, ToolCallPart, _StreamAccumulator, _strip_think_blocks, classify_error_type

# ---------------------------------------------------------------------------
# _StreamAccumulator tests
# ---------------------------------------------------------------------------


class TestStreamAccumulatorEmpty:
    """Empty stream produces sensible defaults."""

    def test_empty_stream(self) -> None:
        acc = _StreamAccumulator()
        # No chunks processed
        assert acc.content_parts == []
        assert acc.tc_accum == {}
        assert acc.finish_reason == "stop"
        assert acc.tokens_in == 0
        assert acc.tokens_out == 0
        assert acc.cost == 0.0

        tool_calls = acc.build_tool_calls(None)
        assert tool_calls == []


class TestStreamAccumulatorTextOnly:
    """Text-only chunks accumulate correctly."""

    def test_text_only_chunks(self) -> None:
        acc = _StreamAccumulator()
        chunks = [
            {"choices": [{"delta": {"content": "Hello"}, "finish_reason": None}]},
            {"choices": [{"delta": {"content": " world"}, "finish_reason": None}]},
            {"choices": [{"delta": {}, "finish_reason": "stop"}]},
        ]
        collected: list[str] = []

        def on_chunk(text: str) -> None:
            collected.append(text)

        for chunk in chunks:
            acc.process_chunk(json.dumps(chunk), on_chunk)

        assert "".join(acc.content_parts) == "Hello world"
        assert collected == ["Hello", " world"]
        assert acc.finish_reason == "stop"


class TestStreamAccumulatorToolCalls:
    """Tool call assembly from streaming deltas."""

    def test_tool_call_assembly(self) -> None:
        acc = _StreamAccumulator()
        chunks = [
            {
                "choices": [
                    {
                        "delta": {
                            "tool_calls": [
                                {
                                    "index": 0,
                                    "id": "call_1",
                                    "function": {"name": "read_file", "arguments": '{"pa'},
                                }
                            ]
                        },
                        "finish_reason": None,
                    }
                ]
            },
            {
                "choices": [
                    {
                        "delta": {
                            "tool_calls": [
                                {
                                    "index": 0,
                                    "function": {"arguments": 'th": "main.py"}'},
                                }
                            ]
                        },
                        "finish_reason": None,
                    }
                ]
            },
            {"choices": [{"delta": {}, "finish_reason": "tool_calls"}]},
        ]

        for chunk in chunks:
            acc.process_chunk(json.dumps(chunk), None)

        tool_calls = acc.build_tool_calls(None)
        assert len(tool_calls) == 1
        assert tool_calls[0].id == "call_1"
        assert tool_calls[0].name == "read_file"
        assert json.loads(tool_calls[0].arguments) == {"path": "main.py"}
        assert acc.finish_reason == "tool_calls"


class TestStreamAccumulatorMixed:
    """Mixed content and tool calls."""

    def test_mixed_content_and_tools(self) -> None:
        acc = _StreamAccumulator()
        chunks = [
            {"choices": [{"delta": {"content": "Thinking..."}, "finish_reason": None}]},
            {
                "choices": [
                    {
                        "delta": {
                            "tool_calls": [
                                {
                                    "index": 0,
                                    "id": "tc-1",
                                    "function": {"name": "echo", "arguments": "{}"},
                                }
                            ]
                        },
                        "finish_reason": None,
                    }
                ]
            },
            {"choices": [{"delta": {}, "finish_reason": "tool_calls"}]},
        ]

        for chunk in chunks:
            acc.process_chunk(json.dumps(chunk), None)

        assert "".join(acc.content_parts) == "Thinking..."
        assert len(acc.build_tool_calls(None)) == 1


class TestStreamAccumulatorInvalidJSON:
    """Invalid JSON chunks are silently skipped."""

    def test_invalid_json_skipped(self) -> None:
        acc = _StreamAccumulator()
        acc.process_chunk("not valid json", None)
        acc.process_chunk("{malformed", None)

        assert acc.content_parts == []
        assert acc.tc_accum == {}


class TestStreamAccumulatorDoneSignal:
    """[DONE] signal is handled by the caller, not by process_chunk."""

    def test_done_signal_is_just_text(self) -> None:
        """process_chunk does not handle [DONE] — the caller checks for it."""
        acc = _StreamAccumulator()
        # [DONE] is not valid JSON, so it gets skipped
        acc.process_chunk("[DONE]", None)
        assert acc.content_parts == []


class TestStreamAccumulatorUsage:
    """Usage tokens from the final chunk are captured."""

    def test_usage_from_final_chunk(self) -> None:
        acc = _StreamAccumulator()
        chunk = {
            "choices": [{"delta": {"content": "hi"}, "finish_reason": "stop"}],
            "usage": {"prompt_tokens": 100, "completion_tokens": 50},
        }
        acc.process_chunk(json.dumps(chunk), None)

        assert acc.tokens_in == 100
        assert acc.tokens_out == 50


class TestStreamAccumulatorOnChunkCallback:
    """on_chunk callback is invoked for each text delta."""

    def test_on_chunk_callback(self) -> None:
        acc = _StreamAccumulator()
        called_with: list[str] = []
        chunks = [
            {"choices": [{"delta": {"content": "A"}, "finish_reason": None}]},
            {"choices": [{"delta": {"content": "B"}, "finish_reason": None}]},
        ]

        for chunk in chunks:
            acc.process_chunk(json.dumps(chunk), lambda t: called_with.append(t))

        assert called_with == ["A", "B"]


class TestStreamAccumulatorOnToolCallCallback:
    """on_tool_call callback fires when build_tool_calls runs."""

    def test_on_tool_call_callback(self) -> None:
        acc = _StreamAccumulator()
        chunk = {
            "choices": [
                {
                    "delta": {
                        "tool_calls": [
                            {
                                "index": 0,
                                "id": "tc-cb",
                                "function": {"name": "test_tool", "arguments": "{}"},
                            }
                        ]
                    },
                    "finish_reason": None,
                }
            ]
        }
        acc.process_chunk(json.dumps(chunk), None)

        received: list[ToolCallPart] = []
        acc.build_tool_calls(lambda tc: received.append(tc))
        assert len(received) == 1
        assert received[0].name == "test_tool"


# ---------------------------------------------------------------------------
# Think-token filtering tests
# ---------------------------------------------------------------------------


class TestStripThinkBlocks:
    """Module-level _strip_think_blocks removes <think>...</think> from final text."""

    def test_single_block(self) -> None:
        assert _strip_think_blocks("<think>reasoning here</think>Answer") == "Answer"

    def test_multiple_blocks(self) -> None:
        text = "<think>step 1</think>Hello <think>step 2</think>world"
        assert _strip_think_blocks(text) == "Hello world"

    def test_no_think_blocks(self) -> None:
        assert _strip_think_blocks("plain text") == "plain text"

    def test_empty_string(self) -> None:
        assert _strip_think_blocks("") == ""

    def test_multiline_think_block(self) -> None:
        text = "<think>\nline 1\nline 2\n</think>\nResult"
        assert _strip_think_blocks(text) == "Result"

    def test_leading_whitespace_stripped(self) -> None:
        assert _strip_think_blocks("<think>x</think>  Answer") == "Answer"


class TestStreamAccumulatorThinkTokenFilter:
    """_StreamAccumulator strips <think> blocks from on_chunk callbacks."""

    def test_think_block_in_single_chunk(self) -> None:
        acc = _StreamAccumulator()
        collected: list[str] = []
        chunk = {"choices": [{"delta": {"content": "<think>reason</think>Answer"}, "finish_reason": None}]}
        acc.process_chunk(json.dumps(chunk), lambda t: collected.append(t))

        # on_chunk receives only visible text
        assert collected == ["Answer"]
        # content_parts stores everything (including think)
        assert "".join(acc.content_parts) == "<think>reason</think>Answer"

    def test_think_block_spans_multiple_chunks(self) -> None:
        """<think> opens in one chunk, </think> closes in another."""
        acc = _StreamAccumulator()
        collected: list[str] = []
        chunks = [
            {"choices": [{"delta": {"content": "Hi <think>start of"}, "finish_reason": None}]},
            {"choices": [{"delta": {"content": " reasoning"}, "finish_reason": None}]},
            {"choices": [{"delta": {"content": "</think> done"}, "finish_reason": None}]},
        ]
        for chunk in chunks:
            acc.process_chunk(json.dumps(chunk), lambda t: collected.append(t))

        # Only non-think text reaches the callback
        assert "".join(collected) == "Hi  done"

    def test_no_think_tokens_passthrough(self) -> None:
        """Normal text passes through unchanged."""
        acc = _StreamAccumulator()
        collected: list[str] = []
        chunks = [
            {"choices": [{"delta": {"content": "Hello"}, "finish_reason": None}]},
            {"choices": [{"delta": {"content": " world"}, "finish_reason": None}]},
        ]
        for chunk in chunks:
            acc.process_chunk(json.dumps(chunk), lambda t: collected.append(t))

        assert collected == ["Hello", " world"]

    def test_empty_think_block_suppressed(self) -> None:
        """<think></think> produces no callback invocation."""
        acc = _StreamAccumulator()
        collected: list[str] = []
        chunk = {"choices": [{"delta": {"content": "<think></think>"}, "finish_reason": None}]}
        acc.process_chunk(json.dumps(chunk), lambda t: collected.append(t))

        assert collected == []

    def test_long_think_block_suppressed(self) -> None:
        """Simulates the qwen3 4000+ char think block."""
        acc = _StreamAccumulator()
        collected: list[str] = []
        # Open think tag
        chunks = [
            {"choices": [{"delta": {"content": "<think>"}, "finish_reason": None}]},
        ]
        # 100 chunks of reasoning (simulating long think block)
        chunks.extend(
            {"choices": [{"delta": {"content": f"reasoning step {i} " * 5}, "finish_reason": None}]} for i in range(100)
        )
        # Close think tag and answer
        chunks.append({"choices": [{"delta": {"content": "</think>The answer is 42."}, "finish_reason": None}]})

        for chunk in chunks:
            acc.process_chunk(json.dumps(chunk), lambda t: collected.append(t))

        assert "".join(collected) == "The answer is 42."


# ---------------------------------------------------------------------------
# classify_error_type tests
# ---------------------------------------------------------------------------


class TestClassifyErrorType:
    """Tests for classify_error_type function."""

    def test_tpm_exceeded(self) -> None:
        """429 with 'tokens per minute' body classifies as tpm_exceeded."""
        exc = LLMError(429, "model", "Rate limit: tokens per minute exceeded")
        assert classify_error_type(exc) == "tpm_exceeded"

    def test_tpm_short_keyword(self) -> None:
        """429 with 'tpm' body classifies as tpm_exceeded."""
        exc = LLMError(429, "model", "tpm limit reached")
        assert classify_error_type(exc) == "tpm_exceeded"

    def test_billing(self) -> None:
        """402 classifies as billing."""
        exc = LLMError(402, "model", "payment required")
        assert classify_error_type(exc) == "billing"

    def test_billing_from_body(self) -> None:
        """Non-402 with billing keywords classifies as billing."""
        exc = LLMError(400, "model", "insufficient credits")
        assert classify_error_type(exc) == "billing"

    def test_auth(self) -> None:
        """401 classifies as auth."""
        exc = LLMError(401, "model", "unauthorized access")
        assert classify_error_type(exc) == "auth"

    def test_auth_403(self) -> None:
        """403 classifies as auth."""
        exc = LLMError(403, "model", "forbidden")
        assert classify_error_type(exc) == "auth"

    def test_rate_limit_default(self) -> None:
        """429 without TPM keywords classifies as rate_limit."""
        exc = LLMError(429, "model", "too many requests")
        assert classify_error_type(exc) == "rate_limit"

    def test_unknown_error_returns_none(self) -> None:
        """Unrecognized error returns None."""
        exc = LLMError(500, "model", "internal server error")
        assert classify_error_type(exc) is None

    def test_auth_from_body_keywords(self) -> None:
        """Non-401/403 with auth keywords in body classifies as auth."""
        exc = LLMError(400, "model", "invalid api key provided")
        assert classify_error_type(exc) == "auth"
