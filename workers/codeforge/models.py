"""Domain models for task messages exchanged between Go Core and Python Workers."""

from __future__ import annotations

from enum import StrEnum
from typing import Annotated, Any

from pydantic import AfterValidator, BaseModel, Field, field_validator

from codeforge._validators import clamp_tool_output_max_chars, clamp_top_k, coerce_none_to_list
from codeforge.mcp_models import MCPServerDef  # noqa: TC001 — Pydantic needs at runtime

# agent.tool_output_max_chars from Go (0 = the worker's default), clamped to
# 0..MAX_TOOL_OUTPUT_MAX_CHARS: the Go Core refuses other values at startup,
# and the worker never fails a message over it or lifts the bound.
ToolOutputMaxChars = Annotated[int, AfterValidator(clamp_tool_output_max_chars)]


class TaskStatus(StrEnum):
    """Status of a task in the pipeline."""

    PENDING = "pending"
    QUEUED = "queued"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


class TaskMessage(BaseModel):
    """Message received from NATS when a task is assigned to a worker (Go TaskAgentPayload)."""

    model_config = {"populate_by_name": True}

    id: str = Field(alias="task_id")
    project_id: str
    tenant_id: str = ""
    agent_id: str = ""
    title: str
    prompt: str
    # The agent backend and the project workspace it works in; "" in tasks
    # published before they were part of the payload.
    backend: str = ""
    workspace_path: str = ""
    config: dict[str, str] = Field(default_factory=dict)
    # How often to report the work alive (Go runtime.heartbeat_interval;
    # 0 = the worker's default).
    heartbeat_seconds: int = 0
    # This dispatch of the task; named on its heartbeats, which the Go Core
    # counts for this dispatch only ("" in tasks published without one).
    dispatch_id: str = ""
    # The tenant's tool UID the backend CLI runs as (KI-96); 0: none (the
    # Go Core runs with workspace.tool_acls off).
    tool_uid: int = Field(default=0, ge=0)


class TaskResult(BaseModel):
    """Result sent back to NATS after task execution."""

    task_id: str
    tenant_id: str = ""
    project_id: str = ""
    # The dispatch the result reports (TaskMessage.dispatch_id): the Go Core
    # ignores a result of a dispatch that is no longer the task's current one.
    dispatch_id: str = ""
    status: TaskStatus
    output: str = ""
    files: list[str] = Field(default_factory=list)
    error: str = ""
    tokens_in: int = 0
    tokens_out: int = 0
    cost_usd: float = 0.0


# --- Run Protocol Models (Phase 4B) ---


class TerminationConfig(BaseModel):
    """Termination conditions received from the Go control plane."""

    max_steps: int = 50
    timeout_seconds: int = 600
    max_cost: float = 5.0


class ContextEntry(BaseModel):
    """A single context entry delivered with a run start message (Phase 5D)."""

    kind: str = "file"
    path: str = ""
    content: str = ""
    tokens: int = 0
    priority: int = 50


class ModeConfig(BaseModel):
    """Agent mode metadata received from the Go control plane."""

    id: str = ""
    prompt_prefix: str = ""
    tools: list[str] = Field(default_factory=list)
    denied_tools: list[str] = Field(default_factory=list)
    denied_actions: list[str] = Field(default_factory=list)
    required_artifact: str = ""
    llm_scenario: str = ""
    output_schema: str = ""
    model_adaptations: dict[str, str] = Field(default_factory=dict)


class TrustAnnotation(BaseModel):
    """Trust metadata attached to inter-agent messages (Phase 23A)."""

    origin: str = "internal"
    trust_level: str = "full"
    source_id: str = ""
    signature: str = ""
    timestamp: str = ""


