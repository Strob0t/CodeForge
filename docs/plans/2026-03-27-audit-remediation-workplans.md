# Audit Remediation — Atomic Work Plans

**Date:** 2026-03-27
**Source:** `docs/audits/2026-03-27-universal-audit-report.md`
**Research:** GitHub repos, CJEU decisions, EDPB guidance, SRE literature, framework docs

---

## Overview

22 worktrees covering all 45 findings, ordered by priority. Each plan is atomic: one branch, one PR, zero file overlap with other worktrees.

```
Tier 1 (CRITICAL):  WT-1 ──── WT-2                                          (parallel)
Tier 2 (HIGH):      WT-3 ── WT-4 ── WT-5 ── WT-6 ── WT-7 ── WT-11         (parallel)
Tier 3 (MEDIUM):    WT-8 ── WT-9 ── WT-15 ── WT-16 ── WT-17               (parallel, after WT-5)
Tier 4 (LOW):       WT-12 ── WT-13 ── WT-14 ── WT-22                       (parallel)
Tier 5 (INFO):      WT-10 ── WT-18 ── WT-19 ── WT-20 ── WT-21             (parallel, after WT-2)
```

### Finding Coverage Matrix

| Finding | Worktree | Finding | Worktree | Finding | Worktree |
|---|---|---|---|---|---|
| F-001 | WT-1 | F-016 | WT-5 | F-031 | WT-8 |
| F-002 | WT-2 | F-017 | WT-10 | F-032 | WT-12 |
| F-003 | WT-1 | F-018 | WT-1 | F-033 | WT-13 |
| F-004 | WT-8 | F-019 | WT-9 | F-034 | WT-14 |
| F-005 | WT-5 | F-020 | WT-4 | F-035 | WT-22 |
| F-006 | WT-10 | F-021 | WT-9 | F-036 | WT-7 |
| F-007 | WT-4 | F-022 | WT-9 | F-037 | WT-9 |
| F-008 | WT-3 | F-023 | WT-9 | F-038 | WT-9 |
| F-009 | WT-6 | F-024 | WT-9 | F-039 | WT-9 |
| F-010 | WT-7 | F-025 | WT-3 | F-040 | WT-8 |
| F-011 | WT-11 | F-026 | WT-3 | F-041 | WT-7 |
| F-012 | WT-1 | F-027 | WT-3 | F-042 | WT-18 |
| F-013 | WT-1 | F-028 | WT-16 | F-043 | WT-18 |
| F-014 | WT-8 | F-029 | WT-17 | F-044 | WT-19 |
| F-015 | WT-5 | F-030 | WT-15 | F-045 | WT-20 |
|  |  |  |  | F-046 | WT-21 |

---

## WT-1: `fix/config-secrets` — Config Secret Hardening

**Findings:** F-001, F-003, F-012, F-013, F-018
**Priority:** CRITICAL
**Estimated scope:** Small (3 files)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| `json:"-"` on all sensitive fields | Coder, Consul, Traefik | Apply to 10 fields |
| Blocked defaults list | Mattermost, Nakama | Extend `loader.go` blocklist |
| Entropy validation | go-password-validator (50-70 bits) | Already partially implemented |
| `Config.Sanitized()` copy method | HashiCorp Vault | Future improvement (not this WT) |

### Steps

#### Step 1: Add `json:"-"` to sensitive config fields
**File:** `internal/config/config.go`
**Action:** Add `json:"-"` tag to these 10 fields (matching existing `JWTSecret`/`LLMKeyEncryptionSecret` pattern):
- Line 75: `InternalKey` → `yaml:"internal_key" json:"-"`
- Line 164: `APIToken` → `yaml:"api_token" json:"-"`
- Line 200: `GitHubSecret` → `yaml:"github_secret" json:"-"`
- Line 201: `GitLabToken` → `yaml:"gitlab_token" json:"-"`
- Line 202: `PlaneSecret` → `yaml:"plane_secret" json:"-"`
- Line 213: `SMTPPassword` → `yaml:"smtp_password" json:"-"`
- Line 225: `ClientSecret` → `yaml:"client_secret" json:"-"`
- Line 360: `MasterKey` → `yaml:"master_key" json:"-"`
- Line 407: `APIKeys` → `yaml:"api_keys" json:"-"`
- Line 424: `APIKey` → `yaml:"api_key" json:"-"`

**Test:** Add `TestSensitiveFieldsExcludedFromJSON` in `config_test.go` — marshal `Config` to JSON, assert none of the 12 sensitive field names appear in output.

#### Step 2: Extend JWT secret blocklist
**File:** `internal/config/loader.go`
**Action:** At the blocked defaults check (~line 399), expand the blocklist:
```go
blockedSecrets := []string{
    "codeforge-dev-jwt-secret-change-in-production",
    "e2e-test-secret-key-minimum-32-bytes-long",
    "changeme",
    "secret",
    "password",
    "test-secret",
}
```
Apply in ALL non-development environments (not just production).

**Test:** Table-driven `TestBlockedJWTSecrets` — each blocked string must return error in `APP_ENV=staging`.

#### Step 3: Clear hardcoded secrets from `codeforge.yaml`
**File:** `codeforge.yaml`
**Action:**
- Line 22: `master_key: ""` (was `"sk-codeforge-dev"`)
- Line 102: `jwt_secret: ""` (was `"e2e-test-secret-key-minimum-32-bytes-long"`)
- Both rely on env vars or auto-generation (already implemented in `loader.go:446-453`)

#### Step 4: Extend `sslmode=disable` rejection to staging
**File:** `internal/config/loader.go`
**Action:** Change the `sslmode=disable` check (~line 436) from `appEnv == "production"` to `appEnv != "development"`.

**Test:** `TestSSLModeRejectedInStaging` — assert error when `sslmode=disable` and `APP_ENV=staging`.

### Acceptance Criteria
- [ ] `json.Marshal(cfg)` never contains any of the 12 sensitive field names
- [ ] `APP_ENV=staging` rejects all blocked JWT secrets
- [ ] `APP_ENV=staging` rejects `sslmode=disable`
- [ ] `codeforge.yaml` contains no plaintext secrets
- [ ] All existing tests pass
- [ ] New tests: `TestSensitiveFieldsExcludedFromJSON`, `TestBlockedJWTSecrets`, `TestSSLModeRejectedInStaging`

---

## WT-2: `fix/nats-trajectory-duplicate` — Remove Duplicate NATS Subscription

**Findings:** F-002
**Priority:** CRITICAL
**Estimated scope:** Small (2 files)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| Single subscription per subject | NATS best practices | Remove duplicate |
| Switch-based event dispatch | Existing `handleTrajectoryEvent` | Extend with missing cases |
| Idempotent handlers | CLAUDE.md | Already implemented via dedup |

### Steps

#### Step 1: Identify missing event types in extracted handler
**File:** `internal/service/runtime_subscribers.go`
**Action:** Read the inline handler in `runtime.go:591-763`. Identify the two event types NOT in `handleTrajectoryEvent`:
- `"agent.roadmap_proposed"` (~line 704)
- `"agent.subagent_requested"` (~line 737)

