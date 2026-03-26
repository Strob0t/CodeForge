# Worktree 1: fix/tenant-isolation — Tenant-Lücken schliessen

**Branch:** `fix/tenant-isolation`
**Priority:** SOFORT
**Scope:** 4 findings (F-COM-003, F-SEC-009, F-COM-008, F-ARC-009)
**Estimated effort:** Small (1-2 days)

## Research Summary

- PostgreSQL RLS as defense-in-depth layer (Crunchy Data, AWS Prescriptive Guidance)
- Application-level `tenantFromCtx(ctx)` pattern already established in 189 files
- NATS tenant propagation via payload `tenant_id` field (existing pattern in `ConversationRunStartPayload`)
- Atlas GopherCon 2025: centralize tenant filtering, every background process must carry context

## Steps

### 1. F-COM-003: Password Reset Tokens — Add tenant_id to all 4 CRUD operations

**File:** `internal/adapter/postgres/store_user.go:91-127`

- `CreatePasswordResetToken`: Add `tenant_id` column to INSERT, get from `tenantFromCtx(ctx)`
- `GetPasswordResetTokenByHash`: Add `AND tenant_id = $N` to WHERE clause
- `MarkPasswordResetTokenUsed`: Add `AND tenant_id = $N` to WHERE clause
- `DeleteExpiredPasswordResetTokens`: Add `AND tenant_id = $N` to WHERE clause

**Test:** Add cross-tenant isolation test in `store_user_test.go`:
```go
// Create token for tenant A, attempt read from tenant B — must fail
```

### 2. F-SEC-009: A2A CreateA2APushConfig — Add tenant ownership check

**File:** `internal/adapter/postgres/store_a2a.go:249-260`

- Add subquery check: `WHERE EXISTS (SELECT 1 FROM a2a_tasks WHERE id = $1 AND tenant_id = $5)`
- Or validate ownership in service layer before calling store

**Test:** Cross-tenant push config test — must fail when task belongs to different tenant.

### 3. F-COM-008: CreateToolMessages — Add explicit tenant check

**File:** `internal/adapter/postgres/store_conversation.go:197-235`

- Use same INSERT...FROM subquery pattern as `CreateMessage` (line 80-84)
- Include `WHERE tenant_id = $N` clause on the conversations table join

**Test:** Verify tool message batch insert fails for conversation owned by different tenant.

### 4. F-ARC-009: RunCompletePayload — Add tenant_id field

**Go file:** `internal/port/messagequeue/schemas.go:456` (`ConversationRunCompletePayload`)
- Add `TenantID string json:"tenant_id,omitempty"`

**Python file:** `workers/codeforge/models.py:503` (`ConversationRunCompleteMessage`)
- Add `tenant_id: str = ""`

**Publisher:** `workers/codeforge/consumer/_conversation.py:364` (`_publish_run_complete`)
- Populate `tenant_id` from the run message

**Consumer:** Go handler must use `tenantctx.WithTenant(ctx, payload.TenantID)` when processing

### 5. (Optional) Defense-in-depth: RLS migration for password_reset_tokens

**Migration file:** `migrations/XXX_rls_password_reset_tokens.sql`

```sql
ALTER TABLE password_reset_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE password_reset_tokens FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_select ON password_reset_tokens
  FOR SELECT USING (tenant_id = current_setting('app.current_tenant_id')::uuid);
-- + INSERT, UPDATE, DELETE policies
-- + service_bypass policy for migrations
```

Requires setting `SET app.current_tenant_id` in Go DB connection checkout.

## Verification

- All existing tests pass
- New cross-tenant isolation tests pass for all 4 gaps
- NATS round-trip test: Go struct -> JSON -> Python model -> JSON -> Go struct (with tenant_id)
- `grep -r "password_reset_tokens" internal/adapter/postgres/` — all queries include tenant_id

## Sources

- [Crunchy Data: RLS for Tenants](https://www.crunchydata.com/blog/row-level-security-for-tenants-in-postgres)
- [AWS: Multi-Tenant Data Isolation with RLS](https://aws.amazon.com/blogs/database/multi-tenant-data-isolation-with-postgresql-row-level-security/)
- [Atlas: Scalable Multi-Tenant Apps in Go (GopherCon 2025)](https://atlasgo.io/blog/2025/05/26/gophercon-scalable-multi-tenant-apps-in-go)
- [Rost Glukhov: Multi-Tenancy DB Patterns in Go](https://medium.com/@rosgluk/multi-tenancy-database-patterns-with-examples-in-go-ade087d642c8)