class RunStartMessage(BaseModel):
    """Message received from NATS when a run is started."""

    run_id: str
    task_id: str
    tenant_id: str = ""
    project_id: str
    agent_id: str
    prompt: str
    policy_profile: str = ""
    exec_mode: str = "mount"
    deliver_mode: str = ""
    mode: ModeConfig = Field(default_factory=ModeConfig)
    config: dict[str, str] = Field(default_factory=dict)
    termination: TerminationConfig = Field(default_factory=TerminationConfig)

    mcp_servers: list[MCPServerDef] = Field(default_factory=list)
    context: list[ContextEntry] = Field(default_factory=list)
    microagent_prompts: list[str] = Field(default_factory=list)
    trust: TrustAnnotation | None = None
    # The project workspace the run's tools work in, and the agent's backend
    # (informational: runs execute in the worker's own agent loop).
    workspace_path: str = ""
    backend: str = ""
    # Go's HITL approval timeout; tool call decisions are awaited longer
    # (0 = the worker's default).
    approval_timeout_seconds: int = 0
    # How often to report the work alive (Go runtime.heartbeat_interval;
    # 0 = the worker's default).
    heartbeat_seconds: int = 0
    # agent.tool_output_max_chars from Go; 0 = the worker's default.
    tool_output_max_chars: ToolOutputMaxChars = 0
    # The tenant's tool UID the run's tool processes run as (KI-96); 0: none.
    tool_uid: int = Field(default=0, ge=0)

    @field_validator("config", mode="before")
    @classmethod
    def _coerce_config_none(cls, v: dict[str, str] | None) -> dict[str, str]:
        """Go serializes nil maps as null; coerce to empty dict."""
        return v if v is not None else {}

    @field_validator("mcp_servers", "context", "microagent_prompts", mode="before")
    @classmethod
    def _coerce_list_fields(cls, v: list | None) -> list:
        return coerce_none_to_list(v)


class ToolCallDecision(BaseModel):
    """Response from Go control plane for a tool call permission request."""

    run_id: str = ""
    call_id: str
    decision: str  # allow, deny, ask
    reason: str = ""
    exec_mode: str = ""
    container_id: str = ""


class RunCompleteMessage(BaseModel):
    """Completion message sent to Go control plane when a run finishes."""

    run_id: str
    task_id: str
    tenant_id: str = ""
    project_id: str
    status: str = "completed"
    output: str = ""
    error: str = ""
    cost_usd: float = 0.0
    step_count: int = 0
    tokens_in: int = 0
    tokens_out: int = 0
    model: str = ""


# --- Quality Gate Models (Phase 4C) ---


class QualityGateRequest(BaseModel):
    """Request from Go control plane to execute quality gate checks."""

    run_id: str
    project_id: str
    tenant_id: str = ""
    workspace_path: str
    run_tests: bool = False
    run_lint: bool = False
    test_command: str = ""
    lint_command: str = ""
    # Per-command timeout (runtime.quality_gate_timeout); 0 = the worker's default.
    timeout_seconds: int = Field(default=0, ge=0)
    # How often to report the running gate (runs.heartbeat, phase
    # quality_gate); 0 = the worker's default.
    heartbeat_seconds: int = Field(default=0, ge=0)
    # The tenant's tool UID the gate commands run as (KI-96); 0: none.
    tool_uid: int = Field(default=0, ge=0)
    # agent.tool_output_max_chars: each check's output is bounded to it
    # (head and tail, KI-126); 0 = the worker's default.
    tool_output_max_chars: ToolOutputMaxChars = 0


class WorkspaceTestRequest(BaseModel):
    """Request from Go to run one test file of a workspace (auto-agent, KI-81)."""

    request_id: str
    tenant_id: str = ""
    project_id: str = ""
    conversation_id: str = ""
    workspace_path: str
    # A file name matching test_<word>.py in the workspace root.
    test_file: str
    timeout_seconds: int = Field(default=0, ge=0)
    # The tenant's tool UID the test runs as (KI-96); 0: none.
    tool_uid: int = Field(default=0, ge=0)


class WorkspaceTestResult(BaseModel):
    """Outcome of a workspace test run; passed is None when the tests did not run or finish."""

    request_id: str
    tenant_id: str = ""
    conversation_id: str = ""
    passed: bool | None = None
    output: str = ""
    error: str = ""


