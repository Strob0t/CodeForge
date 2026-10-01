"""Shared fixtures for the workers test suite.

TODO: FIX-101: Consolidate FakeLLM helpers. Currently FakeLLM lives in
tests/fake_llm.py and is imported directly. Some test files define their
own inline mock LLM classes. Unify all fake LLM implementations into a
single conftest fixture (e.g., `@pytest.fixture def fake_llm()`) so that
test setup is consistent and mock behavior is centralized.

TODO (FIX-066 to FIX-070): Missing test coverage for the following modules:
  - codeforge/memory/experience.py (ExperiencePool: lookup, store, invalidate)
  - codeforge/consumer/_conversation.py (ConversationHandlerMixin: routing, fallback chain)
  - codeforge/routing/router.py (HybridRouter: cascade routing, rate limiting)
  - codeforge/routing/reward.py (compute_reward: edge cases, config variations)
  - codeforge/quality_tracking.py (compute_rollout_score, should_early_stop)
  - codeforge/plan_act.py (plan/act mode switching, plan validation)
  See audit report for full list. Add tests incrementally.
"""

from __future__ import annotations

import json
from collections import OrderedDict
from pathlib import Path
from typing import TYPE_CHECKING, Any

import pytest

from codeforge import tool_process
from codeforge.config import get_settings
from codeforge.consumer._base import ConsumerBaseMixin
from tests.fake_llm import FakeLLM

if TYPE_CHECKING:
    from collections.abc import Iterator

SCENARIOS_DIR = Path(__file__).parent / "scenarios"


@pytest.fixture(autouse=True)
def _fresh_worker_settings() -> Iterator[None]:
    """Rebuild WorkerSettings for every test.

    get_settings() is a process-wide lru_cache singleton, and modules such as
    codeforge.llm call it at import time. Without a reset, env overrides set by
    a test (monkeypatch.setenv / patch.dict) are never seen, and one test's
    overrides would leak into every later test.
    """
    get_settings.cache_clear()
    yield
    get_settings.cache_clear()


@pytest.fixture(autouse=True)
def _fresh_tool_isolation(monkeypatch: pytest.MonkeyPatch) -> None:
    """Check tool isolation (KI-71) again from each test's settings.

    The isolation status is cached for the worker's lifetime; a test that
    installs one must not leak it into the next.
    """
    monkeypatch.setattr(tool_process, "_status", None)


@pytest.fixture(autouse=True)
def _isolate_consumer_dedup_cache(monkeypatch: pytest.MonkeyPatch) -> None:
    """Give every test an empty consumer dedup cache.

    ``ConsumerBaseMixin._processed_ids`` is class-level state shared by every
    TaskConsumer, so IDs seen in one test would be skipped as duplicates in the
    next. monkeypatch restores the original after each test, even when a test
    rebinds the attribute itself.
    """
    monkeypatch.setattr(ConsumerBaseMixin, "_processed_ids", OrderedDict())


def load_scenario(role: str, scenario: str) -> tuple[dict[str, Any], dict[str, Any], FakeLLM]:
    """Load a scenario's input, expected output, and FakeLLM from fixtures.

    Returns (input_data, expected_output, fake_llm).
    """
    base = SCENARIOS_DIR / role / scenario
    input_data: dict[str, Any] = json.loads((base / "input.json").read_text())
    expected: dict[str, Any] = json.loads((base / "expected_output.json").read_text())
    fake_llm = FakeLLM.from_fixture(base / "llm_responses.json")
    return input_data, expected, fake_llm
