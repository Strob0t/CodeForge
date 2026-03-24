"""Prompt mutator -- generates new prompt variants from tactical fixes and strategic principles.

Part of the GEPA-inspired prompt evolution system. Takes reflection-phase outputs
(tactical fixes for specific failures, strategic principles for general improvement)
and produces rewritten prompt variants that incorporate those improvements.

Inspired by:
- SCOPE (Prompt Evolution, Dec 2025) — mutation operators
- SICA (Self-Improving Coding Agent, ICLR 2025) — tactical/strategic split
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field

import structlog

from codeforge.evaluation.prompt_optimizer import TacticalFix

logger = structlog.get_logger(component="evaluation")

# Length bounds: variant must be 50-300% of original length.
_MIN_LENGTH_RATIO = 0.5
_MAX_LENGTH_RATIO = 3.0

# NATS subject for mutation results.
SUBJECT_MUTATE_COMPLETE = "prompt.evolution.mutate.complete"

_MUTATION_PROMPT = """\
You are a prompt engineering expert. Rewrite the following agent prompt to incorporate \
the tactical fixes and strategic principles listed below.

## Current Prompt (mode: {mode_id}):
{current_content}

## Tactical Fixes to Apply ({fix_count}):
{fixes_text}

## Strategic Principles to Incorporate ({principle_count}):
{principles_text}

## Instructions:
1. Preserve the core intent and structure of the original prompt.
2. Integrate each tactical fix as a specific instruction or constraint.
3. Weave strategic principles into the prompt naturally.
4. Do NOT remove existing instructions unless they directly conflict with a fix.
5. Keep the prompt concise — avoid redundancy.

