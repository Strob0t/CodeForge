"""Tests for typed agent module schemas (Phase 21B)."""

from __future__ import annotations

import json
from dataclasses import dataclass
from unittest.mock import AsyncMock

import pytest
from pydantic import ValidationError

from codeforge.models import ModeConfig
from codeforge.schemas import (
    CodeGenInput,
    CodeGenOutput,
    DecomposeInput,
    DecomposeOutput,
    Issue,
    ModerateInput,
    ModerateOutput,
    ReviewInput,
    ReviewOutput,
    StructuredOutputParser,
    SubTask,
)
from codeforge.schemas.parser import _clean_schema, _pydantic_to_json_schema

# ---------------------------------------------------------------------------
# DecomposeOutput schema tests
# ---------------------------------------------------------------------------


def test_decompose_output_valid() -> None:
    """DecomposeOutput should accept valid data with all fields."""
    data = {
        "subtasks": [
            {
                "title": "Setup DB",
                "description": "Create PostgreSQL schema",
                "estimated_complexity": "large",
                "depends_on": [],
                "suggested_mode": "coder",
            },
        ],
        "execution_order": "sequential",
        "reasoning": "DB must come first.",
    }
    out = DecomposeOutput.model_validate(data)
    assert len(out.subtasks) == 1
    assert out.subtasks[0].title == "Setup DB"
    assert out.execution_order == "sequential"


def test_decompose_output_invalid_execution_order() -> None:
    """DecomposeOutput should reject invalid execution_order values."""
    data = {
        "subtasks": [],
        "execution_order": "random",
        "reasoning": "test",
    }
    with pytest.raises(ValidationError, match="execution_order"):
        DecomposeOutput.model_validate(data)


def test_subtask_invalid_complexity() -> None:
    """SubTask should reject invalid estimated_complexity values."""
    with pytest.raises(ValidationError, match="estimated_complexity"):
        SubTask(
            title="T",
            description="D",
            estimated_complexity="huge",
        )


def test_subtask_defaults() -> None:
    """SubTask should have sensible defaults for optional fields."""
    st = SubTask(title="T", description="D")
    assert st.estimated_complexity == "medium"
    assert st.depends_on == []
    assert st.suggested_mode == ""


def test_decompose_input_valid() -> None:
    """DecomposeInput should accept valid data."""
    inp = DecomposeInput(goal="Build an API", tech_context="Go 1.25", constraints=["no deps"])
    assert inp.goal == "Build an API"
    assert inp.constraints == ["no deps"]


def test_decompose_input_missing_goal() -> None:
    """DecomposeInput should require the goal field."""
    with pytest.raises(ValidationError, match="goal"):
        DecomposeInput.model_validate({})


# ---------------------------------------------------------------------------
# CodeGenOutput schema tests
# ---------------------------------------------------------------------------


def test_codegen_output_valid() -> None:
    """CodeGenOutput should accept valid data."""
    out = CodeGenOutput(
        code="func main() {}",
        tests="func TestMain() {}",
        explanation="Entry point",
        files_modified=["main.go"],
    )
    assert out.code == "func main() {}"
    assert out.files_modified == ["main.go"]


def test_codegen_output_defaults() -> None:
    """CodeGenOutput should have sensible defaults."""
    out = CodeGenOutput(code="x", explanation="e")
    assert out.tests == ""
    assert out.files_modified == []


def test_codegen_input_valid() -> None:
    """CodeGenInput should accept valid data."""
    inp = CodeGenInput(spec="Implement a REST endpoint", language="go", file_path="handler.go")
    assert inp.spec == "Implement a REST endpoint"
    assert inp.language == "go"


def test_codegen_input_missing_spec() -> None:
    """CodeGenInput should require the spec field."""
    with pytest.raises(ValidationError, match="spec"):
        CodeGenInput.model_validate({})


# ---------------------------------------------------------------------------
# ReviewOutput schema tests
# ---------------------------------------------------------------------------


def test_review_output_valid() -> None:
    """ReviewOutput should accept valid data."""
    out = ReviewOutput(
        approved=False,
        issues=[
            Issue(severity="high", file="main.go", line=42, description="Missing error check"),
        ],
        suggestions=["Add error handling"],
        summary="Needs work",
    )
    assert not out.approved
    assert len(out.issues) == 1
    assert out.issues[0].severity == "high"


def test_review_output_defaults() -> None:
    """ReviewOutput should have sensible defaults."""
    out = ReviewOutput(approved=True)
    assert out.issues == []
    assert out.suggestions == []
    assert out.summary == ""