#### Step 2: Add missing handlers to `runtime_subscribers.go`
**File:** `internal/service/runtime_subscribers.go`
**Action:** Add two new methods and their switch cases:

```go
// In handleTrajectoryEvent switch:
case "agent.roadmap_proposed":
    s.handleTrajectoryRoadmapProposed(ctx, payload.RunID, payload.ProjectID, data)
case "agent.subagent_requested":
    s.handleTrajectorySubagentRequested(ctx, payload.RunID, payload.ProjectID, data)
```

Extract the logic from the inline handler (runtime.go:704-737 and 737-760) into these new methods. Preserve exact behavior: AG-UI broadcast, roadmap persistence, sub-agent spawning.

#### Step 3: Remove the inline handler
**File:** `internal/service/runtime.go`
**Action:** Delete lines 591-763 (the entire inline `s.queue.Subscribe(ctx, messagequeue.SubjectTrajectoryEvent, ...)` block and its closure). The subscription at line 574 via the subs table already handles this subject.

#### Step 4: Verify no double processing
**Test:** Add `TestTrajectoryEventProcessedOnce` — publish a trajectory event, assert `events.Append` is called exactly once (not twice). Use a counting mock.

### Acceptance Criteria
- [ ] Only ONE subscription for `SubjectTrajectoryEvent` exists
- [ ] `roadmap_proposed` and `subagent_requested` events are handled
- [ ] AG-UI broadcasts fire exactly once per event
- [ ] DB persistence happens exactly once per event
- [ ] All existing runtime tests pass
- [ ] `StartSubscribers` function is < 50 LOC

---

## WT-3: `fix/gdpr-compliance` — GDPR Audit Log Anonymization + Tests

**Findings:** F-008, F-025, F-026, F-027
**Priority:** HIGH
**Estimated scope:** Medium (4 files + 1 new migration + 1 new test file)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| Store `user_id` FK, not PII | CNIL Developer Guide, Axiom blog | Remove `admin_email` column |
| Anonymize-before-delete | ADR-009 (own project), Homi.so | Implement in `gdpr.go` |
| Seed-Delete-Scan test | Homi.so engineering, Three Dots Labs | New `gdpr_test.go` |
| IP retention: 6 months | CNIL traceability recommendation | Add retention config |
| CJEU Breyer: IPs are personal data | C-582/14 (2016) | Document legal basis |

### Steps

#### Step 1: Create migration to make `admin_email` nullable
**File:** `internal/adapter/postgres/migrations/089_audit_log_anonymize.sql` (new)
```sql
-- +goose Up
ALTER TABLE audit_log ALTER COLUMN admin_email DROP NOT NULL;
ALTER TABLE audit_log ALTER COLUMN admin_email SET DEFAULT NULL;
COMMENT ON COLUMN audit_log.ip_address IS 'Personal data per CJEU C-582/14. Retention: 180 days.';

-- +goose Down
UPDATE audit_log SET admin_email = 'unknown' WHERE admin_email IS NULL;
ALTER TABLE audit_log ALTER COLUMN admin_email SET NOT NULL;
```

#### Step 2: Add `AnonymizeAuditLogForUser` store method
**File:** `internal/adapter/postgres/store_audit.go`
**Action:** Add method:
```go
func (s *Store) AnonymizeAuditLogForUser(ctx context.Context, userID string) error {
    _, err := s.pool.Exec(ctx,
        `UPDATE audit_log SET admin_email = NULL, ip_address = NULL WHERE admin_id = $1`,
        userID)
    return err
}
```
Add to the `AuditStore` interface in `internal/port/database/store.go`.

#### Step 3: Call anonymization before user deletion in GDPR service
**File:** `internal/service/gdpr.go`
**Action:** In `DeleteUserData`, before `s.store.DeleteUser`:
```go
if err := s.store.AnonymizeAuditLogForUser(ctx, userID); err != nil {
    return fmt.Errorf("anonymize audit log: %w", err)
}
```
Log: `slog.Info("gdpr: audit log anonymized", "user_id", userID)`

#### Step 4: Add IP address retention to RetentionService
**File:** `internal/service/retention.go`
**Action:** Add a cleanup step that anonymizes IP addresses older than 180 days:
```go
// In cleanupTenant, after existing cleanup:
if _, err := s.store.Exec(ctx,
    `UPDATE audit_log SET ip_address = NULL WHERE ip_address IS NOT NULL AND created_at < $1 AND tenant_id = $2`,
    time.Now().Add(-180*24*time.Hour), tenantID); err != nil {
    logBestEffort(err, "anonymize old IP addresses")
}
```

#### Step 5: Create comprehensive GDPR test suite
**File:** `internal/service/gdpr_test.go` (new)
**Tests (table-driven):**

| Test | Description |
|---|---|
| `TestExportUserData_Complete` | Seed user with data in all PII tables, export, assert all categories present |
| `TestExportUserData_EmptyUser` | User with no data returns empty export (not error) |
| `TestExportUserData_PartialFailure` | One store fails, export includes error field but continues |
| `TestDeleteUserData_AnonymizesAuditLog` | Create user + audit entries, delete, assert `admin_email` is NULL |
| `TestDeleteUserData_CascadeComplete` | Delete user, scan all FK tables, assert zero rows remain |
| `TestDeleteUserData_NonExistentUser` | Delete unknown user returns appropriate error |
| `TestDeleteUserData_AuditTrailPreserved` | After deletion, audit log entries still exist (with anonymized fields) |

### Acceptance Criteria
- [ ] `admin_email` is nullable in audit_log
- [ ] GDPR deletion anonymizes audit_log entries before deleting user
- [ ] IP addresses older than 180 days are automatically anonymized
- [ ] 7 table-driven tests pass in `gdpr_test.go`
- [ ] ADR-009's stated design is now implemented
- [ ] Migration up/down works cleanly

---

## WT-4: `fix/error-handling` — Dashboard Errors + Dead Code

**Findings:** F-007, F-020
**Priority:** HIGH
**Estimated scope:** Small (2 files)

### Steps

#### Step 1: Fix swallowed errors in dashboard store
**File:** `internal/adapter/postgres/store_dashboard.go`
**Action:** Replace all 6 `_ = s.pool.QueryRow(...)` patterns with the `logBestEffort` pattern:
```go
if err := s.pool.QueryRow(ctx, query, tid).Scan(&ds.ActiveRuns); err != nil {
    logBestEffort(err, "dashboard: count active runs", "tenant_id", tid)
}
```
Import `logBestEffort` from `internal/service/log_best_effort.go` or replicate the pattern locally if import direction forbids it (adapter → service). If so, use `slog.Warn("dashboard query failed", "metric", "active_runs", "error", err)`.

**Test:** `TestDashboardStats_DBError` — mock pool to return error, assert structured log output and zero-valued (not panicked) response.

