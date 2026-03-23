# WT-7: Observability & Audit Trail — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add admin audit logging (table + middleware + API), PII log redaction, NATS consumer metrics, Prometheus alerting config, and idempotency verification.

**Architecture:** New `audit_log` table (migration 087), audit middleware wrapping admin endpoints, slog handler wrapper for PII redaction, OTEL gauge for NATS consumer lag.

**Tech Stack:** Go 1.25, PostgreSQL, slog, OTEL API, Prometheus, chi middleware

**Best Practice:**
- SOC 2 CC6.1: All security-relevant admin actions must have an immutable audit trail.
- OWASP Logging: Never log passwords, tokens, or PII. Use structured redaction.
- NATS monitoring: Export consumer pending count as a gauge metric for alerting.

---

### Task 1: Create Audit Log Migration

**Files:**
- Create: `internal/adapter/postgres/migrations/087_create_audit_log.sql`

- [ ] **Step 1: Write migration**

```sql
-- +goose Up
CREATE TABLE audit_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    admin_id    UUID NOT NULL,
    admin_email TEXT NOT NULL,
    action      TEXT NOT NULL,
    resource    TEXT NOT NULL,
    resource_id TEXT,
    details     JSONB,
    ip_address  INET,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_log_tenant_created ON audit_log (tenant_id, created_at DESC);
CREATE INDEX idx_audit_log_action ON audit_log (action);
CREATE INDEX idx_audit_log_admin ON audit_log (admin_id);

-- +goose Down
DROP TABLE IF EXISTS audit_log;
```

- [ ] **Step 2: Verify migration syntax**

```bash
go run ./cmd/codeforge/ migrate up
```

- [ ] **Step 3: Commit**

```bash
git add internal/adapter/postgres/migrations/
git commit -m "feat: add audit_log table (migration 087)"
```

---

### Task 2: Audit Log Store Methods

**Files:**
- Create: `internal/adapter/postgres/store_audit_log.go`

- [ ] **Step 1: Implement store methods**

```go
package postgres

import (
    "context"
    "time"
    "github.com/Strob0t/CodeForge/internal/tenantctx"
)

type AuditEntry struct {
    ID         string    `json:"id"`
    TenantID   string    `json:"tenant_id"`
    AdminID    string    `json:"admin_id"`
    AdminEmail string    `json:"admin_email"`
    Action     string    `json:"action"`
    Resource   string    `json:"resource"`
    ResourceID string    `json:"resource_id,omitempty"`
    Details    any       `json:"details,omitempty"`
    IPAddress  string    `json:"ip_address,omitempty"`
    CreatedAt  time.Time `json:"created_at"`
}

func (s *Store) InsertAuditEntry(ctx context.Context, e *AuditEntry) error {
    tid := tenantctx.FromContext(ctx)
    _, err := s.pool.Exec(ctx,
        `INSERT INTO audit_log (tenant_id, admin_id, admin_email, action, resource, resource_id, details, ip_address)
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8::inet)`,
        tid, e.AdminID, e.AdminEmail, e.Action, e.Resource, e.ResourceID, e.Details, e.IPAddress)
    return err
}

func (s *Store) ListAuditEntries(ctx context.Context, action string, limit, offset int) ([]AuditEntry, error) {
    tid := tenantctx.FromContext(ctx)
    query := `SELECT id, tenant_id, admin_id, admin_email, action, resource, resource_id, details, ip_address, created_at
              FROM audit_log WHERE tenant_id = $1`
    args := []any{tid}
    argIdx := 2
    if action != "" {
        query += ` AND action = $` + itoa(argIdx)
        args = append(args, action)
        argIdx++
    }
    query += ` ORDER BY created_at DESC LIMIT $` + itoa(argIdx) + ` OFFSET $` + itoa(argIdx+1)
    args = append(args, limit, offset)

    rows, err := s.pool.Query(ctx, query, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var entries []AuditEntry
    for rows.Next() {
        var e AuditEntry
        if err := rows.Scan(&e.ID, &e.TenantID, &e.AdminID, &e.AdminEmail, &e.Action,
            &e.Resource, &e.ResourceID, &e.Details, &e.IPAddress, &e.CreatedAt); err != nil {
            return nil, err
        }
        entries = append(entries, e)
    }
    return entries, rows.Err()
}

func itoa(n int) string {
    return fmt.Sprintf("%d", n)
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/adapter/postgres/store_audit_log.go
git commit -m "feat: add audit log store methods"
```

---

### Task 3: Audit Middleware

**Files:**
- Create: `internal/middleware/audit.go`

- [ ] **Step 1: Create audit middleware**

