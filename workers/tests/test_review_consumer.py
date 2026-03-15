"""Tests for the review NATS consumer handler mixin."""

from __future__ import annotations

import pytest

from codeforge.models import ReviewTriggerRequestPayload


def test_review_trigger_payload_parsing() -> None:
    raw = '{"project_id": "p1", "tenant_id": "t1", "commit_sha": "abc", "source": "manual"}'
    payload = ReviewTriggerRequestPayload.model_validate_json(raw)
    assert payload.project_id == "p1"
    assert payload.source == "manual"


def test_review_trigger_payload_rejects_invalid() -> None:
    with pytest.raises(ValueError):
        ReviewTriggerRequestPayload.model_validate_json('{"bad": "data"}')
