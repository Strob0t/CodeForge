"""NATS stream and subject constants for the TaskConsumer."""

from __future__ import annotations

STREAM_NAME = "CODEFORGE"
STREAM_SUBJECTS = [
    "tasks.>",
    "agents.>",
    "runs.>",
    "context.>",
    "repomap.>",
    "retrieval.>",
    "graph.>",
    "conversation.>",
    "benchmark.>",
    "evaluation.>",
    "mcp.>",
    "a2a.>",
    "memory.>",
    "handoff.>",
    "backends.>",
    "review.>",
    "prompt.>",
]

# Task subjects
SUBJECT_AGENT = "tasks.agent.*"
SUBJECT_RESULT = "tasks.result"
SUBJECT_OUTPUT = "tasks.output"
SUBJECT_TASK_CANCEL = "tasks.cancel"

# Run subjects
SUBJECT_RUN_START = "runs.start"

# Quality gate
SUBJECT_QG_REQUEST = "runs.qualitygate.request"
SUBJECT_QG_RESULT = "runs.qualitygate.result"

# Repomap
SUBJECT_REPOMAP_REQUEST = "repomap.generate.request"
SUBJECT_REPOMAP_RESULT = "repomap.generate.result"

# Retrieval
SUBJECT_RETRIEVAL_INDEX_REQUEST = "retrieval.index.request"
SUBJECT_RETRIEVAL_INDEX_RESULT = "retrieval.index.result"
SUBJECT_RETRIEVAL_SEARCH_REQUEST = "retrieval.search.request"
SUBJECT_RETRIEVAL_SEARCH_RESULT = "retrieval.search.result"
SUBJECT_SUBAGENT_SEARCH_REQUEST = "retrieval.subagent.request"
SUBJECT_SUBAGENT_SEARCH_RESULT = "retrieval.subagent.result"

# Graph
SUBJECT_GRAPH_BUILD_REQUEST = "graph.build.request"
SUBJECT_GRAPH_BUILD_RESULT = "graph.build.result"
SUBJECT_GRAPH_SEARCH_REQUEST = "graph.search.request"
SUBJECT_GRAPH_SEARCH_RESULT = "graph.search.result"

# Context re-ranking (Phase 3 — Context Intelligence)
SUBJECT_CONTEXT_RERANK_REQUEST = "context.rerank.request"
SUBJECT_CONTEXT_RERANK_RESULT = "context.rerank.result"

# Conversation
SUBJECT_CONVERSATION_RUN_START = "conversation.run.start"
SUBJECT_CONVERSATION_RUN_COMPLETE = "conversation.run.complete"
SUBJECT_CONVERSATION_COMPACT_REQUEST = "conversation.compact.request"
SUBJECT_CONVERSATION_COMPACT_COMPLETE = "conversation.compact.complete"

# Benchmark
SUBJECT_BENCHMARK_RUN_REQUEST = "benchmark.run.request"
SUBJECT_BENCHMARK_RUN_RESULT = "benchmark.run.result"
SUBJECT_BENCHMARK_TASK_STARTED = "benchmark.task.started"
SUBJECT_BENCHMARK_TASK_PROGRESS = "benchmark.task.progress"

# Evaluation
SUBJECT_EVAL_GEMMAS_REQUEST = "evaluation.gemmas.request"
SUBJECT_EVAL_GEMMAS_RESULT = "evaluation.gemmas.result"

# Memory
SUBJECT_MEMORY_STORE = "memory.store"
SUBJECT_MEMORY_RECALL = "memory.recall"
SUBJECT_MEMORY_RECALL_RESULT = "memory.recall.result"

# A2A
SUBJECT_A2A_TASK_CREATED = "a2a.task.created"
SUBJECT_A2A_TASK_COMPLETE = "a2a.task.complete"
SUBJECT_A2A_TASK_CANCEL = "a2a.task.cancel"

# Handoff
SUBJECT_HANDOFF_REQUEST = "handoff.request"

# Backend health
SUBJECT_BACKEND_HEALTH_REQUEST = "backends.health.request"
SUBJECT_BACKEND_HEALTH_RESULT = "backends.health.result"

# Trajectory events
SUBJECT_TRAJECTORY_EVENT = "runs.trajectory.event"

# Review/Refactor subjects (Phase 31)
SUBJECT_REVIEW_TRIGGER_REQUEST = "review.trigger.request"
SUBJECT_REVIEW_TRIGGER_COMPLETE = "review.trigger.complete"
SUBJECT_REVIEW_APPROVAL_REQUIRED = "review.approval.required"

# Prompt evolution subjects (Phase 33)
SUBJECT_PROMPT_EVOLUTION_REFLECT = "prompt.evolution.reflect"
SUBJECT_PROMPT_EVOLUTION_REFLECT_COMPLETE = "prompt.evolution.reflect.complete"
SUBJECT_PROMPT_EVOLUTION_MUTATE_COMPLETE = "prompt.evolution.mutate.complete"

# Headers
HEADER_REQUEST_ID = "X-Request-ID"
HEADER_RETRY_COUNT = "Retry-Count"
MAX_RETRIES = 3


def consumer_name(subject: str) -> str:
    """Build a deterministic durable consumer name from a NATS subject."""
    return "codeforge-py-" + subject.replace(".", "-").replace("*", "all").replace(">", "all")
