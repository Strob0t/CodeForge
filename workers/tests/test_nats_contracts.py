"""Validate Go-generated JSON fixtures against Python Pydantic models.

Each test loads a JSON fixture from internal/port/messagequeue/testdata/contracts/
and validates it against the corresponding Pydantic model.  This catches field name
mismatches, type incompatibilities, and missing required fields between Go and Python.
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import pytest
from pydantic import BaseModel  # noqa: TC002 — needed at runtime for model_validate()

from codeforge.models import (
    A2ATaskCompleteMessage,
    A2ATaskCreatedMessage,
    BenchmarkRunRequest,
    BenchmarkRunResult,
    ConversationRunCompleteMessage,
    ConversationRunStartMessage,
    GemmasEvalRequest,
    GemmasEvalResult,
    GraphBuildRequest,
    GraphBuildResult,
    GraphSearchRequest,
    GraphSearchResult,
    RepoMapRequest,
    RepoMapResult,
    RetrievalIndexRequest,
    RetrievalIndexResult,
    RetrievalSearchRequest,
    RetrievalSearchResult,
    SubAgentSearchRequest,
    SubAgentSearchResult,
)

FIXTURES_DIR = Path(__file__).parent.parent.parent / "internal" / "port" / "messagequeue" / "testdata" / "contracts"

# Map fixture filename stem (dots replaced with underscores) to the Python model.
# The filename convention is the NATS subject with dots replaced by underscores.
SUBJECT_MODEL_MAP: dict[str, type[BaseModel]] = {
    "conversation.run.start": ConversationRunStartMessage,
    "conversation.run.complete": ConversationRunCompleteMessage,
    "benchmark.run.request": BenchmarkRunRequest,
    "benchmark.run.result": BenchmarkRunResult,
    "evaluation.gemmas.request": GemmasEvalRequest,
    "evaluation.gemmas.result": GemmasEvalResult,
    "repomap.generate.request": RepoMapRequest,
    "repomap.generate.result": RepoMapResult,
    "retrieval.index.request": RetrievalIndexRequest,
    "retrieval.index.result": RetrievalIndexResult,
    "retrieval.search.request": RetrievalSearchRequest,
    "retrieval.search.result": RetrievalSearchResult,
    "retrieval.subagent.request": SubAgentSearchRequest,
    "retrieval.subagent.result": SubAgentSearchResult,
    "graph.build.request": GraphBuildRequest,
    "graph.build.result": GraphBuildResult,
    "graph.search.request": GraphSearchRequest,
    "graph.search.result": GraphSearchResult,
    "a2a.task.created": A2ATaskCreatedMessage,
    "a2a.task.complete": A2ATaskCompleteMessage,
}


def _subject_to_filename(subject: str) -> str:
    """Convert a NATS subject like 'conversation.run.start' to 'conversation_run_start'."""
    return subject.replace(".", "_")


def _fixture_path(subject: str) -> Path:
    """Return the path to the JSON fixture for a given NATS subject."""
    return FIXTURES_DIR / f"{_subject_to_filename(subject)}.json"


def _has_alias(model_cls: type[BaseModel], field_name: str) -> bool:
    """Check if the model has a field with this name as an alias."""
    return any(field_info.alias == field_name for field_info in model_cls.model_fields.values())


def _model_accepts_field(model_cls: type[BaseModel], field_name: str) -> bool:
    """Check whether the model declares this field (by name or alias)."""
    if field_name in model_cls.model_fields:
        return True
    return _has_alias(model_cls, field_name)


def _collect_extra_fields(raw: dict[str, Any], model_cls: type[BaseModel]) -> list[str]:
    """Return Go fixture keys that the Python model does not declare."""
    return [key for key in raw if not _model_accepts_field(model_cls, key)]


# ---------------------------------------------------------------------------
# Parametrized IDs: use the NATS subject as the test-case ID.
# ---------------------------------------------------------------------------

_PARAMS = list(SUBJECT_MODEL_MAP.items())
_IDS = [subject for subject, _ in _PARAMS]


@pytest.mark.parametrize(("subject", "model_cls"), _PARAMS, ids=_IDS)
def test_go_fixture_validates_against_pydantic(
    subject: str,
    model_cls: type[BaseModel],
) -> None:
    """Load Go-generated JSON fixture and validate against Pydantic model.

    A ValidationError here means Python cannot deserialize what Go serialized --
    a genuine contract violation.
    """
    fixture = _fixture_path(subject)
    if not fixture.exists():
        pytest.skip(f"Fixture not yet generated: {fixture}")

    raw: dict[str, Any] = json.loads(fixture.read_text())

    # Core assertion: Pydantic can parse the Go-serialized JSON.
    instance = model_cls.model_validate(raw)

    # Re-serialize and verify no fields were silently dropped.
    roundtrip: dict[str, Any] = json.loads(instance.model_dump_json(by_alias=True))

    for key in raw:
        has_key = key in roundtrip or _has_alias(model_cls, key)
        if not has_key:
            # Extra fields in Go that Python ignores are contract warnings,
            # not necessarily fatal -- but we surface them.
            extras = _collect_extra_fields(raw, model_cls)
            if extras:
                pytest.fail(
                    f"Go fixture for '{subject}' contains fields not declared in {model_cls.__name__}: {extras}"
                )


@pytest.mark.parametrize(("subject", "model_cls"), _PARAMS, ids=_IDS)
def test_pydantic_roundtrip_matches_go_fixture(
    subject: str,
    model_cls: type[BaseModel],
) -> None:
    """Verify that Pydantic deserialization + re-serialization is lossless.

    Parses the Go fixture, serializes it back to JSON, and re-parses.
    The two Python instances must be equal.
    """
    fixture = _fixture_path(subject)
    if not fixture.exists():
        pytest.skip(f"Fixture not yet generated: {fixture}")

    raw: dict[str, Any] = json.loads(fixture.read_text())
    instance = model_cls.model_validate(raw)

    # Serialize back to JSON string and re-validate.
    serialized = instance.model_dump_json(by_alias=True)
    reparsed = model_cls.model_validate_json(serialized)

    assert reparsed == instance, f"Roundtrip mismatch for {subject}: fields differ after serialize/deserialize cycle"


@pytest.mark.parametrize(("subject", "model_cls"), _PARAMS, ids=_IDS)
def test_go_fixture_field_coverage(
    subject: str,
    model_cls: type[BaseModel],
) -> None:
    """Detect fields present in Go fixtures but absent from the Python model.

    These are contract gaps: Go is sending data that Python silently ignores.
    """
    fixture = _fixture_path(subject)
    if not fixture.exists():
        pytest.skip(f"Fixture not yet generated: {fixture}")

    raw: dict[str, Any] = json.loads(fixture.read_text())
    extras = _collect_extra_fields(raw, model_cls)

    assert not extras, f"Go fixture for '{subject}' has fields not in {model_cls.__name__}: {extras}"


@pytest.mark.parametrize(("subject", "model_cls"), _PARAMS, ids=_IDS)
def test_python_required_fields_in_go_fixture(
    subject: str,
    model_cls: type[BaseModel],
) -> None:
    """Verify every required Python field is present in the Go fixture.

    If Python declares a required field that Go never sends, the worker
    will crash at runtime on real NATS messages.
    """
    fixture = _fixture_path(subject)
    if not fixture.exists():
        pytest.skip(f"Fixture not yet generated: {fixture}")

    raw: dict[str, Any] = json.loads(fixture.read_text())

    missing: list[str] = []
    for field_name, field_info in model_cls.model_fields.items():
        # A field is required if it has no default and no default_factory.
        is_required = field_info.is_required()
        if not is_required:
            continue

        # Check both the field name and its alias.
        json_key = field_info.alias or field_name
        if json_key not in raw and field_name not in raw:
            missing.append(f"{field_name} (json: {json_key})")

    assert not missing, f"Required Python fields missing from Go fixture for '{subject}': {missing}"