#### Step 2: Remove dead model resolution
**File:** `internal/service/conversation_agent.go`
**Action:** At line 612-613, replace:
```go
model, _, modeAutonomy, _ := s.resolveModelAndMode(req.Model, req.Mode, conv.Mode)
_ = model
```
With a focused method that only resolves what's needed:
```go
_, _, modeAutonomy, err := s.resolveModelAndMode(req.Model, req.Mode, conv.Mode)
if err != nil {
    slog.Warn("mode resolution failed, using default autonomy", "error", err)
}
```

### Acceptance Criteria
- [ ] Zero `_ = s.pool.QueryRow` patterns in `store_dashboard.go`
- [ ] All 6 error paths log with structured context
- [ ] No `_ = model` dead assignment in `conversation_agent.go`
- [ ] Error from `resolveModelAndMode` is checked
- [ ] Existing tests pass

---

## WT-5: `refactor/hexagonal-handlers` — Extract Business Logic from Handlers

**Findings:** F-005, F-015, F-016
**Priority:** HIGH
**Estimated scope:** Medium (4-5 files)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| Thin adapter handlers | Three Dots Labs / Wild Workouts | Extract to service |
| Filesystem via port interface | Hexagonal Architecture | Use `filesystem.Provider` |
| Sub-handler decomposition | Existing CodeForge pattern | Continue for remaining methods |

### Steps

#### Step 1: Extract `AllowAlways` to PolicyService
**Files:** `internal/service/policy.go`, `internal/adapter/http/handlers_policy_crud.go`
**Action:**
1. Add `AllowAlways(ctx, projectID, tool, command string) (*PolicyProfile, error)` to `PolicyService`
2. Move the 89 lines of logic from `handlers_policy_crud.go:126-214` into this method
3. Handler becomes: parse request → call `s.Policies.AllowAlways()` → write response

**Test:** `TestPolicyService_AllowAlways` — table-driven: preset profile (must clone), custom profile (direct prepend), missing profile (error).

#### Step 2: Move filesystem I/O from handlers to services
**Files:** `internal/adapter/http/handlers_goals.go`, `internal/service/goal_discovery.go`
**Action:**
1. In `handlers_goals.go:146`: Replace `os.ReadFile(docPath)` with a service method call
2. Add `GatherContextFiles(ctx, projectID string) ([]byte, error)` to `GoalDiscoveryService`
3. The service uses `filesystem.Provider` port (already exists) instead of direct `os` calls

**Files:** `internal/adapter/http/handlers_policy_crud.go`
**Action:** The filesystem persistence in `handlers_policy_crud.go:86,115,203` moves into `PolicyService.AllowAlways()` (done in Step 1).

#### Step 3: Verify handler LOC reduction
**Validation:** After refactoring, `handlers_policy_crud.go:AllowAlwaysPolicy` should be < 20 LOC (parse, delegate, respond). `handlers_goals.go` goal context handler should be < 15 LOC.

### Acceptance Criteria
- [ ] Zero `os.ReadFile`, `os.MkdirAll`, `os.Remove` calls in any handler file
- [ ] `AllowAlways` business logic lives in `PolicyService`
- [ ] Goal context file reading goes through service layer
- [ ] Handlers are pure transport adapters (parse → delegate → respond)
- [ ] New service method tests pass
- [ ] Existing E2E/integration tests pass

---

## WT-6: `test/auth-token` — Auth Token Manager Test Coverage

**Findings:** F-009
**Priority:** HIGH
**Estimated scope:** Small (1 new test file)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| Table-driven JWT lifecycle tests | golang-jwt, Vault | Apply to CodeForge tokens |
| Time mocking for expiry | Gitea `timeutil.MockSet` | Inject clock interface or mock |
| Concurrent refresh race test | lestrrat-go/jwx `sync.WaitGroup` | Test double-refresh |
| Token reuse detection | ORY Fosite | Revoke then refresh → error |
| Transaction boundary testing | Fosite `BeginTX/Commit/Rollback` | Mock store transactions |

### Steps

#### Step 1: Create `auth_token_test.go`
**File:** `internal/service/auth_token_test.go` (new)
**Tests:**

| Test | Description |
|---|---|
| `TestGenerateAccessToken_Valid` | Generate token, parse back, assert claims match |
| `TestGenerateAccessToken_Expiry` | Generate with short TTL, advance time, assert expired |
| `TestRefreshToken_Valid` | Create refresh token, use it, get new access+refresh pair |
| `TestRefreshToken_Expired` | Expired refresh token returns `ErrTokenExpired` |
| `TestRefreshToken_Revoked` | Revoke then refresh → `ErrTokenRevoked` |
| `TestRefreshToken_ReuseDetection` | Use refresh token twice → second attempt returns error |
| `TestRefreshToken_ConcurrentRace` | Two goroutines refresh same token → at most one succeeds |
| `TestRevokeToken_Idempotent` | Revoking already-revoked token does not error |
| `TestRevokeAllUserTokens` | Revoke all, then any refresh attempt fails |
| `TestCleanupExpiredTokens` | Seed expired tokens, run cleanup, assert removed |
| `TestHMACSignatureVerification` | Tampered token signature → verification fails |

**Approach:** Use `sync.WaitGroup` + goroutines for concurrent test (lestrrat-go/jwx pattern). Mock the store interface for unit isolation. Use time injection for expiry tests.

### Acceptance Criteria
- [ ] 11 tests in `auth_token_test.go`
- [ ] Concurrent refresh race covered
- [ ] Token reuse detection covered
- [ ] All tests pass
- [ ] Coverage of `auth_token.go` > 80%

---

## WT-7: `fix/frontend-compliance` — AGPL Link + Accessibility + API Client

**Findings:** F-010, F-036, F-041
**Priority:** HIGH
**Estimated scope:** Small (5 frontend files)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| Footer source link | Mastodon `source_url`, Nextcloud footer | Add to sidebar |
| `aria-label` on batch checkboxes | WAI-ARIA APG Checkbox Pattern, Adrian Roselli | Per-item label |
| Centralized API client | Liferay `no-global-fetch` ESLint | Route 4 calls through client |

### Steps

#### Step 1: Add AGPL source code link
**File:** `frontend/src/ui/layout/Sidebar.tsx`
**Action:** Add a "Source Code" link in the sidebar footer area (near the existing version display):
```tsx
<a href="https://github.com/Strob0t/CodeForge"
   target="_blank" rel="noopener noreferrer"
   class="text-xs text-cf-text-muted hover:text-cf-text-secondary">
   AGPL-3.0 Source
</a>
```
This satisfies AGPL Section 13's "prominently offer" requirement.

#### Step 2: Fix unlabeled checkboxes
**File:** `frontend/src/features/dashboard/ProjectCard.tsx`
**Action:** Add `aria-label` to the batch selection checkbox (line 40-44):
```tsx
<input type="checkbox" ... aria-label={`Select ${props.project.name}`} />
```

**File:** `frontend/src/features/project/TrajectoryPanel.tsx`
**Action:** Add `aria-label` to any unlabeled inputs (~line 398).

#### Step 3: Route direct `fetch()` calls through API client
**Files:**
- `frontend/src/features/chat/commandStore.ts:27` — replace `fetch("/api/v1/commands")` with API client call
- `frontend/src/features/chat/commandExecutor.ts:47` — replace `fetch(...)` with API client call
- `frontend/src/features/project/RefactorApproval.tsx:45,61` — replace both `fetch(...)` calls

