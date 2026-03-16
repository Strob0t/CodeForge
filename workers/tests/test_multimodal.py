"""Tests for multimodal image support in the Python worker pipeline (Phase 32I).

Covers:
1. MessageImagePayload model validation
2. ConversationMessagePayload with images round-trip JSON
3. _to_msg_dict with images -> content-array format
4. _to_msg_dict without images -> plain string (backward compat)
5. Image token estimation (~1000 per image)
6. Go-generated JSON fixture parsing with Pydantic
"""

from __future__ import annotations

from codeforge.history import (
    ConversationHistoryManager,
    HistoryConfig,
    estimate_tokens,
)
from codeforge.models import (
    ConversationMessagePayload,
    ConversationToolCallFunction,
    ConversationToolCallPayload,
    MessageImagePayload,
)

# ---------------------------------------------------------------------------
# 1. MessageImagePayload validates correctly
# ---------------------------------------------------------------------------


class TestMessageImagePayload:
    """MessageImagePayload model validation."""

    def test_basic_construction(self) -> None:
        img = MessageImagePayload(data="abc123", media_type="image/png")
        assert img.data == "abc123"
        assert img.media_type == "image/png"
        assert img.alt_text == ""

    def test_with_alt_text(self) -> None:
        img = MessageImagePayload(data="xyz", media_type="image/jpeg", alt_text="a photo")
        assert img.alt_text == "a photo"

    def test_json_round_trip(self) -> None:
        img = MessageImagePayload(data="AAAA", media_type="image/webp", alt_text="test")
        json_str = img.model_dump_json()
        restored = MessageImagePayload.model_validate_json(json_str)
        assert restored.data == "AAAA"
        assert restored.media_type == "image/webp"
        assert restored.alt_text == "test"

    def test_empty_data_allowed(self) -> None:
        img = MessageImagePayload(data="", media_type="image/png")
        assert img.data == ""

    def test_empty_media_type_allowed(self) -> None:
        img = MessageImagePayload(data="abc", media_type="")
        assert img.media_type == ""


# ---------------------------------------------------------------------------
# 2. ConversationMessagePayload with images round-trips JSON
# ---------------------------------------------------------------------------


class TestConversationMessagePayloadImages:
    """ConversationMessagePayload with images field."""

    def test_default_images_empty(self) -> None:
        msg = ConversationMessagePayload(role="user", content="hello")
        assert msg.images == []

    def test_images_populated(self) -> None:
        msg = ConversationMessagePayload(
            role="user",
            content="describe this",
            images=[
                MessageImagePayload(data="abc123", media_type="image/png", alt_text="sketch"),
            ],
        )
        assert len(msg.images) == 1
        assert msg.images[0].media_type == "image/png"

    def test_multiple_images(self) -> None:
        msg = ConversationMessagePayload(
            role="user",
            content="compare",
            images=[
                MessageImagePayload(data="a", media_type="image/png"),
                MessageImagePayload(data="b", media_type="image/jpeg"),
                MessageImagePayload(data="c", media_type="image/webp"),
            ],
        )
        assert len(msg.images) == 3

    def test_json_round_trip_with_images(self) -> None:
        msg = ConversationMessagePayload(
            role="user",
            content="look at this",
            images=[
                MessageImagePayload(data="base64data", media_type="image/png", alt_text="diagram"),
            ],
        )
        json_str = msg.model_dump_json()
        restored = ConversationMessagePayload.model_validate_json(json_str)
        assert len(restored.images) == 1
        assert restored.images[0].data == "base64data"
        assert restored.images[0].alt_text == "diagram"

    def test_json_round_trip_without_images(self) -> None:
        """Backward compatibility: messages without images field still parse."""
        msg = ConversationMessagePayload(role="assistant", content="hi")
        json_str = msg.model_dump_json()
        restored = ConversationMessagePayload.model_validate_json(json_str)
        assert restored.images == []

    def test_backward_compat_no_images_key(self) -> None:
        """JSON without 'images' key parses with default empty list."""
        raw = '{"role":"user","content":"hello"}'
        msg = ConversationMessagePayload.model_validate_json(raw)
        assert msg.images == []
        assert msg.content == "hello"


# ---------------------------------------------------------------------------
# 3. _to_msg_dict with images -> content-array format
# ---------------------------------------------------------------------------


