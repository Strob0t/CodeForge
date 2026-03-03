"""Tests for Layer 1: ComplexityAnalyzer (Phase 26F)."""

from __future__ import annotations

import time

from codeforge.routing.complexity import (
    ComplexityAnalyzer,
    _infer_task_type,
    _score_code_presence,
    _score_context_requirements,
    _score_multi_step,
    _score_output_complexity,
    _score_prompt_length,
    _score_reasoning_markers,
    _score_technical_terms,
)
from codeforge.routing.models import ComplexityTier, TaskType

analyzer = ComplexityAnalyzer()


# -- Tier classification -----------------------------------------------------


def test_simple_greeting() -> None:
    result = analyzer.analyze("Hello")
    assert result.complexity_tier == ComplexityTier.SIMPLE


def test_simple_question() -> None:
    result = analyzer.analyze("What is Python?")
    assert result.complexity_tier == ComplexityTier.SIMPLE


def test_simple_short() -> None:
    result = analyzer.analyze("Say hi")
    assert result.complexity_tier == ComplexityTier.SIMPLE


def test_empty_string() -> None:
    result = analyzer.analyze("")
    assert result.complexity_tier == ComplexityTier.SIMPLE


def test_single_word() -> None:
    result = analyzer.analyze("hello")
    assert result.complexity_tier == ComplexityTier.SIMPLE


def test_medium_code_task() -> None:
    result = analyzer.analyze("Write a function to sort a list in Python")
    assert result.complexity_tier in {ComplexityTier.SIMPLE, ComplexityTier.MEDIUM}


def test_complex_multi_file() -> None:
    prompt = (
        "Refactor this microservice to use event sourcing across 5 files. "
        "The codebase uses kubernetes and docker for deployment. "
        "Step 1: Update the database schema. "
        "Step 2: Modify the API endpoints. "
        "Step 3: Update the repository layer. "
        "Step 4: Add integration tests. "
        "Step 5: Update the deployment pipeline."
    )
    result = analyzer.analyze(prompt)
    # Multi-step + technical terms push this to MEDIUM or higher.
    assert result.complexity_tier in {ComplexityTier.MEDIUM, ComplexityTier.COMPLEX, ComplexityTier.REASONING}


def test_reasoning_prompt() -> None:
    prompt = (
        "Compare Redis vs Memcached for our use case, analyze the trade-offs "
        "between consistency and performance. Evaluate the pros and cons of each "
        "approach considering our microservice architecture with kubernetes, docker, "
        "and the existing database schema. Which is better for our caching layer? "
        "Consider the implications for our API endpoints and load balancer setup."
    )
    result = analyzer.analyze(prompt)
    # High reasoning markers and technical terms push this to MEDIUM or higher.
    assert result.complexity_tier in {ComplexityTier.MEDIUM, ComplexityTier.COMPLEX, ComplexityTier.REASONING}
    # Should have high reasoning markers.
    assert result.dimensions["reasoning_markers"] >= 0.7


# -- Individual dimension scores ---------------------------------------------


def test_score_code_presence_none() -> None:
    assert _score_code_presence("Hello world") == 0.0


def test_score_code_presence_backtick_block() -> None:
    prompt = "Here is code:\n```python\ndef foo(): pass\n```"
    score = _score_code_presence(prompt)
    assert score >= 0.4


def test_score_code_presence_file_extensions() -> None:
    prompt = "Check main.py and utils.go and config.yaml"
    score = _score_code_presence(prompt)
    assert score >= 0.2


def test_score_code_presence_imports() -> None:
    prompt = "You need to import os and from pathlib import Path"
    score = _score_code_presence(prompt)
    assert score >= 0.15


def test_score_code_presence_keywords() -> None:
    prompt = "Define a class with def method and a function with const"
    score = _score_code_presence(prompt)
    assert score >= 0.1


def test_score_code_presence_max_cap() -> None:
    prompt = (
        "```python\nimport os\nfrom foo import bar\ndef func(): pass\nclass Cls: pass\n```\n"
        "Check main.py and utils.py and config.py and test.py"
    )
    score = _score_code_presence(prompt)
    assert score <= 1.0


def test_score_reasoning_markers_none() -> None:
    assert _score_reasoning_markers("Hello world") == 0.0


def test_score_reasoning_markers_one() -> None:
    score = _score_reasoning_markers("Analyze this code")
    assert score == 0.4


def test_score_reasoning_markers_two() -> None:
    score = _score_reasoning_markers("Analyze and compare these approaches")
    assert score == 0.7


def test_score_reasoning_markers_many() -> None:
    score = _score_reasoning_markers("Analyze and compare and evaluate the trade-offs")
    assert score == 1.0


def test_score_technical_terms_none() -> None:
    assert _score_technical_terms("Hello how are you") == 0.0


def test_score_technical_terms_few() -> None:
    score = _score_technical_terms("Setup the api with database")
    assert score == 0.3


def test_score_technical_terms_moderate() -> None:
    score = _score_technical_terms("Setup the api with database and redis caching for the microservice")
    assert score >= 0.6


def test_score_technical_terms_many() -> None:
    score = _score_technical_terms(
        "Setup kubernetes docker api database schema microservice "
        "redis postgres nginx terraform pipeline deployment container"
    )
    assert score >= 0.8


def test_score_prompt_length_empty() -> None:
    assert _score_prompt_length("") == 0.0


def test_score_prompt_length_short() -> None:
    # <15 tokens = <60 chars
    assert _score_prompt_length("Hello") == 0.0


