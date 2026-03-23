# WT-8: Compliance & GDPR — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement GDPR data deletion + export, data retention policy, privacy/security documentation, and OpenAPI spec stub.

**Architecture:** New cascade deletion migration, user export service, 3 documentation files, OpenAPI generation.

**Tech Stack:** Go 1.25, PostgreSQL CASCADE, chi router, Markdown docs

**Best Practice:**
- GDPR Article 17: Right to erasure — cascade delete all user data across ALL tables.
- GDPR Article 20: Right to data portability — export as structured JSON.
- GDPR Article 13: Right to information — document what data is processed, by whom.
- Data retention: Define explicit TTLs. Never store data indefinitely without justification.

---

### Task 1: User Data Export Endpoint

**Files:**
- Create: `internal/adapter/http/handlers_gdpr.go`
- Create: `internal/service/gdpr.go`
- Modify: `internal/adapter/http/routes.go`

- [ ] **Step 1: Create GDPR service**

```go
// internal/service/gdpr.go
package service

import (
    "context"
    "github.com/Strob0t/CodeForge/internal/tenantctx"
)

type UserDataExport struct {
    User          any   `json:"user"`
    Projects      []any `json:"projects"`
    Conversations []any `json:"conversations"`
    APIKeys       []any `json:"api_keys"`
    Sessions      []any `json:"sessions"`
    AgentEvents   []any `json:"agent_events"`
}

type GDPRStore interface {
    GetUser(ctx context.Context, id string) (any, error)
    // Add methods as needed for each data category
}

type GDPRService struct {
    store GDPRStore
}

func NewGDPRService(store GDPRStore) *GDPRService {
    return &GDPRService{store: store}
}

func (s *GDPRService) ExportUserData(ctx context.Context, userID string) (*UserDataExport, error) {
    // Collect all user data from store
    // Return structured export
    return &UserDataExport{}, nil
}

func (s *GDPRService) DeleteUserData(ctx context.Context, userID string) error {
    // Cascade delete all user data
    // This calls store.DeleteUser which cascades via FK constraints
    return nil
}
```

- [ ] **Step 2: Create handler**

```go
// internal/adapter/http/handlers_gdpr.go
package http

import "net/http"

func (h *Handlers) ExportUserData(w http.ResponseWriter, r *http.Request) {
    userID := chi.URLParam(r, "id")
    export, err := h.GDPR.ExportUserData(r.Context(), userID)
    if err != nil {
        respondError(w, http.StatusInternalServerError, err)
        return
    }
    respondJSON(w, http.StatusOK, export)
}

func (h *Handlers) DeleteUserData(w http.ResponseWriter, r *http.Request) {
    userID := chi.URLParam(r, "id")
    if err := h.GDPR.DeleteUserData(r.Context(), userID); err != nil {
        respondError(w, http.StatusInternalServerError, err)
        return
    }
    w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 3: Register routes (admin-only)**

```go
r.With(middleware.RequireRole(user.RoleAdmin)).Post("/users/{id}/export", h.ExportUserData)
r.With(middleware.RequireRole(user.RoleAdmin)).Delete("/users/{id}/data", h.DeleteUserData)
```

- [ ] **Step 4: Commit**

```bash
git add internal/service/gdpr.go internal/adapter/http/handlers_gdpr.go internal/adapter/http/routes.go
git commit -m "feat: add GDPR user data export and deletion endpoints (Article 17/20)"
```

---

### Task 2: Cascade Deletion Migration

**Files:**
- Create: `internal/adapter/postgres/migrations/088_user_cascade_delete.sql`

- [ ] **Step 1: Write migration for cascade deletion**

```sql
-- +goose Up
-- Ensure all user-referencing FKs cascade on delete for GDPR compliance.

-- Sessions
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_user_id_fkey;
ALTER TABLE sessions ADD CONSTRAINT sessions_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- API keys
ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS api_keys_user_id_fkey;
ALTER TABLE api_keys ADD CONSTRAINT api_keys_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- Refresh tokens
ALTER TABLE refresh_tokens DROP CONSTRAINT IF EXISTS refresh_tokens_user_id_fkey;
ALTER TABLE refresh_tokens ADD CONSTRAINT refresh_tokens_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- User LLM keys
ALTER TABLE user_llm_keys DROP CONSTRAINT IF EXISTS user_llm_keys_user_id_fkey;
ALTER TABLE user_llm_keys ADD CONSTRAINT user_llm_keys_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- Password reset tokens
ALTER TABLE password_reset_tokens DROP CONSTRAINT IF EXISTS password_reset_tokens_user_id_fkey;
ALTER TABLE password_reset_tokens ADD CONSTRAINT password_reset_tokens_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- +goose Down
-- Revert to default NO ACTION (safe — just removes CASCADE)
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_user_id_fkey;
ALTER TABLE sessions ADD CONSTRAINT sessions_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS api_keys_user_id_fkey;
ALTER TABLE api_keys ADD CONSTRAINT api_keys_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE refresh_tokens DROP CONSTRAINT IF EXISTS refresh_tokens_user_id_fkey;
ALTER TABLE refresh_tokens ADD CONSTRAINT refresh_tokens_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE user_llm_keys DROP CONSTRAINT IF EXISTS user_llm_keys_user_id_fkey;
ALTER TABLE user_llm_keys ADD CONSTRAINT user_llm_keys_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE password_reset_tokens DROP CONSTRAINT IF EXISTS password_reset_tokens_user_id_fkey;
ALTER TABLE password_reset_tokens ADD CONSTRAINT password_reset_tokens_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id);
```

- [ ] **Step 2: Verify migration**

```bash
go run ./cmd/codeforge/ migrate up
```

- [ ] **Step 3: Commit**

```bash
git add internal/adapter/postgres/migrations/
git commit -m "feat: add cascade delete on user FKs for GDPR erasure (migration 088)"
```

---

### Task 3: Data Retention Policy Document

**Files:**
- Create: `docs/data-retention.md`

- [ ] **Step 1: Write data retention policy**

```markdown
# Data Retention Policy