```go
package middleware

import (
    "context"
    "log/slog"
    "net/http"
    "strings"
)

type AuditLogger interface {
    LogAudit(ctx context.Context, adminID, adminEmail, action, resource, resourceID, ip string)
}

func AuditLog(logger AuditLogger, action, resource string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            claims := claimsFromCtx(r.Context())
            if claims != nil {
                resourceID := extractResourceID(r)
                ip := strings.Split(r.RemoteAddr, ":")[0]
                logger.LogAudit(r.Context(), claims.Subject, claims.Email, action, resource, resourceID, ip)
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

- [ ] **Step 2: Wire audit middleware to critical routes in routes.go**

Add `AuditLog(auditSvc, "action", "resource")` to:
- User CRUD endpoints
- Policy create/delete
- Quarantine approve/reject
- Mode create/delete

- [ ] **Step 3: Commit**

```bash
git add internal/middleware/audit.go internal/adapter/http/routes.go
git commit -m "feat: add audit middleware for admin operations (SOC 2 CC6.1)"
```

---

### Task 4: Audit Log HTTP Endpoint

**Files:**
- Create: `internal/adapter/http/handlers_audit.go`
- Modify: `internal/adapter/http/routes.go`

- [ ] **Step 1: Create handler**

```go
func (h *Handlers) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
    action := r.URL.Query().Get("action")
    limit, offset := parsePagination(r)
    entries, err := h.Store.ListAuditEntries(r.Context(), action, limit, offset)
    if err != nil {
        respondError(w, http.StatusInternalServerError, err)
        return
    }
    respondJSON(w, http.StatusOK, entries)
}
```

- [ ] **Step 2: Register route (admin-only)**

```go
r.With(middleware.RequireRole(user.RoleAdmin)).Get("/audit-logs", h.ListAuditLogs)
```

- [ ] **Step 3: Commit**

```bash
git add internal/adapter/http/handlers_audit.go internal/adapter/http/routes.go
git commit -m "feat: add GET /audit-logs endpoint (admin-only)"
```

---

### Task 5: PII Redaction in Logs

**Files:**
- Create: `internal/logger/redact.go`

- [ ] **Step 1: Create redacting slog handler**

```go
package logger

import (
    "context"
    "log/slog"
    "regexp"
    "strings"
)

var sensitivePatterns = []*regexp.Regexp{
    regexp.MustCompile(`(?i)(sk-[a-zA-Z0-9_-]{20,})`),           // API keys
    regexp.MustCompile(`(?i)(ghp_[a-zA-Z0-9]{36})`),              // GitHub PAT
    regexp.MustCompile(`(?i)(password|passwd|secret)=\S+`),        // Key=value
    regexp.MustCompile(`(?i)([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+)`), // Email
}

const redacted = "[REDACTED]"

type RedactHandler struct {
    inner slog.Handler
}

func NewRedactHandler(inner slog.Handler) *RedactHandler {
    return &RedactHandler{inner: inner}
}

func (h *RedactHandler) Enabled(ctx context.Context, level slog.Level) bool {
    return h.inner.Enabled(ctx, level)
}

func (h *RedactHandler) Handle(ctx context.Context, r slog.Record) error {
    r.Message = redactString(r.Message)
    attrs := make([]slog.Attr, 0)
    r.Attrs(func(a slog.Attr) bool {
        attrs = append(attrs, redactAttr(a))
        return true
    })
    newRecord := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
    for _, a := range attrs {
        newRecord.AddAttrs(a)
    }
    return h.inner.Handle(ctx, newRecord)
}

func (h *RedactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
    return &RedactHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *RedactHandler) WithGroup(name string) slog.Handler {
    return &RedactHandler{inner: h.inner.WithGroup(name)}
}

func redactString(s string) string {
    for _, p := range sensitivePatterns {
        s = p.ReplaceAllString(s, redacted)
    }
    return s
}

func redactAttr(a slog.Attr) slog.Attr {
    if a.Value.Kind() == slog.KindString {
        a.Value = slog.StringValue(redactString(a.Value.String()))
    }
    return a
}
```

- [ ] **Step 2: Wire into logger initialization**

In `internal/logger/logger.go`, wrap the handler:
```go
h = NewRedactHandler(h)
```

- [ ] **Step 3: Commit**

```bash
git add internal/logger/redact.go internal/logger/logger.go
git commit -m "feat: add PII/secret redaction handler to structured logging"
```

---

### Task 6: NATS Consumer Lag Metrics

**Files:**
- Modify: `internal/adapter/nats/nats.go`

- [ ] **Step 1: Export consumer pending count as metric**

In `monitorConsumer()`, add OTEL gauge recording:
```go
import "go.opentelemetry.io/otel/metric"

// In Queue struct or init:
meter := otel.Meter("codeforge.nats")
pendingGauge, _ := meter.Int64Gauge("nats.consumer.pending",
    metric.WithDescription("Number of pending messages for NATS consumer"))

// In monitorConsumer loop:
info, err := cons.Info(ctx)
if err == nil {
    pendingGauge.Record(ctx, int64(info.NumPending),
        metric.WithAttributes(attribute.String("consumer", name)))
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/adapter/nats/nats.go
git commit -m "feat: export NATS consumer pending count as OTEL gauge"
```

---

### Task 7: Prometheus Alerting Config

**Files:**
- Create: `configs/prometheus/alerts.yml`

- [ ] **Step 1: Create basic alert rules**

```yaml
groups:
  - name: codeforge
    rules:
      - alert: NATSConsumerLag
        expr: nats_consumer_pending > 1000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "NATS consumer {{ $labels.consumer }} has high lag"

      - alert: HighMemoryUsage
        expr: container_memory_usage_bytes / container_spec_memory_limit_bytes > 0.9
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Container {{ $labels.name }} memory usage > 90%"

      - alert: HealthCheckFailing
        expr: probe_success == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Health check failing for {{ $labels.instance }}"
```

- [ ] **Step 2: Commit**

```bash
git add configs/prometheus/
git commit -m "feat: add Prometheus alert rules for NATS lag, memory, health"
```