**Action:** Add `commands` and `runApproval` resources to `frontend/src/api/resources/` (following existing patterns like `projects.ts`), then update the 4 call sites.

### Acceptance Criteria
- [ ] AGPL source link visible in sidebar on every page
- [ ] All form inputs have either `<label>` or `aria-label`
- [ ] Zero direct `fetch()` calls outside `api/client.ts`
- [ ] New API resources follow existing factory pattern
- [ ] Frontend builds without errors

---

## WT-8: `fix/infra-hardening` — JetStream Limits + Bash Safety + Monitoring

**Findings:** F-004, F-014, F-031, F-040
**Priority:** MEDIUM
**Estimated scope:** Medium (4 files)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| JetStream: `MaxAge` + `MaxBytes` safety cap | NATS docs, goes framework | 30d / 10GB |
| Bash: document as secondary defense | OpenHands (no cmd filtering), SWE-agent (structured tools) | Add header comment |
| Prometheus: golden signals alerts | Google SRE Ch.6, awesome-prometheus-alerts | Add 6 rules |
| Backup encryption | CIS PostgreSQL 8.2 | GPG symmetric |

### Steps

#### Step 1: Add JetStream retention limits
**File:** `internal/adapter/nats/nats.go`
**Action:** Update `StreamConfig` at ~line 74:
```go
_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
    Name:        streamName,
    Subjects:    subjects,
    Duplicates:  2 * time.Minute,
    Retention:   jetstream.LimitsPolicy,
    Storage:     jetstream.FileStorage,
    MaxAge:      30 * 24 * time.Hour,         // 30 days
    MaxBytes:    10 * 1024 * 1024 * 1024,      // 10 GB safety cap
    Discard:     jetstream.DiscardOld,
    Compression: jetstream.S2Compression,
})
```

**Test:** `TestStreamConfigHasRetentionLimits` — assert `MaxAge > 0` and `MaxBytes > 0`.

#### Step 2: Document bash blocklist as secondary defense
**File:** `workers/codeforge/tools/bash.py`
**Action:** Add docstring at top of `blocked_patterns` list:
```python
# SECONDARY DEFENSE ONLY — trivially bypassable via shell expansion,
# flag reordering, or interpreter wrapping. The Go policy engine
# (internal/service/policy.go) is the PRIMARY security boundary.
# This blocklist catches only the most obvious destructive commands
# as a defense-in-depth measure.
```

#### Step 3: Expand Prometheus alert rules
**File:** `configs/prometheus/alerts.yml`
**Action:** Add 6 rules (from awesome-prometheus-alerts + Google SRE golden signals):
- `HighHTTPErrorRate` — 5xx rate > 5% for 5m → critical
- `HighLatencyP95` — P95 > 2s for 10m → warning
- `DiskSpaceLow` — < 10% free → critical
- `DiskFillPrediction` — `predict_linear` fills in 24h → warning
- `SSLCertExpiring` — < 20 days → warning, < 3 days → critical
- `NATSStreamStorageHigh` — stream bytes > 80% of MaxBytes → warning

#### Step 4: Add encryption to backup script
**File:** `scripts/backup-postgres.sh`
**Action:** After `pg_dump`, add optional GPG encryption:
```bash
if [ -n "${BACKUP_ENCRYPTION_KEY_FILE:-}" ]; then
    gpg --symmetric --batch --yes --passphrase-file "$BACKUP_ENCRYPTION_KEY_FILE" \
        --output "$BACKUP_DIR/$FILENAME.gpg" "$BACKUP_DIR/$FILENAME"
    rm "$BACKUP_DIR/$FILENAME"
    FILENAME="$FILENAME.gpg"
fi
```

### Acceptance Criteria
- [ ] JetStream stream has `MaxAge` and `MaxBytes` set
- [ ] Bash blocklist has defense-in-depth documentation
- [ ] Prometheus config has 9 total alert rules (3 existing + 6 new)
- [ ] Backup script supports optional GPG encryption
- [ ] Go tests pass, backup script runs without errors

---

## WT-9: `refactor/type-safety` — Type Safety Across Go + Python

**Findings:** F-019, F-021, F-022, F-037, F-038, F-039
**Priority:** MEDIUM
**Estimated scope:** Medium-Large (8+ files)
**Dependency:** After WT-5 (shares `conversation*.go` files)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| PEP 544 Protocol | LangChain, FastAPI | Replace `object` params |
| Response structs per endpoint | Huma, Coder | Replace `map[string]any` |
| `readJSON[T]()` consistency | Existing CodeForge pattern | Convert remaining raw decodes |
| Extract nested logic | CLAUDE.md "flat > nested" | Flatten context optimizer |
| Domain-aligned model split | Python packaging | Split `models.py` |

### Steps

#### Step 1: Define Python Protocol classes
**File:** `workers/codeforge/consumer/_conversation.py`
**Action:** Replace `object` type hints with Protocol classes:
```python
from typing import Protocol

class RoutingLayer(Protocol):
    def select_model(self, task: str, complexity: str) -> str: ...
    def get_scenario(self, tags: list[str]) -> str: ...

class ToolRegistry(Protocol):
    def get_tools(self, mode: str) -> list[dict]: ...
    def filter_tools(self, config: dict) -> list[dict]: ...
```
Update 5 occurrences at lines 416, 418, 472, 474, 541.

#### Step 2: Replace `map[string]any` with response structs
**Files:** `handlers_agent_features.go`, `handlers_backend_health.go`, `handlers_llm.go`, `handlers_roadmap.go`, `handlers_routing.go`, `handlers_scope.go`
**Action:** For each `writeJSON(w, http.StatusOK, map[string]any{...})`, define a typed response struct in the same file:
```go
type startBenchmarkResponse struct {
    Status string `json:"status"`
    RunID  string `json:"run_id"`
}
```
Target: eliminate 15+ `map[string]any` occurrences.

#### Step 3: Fix inconsistent JSON decode
**File:** `internal/adapter/http/handlers_agent_features.go`
**Action:** Convert raw `json.NewDecoder().Decode()` at lines 110 and 225 to use `readJSON[T]()` helper. Ensure `MaxBytesReader` wrapping is applied.

#### Step 4: Flatten context optimizer nesting
**File:** `internal/service/context_optimizer.go`
**Action:** Extract inner loop body of `fetchKnowledgeBaseEntries` (~lines 495-572) into:
```go
func (s *ContextOptimizer) processKnowledgeBase(ctx context.Context, kb KnowledgeBase, query string) []cfcontext.ContextEntry
```
Reduces nesting from 8 levels to 4-5.

#### Step 5: Extract common dispatch logic
**Files:** `internal/service/conversation.go`, `conversation_agent.go`
**Action:** Extract shared dispatch steps (~40 lines) from `SendMessage` and `dispatchAgenticRun` into:
```go
func (s *ConversationService) buildAndPublishRun(ctx context.Context, conv *Conversation, opts runOptions) error
```