def test_score_prompt_length_brief() -> None:
    # 15-100 tokens (60-400 chars) — short but non-trivial
    prompt = "x" * 100  # ~25 tokens
    assert _score_prompt_length(prompt) == 0.2


def test_score_prompt_length_medium() -> None:
    # 100-300 tokens (400-1200 chars)
    prompt = "x" * 600  # ~150 tokens
    assert _score_prompt_length(prompt) == 0.5


def test_score_prompt_length_long() -> None:
    # 300-750 tokens (1200-3000 chars)
    prompt = "x" * 2000  # ~500 tokens
    assert _score_prompt_length(prompt) == 0.8


def test_score_prompt_length_huge() -> None:
    # >750 tokens = >3000 chars
    prompt = "x" * 5000
    assert _score_prompt_length(prompt) == 1.0


def test_score_multi_step_none() -> None:
    assert _score_multi_step("Do this thing") == 0.0


def test_score_multi_step_numbered() -> None:
    prompt = "1. First thing\n2. Second thing"
    score = _score_multi_step(prompt)
    assert score >= 0.4


def test_score_multi_step_many() -> None:
    prompt = "1. A\n2. B\n3. C\n4. D\n5. E\n6. F"
    score = _score_multi_step(prompt)
    assert score == 1.0


def test_score_multi_step_bullets() -> None:
    prompt = "- item one\n- item two\n- item three"
    score = _score_multi_step(prompt)
    assert score >= 0.4


def test_score_context_requirements_none() -> None:
    assert _score_context_requirements("Hello") == 0.0


def test_score_context_requirements_file_paths() -> None:
    prompt = "Check /src/main.py and ./config.yaml"
    score = _score_context_requirements(prompt)
    assert score >= 0.4


def test_score_context_requirements_codebase() -> None:
    prompt = "Search across the codebase and repository for multiple files"
    score = _score_context_requirements(prompt)
    assert score >= 0.4


def test_score_output_complexity_none() -> None:
    assert _score_output_complexity("Hello") == 0.0


def test_score_output_complexity_single() -> None:
    score = _score_output_complexity("Generate a function")
    assert score == 0.4


def test_score_output_complexity_multiple() -> None:
    score = _score_output_complexity("Generate and implement and write the full implementation")
    assert score >= 0.7


# -- Task type inference -----------------------------------------------------


def test_task_type_code() -> None:
    assert _infer_task_type("Implement a sorting algorithm") == TaskType.CODE


def test_task_type_review() -> None:
    assert _infer_task_type("Review this pull request for bugs") == TaskType.REVIEW


def test_task_type_debug() -> None:
    assert _infer_task_type("Fix this error in the login flow") == TaskType.DEBUG


def test_task_type_refactor() -> None:
    assert _infer_task_type("Refactor the database module") == TaskType.REFACTOR


def test_task_type_plan() -> None:
    assert _infer_task_type("Design the architecture for the new feature") == TaskType.PLAN


def test_task_type_qa() -> None:
    assert _infer_task_type("Write unit tests for the parser") == TaskType.QA


def test_task_type_chat_default() -> None:
    assert _infer_task_type("Hello, how are you?") == TaskType.CHAT


def test_task_type_priority_review_over_code() -> None:
    # "review" should take priority over "code" since it's checked first.
    assert _infer_task_type("Review the code") == TaskType.REVIEW


def test_task_type_priority_debug_over_code() -> None:
    # "fix" (DEBUG) is checked before "code".
    assert _infer_task_type("Fix this code") == TaskType.DEBUG


# -- PromptAnalysis structure ------------------------------------------------


def test_analysis_has_all_dimensions() -> None:
    result = analyzer.analyze("Hello world")
    expected_dims = {
        "code_presence",
        "reasoning_markers",
        "technical_terms",
        "prompt_length",
        "multi_step",
        "context_requirements",
        "output_complexity",
    }
    assert set(result.dimensions.keys()) == expected_dims


def test_analysis_dimensions_in_range() -> None:
    prompt = (
        "Implement a microservice with kubernetes and docker. "
        "```python\ndef main(): pass\n```\n"
        "Step 1: Setup. Step 2: Deploy. "
        "Analyze the trade-offs and compare approaches."
    )
    result = analyzer.analyze(prompt)
    for dim, score in result.dimensions.items():
        assert 0.0 <= score <= 1.0, f"Dimension {dim} = {score} out of range"


def test_analysis_confidence_in_range() -> None:
    for prompt in ["Hi", "x" * 5000, "Implement a complex system"]:
        result = analyzer.analyze(prompt)
        assert 0.0 <= result.confidence <= 1.0, f"confidence={result.confidence} for prompt={prompt[:20]!r}"


# -- Performance -------------------------------------------------------------


def test_performance_10k_chars() -> None:
    prompt = "Analyze this code " * 500  # ~9000 chars
    start = time.perf_counter_ns()
    analyzer.analyze(prompt)
    elapsed_ms = (time.perf_counter_ns() - start) / 1_000_000
    # Should complete in well under 10ms (target: <1ms).
    assert elapsed_ms < 10, f"analyze() took {elapsed_ms:.2f}ms, expected <10ms"


# -- Unicode edge case -------------------------------------------------------


def test_unicode_prompt() -> None:
    prompt = "Erklare die Architektur des Systems"
    result = analyzer.analyze(prompt)
    assert result.complexity_tier is not None
    assert result.task_type is not None
