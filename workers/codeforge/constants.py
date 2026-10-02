"""Centralized constants for the CodeForge worker.

All magic numbers that were previously scattered across multiple modules
are collected here for easy discovery and consistent usage.
"""

from __future__ import annotations

# -- Token estimation --------------------------------------------------------
CHARS_PER_TOKEN = 4  # Rough heuristic: 1 token ~ 4 characters.

# -- Tool output limits ------------------------------------------------------
MAX_OUTPUT_CHARS = 50_000  # Bash tool: max stdout/stderr before truncation.
MAX_TOOL_RESULTS = 500  # Glob tool: max file paths returned.
MAX_DIR_ENTRIES = 500  # ListDirectory tool: max entries.
MAX_LIST_DEPTH = 3  # ListDirectory tool: max recursive depth.
MAX_SEARCH_MATCHES = 100  # SearchFiles tool: max grep matches.
# Largest workspace file the worker reads into memory (read_file, edit_file,
# benchmark snapshots); the Go Core's file API uses the same cap (KI-95).
MAX_WORKSPACE_FILE_BYTES = 10 * 1024 * 1024

# -- Backend execution -------------------------------------------------------
DEFAULT_BACKEND_TIMEOUT_SECONDS = 600  # 10 minutes per backend task.
DEFAULT_QG_TIMEOUT_SECONDS = 120  # Quality gate command timeout.

# -- Tool call policy decisions ----------------------------------------------
# The Go Core owns the HITL approval timeout (runtime.approval_timeout_seconds)
# and sends it with every run start; a policy response is awaited at least that
# long plus a margin (KI-21). The default mirrors the Go default and applies
# when a run start carries no timeout (a core that predates the field).
DEFAULT_APPROVAL_TIMEOUT_SECONDS = 60
# Time Go may need after its approval wait to answer (policy evaluation, store
# writes, publishing the response).
APPROVAL_RESPONSE_MARGIN_SECONDS = 15

# -- CLI availability checks -------------------------------------------------
CLI_CHECK_TIMEOUT_SECONDS = 10  # Timeout for `--version` probes.