#### Step 6: Split Python `models.py` (optional, if time permits)
**File:** `workers/codeforge/models.py` → `workers/codeforge/models/`
**Action:** Split 52 classes into domain modules:
- `models/run.py` — run/task payloads
- `models/conversation.py` — conversation payloads
- `models/benchmark.py` — benchmark payloads
- `models/retrieval.py` — retrieval/context payloads
- `models/__init__.py` — re-export all for backward compatibility

### Acceptance Criteria
- [ ] Zero `object` type hints in Python consumer
- [ ] Zero `map[string]any` in Go handler response writes
- [ ] All JSON decoding uses `readJSON[T]()` or has `MaxBytesReader`
- [ ] `fetchKnowledgeBaseEntries` nesting depth <= 5
- [ ] Common dispatch logic extracted (DRY)
- [ ] All tests pass across Go and Python
- [ ] mypy/pyright passes on Python changes

---

## WT-10: `refactor/god-objects` — Interface Segregation + Service Decomposition

**Findings:** F-006, F-017
**Priority:** BACKLOG (Large refactor)
**Estimated scope:** Large (20+ files)
**Dependency:** After WT-2 (shares `runtime*.go` files)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| Consumer-defined narrow interfaces | Dave Cheney, Redowan's Reflections, HackerNoon | Services accept sub-interfaces |
| 5-15 role-based groups of 2-5 methods | ISP literature consensus | Already have 34 sub-interfaces |
| Domain-based sub-services | Gitea, Grafana | Split Runtime/Conversation |
| CQRS method objects | Three Dots Labs / Wild Workouts | For complex operations |
| Manual DI → Wire when complex | Go Wire docs | Stay manual for now |

### Steps

#### Step 1: Services accept sub-interfaces instead of `Store`
**Action:** For each service in `internal/service/`, change constructor to accept only the sub-interfaces it needs:
```go
// Before
func NewProjectService(store database.Store, ...) *ProjectService

// After
func NewProjectService(store interface {
    database.ProjectStore
    database.RunStore
}, ...) *ProjectService
```
Or define a consumer-side interface in the service file:
```go
type projectStore interface {
    database.ProjectStore
    database.RunStore
}
```

**Scope:** Start with 5 smallest services, validate pattern, then expand.

#### Step 2: Decompose RuntimeService
**Action:** Extract into focused sub-services:
- `RuntimeLifecycleService` — start/stop/restart runs
- `RuntimeApprovalService` — HITL approve/deny
- `RuntimeSubscriberService` — NATS event handlers
- `RuntimeService` becomes a facade delegating to sub-services

#### Step 3: Decompose ConversationService
**Action:** Continue existing pattern:
- `ConversationMessageService` (already exists)
- `PromptAssemblyService` (already exists)
- Extract `ConversationDispatchService` — the send/dispatch logic
- `ConversationService` becomes a facade

#### Step 4: Update composition root
**File:** `cmd/codeforge/main.go`
**Action:** Wire sub-services through constructors. The composite `Store` is only used at this level.

### Acceptance Criteria
- [ ] No service constructor accepts `database.Store` directly (accepts sub-interfaces)
- [ ] RuntimeService split into 3+ focused sub-services
- [ ] ConversationService split into 3+ focused sub-services
- [ ] Composition root wires everything
- [ ] All tests pass
- [ ] No circular imports

---

## WT-11: `fix/traefik-security` — Apply Defined Security Middlewares

**Finding:** F-011
**Priority:** HIGH
**Estimated scope:** Small (2 files)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| Cross-provider `@file` middleware reference | Traefik docs, SimpleHomelab guide | Reference YAML-defined middlewares from compose labels |
| Middleware chain composition | Traefik dynamic config | Chain `rate-limit` + `security-headers` |
| Per-service CSP | Paul's Blog, OWASP | Strict CSP for API, permissive for frontend |

### Steps

#### Step 1: Verify file provider is enabled
**File:** `traefik/traefik.yaml`
**Action:** Ensure `providers.file.directory` or `providers.file.filename` points to `traefik/dynamic/`. This is required for `@file` references.

#### Step 2: Apply middlewares to blue-green routers
**File:** `docker-compose.blue-green.yml`
**Action:** Add middleware labels to all 4 routers:
```yaml
- "traefik.http.routers.core-blue.middlewares=rate-limit@file,security-headers@file"
- "traefik.http.routers.core-green.middlewares=rate-limit@file,security-headers@file"
- "traefik.http.routers.frontend-blue.middlewares=security-headers@file"
- "traefik.http.routers.frontend-green.middlewares=security-headers@file"
```
Note: Rate-limit on API routers only (frontend is static assets).

#### Step 3: Fix router entrypoints (bonus — from filtered F-008)
**File:** `docker-compose.blue-green.yml`
**Action:** Change all routers from `entrypoints=web` to `entrypoints=websecure` and add TLS:
```yaml
- "traefik.http.routers.core-blue.entrypoints=websecure"
- "traefik.http.routers.core-blue.tls.certresolver=letsencrypt"
```

### Acceptance Criteria
- [ ] All blue-green routers reference `security-headers@file`
- [ ] API routers reference `rate-limit@file`
- [ ] Security headers (HSTS, X-Frame-Options, CSP) are served in HTTP responses
- [ ] Rate limiting is active on API endpoints

---

## WT-12: `fix/websocket-ticket-exchange` — Replace URL Token with Single-Use Ticket

**Finding:** F-032
**Priority:** LOW (accepted risk, but proper fix documented)
**Estimated scope:** Medium (3 files)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| Single-use OTP ticket | programmingpercy.tech, OroCommerce | `sync.Map` with TTL |
| Ticket exchange endpoint | OWASP WS Security Cheat Sheet | `POST /api/v1/ws/ticket` |
| Delete-on-first-use | Matrix spec (deprecating query token) | Immediate delete after verify |

### Steps

#### Step 1: Add ticket store
**File:** `internal/adapter/ws/ticket.go` (new)
**Action:** Implement `TicketStore` using `sync.Map`:
```go
type Ticket struct {
    UserID    string
    TenantID  string
    CreatedAt time.Time
}

type TicketStore struct {
    tickets sync.Map
    ttl     time.Duration // 30 seconds
}

func (ts *TicketStore) Issue(userID, tenantID string) string // returns UUID
func (ts *TicketStore) Redeem(ticket string) (*Ticket, bool) // single-use: deletes on read
func (ts *TicketStore) Cleanup(ctx context.Context)          // background goroutine
```

#### Step 2: Add ticket endpoint
**File:** `internal/adapter/http/handlers.go` (or `handlers_ws.go`)
**Action:** Add `POST /api/v1/ws/ticket` (authenticated) that issues a ticket:
```go
func (h *Handlers) IssueWSTicket(w http.ResponseWriter, r *http.Request) {
    user := auth.UserFromCtx(r.Context())
    ticket := h.TicketStore.Issue(user.ID, user.TenantID)
    writeJSON(w, http.StatusOK, wsTicketResponse{Ticket: ticket, ExpiresIn: 30})
}
```

