"""Stall error contract with the Go Core (S6-F 8).

The Go Core re-plans a plan step whose run stalled (run.Run.Stalled): a
failed run whose error starts with the stall marker. The agent loop's stall
abort must produce such an error; internal/domain/run/testdata/
stall_contract.json is the contract both sides test against.
"""

from __future__ import annotations

import json
from pathlib import Path

from codeforge.stall_detection import STALL_ERROR_MARKER, stall_error

CONTRACT = Path(__file__).parent.parent.parent / "internal" / "domain" / "run" / "testdata" / "stall_contract.json"


def _contract() -> dict[str, object]:
    return json.loads(CONTRACT.read_text())


def test_marker_matches_the_go_contract() -> None:
    assert _contract()["marker"] == STALL_ERROR_MARKER


def test_stall_error_is_the_contract_example() -> None:
    c = _contract()
    error = stall_error(c["worker_repeated_action"], c["worker_escape_count"])
    assert error == c["worker_error"]
    assert error.startswith(STALL_ERROR_MARKER)
