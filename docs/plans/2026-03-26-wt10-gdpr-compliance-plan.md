# Worktree 10: fix/gdpr-compliance — GDPR + AGPL Lücken

**Branch:** `fix/gdpr-compliance`
**Priority:** Hoch
**Scope:** 6 findings (F-COM-004, F-COM-005, F-COM-006, F-COM-010, F-COM-009, F-COM-012)
**Estimated effort:** Medium (1 week)

## Research Summary

- GDPR Art. 15/20: comprehensive data export in machine-readable format
- GDPR Art. 5(1)(e): storage limitation requires automated enforcement
- EDPB Opinion 28/2024: legitimate interest valid for AI processing (with documented LIA)
- AGPL-3.0 s13: source notice mandatory for network-deployed software
- Klaro/CookieConsent for cookie consent; custom dialog for LLM processing consent

## Steps

### 1. F-COM-004: Complete GDPR Data Export

**File:** `internal/service/gdpr.go:31-56`

Expand `UserDataExport`:
```go
type UserDataExport struct {
    ExportedAt    time.Time              `json:"exported_at"`
    FormatVersion string                 `json:"format_version"`
    User          *user.User             `json:"user"`
    APIKeys       []user.APIKey          `json:"api_keys"`
    LLMKeys       []llmkey.LLMKey        `json:"llm_keys"`
    Sessions      []session.Session      `json:"sessions"`
    Conversations []ConversationExport   `json:"conversations"`
    CostRecords   []cost.Record          `json:"cost_records"`
    Channels      []channel.Membership   `json:"channels"`
    AuditTrail    []AuditEntry           `json:"audit_trail"`
}
```

Query conversations through project ownership (projects owned by user → conversations).
Redact `password_hash` before export (already `json:"-"`).

### 2. F-COM-005: Data Retention Enforcement

**New file:** `internal/service/retention.go`

```go
type RetentionConfig struct {
    AgentEvents   time.Duration // default: 90 days
    Sessions      time.Duration // default: 30 days
    Conversations time.Duration // default: 365 days
    CostRecords   time.Duration // default: 365 days
}

func (s *RetentionService) RunCleanup(ctx context.Context) error {
    // Batched DELETE ... WHERE created_at < NOW() - INTERVAL ...
    // LIMIT 1000 per batch to avoid long locks
    // Log row counts to audit table
}
```

Add `retention` section to `codeforge.yaml` config.
Schedule via goroutine with `time.Ticker` (daily at 02:00).

For high-volume tables (agent_events), consider pg_partman + monthly partitions.

### 3. F-COM-006: Consent Mechanism

**Frontend:** Add LLM Data Processing Consent dialog during onboarding.

**Migration:** `migrations/XXX_create_user_consents.sql`
```sql
CREATE TABLE user_consents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    consent_type TEXT NOT NULL,  -- 'llm_processing', 'analytics'
    provider TEXT,               -- 'openai', 'anthropic', null
    granted BOOLEAN NOT NULL,
    policy_version TEXT NOT NULL,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);
```

**Legal basis:** Contractual necessity (Art. 6(1)(b)) as primary basis for core LLM features. Consent (Art. 6(1)(a)) for non-essential processing (analytics). Document Legitimate Interest Assessment for model routing.

### 4. F-COM-010: Audit GDPR Export Events

**File:** `internal/adapter/http/routes.go:527`

Add audit middleware to export endpoint:
```go
r.With(audit("export", "user_data")).Post("/{id}/export", h.ExportUserData)
```

Log before deletion (data won't exist after).

### 5. F-COM-009: AGPL Source Notice in Frontend

Add footer link visible on every page:
```tsx
<footer>
  <span>CodeForge v{__APP_VERSION__}</span>
  <a href="/license">AGPL-3.0</a>
  <a href="https://github.com/Strob0t/CodeForge">Source Code</a>
</footer>
```

Add `/license` route serving full AGPL text.
Add `/api/v1/source` endpoint returning repo URL + Git SHA.

### 6. F-COM-012: Generate SBOM

Add to CI (`ci.yml`):
- `pip-licenses --format=json` for Python
- `go-licenses` for Go
- `license-checker --json` for npm
- Generate `THIRD_PARTY_LICENSES` file

## Verification

- Data export includes all personal data categories listed in privacy policy
- Retention cleanup deletes old records and logs row counts
- Consent dialog appears during onboarding for new users
- Audit log records export events
- Source link visible in footer
- SBOM generated in CI

## Sources

- [GDPR Art. 20: Right to Data Portability](https://gdpr-info.eu/art-20-gdpr/)
- [CNIL GDPR Developer Guide](https://lincnil.github.io/GDPR-Developer-Guide/)
- [EDPB Opinion 28/2024 on AI Models](https://www.edpb.europa.eu/news/news/2024/edpb-opinion-ai-models-gdpr-principles-support-responsible-ai_en)
- [Crunchy Data: pg_partman Guide](https://www.crunchydata.com/blog/auto-archiving-and-data-retention-management-in-postgres-with-pg_partman)
- [FSF: Fundamentals of AGPLv3](https://www.fsf.org/bulletin/2021/fall/the-fundamentals-of-the-agplv3)
- [Klaro Privacy Manager](https://klaro.org/)