def test_issue_invalid_severity() -> None:
    """Issue should reject invalid severity values."""
    with pytest.raises(ValidationError, match="severity"):
        Issue(severity="extreme", description="bad")


def test_issue_defaults() -> None:
    """Issue should have sensible defaults for optional fields."""
    issue = Issue(severity="low", description="Minor style")
    assert issue.file == ""
    assert issue.line == 0
    assert issue.suggestion == ""


def test_review_input_valid() -> None:
    """ReviewInput should accept valid data."""
    inp = ReviewInput(code="func main() {}", spec="REST API", criteria=["security"])
    assert inp.code == "func main() {}"


def test_review_input_missing_code() -> None:
    """ReviewInput should require the code field."""
    with pytest.raises(ValidationError, match="code"):
        ReviewInput.model_validate({})


# ---------------------------------------------------------------------------
# ModerateOutput schema tests
# ---------------------------------------------------------------------------


def test_moderate_output_valid() -> None:
    """ModerateOutput should accept valid data."""
    out = ModerateOutput(
        synthesis="Combined approach",
        decision="Use proposal A with modifications from B",
        reasoning="A is simpler, B handles edge cases",
        accepted_proposals=[0, 1],
        rejected_proposals=[2],
    )
    assert out.accepted_proposals == [0, 1]
    assert out.rejected_proposals == [2]


def test_moderate_output_defaults() -> None:
    """ModerateOutput should have sensible defaults."""
    out = ModerateOutput(synthesis="s", decision="d", reasoning="r")
    assert out.accepted_proposals == []
    assert out.rejected_proposals == []


def test_moderate_input_valid() -> None:
    """ModerateInput should accept valid data."""
    inp = ModerateInput(proposals=["Use REST", "Use gRPC"], context="microservice")
    assert len(inp.proposals) == 2


def test_moderate_input_missing_proposals() -> None:
    """ModerateInput should require the proposals field."""
    with pytest.raises(ValidationError, match="proposals"):
        ModerateInput.model_validate({})


# ---------------------------------------------------------------------------
# StructuredOutputParser tests
# ---------------------------------------------------------------------------


@dataclass
class _FakeChatResponse:
    """Minimal fake for ChatCompletionResponse."""

    content: str
    cost_usd: float = 0.0
    model: str = "fake"
    tokens_in: int = 10
    tokens_out: int = 5
    tool_calls: list = None  # type: ignore[assignment]

    def __post_init__(self) -> None:
        if self.tool_calls is None:
            self.tool_calls = []


def _make_fake_llm(responses: list[str]) -> AsyncMock:
    """Build a mock LiteLLMClient that returns pre-programmed JSON strings."""
    mock = AsyncMock()
    side_effects = [_FakeChatResponse(content=r) for r in responses]
    mock.chat_completion = AsyncMock(side_effect=side_effects)
    return mock


async def test_parser_success_first_try() -> None:
    """StructuredOutputParser should succeed on the first attempt with valid JSON."""
    valid_json = json.dumps(
        {
            "subtasks": [{"title": "A", "description": "Do A"}],
            "execution_order": "sequential",
            "reasoning": "Simple task.",
        }
    )
    llm = _make_fake_llm([valid_json])
    parser = StructuredOutputParser(llm)

    result = await parser.parse(
        messages=[{"role": "user", "content": "decompose"}],
        schema=DecomposeOutput,
    )
    assert len(result.subtasks) == 1
    assert result.subtasks[0].title == "A"
    assert llm.chat_completion.call_count == 1


async def test_parser_retry_on_invalid_json() -> None:
    """StructuredOutputParser should retry when the LLM returns invalid JSON."""
    invalid_json = "not json at all"
    valid_json = json.dumps(
        {
            "subtasks": [],
            "execution_order": "parallel",
            "reasoning": "Fixed.",
        }
    )
    llm = _make_fake_llm([invalid_json, valid_json])
    parser = StructuredOutputParser(llm)

    result = await parser.parse(
        messages=[{"role": "user", "content": "decompose"}],
        schema=DecomposeOutput,
    )
    assert result.execution_order == "parallel"
    assert llm.chat_completion.call_count == 2


async def test_parser_retry_on_validation_error() -> None:
    """StructuredOutputParser should retry when JSON is valid but schema validation fails."""
    # Missing required 'reasoning' field.
    bad_schema = json.dumps({"subtasks": [], "execution_order": "invalid_value"})
    valid_json = json.dumps(
        {
            "subtasks": [],
            "execution_order": "mixed",
            "reasoning": "Corrected.",
        }
    )
    llm = _make_fake_llm([bad_schema, valid_json])
    parser = StructuredOutputParser(llm)

    result = await parser.parse(
        messages=[{"role": "user", "content": "decompose"}],
        schema=DecomposeOutput,
    )
    assert result.reasoning == "Corrected."
    assert llm.chat_completion.call_count == 2


