"""Artifact type validation for run output produced by agent modes.

Mirrors the Go-side artifact domain package for optional fail-fast
pre-validation in Python workers. The Go Core remains authoritative.
"""

from __future__ import annotations

import json
from enum import StrEnum

from pydantic import BaseModel


class ArtifactType(StrEnum):
    """Known artifact types matching Go constants."""

    PLAN_MD = "PLAN.md"
    DIFF = "DIFF"
    REVIEW_MD = "REVIEW.md"
    TEST_REPORT = "TEST_REPORT"
    AUDIT_REPORT = "AUDIT_REPORT"
    DECISION_MD = "DECISION.md"


class ArtifactValidationResult(BaseModel):
    """Result of validating run output against an artifact schema."""

    valid: bool
    artifact_type: str
    errors: list[str] = []


def _validate_plan_md(output: str) -> list[str]:
    errs: list[str] = []
    if len(output) < 100:
        errs.append("PLAN.md must be at least 100 characters")
    if "##" not in output:
        errs.append("PLAN.md must contain at least one markdown H2 header (##)")
    return errs


def _validate_diff(output: str) -> list[str]:
    for line in output.split("\n"):
        if line.startswith(("+", "-")):
            return []
    if "no changes" in output.lower():
        return []
    return ["DIFF must contain diff markers (lines starting with +/-) or indicate 'no changes'"]


def _validate_review_md(output: str) -> list[str]:
    errs: list[str] = []
    if len(output) < 50:
        errs.append("REVIEW.md must be at least 50 characters")
    if "#" not in output:
        errs.append("REVIEW.md must contain at least one markdown header (#)")
    keywords = ("finding", "issue", "comment", "suggestion", "recommendation")
    lower = output.lower()
    if not any(kw in lower for kw in keywords):
        errs.append(
            "REVIEW.md must contain at least one review keyword (finding, issue, comment, suggestion, recommendation)"
        )
    return errs


def _validate_test_report(output: str) -> list[str]:
    trimmed = output.strip()
    try:
        data = json.loads(trimmed)
    except (json.JSONDecodeError, ValueError):
        return ["TEST_REPORT must be valid JSON"]
    if not isinstance(data, dict):
        return ["TEST_REPORT must be a JSON object"]
    if "status" not in data and "passed" not in data:
        return ["TEST_REPORT JSON must contain a 'status' or 'passed' key"]
    return []


def _validate_audit_report(output: str) -> list[str]:
    errs: list[str] = []
    if len(output) < 50:
        errs.append("AUDIT_REPORT must be at least 50 characters")
    if "#" not in output:
        errs.append("AUDIT_REPORT must contain at least one markdown header (#)")
    keywords = ("vulnerability", "risk", "finding", "security")
    lower = output.lower()
    if not any(kw in lower for kw in keywords):
        errs.append("AUDIT_REPORT must contain at least one security keyword (vulnerability, risk, finding, security)")
    return errs


def _validate_decision_md(output: str) -> list[str]:
    errs: list[str] = []
    if len(output) < 50:
        errs.append("DECISION.md must be at least 50 characters")
    lower = output.lower()
    if "decision" not in lower:
        errs.append("DECISION.md must contain the word 'decision'")
    if "rationale" not in lower and "reason" not in lower:
        errs.append("DECISION.md must contain the word 'rationale' or 'reason'")
    return errs


_VALIDATORS: dict[str, callable] = {
    ArtifactType.PLAN_MD: _validate_plan_md,
    ArtifactType.DIFF: _validate_diff,
    ArtifactType.REVIEW_MD: _validate_review_md,
    ArtifactType.TEST_REPORT: _validate_test_report,
    ArtifactType.AUDIT_REPORT: _validate_audit_report,
    ArtifactType.DECISION_MD: _validate_decision_md,
}


def is_known_type(artifact_type: str) -> bool:
    """Check if the artifact type is recognized."""
    return artifact_type in _VALIDATORS


def validate_artifact(artifact_type: str, output: str) -> ArtifactValidationResult:
    """Validate output against the structural requirements of an artifact type.

    An empty artifact_type returns valid=True (no artifact required).
    """
    if not artifact_type:
        return ArtifactValidationResult(valid=True, artifact_type="")

    validator = _VALIDATORS.get(artifact_type)
    if validator is None:
        return ArtifactValidationResult(
            valid=False,
            artifact_type=artifact_type,
            errors=[f"unknown artifact type: {artifact_type}"],
        )

    errs = validator(output)
    return ArtifactValidationResult(
        valid=len(errs) == 0,
        artifact_type=artifact_type,
        errors=errs,
    )
