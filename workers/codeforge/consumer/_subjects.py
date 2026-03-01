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
    "memory.>",
    "handoff.>",
]

# Task subjects
SUBJECT_AGENT = "tasks.agent.*"
SUBJECT_RESULT = "tasks.result"
SUBJECT_OUTPUT = "tasks.output"

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

# Conversation
SUBJECT_CONVERSATION_RUN_START = "conversation.run.start"
SUBJECT_CONVERSATION_RUN_COMPLETE = "conversation.run.complete"

# Benchmark
SUBJECT_BENCHMARK_RUN_REQUEST = "benchmark.run.request"
SUBJECT_BENCHMARK_RUN_RESULT = "benchmark.run.result"

# Evaluation
SUBJECT_EVAL_GEMMAS_REQUEST = "evaluation.gemmas.request"
SUBJECT_EVAL_GEMMAS_RESULT = "evaluation.gemmas.result"

# Memory
SUBJECT_MEMORY_STORE = "memory.store"
SUBJECT_MEMORY_RECALL = "memory.recall"

# Handoff
SUBJECT_HANDOFF_REQUEST = "handoff.request"

# Headers
HEADER_REQUEST_ID = "X-Request-ID"
HEADER_RETRY_COUNT = "Retry-Count"
MAX_RETRIES = 3
