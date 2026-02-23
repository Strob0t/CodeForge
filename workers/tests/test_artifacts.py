"""Tests for artifact validation mirroring Go-side coverage."""

from __future__ import annotations

import pytest

from codeforge.artifacts import ArtifactType, is_known_type, validate_artifact


class TestIsKnownType:
    @pytest.mark.parametrize(
        "t",
        [
            ArtifactType.PLAN_MD,
            ArtifactType.DIFF,
            ArtifactType.REVIEW_MD,
            ArtifactType.TEST_REPORT,
            ArtifactType.AUDIT_REPORT,
            ArtifactType.DECISION_MD,
        ],
    )
    def test_known_types(self, t: str) -> None:
        assert is_known_type(t)

    def test_unknown_type(self) -> None:
        assert not is_known_type("UNKNOWN")


class TestValidateEmpty:
    def test_empty_type_always_valid(self) -> None:
        result = validate_artifact("", "anything")
        assert result.valid
        assert result.errors == []


class TestValidateUnknown:
    def test_unknown_type_fails(self) -> None:
        result = validate_artifact("UNKNOWN", "anything")
        assert not result.valid
        assert any("unknown artifact type" in e for e in result.errors)


class TestValidatePlanMD:
    def test_valid(self) -> None:
        output = "## Architecture Overview\n\n" + "This is a valid plan. " * 10
        result = validate_artifact("PLAN.md", output)
        assert result.valid

    def test_too_short(self) -> None:
        result = validate_artifact("PLAN.md", "## Short\nNot enough")
        assert not result.valid

    def test_no_header(self) -> None:
        result = validate_artifact("PLAN.md", "x" * 120)
        assert not result.valid

    def test_empty(self) -> None:
        result = validate_artifact("PLAN.md", "")
        assert not result.valid


class TestValidateDiff:
    def test_valid_diff(self) -> None:
        output = '--- a/file.go\n+++ b/file.go\n@@ -1,3 +1,4 @@\n+import "fmt"\n func main() {'
        result = validate_artifact("DIFF", output)
        assert result.valid

    def test_minus_line(self) -> None:
        result = validate_artifact("DIFF", "- removed line\n+ added line")
        assert result.valid

    def test_no_changes(self) -> None:
        result = validate_artifact("DIFF", "No changes needed for this task.")
        assert result.valid

    def test_empty(self) -> None:
        result = validate_artifact("DIFF", "")
        assert not result.valid

    def test_no_markers(self) -> None:
        result = validate_artifact("DIFF", "just some text without diff markers")
        assert not result.valid


class TestValidateReviewMD:
    def test_valid(self) -> None:
        output = "# Code Review\n\n## Findings\n\nFound an issue with error handling."
        result = validate_artifact("REVIEW.md", output)
        assert result.valid

    def test_too_short(self) -> None:
        result = validate_artifact("REVIEW.md", "# R\nshort")
        assert not result.valid

    def test_no_header(self) -> None:
        output = "Found an issue but no markdown header present in this output text at all."
        result = validate_artifact("REVIEW.md", output)
        assert not result.valid

    def test_no_keyword(self) -> None:
        output = "# Review\n\nThe code looks good, nothing to report here at all in detail."
        result = validate_artifact("REVIEW.md", output)
        assert not result.valid


class TestValidateTestReport:
    def test_with_status(self) -> None:
        result = validate_artifact("TEST_REPORT", '{"status": "passed", "tests": 10}')
        assert result.valid

    def test_with_passed(self) -> None:
        result = validate_artifact("TEST_REPORT", '{"passed": true, "total": 5}')
        assert result.valid

    def test_not_json(self) -> None:
        result = validate_artifact("TEST_REPORT", "this is not json")
        assert not result.valid

    def test_json_array(self) -> None:
        result = validate_artifact("TEST_REPORT", "[1, 2, 3]")
        assert not result.valid

    def test_no_status_key(self) -> None:
        result = validate_artifact("TEST_REPORT", '{"total": 5, "coverage": 80}')
        assert not result.valid


class TestValidateAuditReport:
    def test_valid(self) -> None:
        output = "# Security Audit\n\n## Findings\n\nFound a vulnerability in auth."
        result = validate_artifact("AUDIT_REPORT", output)
        assert result.valid

    def test_too_short(self) -> None:
        result = validate_artifact("AUDIT_REPORT", "# A\nshort")
        assert not result.valid

    def test_no_keyword(self) -> None:
        output = "# Audit\n\nThe code has no issues found in this review pass overall."
        result = validate_artifact("AUDIT_REPORT", output)
        assert not result.valid


class TestValidateDecisionMD:
    def test_with_rationale(self) -> None:
        output = "## Decision\n\nWe chose approach A. Rationale: it has lower complexity."
        result = validate_artifact("DECISION.md", output)
        assert result.valid

    def test_with_reason(self) -> None:
        output = "The decision was to use PostgreSQL. The reason is that it supports JSONB."
        result = validate_artifact("DECISION.md", output)
        assert result.valid

    def test_no_decision(self) -> None:
        output = "The rationale for this choice is cost efficiency across the board."
        result = validate_artifact("DECISION.md", output)
        assert not result.valid

    def test_no_rationale(self) -> None:
        output = "The decision was to proceed with the current architecture plan as designed."
        result = validate_artifact("DECISION.md", output)
        assert not result.valid

    def test_too_short(self) -> None:
        result = validate_artifact("DECISION.md", "decision reason")
        assert not result.valid
