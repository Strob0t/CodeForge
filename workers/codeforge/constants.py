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

# -- Backend execution -------------------------------------------------------
DEFAULT_BACKEND_TIMEOUT_SECONDS = 600  # 10 minutes per backend task.
DEFAULT_QG_TIMEOUT_SECONDS = 120  # Quality gate command timeout.

# -- NATS protocol -----------------------------------------------------------
NATS_RESPONSE_TIMEOUT_SECONDS = 30  # Timeout waiting for policy response.

# -- CLI availability checks -------------------------------------------------
CLI_CHECK_TIMEOUT_SECONDS = 10  # Timeout for `--version` probes.
