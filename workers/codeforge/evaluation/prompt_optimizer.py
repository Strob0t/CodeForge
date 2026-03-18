"""Prompt optimizer -- analyzes benchmark failures and proposes prompt improvements.

Hybrid approach inspired by:
- SICA (Self-Improving Coding Agent, ICLR 2025)
- SCOPE (Prompt Evolution, Dec 2025)
- MIPROv2 Bootstrap (DSPy/Stanford)
"""

from __future__ import annotations

import json
import logging
from dataclasses import asdict, dataclass, field

logger = logging.getLogger(__name__)


@dataclass
class TacticalFix:
    """SCOPE-style: specific failure -> specific fix."""

    task_id: str
    failure_description: str
    root_cause: str
    proposed_addition: str
    confidence: float


@dataclass
class PromptAnalysisReport:
    """Result of analyzing benchmark failures for a mode + model-family."""

    suite_id: str = ""
    run_id: str = ""
    mode: str = ""
    model_family: str = ""
    total_tasks: int = 0
    failed_tasks: int = 0
    failure_rate: float = 0.0
    tactical_fixes: list[TacticalFix] = field(default_factory=list)
    strategic_principles: list[str] = field(default_factory=list)
    few_shot_candidates: list[str] = field(default_factory=list)


def _is_failed(result: dict[str, object]) -> bool:
    """Determine if a benchmark result is a failure."""
    scores = result.get("scores", {})
    if not isinstance(scores, dict) or not scores:
        return True
    avg = sum(float(v) for v in scores.values()) / len(scores)
    return avg < 0.5


# ---------------------------------------------------------------------------
# Failure pattern classification
# ---------------------------------------------------------------------------

_TOOL_MISUSE_KEYWORDS: tuple[str, ...] = (
    "tool call",
    "tool_call",
    "tool '",
    'tool "',
    "wrong tool",
    "invalid arguments",
    "tool failed",
    "tool misuse",
)

_FORMAT_ERROR_KEYWORDS: tuple[str, ...] = (
    "json parse",
    "json error",
    "syntax error",
    "parse error",
    "unexpected token",
    "format error",
    "malformed",
    "invalid json",
    "decode error",
)


def _classify_failure_pattern(failure: dict[str, object]) -> str:
    """Classify a failure into one of: tool_misuse, format_error, wrong_approach, other.

    Uses simple keyword heuristics on error messages, tool calls, and output.
    Order matters: tool_misuse is checked first, then format_error, then wrong_approach.
    """
    error = str(failure.get("error", "")).lower()
    tool_calls = failure.get("tool_calls")

    # Check tool misuse first (highest priority)
    for keyword in _TOOL_MISUSE_KEYWORDS:
        if keyword in error:
            return "tool_misuse"
    if isinstance(tool_calls, list) and tool_calls and "tool" in error:
        return "tool_misuse"

    # Check format errors
    for keyword in _FORMAT_ERROR_KEYWORDS:
        if keyword in error:
            return "format_error"

    # Check wrong approach: requires meaningful output mismatch or trace
    trace = str(failure.get("trace", "")).strip()
    actual = str(failure.get("actual_output", "")).strip()
    expected = str(failure.get("expected_output", "")).strip()
    if trace or (actual and expected and actual != expected):
        return "wrong_approach"

    return "other"


# ---------------------------------------------------------------------------
# Reflection prompt template
# ---------------------------------------------------------------------------

_REFLECTION_PROMPT = """\
You are a prompt engineering expert specializing in improving coding agent prompts.

## Current System Prompt
```
{current_prompt}
```

## Mode: {mode_id}
## Model Family: {model_family}

## Failure Clusters

{clusters_text}

## Instructions
Analyze the failure traces above. For each cluster:
1. Identify the root cause from the traces
2. Propose a specific prompt addition or modification to prevent this failure pattern
3. Rate your confidence (0.0-1.0) in the fix

Also extract 1-3 overarching strategic principles that would improve the prompt.

Respond in JSON:
{{
  "tactical_fixes": [
    {{
      "task_id": "representative_task_id",
      "failure_description": "what went wrong",
      "root_cause": "why it went wrong",
      "proposed_addition": "specific text to add/change in the prompt",
      "confidence": 0.8
    }}
  ],
  "strategic_principles": ["principle 1", "principle 2"]
}}
"""


