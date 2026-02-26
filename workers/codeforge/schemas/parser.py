"""Structured output parser: validates LLM JSON against Pydantic schemas with retry."""

from __future__ import annotations

import json
import logging
from typing import TYPE_CHECKING, TypeVar

from pydantic import BaseModel, ValidationError

if TYPE_CHECKING:
    from codeforge.llm import LiteLLMClient

logger = logging.getLogger(__name__)

T = TypeVar("T", bound=BaseModel)

MAX_RETRIES = 2


class StructuredOutputParser:
    """Wraps LiteLLM response_format for structured JSON output with Pydantic validation.

    Usage:
        parser = StructuredOutputParser(llm_client)
        result = await parser.parse(
            messages=messages,
            schema=DecomposeOutput,
            model="openai/gpt-4o",
        )
    """

    def __init__(self, llm: LiteLLMClient) -> None:
        self._llm = llm

    async def parse(
        self,
        messages: list[dict[str, object]],
        schema: type[T],
        model: str = "",
        temperature: float = 0.2,
        tags: list[str] | None = None,
    ) -> T:
        """Call LLM with structured output and validate against Pydantic schema.

        Retries up to MAX_RETRIES times on validation failure, including the
        validation error in the retry prompt for self-correction.
        """
        json_schema = _pydantic_to_json_schema(schema)
        last_error: str = ""

        for attempt in range(1 + MAX_RETRIES):
            current_messages = list(messages)
            if attempt > 0 and last_error:
                current_messages.append(
                    {
                        "role": "user",
                        "content": (
                            f"Your previous JSON response failed validation:\n{last_error}\n\n"
                            "Please fix the JSON and try again. Return ONLY valid JSON."
                        ),
                    }
                )

            response = await self._llm.chat_completion(
                messages=current_messages,
                model=model or "ollama/llama3.2",
                temperature=temperature,
                tags=tags,
                response_format={
                    "type": "json_schema",
                    "json_schema": {
                        "name": schema.__name__,
                        "schema": json_schema,
                        "strict": True,
                    },
                },
            )

            content = response.content.strip()
            try:
                parsed = json.loads(content)
                return schema.model_validate(parsed)
            except (json.JSONDecodeError, ValidationError) as exc:
                last_error = str(exc)
                logger.warning(
                    "structured output validation failed (attempt %d/%d): %s",
                    attempt + 1,
                    1 + MAX_RETRIES,
                    last_error,
                )

        msg = f"structured output validation failed after {1 + MAX_RETRIES} attempts: {last_error}"
        raise ValueError(msg)


def _pydantic_to_json_schema(schema: type[BaseModel]) -> dict[str, object]:
    """Convert a Pydantic model to a JSON Schema dict suitable for LLM response_format."""
    raw = schema.model_json_schema()
    # Remove fields not supported by OpenAI's strict JSON schema
    _clean_schema(raw)
    return raw


def _clean_schema(schema: dict[str, object]) -> None:
    """Remove unsupported fields from a JSON schema for LLM response_format compatibility."""
    schema.pop("title", None)
    schema.pop("description", None)
    # Recursively clean nested schemas
    if "$defs" in schema:
        for defn in schema["$defs"].values():
            if isinstance(defn, dict):
                _clean_schema(defn)
    props = schema.get("properties")
    if isinstance(props, dict):
        for prop in props.values():
            if isinstance(prop, dict):
                _clean_schema(prop)
    items = schema.get("items")
    if isinstance(items, dict):
        _clean_schema(items)