Respond with ONLY the rewritten prompt text. No JSON, no markdown fences, no explanation.
"""


@dataclass
class PromptVariant:
    """A mutated prompt variant with lineage metadata."""

    content: str
    version: int
    parent_id: str
    mutation_source: str  # "tactical" | "strategic" | "combined"
    tactical_fixes_applied: list[str] = field(default_factory=list)  # task_ids
    strategic_principles: list[str] = field(default_factory=list)
    validation_passed: bool = False


def validate_variant(original: str, variant_content: str) -> tuple[bool, str]:
    """Validate a prompt variant against the original.

    Checks:
    - Non-empty content
    - Length within 50-300% of original

    Returns (valid, reason) where reason describes the failure if invalid.
    """
    if not variant_content or not variant_content.strip():
        return False, "variant content is empty"

    original_len = len(original)
    if original_len == 0:
        # If original is empty, any non-empty variant is valid.
        return True, "ok"

    variant_len = len(variant_content)
    ratio = variant_len / original_len

    if ratio < _MIN_LENGTH_RATIO:
        return False, (
            f"variant too short: {variant_len} chars is {ratio:.1%} of original "
            f"({original_len} chars), minimum is {_MIN_LENGTH_RATIO:.0%}"
        )

    if ratio > _MAX_LENGTH_RATIO:
        return False, (
            f"variant too long: {variant_len} chars is {ratio:.1%} of original "
            f"({original_len} chars), maximum is {_MAX_LENGTH_RATIO:.0%}"
        )

    return True, "ok"


def mutate_prompt_sync(
    current_content: str,
    tactical_fixes: list[TacticalFix],
    strategic_principles: list[str],
    mode_id: str,
) -> PromptVariant:
    """Synchronous prompt mutation without LLM -- for testing and fallback.

    Appends tactical fix proposed_additions and strategic principles to the
    current content. Deduplicates by content to avoid redundant additions.
    """
    additions: list[str] = []
    seen: set[str] = set()
    task_ids: list[str] = []

    for fix in tactical_fixes:
        text = fix.proposed_addition.strip()
        if text and text not in seen:
            seen.add(text)
            additions.append(text)
            task_ids.append(fix.task_id)

    unique_principles: list[str] = []
    for principle in strategic_principles:
        text = principle.strip()
        if text and text not in seen:
            seen.add(text)
            additions.append(text)
            unique_principles.append(text)

    combined = current_content.rstrip() + "\n\n" + "\n".join(additions) if additions else current_content

    # Determine mutation source.
    has_tactical = len(task_ids) > 0
    has_strategic = len(unique_principles) > 0
    if has_tactical and has_strategic:
        source = "combined"
    elif has_tactical:
        source = "tactical"
    elif has_strategic:
        source = "strategic"
    else:
        source = "tactical"

    valid, reason = validate_variant(current_content, combined)
    if not valid:
        logger.warning("sync variant failed validation", reason=reason)

    return PromptVariant(
        content=combined,
        version=1,
        parent_id="",
        mutation_source=source,
        tactical_fixes_applied=task_ids,
        strategic_principles=unique_principles,
        validation_passed=valid,
    )


async def mutate_prompt(
    current_content: str,
    tactical_fixes: list[TacticalFix],
    strategic_principles: list[str],
    mode_id: str,
    llm_client: object,
) -> PromptVariant:
    """Generate a prompt variant by asking an LLM to rewrite the prompt.

    Builds a mutation prompt that includes the current prompt content and the
    fixes/principles to incorporate, then validates the result against length bounds.
    Falls back to sync mutation if the LLM response is invalid.
    """
    task_ids = [fix.task_id for fix in tactical_fixes]

    fixes_text = (
        "\n".join(
            f"- [{fix.task_id}] Root cause: {fix.root_cause}. "
            f"Fix: {fix.proposed_addition} (confidence: {fix.confidence:.0%})"
            for fix in tactical_fixes
        )
        or "(none)"
    )

    principles_text = "\n".join(f"- {p}" for p in strategic_principles) or "(none)"

    prompt = _MUTATION_PROMPT.format(
        mode_id=mode_id,
        current_content=current_content,
        fix_count=len(tactical_fixes),
        fixes_text=fixes_text,
        principle_count=len(strategic_principles),
        principles_text=principles_text,
    )

    response = await llm_client.chat_completion(  # type: ignore[union-attr]
        messages=[{"role": "user", "content": prompt}],
        model="auto",
    )

    content: str = response.content  # type: ignore[union-attr]

    # Determine mutation source.
    has_tactical = len(tactical_fixes) > 0
    has_strategic = len(strategic_principles) > 0
    if has_tactical and has_strategic:
        source = "combined"
    elif has_tactical:
        source = "tactical"
    else:
        source = "strategic"

    valid, reason = validate_variant(current_content, content)
    if not valid:
        logger.warning("LLM variant failed validation, falling back to sync mutation", reason=reason)
        return mutate_prompt_sync(current_content, tactical_fixes, strategic_principles, mode_id)

    return PromptVariant(
        content=content,
        version=1,
        parent_id="",
        mutation_source=source,
        tactical_fixes_applied=task_ids,
        strategic_principles=list(strategic_principles),
        validation_passed=True,
    )


async def handle_mutate_request(
    payload: dict[str, object],
    llm_client: object,
    nats_client: object,
) -> None:
    """NATS handler for prompt mutation requests.

    Expects payload keys: current_content, tactical_fixes, strategic_principles, mode_id.
    Publishes result to ``prompt.evolution.mutate.complete``.
    """
    current_content = str(payload.get("current_content", ""))
    mode_id = str(payload.get("mode_id", ""))
    raw_fixes = payload.get("tactical_fixes", [])
    raw_principles = payload.get("strategic_principles", [])

    # Deserialize tactical fixes from dicts.
    fixes: list[TacticalFix] = []
    if isinstance(raw_fixes, list):
        fixes.extend(
            TacticalFix(
                task_id=str(item.get("task_id", "")),
                failure_description=str(item.get("failure_description", "")),
                root_cause=str(item.get("root_cause", "")),
                proposed_addition=str(item.get("proposed_addition", "")),
                confidence=float(item.get("confidence", 0.0)),
            )
            for item in raw_fixes
            if isinstance(item, dict)
        )

    principles: list[str] = []
    if isinstance(raw_principles, list):
        principles = [str(p) for p in raw_principles]

    try:
        variant = await mutate_prompt(
            current_content=current_content,
            tactical_fixes=fixes,
            strategic_principles=principles,
            mode_id=mode_id,
            llm_client=llm_client,
        )
        result: dict[str, object] = {
            "status": "complete",
            "mode_id": mode_id,
            "variant": {
                "content": variant.content,
                "version": variant.version,
                "parent_id": variant.parent_id,
                "mutation_source": variant.mutation_source,
                "tactical_fixes_applied": variant.tactical_fixes_applied,
                "strategic_principles": variant.strategic_principles,
                "validation_passed": variant.validation_passed,
            },
        }
    except Exception as exc:
        logger.exception("prompt mutation failed", error=str(exc))
        result = {
            "status": "error",
            "mode_id": mode_id,
            "error": str(exc),
        }

    await nats_client.publish(  # type: ignore[union-attr]
        SUBJECT_MUTATE_COMPLETE,
        json.dumps(result).encode(),
    )