#### Step 3: Update WebSocket upgrade handler
**File:** `internal/adapter/ws/handler.go`
**Action:** Accept both `?ticket=` (new) and `?token=` (legacy, log deprecation warning). For ticket auth, call `TicketStore.Redeem()` instead of JWT validation.

### Acceptance Criteria
- [ ] `POST /api/v1/ws/ticket` returns single-use ticket (30s TTL)
- [ ] WebSocket upgrade accepts `?ticket=` parameter
- [ ] Ticket is deleted after first use (not replayable)
- [ ] Background cleanup removes expired tickets
- [ ] Legacy `?token=` still works (deprecation warning logged)
- [ ] No long-lived tokens in URL query strings for new connections

---

## WT-13: `fix/cors-validation` — Validate Origin Header Before Reflecting

**Finding:** F-033
**Priority:** LOW
**Estimated scope:** Small (1 file)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| `go-chi/cors` with explicit `AllowedOrigins` | rs/cors, go-chi/cors | Replace custom CORS |
| `url.Parse` + exact host match | PentesterLab, jub0bs/fcors | Avoid `strings.Contains` |
| Multi-origin via `AllowOriginFunc` | rs/cors docs | For multi-tenant |

### Steps

#### Step 1: Replace custom CORS middleware with `go-chi/cors`
**File:** `internal/adapter/http/middleware.go`
**Action:** Replace the custom `CORS()` function (~lines 44-71) with `go-chi/cors`:
```go
import "github.com/go-chi/cors"

func CORSMiddleware(allowedOrigin, appEnv string) func(http.Handler) http.Handler {
    origins := []string{allowedOrigin}
    if appEnv == "development" {
        origins = append(origins, "http://localhost:3000", "http://localhost:5173")
    }
    return cors.Handler(cors.Options{
        AllowedOrigins:   origins,
        AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
        AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
        AllowCredentials: true,
        MaxAge:           300,
    })
}
```

`go-chi/cors` automatically validates the incoming `Origin` header against the configured list and only reflects when matched.

#### Step 2: Add dependency
**Action:** `go get github.com/go-chi/cors`

**Test:** `TestCORSRejectsUnknownOrigin` — request with `Origin: https://evil.com` should NOT receive `Access-Control-Allow-Origin` header.

### Acceptance Criteria
- [ ] CORS middleware validates incoming Origin against configured list
- [ ] Unknown origins do not receive `Access-Control-Allow-Origin` header
- [ ] `AllowCredentials` only with explicit origins (never with `*`)
- [ ] Development mode allows localhost origins
- [ ] Test verifies rejection of unknown origins

---

## WT-14: `fix/go-deps-update` — Update Vulnerable Dependencies + CI Scanner

**Finding:** F-034
**Priority:** LOW
**Estimated scope:** Small (2 files)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| `govulncheck` in CI (SARIF → Code Scanning) | Go security team, jvt.me blog | GitHub Action |
| Symbol-level reachability | govulncheck design | Only flags actually-called code |
| Weekly scheduled scan | GitHub Actions cron | Catch newly disclosed CVEs |

### Steps

#### Step 1: Update dependencies
**Action:**
```bash
go get golang.org/x/crypto@latest
go get github.com/jackc/pgx/v5@latest
go mod tidy
```

#### Step 2: Run full test suite
**Action:** `go test ./... -race -count=1` — verify no regressions from pgx/crypto updates.

#### Step 3: Add govulncheck to CI
**File:** `.github/workflows/ci.yml`
**Action:** Add govulncheck job:
```yaml
govulncheck:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
      with:
        go-version-file: go.mod
    - run: go install golang.org/x/vuln/cmd/govulncheck@latest
    - run: govulncheck ./...
```

### Acceptance Criteria
- [ ] `golang.org/x/crypto` at latest version
- [ ] `pgx/v5` at latest version
- [ ] All tests pass with `-race`
- [ ] `govulncheck ./...` reports zero reachable vulnerabilities
- [ ] CI runs govulncheck on every push

---

## WT-15: `fix/audit-log-coverage` — Extend Audit Middleware to Write Endpoints

**Finding:** F-030
**Priority:** MEDIUM
**Estimated scope:** Medium (1 file + tests)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| Hybrid: global API audit + per-handler diff audit | Coder `coderd/audit` | Global middleware for coverage |
| Opt-out via context | kafeiih/go-audit | `WithSkipAudit(ctx)` for health/metrics |
| Systematic coverage check | Static analysis grep | Cross-reference routes vs audit calls |

### Steps

#### Step 1: Identify unaudited write endpoints
**Action:** Cross-reference `routes.go` POST/PUT/DELETE registrations against `audit()` wrapper usage. Currently only 12 of ~100+ write endpoints are audited.

#### Step 2: Add audit() to high-priority write endpoints
**File:** `internal/adapter/http/routes.go`
**Action:** Wrap these endpoint groups with `audit()`:
- Project CRUD: `POST /projects`, `PUT /projects/{id}`, `DELETE /projects/{id}`
- LLM key CRUD: `POST /llm-keys`, `DELETE /llm-keys/{id}`
- MCP server CRUD: `POST /mcp/servers`, `DELETE /mcp/servers/{id}`
- File operations: `POST /projects/{id}/files`, `DELETE /projects/{id}/files`
- Settings: `PUT /settings`
- Conversation deletion: `DELETE /conversations/{id}`

#### Step 3: Add audit coverage test
**File:** `internal/adapter/http/routes_test.go`
**Action:** Add `TestAllWriteEndpointsAudited` — walk chi route tree, collect all POST/PUT/PATCH/DELETE routes, assert each has `audit` middleware in its chain (or is explicitly excluded with a documented reason).

### Acceptance Criteria
- [ ] All project/LLM-key/MCP/file/settings write endpoints have `audit()` middleware
- [ ] Test verifies write endpoint audit coverage
- [ ] Explicitly excluded endpoints are documented (health, metrics, WS)
- [ ] Audit log entries include action type, user, tenant, resource ID

---

## WT-16: `fix/consent-privacy` — Privacy Policy Link + Consent Infrastructure

**Finding:** F-028
**Priority:** MEDIUM
**Estimated scope:** Small (3 files)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| Footer + login page link | TermsFeed, PrivacyPolicies.com, CNIL | Mandatory placement |
| Session-only cookies → no banner needed | Mastodon, Nextcloud | CodeForge uses session cookies only |
| Onboarding wizard consent step | Existing `OnboardingWizard.tsx` | Add privacy acceptance |

### Steps

#### Step 1: Add privacy policy link to login page
**File:** `frontend/src/features/auth/LoginPage.tsx`
**Action:** Add link near the login button:
```tsx
<p class="text-xs text-cf-text-muted mt-4">
  By signing in, you agree to our{" "}
  <a href="/privacy" class="text-cf-accent hover:underline">Privacy Policy</a>.
</p>
```