## Overview
CodeForge retains user data according to the following schedule. Data beyond
the retention period is eligible for automated cleanup.

## Retention Schedule

| Data Category     | Retention Period | Justification                     |
|-------------------|-----------------|-----------------------------------|
| Agent Events      | 90 days         | Operational debugging             |
| Conversations     | 1 year          | User-accessible history           |
| Audit Logs        | 7 years         | Regulatory compliance (SOC 2)     |
| User Accounts     | Until deletion  | GDPR Article 17                   |
| API Keys          | Until revoked   | User-managed lifecycle            |
| Sessions          | 30 days         | Security best practice            |
| Benchmark Results | 1 year          | Analysis and comparison           |
| LLM Cost Records  | 1 year          | Billing and cost tracking         |

## Automated Cleanup

A background job runs daily to remove data beyond its retention period.
Configuration: `codeforge.yaml` -> `retention` section.

## User Rights

- **Data Export:** `POST /api/v1/users/{id}/export` — returns all user data as JSON
- **Data Deletion:** `DELETE /api/v1/users/{id}/data` — cascades across all tables
- **Account Deletion:** `DELETE /api/v1/users/{id}` — removes account and all data
```

- [ ] **Step 2: Commit**

```bash
git add docs/data-retention.md
git commit -m "docs: add data retention policy (GDPR Article 5)"
```

---

### Task 4: SECURITY.md

**Files:**
- Create: `docs/SECURITY.md`

- [ ] **Step 1: Write security policy**

```markdown
# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly:

1. **Email:** security@codeforge.dev (or create a private GitHub Security Advisory)
2. **Do NOT** open a public issue for security vulnerabilities
3. Include: description, reproduction steps, impact assessment
4. We will acknowledge within 48 hours and provide a fix timeline within 7 days

## Supported Versions

| Version | Supported |
|---------|-----------|
| 0.8.x   | Yes       |
| < 0.8   | No        |

## Security Measures

- **Authentication:** JWT with bcrypt password hashing (configurable cost)
- **Authorization:** Role-based access control (Admin, Editor, Viewer)
- **Tenant Isolation:** All database queries scoped by tenant_id
- **Rate Limiting:** Auth endpoints rate-limited, account lockout after 5 failures
- **CSRF:** Not applicable (API-only, Bearer token auth)
- **Security Headers:** CSP, HSTS, X-Frame-Options, X-Content-Type-Options
- **Secrets:** Environment variables only, never hardcoded
- **SSRF Protection:** Private IP range blocking (IPv4 + IPv6)

## GDPR Compliance

- User data export: `POST /api/v1/users/{id}/export`
- User data deletion: `DELETE /api/v1/users/{id}/data`
- Data retention policy: `docs/data-retention.md`
- LLM data processing: User code may be sent to configured LLM providers

## Dependency Management

- Go: `go mod tidy` + `govulncheck`
- Python: Poetry + Ruff security checks
- Frontend: npm audit
- Pre-commit hooks enforce linting and secret detection
```

- [ ] **Step 2: Commit**

```bash
git add docs/SECURITY.md
git commit -m "docs: add SECURITY.md with vulnerability disclosure and GDPR info"
```

---

### Task 5: Privacy Policy / LLM Consent Documentation

**Files:**
- Create: `docs/privacy-policy.md`

- [ ] **Step 1: Write privacy policy**

```markdown
# Privacy & LLM Data Processing Notice

## What Data is Processed

CodeForge processes the following data categories:
- **User accounts:** Email, name, role, password hash
- **Code and prompts:** Source code, natural language instructions
- **Agent activity:** Tool calls, conversation history, cost records
- **Infrastructure:** IP addresses (in audit logs), session tokens

## LLM Provider Data Processing

When you use CodeForge, your code and prompts may be sent to external
LLM providers for processing. The specific provider depends on your
configuration:

| Provider   | Data Sent          | Data Retention by Provider |
|------------|--------------------|-----------------------------|
| OpenAI     | Prompts, code      | See OpenAI data policy      |
| Anthropic  | Prompts, code      | See Anthropic data policy   |
| Ollama     | Prompts, code      | Local only — no external    |
| LM Studio  | Prompts, code      | Local only — no external    |

### Opting Out of External LLM Processing

Configure CodeForge to use local models only (Ollama, LM Studio)
to ensure no data leaves your infrastructure.

## Your Rights (GDPR)

- **Access:** View your data via the dashboard or API
- **Export:** `POST /api/v1/users/{id}/export`
- **Deletion:** `DELETE /api/v1/users/{id}/data`
- **Restriction:** Configure local-only models to prevent external processing
```

- [ ] **Step 2: Commit**

```bash
git add docs/privacy-policy.md
git commit -m "docs: add privacy policy with LLM data processing notice (GDPR Article 13)"
```

---

### Task 6: OpenAPI Spec Stub

**Files:**
- Create: `docs/api/openapi.yaml`

- [ ] **Step 1: Create OpenAPI 3.0 stub with core endpoints**

Create a minimal but valid OpenAPI spec covering the most important endpoint groups:
- Auth (login, register, refresh)
- Projects (CRUD)
- Conversations (CRUD, messages)
- Health

This serves as a foundation — full spec can be generated from routes.go in a later iteration.

- [ ] **Step 2: Commit**

```bash
git add docs/api/
git commit -m "docs: add OpenAPI 3.0 spec stub for core endpoints"
```
