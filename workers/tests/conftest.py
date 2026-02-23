"""Shared fixtures for the workers test suite."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from tests.fake_llm import FakeLLM

SCENARIOS_DIR = Path(__file__).parent / "scenarios"


def load_scenario(role: str, scenario: str) -> tuple[dict[str, Any], dict[str, Any], FakeLLM]:
    """Load a scenario's input, expected output, and FakeLLM from fixtures.

    Returns (input_data, expected_output, fake_llm).
    """
    base = SCENARIOS_DIR / role / scenario
    input_data: dict[str, Any] = json.loads((base / "input.json").read_text())
    expected: dict[str, Any] = json.loads((base / "expected_output.json").read_text())
    fake_llm = FakeLLM.from_fixture(base / "llm_responses.json")
    return input_data, expected, fake_llm
