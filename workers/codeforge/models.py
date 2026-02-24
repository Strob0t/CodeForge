"""Domain models for task messages exchanged between Go Core and Python Workers."""

from __future__ import annotations

from enum import StrEnum

from pydantic import BaseModel, Field, field_validator

from codeforge.mcp_models import MCPServerDef  # noqa: TC001 — Pydantic needs at runtime


class TaskStatus(StrEnum):
    """Status of a task in the pipeline."""

    PENDING = "pending"
    QUEUED = "queued"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


class TaskMessage(BaseModel):
    """Message received from NATS when a task is assigned to a worker."""

    id: str
    project_id: str
    title: str
    prompt: str
    config: dict[str, str] = Field(default_factory=dict)


class TaskResult(BaseModel):
    """Result sent back to NATS after task execution."""

    task_id: str
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


class RunStartMessage(BaseModel):
    """Message received from NATS when a run is started."""

    run_id: str
    task_id: str
    project_id: str
    agent_id: str
    prompt: str
    policy_profile: str = ""
    exec_mode: str = "mount"
    mode: ModeConfig = Field(default_factory=ModeConfig)
    config: dict[str, str] = Field(default_factory=dict)
    termination: TerminationConfig = Field(default_factory=TerminationConfig)

    mcp_servers: list[MCPServerDef] = Field(default_factory=list)

    @field_validator("config", mode="before")
    @classmethod
    def _coerce_config_none(cls, v: dict[str, str] | None) -> dict[str, str]:
        """Go serializes nil maps as null; coerce to empty dict."""
        return v if v is not None else {}

    @field_validator("mcp_servers", mode="before")
    @classmethod
    def _coerce_mcp_servers_none(cls, v: list[MCPServerDef] | None) -> list[MCPServerDef]:
        """Go serializes nil slices as null; coerce to empty list."""
        return v if v is not None else []

    context: list[ContextEntry] = Field(default_factory=list)


class ToolCallDecision(BaseModel):
    """Response from Go control plane for a tool call permission request."""

    call_id: str
    decision: str  # allow, deny, ask
    reason: str = ""


class RunCompleteMessage(BaseModel):
    """Completion message sent to Go control plane when a run finishes."""

    run_id: str
    task_id: str
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
    workspace_path: str
    run_tests: bool = False
    run_lint: bool = False
    test_command: str = ""
    lint_command: str = ""


class QualityGateResult(BaseModel):
    """Result of quality gate execution sent back to Go control plane."""

    run_id: str
    tests_passed: bool | None = None
    lint_passed: bool | None = None
    test_output: str = ""
    lint_output: str = ""
    error: str = ""


# --- RepoMap Models (Phase 6A) ---


class RepoMapRequest(BaseModel):
    """Request from Go control plane to generate a repository map."""

    project_id: str
    workspace_path: str
    token_budget: int = 1024
    active_files: list[str] = Field(default_factory=list)

    @field_validator("active_files", mode="before")
    @classmethod
    def _coerce_null_to_empty(cls, v: list[str] | None) -> list[str]:
        """Go marshals nil slices as null; treat as empty list."""
        return v if v is not None else []


class RepoMapResult(BaseModel):
    """Result of repo map generation sent back to Go control plane."""

    project_id: str
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
    workspace_path: str
    embedding_model: str = "text-embedding-3-small"
    file_extensions: list[str] = Field(default_factory=list)


class RetrievalIndexResult(BaseModel):
    """Result of retrieval index build sent back to Go control plane."""

    project_id: str
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
        return max(1, min(v, 500))


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
        return max(1, min(v, 500))

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
    workspace_path: str
    scope_id: str = ""


class GraphBuildResult(BaseModel):
    """Result of graph build sent back to Go control plane."""

    project_id: str
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