async def test_parser_exhausts_retries() -> None:
    """StructuredOutputParser should raise ValueError after all retries are exhausted."""
    bad = "not json"
    llm = _make_fake_llm([bad, bad, bad])
    parser = StructuredOutputParser(llm)

    with pytest.raises(ValueError, match="structured output validation failed after 3 attempts"):
        await parser.parse(
            messages=[{"role": "user", "content": "decompose"}],
            schema=DecomposeOutput,
        )
    assert llm.chat_completion.call_count == 3


async def test_parser_passes_response_format() -> None:
    """StructuredOutputParser should pass response_format with json_schema to the LLM."""
    valid_json = json.dumps(
        {
            "code": "print('hello')",
            "explanation": "Hello world",
        }
    )
    llm = _make_fake_llm([valid_json])
    parser = StructuredOutputParser(llm)

    await parser.parse(
        messages=[{"role": "user", "content": "generate code"}],
        schema=CodeGenOutput,
        model="openai/gpt-4o",
        temperature=0.1,
        tags=["codegen"],
    )

    call_kwargs = llm.chat_completion.call_args
    assert call_kwargs.kwargs["model"] == "openai/gpt-4o"
    assert call_kwargs.kwargs["temperature"] == 0.1
    assert call_kwargs.kwargs["tags"] == ["codegen"]
    rf = call_kwargs.kwargs["response_format"]
    assert rf["type"] == "json_schema"
    assert rf["json_schema"]["name"] == "CodeGenOutput"
    assert rf["json_schema"]["strict"] is True


# ---------------------------------------------------------------------------
# JSON schema generation tests
# ---------------------------------------------------------------------------


def test_pydantic_to_json_schema_removes_title() -> None:
    """_pydantic_to_json_schema should strip title and description from the schema."""
    schema = _pydantic_to_json_schema(SubTask)
    assert "title" not in schema
    assert "description" not in schema


def test_pydantic_to_json_schema_preserves_properties() -> None:
    """_pydantic_to_json_schema should preserve property definitions."""
    schema = _pydantic_to_json_schema(SubTask)
    assert "properties" in schema
    props = schema["properties"]
    assert "title" in props
    assert "description" in props
    assert "estimated_complexity" in props


def test_clean_schema_nested() -> None:
    """_clean_schema should recursively clean nested schemas."""
    schema: dict[str, object] = {
        "title": "Root",
        "description": "Top level",
        "properties": {
            "child": {
                "title": "Child",
                "description": "Nested",
                "type": "string",
            },
        },
        "$defs": {
            "SubDef": {
                "title": "SubDef",
                "description": "A definition",
                "type": "object",
            },
        },
    }
    _clean_schema(schema)
    assert "title" not in schema
    assert "description" not in schema
    assert "title" not in schema["properties"]["child"]  # type: ignore[index]
    assert "title" not in schema["$defs"]["SubDef"]  # type: ignore[index]


# ---------------------------------------------------------------------------
# ModeConfig output_schema field test
# ---------------------------------------------------------------------------


def test_mode_config_output_schema_default() -> None:
    """ModeConfig should have an empty output_schema by default."""
    mode = ModeConfig()
    assert mode.output_schema == ""


def test_mode_config_output_schema_from_json() -> None:
    """ModeConfig should deserialize output_schema from JSON correctly."""
    raw = '{"id": "reviewer", "output_schema": "ReviewOutput"}'
    mode = ModeConfig.model_validate_json(raw)
    assert mode.output_schema == "ReviewOutput"


# ---------------------------------------------------------------------------
# Edge case tests
# ---------------------------------------------------------------------------


def test_decompose_output_empty_subtasks() -> None:
    """DecomposeOutput should accept empty subtasks list."""
    out = DecomposeOutput(subtasks=[], reasoning="Nothing to do")
    assert out.subtasks == []
    assert out.execution_order == "sequential"  # default


def test_review_output_empty_strings() -> None:
    """ReviewOutput should accept empty optional strings."""
    out = ReviewOutput(approved=True, summary="")
    assert out.summary == ""
    assert out.issues == []


def test_codegen_output_empty_strings() -> None:
    """CodeGenOutput should accept empty optional strings."""
    out = CodeGenOutput(code="", explanation="")
    assert out.code == ""
    assert out.tests == ""