class TestToMsgDictWithImages:
    """_to_msg_dict produces OpenAI content-array when images present."""

    def test_user_message_with_text_and_image(self) -> None:
        mgr = ConversationHistoryManager()
        msg = ConversationMessagePayload(
            role="user",
            content="describe this image",
            images=[
                MessageImagePayload(data="abc123", media_type="image/png", alt_text="sketch"),
            ],
        )
        result = mgr._to_msg_dict(msg)

        assert result["role"] == "user"
        content = result["content"]
        assert isinstance(content, list)
        assert len(content) == 2

        # First part is text
        assert content[0]["type"] == "text"
        assert content[0]["text"] == "describe this image"

        # Second part is image_url
        assert content[1]["type"] == "image_url"
        assert content[1]["image_url"]["url"] == "data:image/png;base64,abc123"

    def test_user_message_with_multiple_images(self) -> None:
        mgr = ConversationHistoryManager()
        msg = ConversationMessagePayload(
            role="user",
            content="compare these",
            images=[
                MessageImagePayload(data="img1data", media_type="image/png"),
                MessageImagePayload(data="img2data", media_type="image/jpeg"),
            ],
        )
        result = mgr._to_msg_dict(msg)

        content = result["content"]
        assert isinstance(content, list)
        assert len(content) == 3  # 1 text + 2 images
        assert content[0]["type"] == "text"
        assert content[1]["image_url"]["url"] == "data:image/png;base64,img1data"
        assert content[2]["image_url"]["url"] == "data:image/jpeg;base64,img2data"

    def test_user_message_images_only_no_text(self) -> None:
        """Images without text content should still produce content-array."""
        mgr = ConversationHistoryManager()
        msg = ConversationMessagePayload(
            role="user",
            content="",
            images=[
                MessageImagePayload(data="imgdata", media_type="image/png"),
            ],
        )
        result = mgr._to_msg_dict(msg)

        content = result["content"]
        assert isinstance(content, list)
        assert len(content) == 1  # just the image, no text part
        assert content[0]["type"] == "image_url"
        assert content[0]["image_url"]["url"] == "data:image/png;base64,imgdata"

    def test_non_user_role_with_images_ignored(self) -> None:
        """Only user role gets content-array format for images.

        Assistant or tool messages with images should not produce
        content-array format (LLM APIs only accept multimodal on user msgs).
        """
        mgr = ConversationHistoryManager()
        msg = ConversationMessagePayload(
            role="assistant",
            content="I see the image",
            images=[
                MessageImagePayload(data="abc", media_type="image/png"),
            ],
        )
        result = mgr._to_msg_dict(msg)
        # Content should be plain string, not array
        assert isinstance(result["content"], str)
        assert result["content"] == "I see the image"


# ---------------------------------------------------------------------------
# 4. _to_msg_dict without images -> plain string (backward compat)
# ---------------------------------------------------------------------------


class TestToMsgDictBackwardCompat:
    """_to_msg_dict produces plain string when no images."""

    def test_user_message_plain(self) -> None:
        mgr = ConversationHistoryManager()
        msg = ConversationMessagePayload(role="user", content="hello")
        result = mgr._to_msg_dict(msg)
        assert result["role"] == "user"
        assert result["content"] == "hello"
        assert isinstance(result["content"], str)

    def test_assistant_message_plain(self) -> None:
        mgr = ConversationHistoryManager()
        msg = ConversationMessagePayload(role="assistant", content="hi there")
        result = mgr._to_msg_dict(msg)
        assert isinstance(result["content"], str)

    def test_tool_result_still_truncated(self) -> None:
        mgr = ConversationHistoryManager(HistoryConfig(tool_output_max_chars=50))
        msg = ConversationMessagePayload(role="tool", content="X" * 200, tool_call_id="c1", name="bash")
        result = mgr._to_msg_dict(msg)
        assert isinstance(result["content"], str)
        assert len(result["content"]) < 200
        assert "characters omitted" in result["content"]

    def test_empty_images_list_no_content_array(self) -> None:
        """Explicitly empty images list should not produce content-array."""
        mgr = ConversationHistoryManager()
        msg = ConversationMessagePayload(role="user", content="no images", images=[])
        result = mgr._to_msg_dict(msg)
        assert isinstance(result["content"], str)

    def test_tool_calls_preserved(self) -> None:
        mgr = ConversationHistoryManager()
        msg = ConversationMessagePayload(
            role="assistant",
            content="",
            tool_calls=[
                ConversationToolCallPayload(
                    id="c1",
                    type="function",
                    function=ConversationToolCallFunction(name="bash", arguments="{}"),
                ),
            ],
        )
        result = mgr._to_msg_dict(msg)
        assert "tool_calls" in result
        assert len(result["tool_calls"]) == 1


# ---------------------------------------------------------------------------
# 5. Image tokens estimated (~1000 per image)
# ---------------------------------------------------------------------------