#### Step 2: Create privacy policy placeholder page
**File:** `frontend/src/features/legal/PrivacyPolicy.tsx` (new)
**Action:** Create a placeholder page with configurable content (deployments customize):
```tsx
export function PrivacyPolicy() {
  return (
    <main class="max-w-3xl mx-auto p-8">
      <h1>Privacy Policy</h1>
      <p>This CodeForge instance processes personal data as described below...</p>
      {/* Sections: data collected, legal basis, retention, rights, contact */}
    </main>
  );
}
```
Add route in `App.tsx`: `<Route path="/privacy" component={PrivacyPolicy} />`

#### Step 3: Add privacy link to sidebar footer
**File:** `frontend/src/ui/layout/Sidebar.tsx`
**Action:** Add alongside the AGPL source link (from WT-7):
```tsx
<a href="/privacy" class="text-xs text-cf-text-muted hover:text-cf-text-secondary">Privacy</a>
```

### Acceptance Criteria
- [ ] Privacy policy link visible on login page
- [ ] `/privacy` route renders placeholder page
- [ ] Privacy link in sidebar footer
- [ ] Page content is customizable for deployments
- [ ] No cookie consent banner needed (session cookies only)

---

## WT-17: `docs/breach-notification` — Data Breach Notification Procedure

**Finding:** F-029
**Priority:** MEDIUM
**Estimated scope:** Small (1 new doc file)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| 6-step procedure (Detect → Assess → Notify SA → Notify subjects → Remediate → Document) | ICO, GDPR Art. 33/34 | Standard template |
| 72-hour SA notification | GDPR Art. 33 | Document requirement |
| Phased reporting | Art. 33(4) | Submit initial, update later |
| Breach log table | Art. 33(5) | PostgreSQL table |

### Steps

#### Step 1: Create breach notification procedure document
**File:** `docs/security/breach-notification-procedure.md` (new)
**Content:**
1. **Scope** — applies to all personal data breaches in CodeForge deployments
2. **Detection signals** — mass failed logins, unauthorized data access patterns, credential stuffing, data exfiltration (large exports)
3. **Response timeline** — Contain (0-4h) → Assess (4-24h) → Notify SA (by 72h) → Notify subjects (if high risk)
4. **Notification template** — SA notification: nature of breach, categories of data, approximate number of subjects, DPO contact, likely consequences, measures taken
5. **Documentation requirements** — ALL breaches documented per Art. 33(5), regardless of notification threshold
6. **Post-incident** — root cause analysis, system hardening, policy update, lessons learned

#### Step 2: Add breach detection alerts to Prometheus (cross-reference WT-8)
**File:** `configs/prometheus/alerts.yml`
**Action:** Add rule:
```yaml
- alert: MassFailedLogins
  expr: sum(rate(auth_login_failures_total[5m])) > 10
  for: 2m
  labels:
    severity: warning
  annotations:
    description: "More than 10 failed logins per second — possible brute force"
```

### Acceptance Criteria
- [ ] `docs/security/breach-notification-procedure.md` exists with 6 sections
- [ ] Procedure covers GDPR Art. 33 (72-hour SA notification) and Art. 34 (subject notification)
- [ ] Breach detection signal documented
- [ ] Documentation requirements per Art. 33(5) specified

---

## WT-18: `docs/changelog-adrs` — Retroactive CHANGELOG + Missing ADRs

**Findings:** F-042, F-043
**Priority:** INFO
**Estimated scope:** Medium (docs only)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| git-cliff for retroactive CHANGELOG | git-cliff.org | Generate from tags |
| Keep a Changelog format | keepachangelog.com | Added/Changed/Fixed/Security |
| MADR template with Decision Drivers | adr.github.io | Extend existing template |
| ADR per major decision | AWS, Spotify | 7 missing ADRs |

### Steps

#### Step 1: Generate retroactive CHANGELOG
**Action:**
```bash
# Install git-cliff
cargo install git-cliff
# Create cliff.toml with custom grouping
# Generate from all tags
git-cliff --output CHANGELOG.md
```
Manually refine: group by version, use Keep a Changelog categories (Added/Changed/Fixed/Security), remove noise commits.

#### Step 2: Create 3 priority ADRs
**Files:** New files in `docs/architecture/adr/`
- `010-a2a-protocol-adoption.md` — Why A2A v0.3.0, not custom federation
- `011-trust-quarantine-system.md` — Why 4-level trust + message quarantine
- `012-hybrid-routing-cascade.md` — Why ComplexityAnalyzer → MAB → LLMMetaRouter → defaults

Each follows existing template: Context → Decision → Consequences → Alternatives.

### Acceptance Criteria
- [ ] CHANGELOG.md covers versions 0.1.0 through 0.8.0
- [ ] Keep a Changelog format with Added/Changed/Fixed/Security categories
- [ ] ADRs 010, 011, 012 created with full Context/Decision/Consequences
- [ ] ADRs reference existing implementation files

---

## WT-19: `docs/openapi-expansion` — Expand OpenAPI Spec Coverage

**Finding:** F-044
**Priority:** INFO
**Estimated scope:** Medium-Large (1 file + tooling)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| swaggo/swag annotation-based | Gitea, Coder | Incremental annotation |
| chi `docgen` route inventory | go-chi/docgen | Auto-discover endpoints |
| Spec-first with oapi-codegen | NIST, API standards | For new endpoints |

### Steps

#### Step 1: Generate route inventory with chi docgen
**Action:** Add a dev-only endpoint or script that walks the chi router tree and outputs all registered routes with methods:
```go
docgen.PrintRoutes(r) // outputs all routes
```
Cross-reference against `docs/api/openapi.yaml` to identify gaps.

#### Step 2: Add swaggo/swag annotations to priority endpoints
**Files:** Handler files in `internal/adapter/http/`
**Action:** Annotate the 20 most-used endpoints first:
- Auth (login, register, refresh, logout)
- Projects CRUD
- Conversations CRUD
- Runs (start, approve, cancel)
- Settings

Example annotation:
```go
// @Summary Create a new project
// @Tags projects
// @Accept json
// @Produce json
// @Param body body CreateProjectRequest true "Project details"
// @Success 201 {object} Project
// @Failure 400 {object} APIError
// @Router /api/v1/projects [post]
```

#### Step 3: Add `swag init` to Makefile / CI
**Action:** `swag init -g cmd/codeforge/main.go -o docs/api/ --outputTypes yaml`

### Acceptance Criteria
- [ ] Route inventory generated (263 endpoints documented)
- [ ] Top 20 endpoints have swag annotations
- [ ] `swag init` generates valid OpenAPI spec
- [ ] CI validates spec is up-to-date

---

## WT-20: `refactor/frontend-decomposition` — ProjectDetailPage Decomposition

**Finding:** F-045
**Priority:** INFO
**Estimated scope:** Small (2-3 files)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| Panel registry pattern | Grafana, VS Code | Self-registering panels |
| `lazy()` + `<Suspense>` | SolidJS docs | Per-panel code splitting |
| Feature directory per panel | developerway.com | Explicit public API |

### Steps

#### Step 1: Extract PanelSelector component
**File:** `frontend/src/features/project/ProjectDetailPage.tsx`
**Action:** Extract `PanelSelector` (lines ~80-190) into `frontend/src/features/project/PanelSelector.tsx`. Export as named export.