# ---------------------------------------------------------------------------
# Sync reflection (no LLM)
# ---------------------------------------------------------------------------

_PATTERN_DESCRIPTIONS: dict[str, str] = {
    "tool_misuse": "Agent misused tools (wrong arguments, wrong tool name, or failed tool calls)",
    "format_error": "Agent produced malformed output (JSON parse errors, syntax errors)",
    "wrong_approach": "Agent used an incorrect algorithmic or logical approach",
    "other": "Unclassified failure pattern",
}


def reflect_on_failures_sync(
    failures: list[dict[str, object]],
    current_prompt: str,
    mode_id: str,
    model_family: str,
) -> PromptAnalysisReport:
    """Reflect on failures without LLM -- clusters failures and creates basic TacticalFix entries.

    Groups failures by pattern (tool_misuse, format_error, wrong_approach, other)
    and produces a TacticalFix for each failed task with pattern-based root cause.
    """
    total = len(failures)
    failed_items = [f for f in failures if _is_failed(f)]

    report = PromptAnalysisReport(
        mode=mode_id,
        model_family=model_family,
        total_tasks=total,
        failed_tasks=len(failed_items),
        failure_rate=len(failed_items) / total if total > 0 else 0.0,
    )

    if not failed_items:
        return report

    # Group by pattern
    clusters: dict[str, list[dict[str, object]]] = {}
    for f in failed_items:
        pattern = _classify_failure_pattern(f)
        clusters.setdefault(pattern, []).append(f)

    # Create tactical fixes per failed task
    for pattern, items in clusters.items():
        description = _PATTERN_DESCRIPTIONS.get(pattern, "Unknown pattern")
        for item in items:
            error = str(item.get("error", ""))[:200]
            actual = str(item.get("actual_output", ""))[:100]
            fix_description = error or (f"Output: {actual}" if actual else description)
            report.tactical_fixes.append(
                TacticalFix(
                    task_id=str(item.get("task_id", "")),
                    failure_description=fix_description,
                    root_cause=f"Pattern: {pattern} - {description}",
                    proposed_addition="",
                    confidence=0.3,
                )
            )

    # Generate strategic principles from clusters found
    for pattern, items in clusters.items():
        count = len(items)
        description = _PATTERN_DESCRIPTIONS.get(pattern, pattern)
        report.strategic_principles.append(f"{count} failure(s) from {pattern}: {description}")

    return report


# ---------------------------------------------------------------------------
# Async reflection (with LLM)
# ---------------------------------------------------------------------------


def _build_clusters_text(
    failed_items: list[dict[str, object]],
) -> tuple[str, dict[str, list[dict[str, object]]]]:
    """Build cluster text for the reflection prompt and return clusters."""
    clusters: dict[str, list[dict[str, object]]] = {}
    for f in failed_items:
        pattern = _classify_failure_pattern(f)
        clusters.setdefault(pattern, []).append(f)

    sections: list[str] = []
    for pattern, items in clusters.items():
        description = _PATTERN_DESCRIPTIONS.get(pattern, pattern)
        section_lines = [f"### {pattern} ({len(items)} failures): {description}"]
        for item in items[:10]:  # Limit traces per cluster
            task_id = item.get("task_id", "?")
            error = str(item.get("error", ""))[:200]
            actual = str(item.get("actual_output", ""))[:150]
            expected = str(item.get("expected_output", ""))[:150]
            trace = str(item.get("trace", ""))[:300]
            section_lines.append(f"- Task {task_id}: error={error}, actual={actual}, expected={expected}")
            if trace:
                section_lines.append(f"  trace: {trace}")
        sections.append("\n".join(section_lines))

    return "\n\n".join(sections), clusters


