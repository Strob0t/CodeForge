"""Tests for PromptEvolutionHandlerMixin (FIX-033).

Verifies:
- Mixin class exists and has expected methods
- Uses _handle_request pattern (delegates msg.ack/nak)
- Error handling uses `except Exception as exc:` (not bare)
"""

from __future__ import annotations

import inspect

from codeforge.consumer._prompt_evolution import PromptEvolutionHandlerMixin


class TestPromptEvolutionStructure:
    """PromptEvolutionHandlerMixin has expected interface."""

    def test_class_exists(self) -> None:
        assert inspect.isclass(PromptEvolutionHandlerMixin)

    def test_has_handle_reflect(self) -> None:
        assert hasattr(PromptEvolutionHandlerMixin, "_handle_prompt_evolution_reflect")
        assert inspect.iscoroutinefunction(PromptEvolutionHandlerMixin._handle_prompt_evolution_reflect)

    def test_has_do_reflect(self) -> None:
        assert hasattr(PromptEvolutionHandlerMixin, "_do_prompt_evolution_reflect")
        assert inspect.iscoroutinefunction(PromptEvolutionHandlerMixin._do_prompt_evolution_reflect)


class TestPromptEvolutionErrorHandling:
    """Verify error handling patterns in prompt evolution mixin source."""

    def test_do_reflect_uses_except_exception(self) -> None:
        source = inspect.getsource(PromptEvolutionHandlerMixin._do_prompt_evolution_reflect)
        assert "except Exception as exc" in source, (
            "_do_prompt_evolution_reflect must use `except Exception as exc:`, not bare except"
        )

    def test_handle_method_delegates_to_handle_request(self) -> None:
        source = inspect.getsource(PromptEvolutionHandlerMixin._handle_prompt_evolution_reflect)
        assert "_handle_request" in source, "_handle_prompt_evolution_reflect must delegate to _handle_request"

    def test_publishes_reflect_complete(self) -> None:
        """Must publish to SUBJECT_PROMPT_EVOLUTION_REFLECT_COMPLETE."""
        source = inspect.getsource(PromptEvolutionHandlerMixin._do_prompt_evolution_reflect)
        assert "SUBJECT_PROMPT_EVOLUTION_REFLECT_COMPLETE" in source

    def test_publishes_mutate_complete(self) -> None:
        """Must publish to SUBJECT_PROMPT_EVOLUTION_MUTATE_COMPLETE on success."""
        source = inspect.getsource(PromptEvolutionHandlerMixin._do_prompt_evolution_reflect)
        assert "SUBJECT_PROMPT_EVOLUTION_MUTATE_COMPLETE" in source

    def test_uses_nats_msg_id_header(self) -> None:
        """Outgoing publishes should include Nats-Msg-Id for dedup."""
        source = inspect.getsource(PromptEvolutionHandlerMixin._do_prompt_evolution_reflect)
        assert "Nats-Msg-Id" in source
