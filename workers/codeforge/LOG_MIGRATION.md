# Logging Migration Plan (FIX-092)

## Current State

The Python worker codebase uses a mix of two logging approaches:

- **`logging` (stdlib)**: Used in `routing/`, `evaluation/`, `memory/`, and older modules
- **`structlog`**: Used in `consumer/`, `agent_loop.py`, `tools/`, and newer modules

This inconsistency means log output is not uniformly structured JSON, making
it harder to parse with `docker compose logs | jq`.

## Target State

All Python modules should use **`structlog`** with JSON output, matching the
Go backend's structured logging approach.

## Migration Steps

1. **Audit**: Identify all files using `logging.getLogger(__name__)`:
   - `workers/codeforge/routing/blocklist.py`
   - `workers/codeforge/routing/key_filter.py`
   - `workers/codeforge/routing/rate_limit.py`
   - `workers/codeforge/routing/reward.py`
   - `workers/codeforge/evaluation/` (multiple files)
   - `workers/codeforge/memory/` (multiple files)
   - `workers/codeforge/graphrag/` (multiple files)

2. **Replace**: Change `import logging` to `import structlog` and
   `logging.getLogger(__name__)` to `structlog.get_logger()`.

3. **Update call sites**: Replace format-string logging
   (`logger.warning("msg %s", val)`) with keyword-arg logging
   (`logger.warning("msg", key=val)`).

4. **Verify**: Run `poetry run pytest -v` to ensure no regressions.

## Priority

LOW -- this is a consistency improvement, not a bug fix. The current mix
works correctly; both loggers write to stdout and are captured by Docker.
