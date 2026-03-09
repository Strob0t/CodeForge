"""Prompt optimizer -- analyzes benchmark failures and proposes prompt improvements.

Hybrid approach inspired by:
- SICA (Self-Improving Coding Agent, ICLR 2025)
- SCOPE (Prompt Evolution, Dec 2025)
- MIPROv2 Bootstrap (DSPy/Stanford)
"""

from __future__ import annotations

from dataclasses import dataclass, field


@dataclass
class TacticalFix:
    """SCOPE-style: specific failure -> specific fix."""

    task_id: str
    failure_description: str
    root_cause: str
    proposed_addition: str
    confidence: float


@dataclass
class PromptPatch:
    """A concrete prompt change to apply to a mode's YAML."""

    mode: str
    model_family: str
    patch_type: str  # "tactical" | "strategic" | "few_shot"
    action: str  # "add" | "replace" | "remove"
    content: str
    location: str  # "model_adaptations" | "prompt_template"
    rationale: str
    source_task_ids: list[str] = field(default_factory=list)


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


_ANALYSIS_PROMPT = """\
You are a prompt optimization expert. Analyze the following benchmark failures and suggest improvements.

Mode: {mode}
Model Family: {model_family}

## Failed Tasks ({count}):
{failures_text}

## Instructions:
1. Identify common failure patterns (tactical fixes)
2. Extract overarching principles (strategic improvements)
3. Suggest 1-3 successful patterns as few-shot candidates

Respond in JSON:
{{
  "tactical_fixes": [
    {{"task_id": "...", "failure_description": "...", "root_cause": "...", \
"proposed_addition": "...", "confidence": 0.8}}
  ],
  "strategic_principles": ["principle 1", "principle 2"],
  "few_shot_candidates": ["example trace 1"]
}}
"""


def analyze_failures(
    failures: list[dict[str, object]],
    mode: str,
    model_family: str,
    llm_client: object | None = None,
    suite_id: str = "",
    run_id: str = "",
) -> PromptAnalysisReport:
    """Analyze benchmark failures and produce a PromptAnalysisReport.

    When llm_client is None, returns a basic structural report
    without LLM analysis (useful for testing).
    """
    total = len(failures)
    failed = sum(1 for f in failures if _is_failed(f))

    report = PromptAnalysisReport(
        suite_id=suite_id,
        run_id=run_id,
        mode=mode,
        model_family=model_family,
        total_tasks=total,
        failed_tasks=failed,
        failure_rate=failed / total if total > 0 else 0.0,
    )

    if llm_client is None:
        for f in failures:
            if _is_failed(f):
                expected = str(f.get("expected_output", ""))[:50]
                report.tactical_fixes.append(
                    TacticalFix(
                        task_id=str(f.get("task_id", "")),
                        failure_description=f"Expected: {expected}",
                        root_cause="Analysis requires LLM",
                        proposed_addition="",
                        confidence=0.0,
                    )
                )
        return report

    return report


async def analyze_failures_async(
    failures: list[dict[str, object]],
    mode: str,
    model_family: str,
    llm_client: object,
    suite_id: str = "",
    run_id: str = "",
) -> PromptAnalysisReport:
    """Async version that calls LLM for analysis."""
    import json as json_mod

    total = len(failures)
    failed_items = [f for f in failures if _is_failed(f)]

    failures_text = "\n".join(
        f"- Task {f.get('task_id')}: expected={str(f.get('expected_output', ''))[:100]}, "
        f"got={str(f.get('actual_output', ''))[:100]}, scores={f.get('scores', {})}"
        for f in failed_items[:20]
    )

    prompt = _ANALYSIS_PROMPT.format(
        mode=mode,
        model_family=model_family,
        count=len(failed_items),
        failures_text=failures_text,
    )

    response = await llm_client.chat_completion(  # type: ignore[union-attr]
        messages=[{"role": "user", "content": prompt}],
        model="auto",
    )

    content: str = response.get("choices", [{}])[0].get("message", {}).get("content", "{}")  # type: ignore[union-attr]
    try:
        data: dict[str, object] = json_mod.loads(content)
    except json_mod.JSONDecodeError:
        data = {}

    report = PromptAnalysisReport(
        suite_id=suite_id,
        run_id=run_id,
        mode=mode,
        model_family=model_family,
        total_tasks=total,
        failed_tasks=len(failed_items),
        failure_rate=len(failed_items) / total if total > 0 else 0.0,
    )

    for fix_data in data.get("tactical_fixes", []):  # type: ignore[union-attr]
        report.tactical_fixes.append(TacticalFix(**fix_data))  # type: ignore[arg-type]

    report.strategic_principles = data.get("strategic_principles", [])  # type: ignore[assignment]
    report.few_shot_candidates = data.get("few_shot_candidates", [])  # type: ignore[assignment]

    return report


def _is_failed(result: dict[str, object]) -> bool:
    """Determine if a benchmark result is a failure."""
    scores = result.get("scores", {})
    if not isinstance(scores, dict) or not scores:
        return True
    avg = sum(float(v) for v in scores.values()) / len(scores)
    return avg < 0.5