class WorkspaceDeleteRequest(BaseModel):
    """Request from Go to remove a deleted project's workspace as the tenant's tool UID (KI-96 D11)."""

    deletion_id: str
    tenant_id: str
    tool_uid: int = Field(default=0, ge=0)
    project_id: str = ""
    workspace_path: str


class WorkspaceDeleteResult(BaseModel):
    """Outcome of a workspace deletion; ok False with the reason in error."""

    deletion_id: str
    tenant_id: str = ""
    ok: bool = False
    error: str = ""


class QualityGateResult(BaseModel):
    """Result of quality gate execution sent back to Go control plane."""

    run_id: str
    tenant_id: str = ""
    tests_passed: bool | None = None
    lint_passed: bool | None = None
    test_output: str = ""
    lint_output: str = ""
    error: str = ""


# --- RepoMap Models (Phase 6A) ---


class RepoMapRequest(BaseModel):
    """Request from Go control plane to generate a repository map."""

    project_id: str
    tenant_id: str = ""
    workspace_path: str
    token_budget: int = 1024
    active_files: list[str] = Field(default_factory=list)

    @field_validator("active_files", mode="before")
    @classmethod
    def _coerce_null_to_empty(cls, v: list[str] | None) -> list[str]:
        return coerce_none_to_list(v)


class RepoMapResult(BaseModel):
    """Result of repo map generation sent back to Go control plane."""

    project_id: str
    tenant_id: str = ""
    map_text: str
    token_count: int
    file_count: int
    symbol_count: int
    languages: list[str]
    error: str = ""


# --- Retrieval Models (Phase 6B) ---


class RetrievalIndexRequest(BaseModel):
    """Request from Go control plane to build a hybrid retrieval index."""

    project_id: str
    tenant_id: str = ""
    workspace_path: str
    embedding_model: str = "text-embedding-3-small"
    file_extensions: list[str] = Field(default_factory=list)
    # Set instead of workspace_path for a knowledge base ("kb:<id>"): its
    # content, relative to the worker's knowledge content root (KI-105).
    knowledge_path: str = ""


class RetrievalIndexResult(BaseModel):
    """Result of retrieval index build sent back to Go control plane."""

    project_id: str
    tenant_id: str = ""
    status: str
    file_count: int = 0
    chunk_count: int = 0
    embedding_model: str = ""
    error: str = ""
    incremental: bool = False
    files_changed: int = 0
    files_unchanged: int = 0


class RetrievalSearchRequest(BaseModel):
    """Request from Go control plane to search a project's retrieval index."""

    project_id: str
    query: str
    request_id: str
    top_k: int = 20
    bm25_weight: float = 0.5
    semantic_weight: float = 0.5
    scope_id: str = ""

    @field_validator("top_k")
    @classmethod
    def _clamp_top_k(cls, v: int) -> int:
        return clamp_top_k(v)


class RetrievalSearchHit(BaseModel):
    """A single search result from hybrid retrieval."""

    filepath: str
    start_line: int
    end_line: int
    content: str
    language: str
    symbol_name: str = ""
    score: float = 0.0
    bm25_rank: int = 0
    semantic_rank: int = 0
    project_id: str = ""


class RetrievalSearchResult(BaseModel):
    """Result of a retrieval search sent back to Go control plane."""

    project_id: str
    query: str
    request_id: str
    results: list[RetrievalSearchHit] = Field(default_factory=list)
    error: str = ""


# --- Retrieval Sub-Agent Models (Phase 6C) ---


class SubAgentSearchRequest(BaseModel):
    """Request for LLM-guided multi-query retrieval."""

    project_id: str
    query: str
    request_id: str
    top_k: int = 20
    max_queries: int = 5
    model: str = ""
    rerank: bool = True
    scope_id: str = ""
    expansion_prompt: str = ""

    @field_validator("top_k")
    @classmethod
    def _clamp_top_k(cls, v: int) -> int:
        return clamp_top_k(v)

    @field_validator("max_queries")
    @classmethod
    def _clamp_max_queries(cls, v: int) -> int:
        return max(1, min(v, 20))