class TestImageTokenEstimation:
    """_msg_tokens accounts for images in content-array format."""

    def test_text_only_tokens(self) -> None:
        mgr = ConversationHistoryManager()
        msg_dict: dict[str, object] = {"role": "user", "content": "hello world"}
        tokens = mgr._msg_tokens(msg_dict)
        expected = estimate_tokens("hello world")
        assert tokens == expected

    def test_single_image_tokens(self) -> None:
        mgr = ConversationHistoryManager()
        msg_dict: dict[str, object] = {
            "role": "user",
            "content": [
                {"type": "text", "text": "describe"},
                {"type": "image_url", "image_url": {"url": "data:image/png;base64,abc"}},
            ],
        }
        tokens = mgr._msg_tokens(msg_dict)
        text_tokens = estimate_tokens("describe")
        # text + 1000 per image
        assert tokens == text_tokens + 1000

    def test_multiple_images_tokens(self) -> None:
        mgr = ConversationHistoryManager()
        msg_dict: dict[str, object] = {
            "role": "user",
            "content": [
                {"type": "text", "text": "compare"},
                {"type": "image_url", "image_url": {"url": "data:image/png;base64,a"}},
                {"type": "image_url", "image_url": {"url": "data:image/jpeg;base64,b"}},
            ],
        }
        tokens = mgr._msg_tokens(msg_dict)
        text_tokens = estimate_tokens("compare")
        assert tokens == text_tokens + 2000

    def test_images_only_no_text_tokens(self) -> None:
        mgr = ConversationHistoryManager()
        msg_dict: dict[str, object] = {
            "role": "user",
            "content": [
                {"type": "image_url", "image_url": {"url": "data:image/png;base64,x"}},
            ],
        }
        tokens = mgr._msg_tokens(msg_dict)
        assert tokens == 1000

    def test_empty_content_array_tokens(self) -> None:
        mgr = ConversationHistoryManager()
        msg_dict: dict[str, object] = {"role": "user", "content": []}
        tokens = mgr._msg_tokens(msg_dict)
        # Empty content -> min 1 token
        assert tokens >= 1


# ---------------------------------------------------------------------------
# 6. Go-generated JSON fixture parsing
# ---------------------------------------------------------------------------


class TestGoJsonFixture:
    """Parse JSON that matches Go's output format for cross-language contract."""

    def test_go_message_with_images(self) -> None:
        go_json = '{"role":"user","content":"describe this","images":[{"data":"abc123","media_type":"image/png","alt_text":"sketch"}]}'
        msg = ConversationMessagePayload.model_validate_json(go_json)
        assert msg.role == "user"
        assert msg.content == "describe this"
        assert len(msg.images) == 1
        assert msg.images[0].data == "abc123"
        assert msg.images[0].media_type == "image/png"
        assert msg.images[0].alt_text == "sketch"

    def test_go_message_multiple_images(self) -> None:
        go_json = (
            '{"role":"user","content":"compare","images":['
            '{"data":"aaa","media_type":"image/png","alt_text":""},'
            '{"data":"bbb","media_type":"image/jpeg","alt_text":"photo"}'
            "]}"
        )
        msg = ConversationMessagePayload.model_validate_json(go_json)
        assert len(msg.images) == 2
        assert msg.images[0].media_type == "image/png"
        assert msg.images[1].media_type == "image/jpeg"
        assert msg.images[1].alt_text == "photo"

    def test_go_message_no_images_field(self) -> None:
        """Go messages without images field (backward compat)."""
        go_json = '{"role":"assistant","content":"I see"}'
        msg = ConversationMessagePayload.model_validate_json(go_json)
        assert msg.images == []

    def test_go_message_empty_images(self) -> None:
        go_json = '{"role":"user","content":"hello","images":[]}'
        msg = ConversationMessagePayload.model_validate_json(go_json)
        assert msg.images == []

    def test_go_message_images_with_tool_calls(self) -> None:
        """Ensure images and tool_calls coexist (though unusual for user role)."""
        go_json = '{"role":"user","content":"see this","images":[{"data":"x","media_type":"image/png","alt_text":""}],"tool_calls":[],"tool_call_id":"","name":""}'
        msg = ConversationMessagePayload.model_validate_json(go_json)
        assert len(msg.images) == 1
        assert msg.tool_calls == []


# ---------------------------------------------------------------------------
# Integration: full pipeline build_messages with images
# ---------------------------------------------------------------------------


class TestBuildMessagesWithImages:
    """End-to-end: images flow through build_messages correctly."""

    def test_user_image_in_build_messages(self) -> None:
        mgr = ConversationHistoryManager()
        history = [
            ConversationMessagePayload(
                role="user",
                content="what is this?",
                images=[
                    MessageImagePayload(data="imgdata", media_type="image/png"),
                ],
            ),
            ConversationMessagePayload(role="assistant", content="It is a diagram."),
        ]
        msgs = mgr.build_messages("You are helpful.", history)

        assert len(msgs) == 3
        assert msgs[0]["role"] == "system"

        user_msg = msgs[1]
        assert user_msg["role"] == "user"
        content = user_msg["content"]
        assert isinstance(content, list)
        assert content[0]["type"] == "text"
        assert content[0]["text"] == "what is this?"
        assert content[1]["type"] == "image_url"

        # Assistant message is still plain string
        assert isinstance(msgs[2]["content"], str)

    def test_mixed_messages_with_and_without_images(self) -> None:
        mgr = ConversationHistoryManager()
        history = [
            ConversationMessagePayload(role="user", content="hello"),
            ConversationMessagePayload(role="assistant", content="hi"),
            ConversationMessagePayload(
                role="user",
                content="now look at this",
                images=[MessageImagePayload(data="pic", media_type="image/jpeg")],
            ),
            ConversationMessagePayload(role="assistant", content="I see it"),
        ]
        msgs = mgr.build_messages("sys", history)

        # First user message: plain string
        assert isinstance(msgs[1]["content"], str)
        # Second user message: content-array
        assert isinstance(msgs[3]["content"], list)
