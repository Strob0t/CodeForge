"""Layer 1: Rule-based prompt complexity analysis."""

from __future__ import annotations

import re

from codeforge.routing.models import ComplexityTier, PromptAnalysis, RoutingConfig, TaskType

_DEFAULT_WEIGHTS: dict[str, float] = {
    "code_presence": 0.20,
    "reasoning_markers": 0.20,
    "technical_terms": 0.15,
    "prompt_length": 0.10,
    "multi_step": 0.15,
    "context_requirements": 0.10,
    "output_complexity": 0.10,
}

_DEFAULT_TIER_THRESHOLDS: list[tuple[float, ComplexityTier]] = [
    (0.75, ComplexityTier.REASONING),
    (0.50, ComplexityTier.COMPLEX),
    (0.25, ComplexityTier.MEDIUM),
    (0.0, ComplexityTier.SIMPLE),
]

_DEFAULT_TASK_TYPE_BOOST: dict[TaskType, float] = {
    TaskType.CHAT: 0.0,
    TaskType.CODE: 0.10,
    TaskType.DEBUG: 0.20,
    TaskType.QA: 0.15,
    TaskType.REFACTOR: 0.20,
    TaskType.REVIEW: 0.25,
    TaskType.PLAN: 0.25,
}

_RE_CODE_BLOCKS = re.compile(r"```[\s\S]*?```|`[^`]+`")
_RE_FILE_EXTENSIONS = re.compile(
    r"\b\w+\.(py|go|ts|tsx|js|jsx|java|rs|cpp|c|h|rb|php|swift|kt|scala|sql|yaml|yml|json|toml|sh|bash)\b",
    re.IGNORECASE,
)
_RE_IMPORT_PATTERNS = re.compile(r"\b(import|from|require|include|using|package)\b", re.IGNORECASE)
_RE_CODE_KEYWORDS = re.compile(
    r"\b(function|class|def|const|let|var|struct|enum|interface|type|fn|pub|impl|async|await)\b"
)

_RE_REASONING = re.compile(
    r"\b(analy[sz]e|compare|trade[- ]?off|design|evaluate|pros and cons|which is better|"
    r"should [iI]|why|explain the difference|consider|weigh|assess|critique|justify|"
    r"what are the advantages|disadvantages|implications)\b",
    re.IGNORECASE,
)

_TECHNICAL_TERMS = {
    "api",
    "database",
    "schema",
    "microservice",
    "kubernetes",
    "docker",
    "ci/cd",
    "algorithm",
    "complexity",
    "architecture",
    "refactor",
    "migration",
    "orm",
    "rest",
    "graphql",
    "websocket",
    "grpc",
    "protobuf",
    "redis",
    "postgres",
    "mongodb",
    "nginx",
    "terraform",
    "ansible",
    "pipeline",
    "deployment",
    "container",
    "orchestration",
    "authentication",
    "authorization",
    "oauth",
    "jwt",
    "ssl",
    "tls",
    "encryption",
    "hashing",
    "caching",
    "load balancer",
    "proxy",
    "middleware",
    "endpoint",
    "payload",
    "serialization",
    "deserialization",
    "concurrency",
    "parallelism",
    "async",
    "thread",
    "mutex",
    "semaphore",
    "dependency injection",
    "singleton",
    "factory",
    "observer",
    "strategy",
}

_RE_MULTI_STEP = re.compile(
    r"(\d+\.\s|\bstep\s+\d+\b|\bfirst\b.*\bthen\b|\bfinally\b|"
    r"\b(first|second|third|next|after that|lastly|subsequently)\b|"
    r"^\s*[-*]\s+)",
    re.IGNORECASE | re.MULTILINE,
)

_RE_CONTEXT = re.compile(
    r"(/[\w./-]+\.\w+|\.?/[\w./-]+|\bcodebase\b|\brepository\b|\brepo\b|"
    r"\bproject\b|\bacross files\b|\bmultiple files\b|\bseveral files\b|"
    r"\bdirectory\b|\bfolder\b)",
    re.IGNORECASE,
)

_RE_OUTPUT = re.compile(
    r"\b(generate|implement|write|create a|build|full implementation|"
    r"complete code|write a|develop|produce|code for|scaffold|boilerplate)\b",
    re.IGNORECASE,
)

_TASK_PATTERNS: list[tuple[TaskType, re.Pattern[str]]] = [
    (
        TaskType.REVIEW,
        re.compile(r"\b(review|check|audit|find bugs|code quality|code review|inspect|lint)\b", re.IGNORECASE),
    ),
    (
        TaskType.DEBUG,
        re.compile(
            r"\b(fix|debug|error|bug|broken|not working|crash|exception|traceback|stacktrace|failing)\b", re.IGNORECASE
        ),
    ),
    (
        TaskType.REFACTOR,
        re.compile(r"\b(refactor|clean up|simplify|reorganize|restructure|rename|extract|inline)\b", re.IGNORECASE),
    ),
    (
        TaskType.PLAN,
        re.compile(r"\b(plan|design|architect|strategy|roadmap|approach|proposal|blueprint)\b", re.IGNORECASE),
    ),
    (
        TaskType.QA,
        re.compile(r"\b(tests?|testing|coverage|assertion|unit tests?|integration tests?|e2e|spec)\b", re.IGNORECASE),
    ),
    (
        TaskType.CODE,
        re.compile(
            r"\b(implement|write code|generate|create a function|add a method|write a|build|develop|code|program|script)\b",
            re.IGNORECASE,
        ),
    ),
]