class SubAgentSearchResult(BaseModel):
    """Result from LLM-guided multi-query retrieval."""

    project_id: str
    query: str
    request_id: str
    results: list[RetrievalSearchHit] = Field(default_factory=list)
    expanded_queries: list[str] = Field(default_factory=list)
    total_candidates: int = 0
    error: str = ""
    model: str = ""
    tokens_in: int = 0
    tokens_out: int = 0
    cost_usd: float = 0.0


# --- GraphRAG Models (Phase 6D) ---


class GraphBuildRequest(BaseModel):
    """Request from Go control plane to build a code graph for a project."""

    project_id: str
    tenant_id: str = ""
    workspace_path: str
    scope_id: str = ""


class GraphBuildResult(BaseModel):
    """Result of graph build sent back to Go control plane."""

    project_id: str
    tenant_id: str = ""
    status: str  # "ready" or "error"
    node_count: int = 0
    edge_count: int = 0
    languages: list[str] = Field(default_factory=list)
    error: str = ""


class GraphSearchRequest(BaseModel):
    """Request from Go control plane to search the code graph."""

    project_id: str
    request_id: str
    seed_symbols: list[str]
    max_hops: int = 2
    top_k: int = 10
    scope_id: str = ""


class GraphSearchHit(BaseModel):
    """A single node returned from graph traversal."""

    filepath: str
    symbol_name: str
    kind: str  # "function", "class", "method", "module"
    start_line: int = 0
    end_line: int = 0
    distance: int  # hops from seed
    score: float
    edge_path: list[str] = Field(default_factory=list)
    project_id: str = ""


class GraphSearchResult(BaseModel):
    """Result of a graph search sent back to Go control plane."""

    project_id: str
    request_id: str
    results: list[GraphSearchHit] = Field(default_factory=list)
    error: str = ""


# --- Context Re-ranking Models (Phase 3 — Context Intelligence) ---


class ContextRerankEntryPayload(BaseModel):
    """A single context entry for re-ranking."""

    path: str = ""
    kind: str = "file"
    content: str = ""
    priority: int = 50
    tokens: int = 0


class ContextRerankRequest(BaseModel):
    """Request payload for context re-ranking via NATS."""

    request_id: str
    project_id: str = ""
    query: str = ""
    entries: list[ContextRerankEntryPayload] = Field(default_factory=list)
    model: str = ""


class ContextRerankResult(BaseModel):
    """Result payload from context re-ranking."""

    request_id: str
    entries: list[ContextRerankEntryPayload] = Field(default_factory=list)
    fallback_used: bool = False
    tokens_in: int = 0
    tokens_out: int = 0
    cost_usd: float = 0.0
    error: str = ""


# --- Conversation Run Models (Phase 17C) ---


class ConversationToolCallFunction(BaseModel):
    """Function details within a tool call."""

    name: str
    arguments: str


class ConversationToolCallPayload(BaseModel):
    """A single tool call in an assistant message."""

    id: str
    type: str = "function"
    function: ConversationToolCallFunction


class MessageImagePayload(BaseModel):
    """An image attached to a conversation message."""

    data: str  # base64-encoded
    media_type: str  # e.g. "image/png"
    alt_text: str = ""


class ConversationMessagePayload(BaseModel):
    """A chat message in the conversation run protocol."""

    role: str
    content: str = ""
    tool_calls: list[ConversationToolCallPayload] = Field(default_factory=list)
    tool_call_id: str = ""
    name: str = ""
    images: list[MessageImagePayload] = Field(default_factory=list)


class SessionMetaPayload(BaseModel):
    """Session operation context for resumed/forked/rewound sessions."""

    parent_session_id: str = ""
    parent_run_id: str = ""
    fork_event_id: str = ""
    rewind_event_id: str = ""
    operation: str = ""  # "resume" | "fork" | "rewind" | ""


