"""Shared fixtures for the workers test suite.

TODO: FIX-101: Consolidate FakeLLM helpers. Currently FakeLLM lives in
tests/fake_llm.py and is imported directly. Some test files define their
own inline mock LLM classes. Unify all fake LLM implementations into a
single conftest fixture (e.g., `@pytest.fixture def fake_llm()`) so that
test setup is consistent and mock behavior is centralized.

TODO (FIX-066 to FIX-070): Missing test coverage for the following modules:
  - codeforge/memory/storage.py (MemoryStore: store, recall, embedding edge cases)
  - codeforge/memory/experience.py (ExperiencePool: lookup, store, invalidate)
  - codeforge/consumer/_conversation.py (ConversationHandlerMixin: routing, fallback chain)
  - codeforge/routing/router.py (HybridRouter: cascade routing, rate limiting)
  - codeforge/routing/reward.py (compute_reward: edge cases, config variations)
  - codeforge/agent_loop.py (_compute_rollout_score, _should_early_stop, MultiRolloutExecutor)
  - codeforge/plan_act.py (plan/act mode switching, plan validation)
  See audit report for full list. Add tests incrementally.
"""

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