async def reflect_on_failures(
    failures: list[dict[str, object]],
    current_prompt: str,
    mode_id: str,
    model_family: str,
    llm_client: object,
) -> PromptAnalysisReport:
    """Reflect on failures using LLM analysis.

    1. Groups failures by pattern (tool_misuse, wrong_approach, format_error, other)
    2. For each cluster: calls LLM to diagnose root cause from traces
    3. For each root cause: LLM proposes specific prompt changes
    4. Returns a PromptAnalysisReport with TacticalFix[] and strategic_principles

    Falls back to sync analysis if the LLM call fails or returns invalid JSON.
    """
    total = len(failures)
    failed_items = [f for f in failures if _is_failed(f)]

    if not failed_items:
        return PromptAnalysisReport(
            mode=mode_id,
            model_family=model_family,
            total_tasks=total,
            failed_tasks=0,
            failure_rate=0.0,
        )

    clusters_text, _clusters = _build_clusters_text(failed_items)

    prompt = _REFLECTION_PROMPT.format(
        current_prompt=current_prompt[:2000],
        mode_id=mode_id,
        model_family=model_family,
        clusters_text=clusters_text,
    )

    try:
        response = await llm_client.chat_completion(  # type: ignore[union-attr]
            messages=[{"role": "user", "content": prompt}],
            model="auto",
        )
        content: str = response.content  # type: ignore[union-attr]
        data: dict[str, object] = json.loads(content)
    except (json.JSONDecodeError, AttributeError):
        # LLM returned non-JSON — fall back to sync analysis
        logger.warning("LLM returned non-JSON for reflection, falling back to sync")
        return reflect_on_failures_sync(failures, current_prompt, mode_id, model_family)
    except Exception as exc:
        logger.warning("LLM call failed for reflection, falling back to sync", exc_info=exc)
        return reflect_on_failures_sync(failures, current_prompt, mode_id, model_family)

    report = PromptAnalysisReport(
        mode=mode_id,
        model_family=model_family,
        total_tasks=total,
        failed_tasks=len(failed_items),
        failure_rate=len(failed_items) / total if total > 0 else 0.0,
    )

    for fix_data in data.get("tactical_fixes", []):  # type: ignore[union-attr]
        if isinstance(fix_data, dict):
            try:
                report.tactical_fixes.append(TacticalFix(**fix_data))
            except TypeError:
                logger.debug("Skipping invalid tactical fix: %s", fix_data)

    raw_principles = data.get("strategic_principles", [])
    if isinstance(raw_principles, list):
        report.strategic_principles = [str(p) for p in raw_principles]

    raw_candidates = data.get("few_shot_candidates", [])
    if isinstance(raw_candidates, list):
        report.few_shot_candidates = [str(c) for c in raw_candidates]

    return report


# ---------------------------------------------------------------------------
# NATS handler
# ---------------------------------------------------------------------------

SUBJECT_REFLECT_COMPLETE = "prompt.evolution.reflect.complete"


async def handle_reflect_request(
    payload: dict[str, object],
    llm_client: object,
    nats_client: object,
) -> None:
    """Handle a prompt reflection request from NATS.

    Extracts failures, current_prompt, mode_id, model_family from the payload,
    calls reflect_on_failures(), and publishes the result to
    ``prompt.evolution.reflect.complete``.
    """
    failures = payload.get("failures", [])
    if not isinstance(failures, list):
        failures = []
    current_prompt = str(payload.get("current_prompt", ""))
    mode_id = str(payload.get("mode_id", ""))
    model_family = str(payload.get("model_family", ""))

    try:
        report = await reflect_on_failures(
            failures=failures,
            current_prompt=current_prompt,
            mode_id=mode_id,
            model_family=model_family,
            llm_client=llm_client,
        )
    except Exception as exc:
        logger.exception("reflect_on_failures failed", exc_info=exc)
        # Publish error report so the Go side knows about the failure
        report = PromptAnalysisReport(
            mode=mode_id,
            model_family=model_family,
            total_tasks=len(failures),
            failed_tasks=0,
            failure_rate=0.0,
        )

    result_payload = json.dumps(asdict(report)).encode()
    await nats_client.publish(SUBJECT_REFLECT_COMPLETE, result_payload)  # type: ignore[union-attr]