class ConversationRunStartMessage(BaseModel):
    """Message received from NATS when a conversation run is started."""

    run_id: str
    conversation_id: str
    session_id: str = ""
    project_id: str
    messages: list[ConversationMessagePayload]
    system_prompt: str
    model: str
    policy_profile: str = "standard"
    workspace_path: str = ""
    mode: ModeConfig | None = None
    termination: TerminationConfig = Field(default_factory=TerminationConfig)
    context: list[ContextEntry] = Field(default_factory=list)
    mcp_servers: list[MCPServerDef] = Field(default_factory=list)
    tools: list[str] = Field(default_factory=list)
    microagent_prompts: list[str] = Field(default_factory=list)
    trust: TrustAnnotation | None = None
    routing_enabled: bool = False
    agentic: bool = True
    plan_act_enabled: bool = False
    provider_api_key: str = ""
    tenant_id: str = ""
    session_meta: SessionMetaPayload | None = None
    reminders: list[str] = Field(default_factory=list)
    rollout_count: int = 1
    summarize_threshold: int = 0
    # agent.tool_output_max_chars from Go; 0 = the worker's default.
    tool_output_max_chars: ToolOutputMaxChars = 0
    # Identifies this run of the conversation; echoed on every tool call.
    turn_id: str = ""
    # The tenant's tool UID the turn's tool processes run as (KI-96); 0: none.
    tool_uid: int = Field(default=0, ge=0)
    # Go's HITL approval timeout; tool call decisions are awaited longer
    # (0 = the worker's default).
    approval_timeout_seconds: int = 0
    # How often to report the work alive (Go runtime.heartbeat_interval;
    # 0 = the worker's default).
    heartbeat_seconds: int = 0

    @field_validator("mcp_servers", "context", "tools", "microagent_prompts", "reminders", mode="before")
    @classmethod
    def _coerce_list_fields(cls, v: list | None) -> list:
        return coerce_none_to_list(v)

    @field_validator("plan_act_enabled", mode="before")
    @classmethod
    def _coerce_plan_act(cls, v: bool | None) -> bool:
        """Go may omit or send null for false booleans; coerce to False."""
        return bool(v) if v is not None else False


class ConversationRunCompleteMessage(BaseModel):
    """Completion message sent to Go control plane when a conversation run finishes."""

    run_id: str
    conversation_id: str
    session_id: str = ""
    assistant_content: str = ""
    tool_messages: list[ConversationMessagePayload] = Field(default_factory=list)
    status: str = "completed"
    error: str = ""
    cost_usd: float = 0.0
    tokens_in: int = 0
    tokens_out: int = 0
    step_count: int = 0
    model: str = ""
    tenant_id: str = ""
    # The turn of the run start this completion ends (echoed from it).
    turn_id: str = ""


class AgentLoopResult(BaseModel):
    """Result returned from the agentic conversation loop."""

    final_content: str = ""
    tool_messages: list[ConversationMessagePayload] = Field(default_factory=list)
    total_cost: float = 0.0
    total_tokens_in: int = 0
    total_tokens_out: int = 0
    step_count: int = 0
    model: str = ""
    error: str = ""
    metadata: dict[str, object] = Field(default_factory=dict)


# ---------------------------------------------------------------------------
# Benchmark run messages (Phase 20A — dev-mode only)
# ---------------------------------------------------------------------------


class BenchmarkRunRequest(BaseModel):
    """Request to execute a benchmark evaluation run."""

    run_id: str
    tenant_id: str = ""
    dataset_path: str
    model: str
    metrics: list[str] = Field(default_factory=lambda: ["correctness"])
    benchmark_type: str = "simple"
    suite_id: str = ""
    exec_mode: str = ""
    evaluators: list[str] = Field(default_factory=list)
    hybrid_verification: bool = False
    rollout_count: int = 1
    rollout_strategy: str = "best"
    provider_name: str = ""
    provider_config: dict[str, Any] = Field(default_factory=dict)
    # The tenant's tool UID the benchmark's tool processes run as (KI-96); 0: none.
    tool_uid: int = Field(default=0, ge=0)