_RE_SHORT_OUTPUT = re.compile(
    r"\b(briefly|short|concise|one[- ]line|yes or no|true or false|"
    r"what is|is it|does it|can you|summarize in a sentence)\b",
    re.IGNORECASE,
)

_RE_LONG_OUTPUT = re.compile(
    r"\b(full implementation|complete code|write a complete|implement a complete|"
    r"entire module|entire file|all files|full code|scaffold|boilerplate|"
    r"write the full|develop a full|build a complete|implement all|"
    r"refactor the entire|rewrite the entire|whole file)\b",
    re.IGNORECASE,
)


class ComplexityAnalyzer:
    def __init__(self, config: RoutingConfig | None = None) -> None:
        if config is not None:
            self._weights = config.complexity_weights
            self._tier_thresholds: list[tuple[float, ComplexityTier]] = [
                (thresh, ComplexityTier(name)) for thresh, name in config.tier_thresholds
            ]
            self._task_type_boost: dict[TaskType, float] = {TaskType(k): v for k, v in config.task_type_boost.items()}
        else:
            self._weights = _DEFAULT_WEIGHTS
            self._tier_thresholds = _DEFAULT_TIER_THRESHOLDS
            self._task_type_boost = _DEFAULT_TASK_TYPE_BOOST

    def analyze(self, prompt: str) -> PromptAnalysis:
        dimensions = {
            "code_presence": _score_code_presence(prompt),
            "reasoning_markers": _score_reasoning_markers(prompt),
            "technical_terms": _score_technical_terms(prompt),
            "prompt_length": _score_prompt_length(prompt),
            "multi_step": _score_multi_step(prompt),
            "context_requirements": _score_context_requirements(prompt),
            "output_complexity": _score_output_complexity(prompt),
        }

        weighted_score = sum(dimensions[dim] * weight for dim, weight in self._weights.items())
        task_type = _infer_task_type(prompt)
        boosted_score = min(1.0, weighted_score + self._task_type_boost.get(task_type, 0.0))

        tier = ComplexityTier.SIMPLE
        for threshold, candidate_tier in self._tier_thresholds:
            if boosted_score >= threshold:
                tier = candidate_tier
                break

        return PromptAnalysis(
            complexity_tier=tier,
            task_type=task_type,
            dimensions=dimensions,
            confidence=min(1.0, boosted_score + 0.3),
            estimated_output_tokens=_estimate_output_tokens(prompt, dimensions),
        )


def _score_code_presence(prompt: str) -> float:
    score = 0.0
    if _RE_CODE_BLOCKS.search(prompt):
        score += 0.4
    ext_count = len(_RE_FILE_EXTENSIONS.findall(prompt))
    score += min(0.3, ext_count * 0.1)
    if _RE_IMPORT_PATTERNS.search(prompt):
        score += 0.15
    kw_count = len(_RE_CODE_KEYWORDS.findall(prompt))
    score += min(0.15, kw_count * 0.05)
    return min(1.0, score)


def _score_reasoning_markers(prompt: str) -> float:
    matches = len(_RE_REASONING.findall(prompt))
    if matches == 0:
        return 0.0
    if matches == 1:
        return 0.4
    if matches == 2:
        return 0.7
    return 1.0


def _score_technical_terms(prompt: str) -> float:
    lower = prompt.lower()
    count = sum(1 for term in _TECHNICAL_TERMS if term in lower)
    if count == 0:
        return 0.0
    if count <= 2:
        return 0.3
    if count <= 5:
        return 0.6
    if count <= 10:
        return 0.8
    return 1.0


def _score_prompt_length(prompt: str) -> float:
    tokens = len(prompt) / 4
    if tokens < 15:
        return 0.0
    if tokens < 100:
        return 0.2
    if tokens < 300:
        return 0.5
    if tokens < 750:
        return 0.8
    return 1.0


def _score_multi_step(prompt: str) -> float:
    matches = len(_RE_MULTI_STEP.findall(prompt))
    if matches == 0:
        return 0.0
    if matches <= 2:
        return 0.4
    if matches <= 5:
        return 0.7
    return 1.0


def _score_context_requirements(prompt: str) -> float:
    matches = len(_RE_CONTEXT.findall(prompt))
    if matches == 0:
        return 0.0
    if matches <= 2:
        return 0.4
    if matches <= 5:
        return 0.7
    return 1.0


def _score_output_complexity(prompt: str) -> float:
    matches = len(_RE_OUTPUT.findall(prompt))
    if matches == 0:
        return 0.0
    if matches == 1:
        return 0.4
    if matches <= 3:
        return 0.7
    return 1.0


def _infer_task_type(prompt: str) -> TaskType:
    for task_type, pattern in _TASK_PATTERNS:
        if pattern.search(prompt):
            return task_type
    return TaskType.CHAT


def _estimate_output_tokens(prompt: str, dimensions: dict[str, float]) -> int:
    if _RE_SHORT_OUTPUT.search(prompt):
        return 150
    if _RE_LONG_OUTPUT.search(prompt):
        return 2000
    output_score = dimensions.get("output_complexity", 0.0)
    length_score = dimensions.get("prompt_length", 0.0)
    combined = 0.7 * output_score + 0.3 * length_score
    if combined <= 0.1:
        return 150
    if combined <= 0.4:
        return 500
    if combined <= 0.7:
        return 1000
    return 2000
