# Worktree 12: fix/minor-cleanups — Kleine Einzelfixes

**Branch:** `fix/minor-cleanups`
**Priority:** Niedrig
**Scope:** 13 findings (miscellaneous LOW/INFO)
**Estimated effort:** Small (1-2 days, can be done incrementally)

## Steps

### Quick fixes (1 commit each)

1. **F-COM-001:** OpenAPI license → `name: AGPL-3.0-or-later` in `docs/api/openapi.yaml:10`

2. **F-INF-023:** Add missing log redaction patterns to `internal/logger/redact.go`:
   ```go
   regexp.MustCompile(`(?i)(github_pat_[a-zA-Z0-9_]{36,})`),
   regexp.MustCompile(`(?i)(gsk_[a-zA-Z0-9]{20,})`),
   regexp.MustCompile(`(?i)(hf_[a-zA-Z0-9]{20,})`),
   regexp.MustCompile(`(?i)(sk-ant-[a-zA-Z0-9_-]{20,})`),
   regexp.MustCompile(`(?i)(AIza[a-zA-Z0-9_-]{30,})`),
   ```

3. **F-QUA-014:** Move `stopWords` map to package-level var in `context_optimizer.go:818`

4. **F-QUA-017:** Return `RateLimitInfo` directly from `_extract_rate_info` in `llm.py`

5. **F-ARC-008:** Consolidate NATS subjects — import from `consumer/_subjects.py` in `runtime.py`, `prompt_mutator.py`, `prompt_optimizer.py`

6. **F-ARC-011:** Move crypto delegation from `domain/vcsaccount/crypto.go` to service layer

7. **F-COM-013:** Standardize `alt` attribute pattern on file icons (pick one: descriptive or empty)

8. **F-COM-014:** Backfill CHANGELOG entries from git history

### Larger items (optional, lower priority)

9. **F-QUA-018:** `conversation_agent.go` at 1110 LOC — extract `AgenticDispatcher` and `ConversationContextBuilder` (partially covered by wt05/wt08)

10. **F-ARC-014:** Split `schemas.go` (723 LOC, 64 structs) into domain-scoped files: `schemas_run.go`, `schemas_conversation.go`, `schemas_benchmark.go`, etc.

11. **F-SEC-011:** WebSocket token — consider one-time ticket approach (accepted risk, low priority)

12. **F-COM-011:** Keyboard navigation — audit interactive components, add `onKeyDown` handlers

13. **F-QUA-016:** 28 Go service files without tests — prioritize `auth_apikey.go`, `auth_token.go`, `gdpr.go`, `tenant.go`

## Verification

- All linters pass
- Log redaction test: verify new token patterns are masked
- NATS subjects: verify no duplicate constant definitions remain
- `go build ./...` and `pytest` pass