class BenchmarkTaskResult(BaseModel):
    """Result of a single benchmark task."""

    task_id: str
    tenant_id: str = ""
    task_name: str
    scores: dict[str, float] = Field(default_factory=dict)
    actual_output: str = ""
    expected_output: str = ""
    tool_calls: list[dict[str, str]] = Field(default_factory=list)
    cost_usd: float = 0.0
    tokens_in: int = 0
    tokens_out: int = 0
    duration_ms: int = 0
    evaluator_scores: dict[str, dict[str, float]] = Field(default_factory=dict)
    # Dimensions an evaluator could not score (dimension name -> error); they
    # are never in scores or evaluator_scores, so no average counts them.
    evaluation_errors: dict[str, str] = Field(default_factory=dict)
    files_changed: list[str] = Field(default_factory=list)
    functional_test_output: str = ""
    rollout_id: int = 0
    rollout_count: int = 1
    is_best_rollout: bool = True
    diversity_score: float = 0.0
    selected_model: str = ""
    routing_reason: str = ""
    fallback_chain: str = ""
    fallback_count: int = 0
    provider_errors: str = ""


class BenchmarkRunResult(BaseModel):
    """Result of a complete benchmark run."""

    run_id: str
    tenant_id: str = ""
    status: str = "completed"
    results: list[BenchmarkTaskResult] = Field(default_factory=list)
    summary: dict[str, object] = Field(default_factory=dict)
    total_cost: float = 0.0
    total_tokens: int = 0
    total_duration_ms: int = 0
    error: str = ""


# --- GEMMAS Evaluation Models (Phase 20G) ---


class GemmasAgentMessage(BaseModel):
    """A single agent message for GEMMAS evaluation (matches Go GemmasAgentMessagePayload)."""

    agent_id: str = ""
    content: str = ""
    round: int = 0
    parent_agent_id: str = ""


class GemmasEvalRequest(BaseModel):
    """Request to compute GEMMAS metrics for a completed plan."""

    plan_id: str
    messages: list[GemmasAgentMessage] = Field(default_factory=list)


class GemmasEvalResult(BaseModel):
    """Result of GEMMAS metric computation."""

    plan_id: str
    information_diversity_score: float = 0.0
    unnecessary_path_ratio: float = 0.0
    error: str = ""


# --- A2A Task Models (Phase 27) ---


class A2ATaskCreatedMessage(BaseModel):
    """Inbound A2A task from Go (matches Go A2ATaskCreatedPayload)."""

    task_id: str
    tenant_id: str = ""
    skill_id: str = ""
    prompt: str = ""


class A2ATaskCompleteMessage(BaseModel):
    """A2A task completion to publish back to Go (matches Go A2ATaskCompletePayload)."""

    task_id: str
    tenant_id: str = ""
    state: str = "completed"
    error: str = ""


# --- Prompt Evolution Models (Phase 33) ---


class PromptEvolutionTacticalFix(BaseModel):
    """A single failure-specific fix (matches Go PromptEvolutionTacticalFix)."""

    task_id: str = ""
    failure_description: str = ""
    root_cause: str = ""
    proposed_addition: str = ""
    confidence: float = 0.0


class PromptEvolutionReflectRequest(BaseModel):
    """Request to perform failure reflection (matches Go PromptEvolutionReflectPayload)."""

    tenant_id: str = ""
    mode_id: str = ""
    model_family: str = ""
    current_prompt: str = ""
    failures: list[dict[str, object]] = Field(default_factory=list)


class PromptEvolutionReflectComplete(BaseModel):
    """Reflection results sent back to Go (matches Go PromptEvolutionReflectCompletePayload)."""

    tenant_id: str = ""
    mode_id: str = ""
    model_family: str = ""
    tactical_fixes: list[PromptEvolutionTacticalFix] = Field(default_factory=list)
    strategic_principles: list[str] = Field(default_factory=list)
    error: str = ""


class PromptEvolutionMutateComplete(BaseModel):
    """Mutation results sent back to Go (matches Go PromptEvolutionMutateCompletePayload)."""

    tenant_id: str = ""
    mode_id: str = ""
    model_family: str = ""
    variant_content: str = ""
    version: int = 0
    parent_id: str = ""
    mutation_source: str = ""
    validation_passed: bool = False
    error: str = ""