#### Step 2: Lazy-load heavy panels
**File:** `frontend/src/features/project/ProjectDetailPage.tsx`
**Action:** Wrap infrequently-used panels with `lazy()`:
```tsx
const TrajectoryPanel = lazy(() => import('./TrajectoryPanel'));
const BenchmarkPanel = lazy(() => import('../benchmark/BenchmarkPanel'));
const CanvasPanel = lazy(() => import('../canvas/CanvasPanel'));
```
Wrap renders in `<Suspense fallback={<PanelFallback />}>`.

### Acceptance Criteria
- [ ] `PanelSelector` is a separate file
- [ ] At least 5 heavy panels are lazy-loaded
- [ ] `ProjectDetailPage.tsx` is < 600 LOC
- [ ] No visual regressions (panels render correctly)

---

## WT-21: `refactor/pydantic-validators` — Deduplicate Pydantic Validators

**Finding:** F-046
**Priority:** INFO
**Estimated scope:** Small (2 files)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| `Annotated` + `AfterValidator` | Pydantic maintainer (Viicos), Discussion #8502 | Reusable type aliases |
| `__get_pydantic_core_schema__` | Pydantic Custom Types docs | For parameterized validators |
| Duplication acceptable for cross-field validators | Pydantic best practices | Keep `@model_validator` in-place |

### Steps

#### Step 1: Create reusable validator types
**File:** `workers/codeforge/validators.py` (new)
```python
from typing import Annotated
from pydantic import AfterValidator

def _coerce_none_to_list(v: list | None) -> list:
    return v if v is not None else []

def _clamp_top_k(v: int) -> int:
    return max(1, min(v, 500))

CoercedList = Annotated[list, AfterValidator(_coerce_none_to_list)]
ClampedTopK = Annotated[int, AfterValidator(_clamp_top_k)]
```

#### Step 2: Replace duplicated validators in models.py
**File:** `workers/codeforge/models.py`
**Action:** Replace duplicated `_coerce_list_fields` (lines 126, 493) and `_clamp_top_k` (lines 252, 299) with `CoercedList` and `ClampedTopK` type aliases from `validators.py`.

### Acceptance Criteria
- [ ] Zero duplicated validator logic across Pydantic models
- [ ] `validators.py` contains reusable `Annotated` type aliases
- [ ] All existing tests pass
- [ ] mypy/pyright passes

---

## WT-22: `docs/config-import-decision` — Document Config Import as Accepted Pattern

**Finding:** F-035
**Priority:** LOW (accepted, document rationale)
**Estimated scope:** Tiny (1 file)

### Research Summary

| Pattern | Source | Adoption |
|---|---|---|
| Config sub-structs as value types are acceptable | Three Dots Labs, Grafana, Gitea | Document as ADR/decision |
| Real violation: calling `config.Load()` in service | Hexagonal purists | CodeForge does NOT do this |
| Pragmatic trade-off documentation | AWS ADR best practices | ADR note |

### Steps

#### Step 1: Add ADR note
**File:** `docs/architecture/adr/013-config-import-in-services.md` (new)
**Content:**
- **Context:** 19 service files import `internal/config` for sub-struct types
- **Decision:** Accepted. Config sub-structs are value types (no I/O, no side effects). Services receive sub-structs via constructors, never call `config.Load()` directly
- **Consequence:** Pragmatic trade-off. Moving sub-structs to a separate package would add indirection without real benefit. The invariant to maintain: services MUST NOT import config loader functions, only type definitions

### Acceptance Criteria
- [ ] ADR documents the decision and rationale
- [ ] Invariant "no config.Load() in services" is explicit

---

## Reference Sources

### Config & Secrets
- HashiCorp Vault `Config.Sanitized()` pattern
- Mattermost `Sanitize()` with `FakeSetting` constant + test suite
- Coder `json:"-"` + `serpent.Annotations` dual approach
- Consul `json:"-"` on ACL blocks
- Traefik `loggable:"false"` struct tag + reflection redactor
- go-password-validator entropy-based validation (50-70 bits)

### NATS JetStream
- NATS official docs: `MaxAge`, `MaxBytes`, `MaxMsgs` semantics
- goes event-sourcing framework: stream retention patterns
- NATS Admin docs: server/account-level resource limits
- Consumer best practices: `MaxDeliver` with exponential backoff

### GDPR & Compliance
- CJEU C-582/14 Breyer: dynamic IPs are personal data
- EDPB Guidelines 1/2024 on Legitimate Interest
- EDPB March 2025 Coordinated Enforcement on Right to Erasure
- CNIL Developer Guide + traceability data recommendation (6-12 months)
- ADR-009 (own project): audit log anonymization design
- Homi.so: cascade deletion pitfalls with soft delete
- ICO breach notification guide + self-assessment tool
- GDPR Art. 33/34: 72-hour SA notification, phased reporting
- Nextcloud, Mastodon: cookie/consent approaches

### Architecture
- Three Dots Labs: Wild Workouts hexagonal Go reference
- Dave Cheney: SOLID Go Design (interface segregation)
- Gitea: `routers → services → models → modules` dependency direction
- Grafana: Wire-based service registration
- Calhoun.io: Service Object pattern

### Testing
- golang-jwt: table-driven JWT parsing tests
- lestrrat-go/jwx: concurrent signing with `sync.WaitGroup`
- ORY Fosite: refresh token rotation + reuse detection
- Gitea: `timeutil.MockSet` for deterministic expiry
- Vault: hierarchical token lifecycle tests

### Frontend & Compliance
- Mastodon: `source_url` API field for AGPL Section 13
- Nextcloud: footer AGPL notice
- WAI-ARIA APG: Checkbox pattern with `aria-checked` states
- Adrian Roselli: "Select All" checkbox placement in tables
- Liferay: `no-global-fetch` ESLint rule
- awesome-prometheus-alerts: 130+ community alert rules
- Google SRE Book Ch. 6: golden signals monitoring
- SolidJS `lazy()` + `<Suspense>` for component decomposition
- developerway.com: React/SolidJS feature directory pattern

### Security
- OpenHands: container-first sandboxing, no command filtering
- SWE-agent ACI: structured tools > raw shell
- Bubblewrap: namespace-based sandbox primitive
- programmingpercy.tech: WebSocket OTP ticket exchange in Go
- OWASP WebSocket Security Cheat Sheet
- rs/cors, go-chi/cors, jub0bs/fcors: CORS validation patterns
- PentesterLab: Go CORS vulnerability patterns

### Dependencies & OpenAPI
- govulncheck: symbol-level reachability analysis for Go
- Renovate vs Dependabot: Go/Python/TS monorepo support
- swaggo/swag: annotation-based OpenAPI from Go handlers
- go-chi/docgen: chi route tree introspection
- Gitea, Coder: OpenAPI maintenance patterns

### Documentation
- git-cliff: retroactive CHANGELOG generation from tags
- Keep a Changelog format (keepachangelog.com)
- AWS, Spotify, ThoughtWorks: ADR best practices
- MADR template with Decision Drivers
- Log4brains: static site generator for ADRs
