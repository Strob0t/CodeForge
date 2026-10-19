# Data Retention Policy

## Overview

CodeForge retains user data according to the following schedule. Data beyond
the retention period is eligible for automated cleanup.

## Retention Schedule

| Data Category             | Retention Period | Counted from   | Enforced | Justification                     |
|---------------------------|------------------|----------------|----------|-----------------------------------|
| Agent Sessions            | 30 days          | last activity  | yes      | Security best practice            |
| Conversations (+messages) | 1 year           | last activity  | yes      | User-accessible history           |
| LLM Cost Records (runs)   | 1 year           | last activity  | yes      | Billing and cost tracking         |
| Audit Logs                | 7 years          | creation       | yes      | Regulatory compliance (SOC 2)     |
| IP addresses in audit logs| 180 days         | creation       | yes (anonymized, entry kept) | Data minimisation |
| Agent Events              | 90 days          | creation       | not yet  | Operational debugging             |
| Benchmark Results         | 1 year           | creation       | not yet  | Analysis and comparison           |
| User Accounts             | Until deletion   | -              | -        | GDPR Article 17; erased immediately on request, see below |
| API Keys                  | Until revoked    | -              | -        | User-managed lifecycle            |

Agent events and benchmark results are not purged yet: purging agent events after 90 days while runs are kept
for a year would break the trajectories of those runs, which needs a product decision first.

## Automated Cleanup

The Go Core runs the retention job across all tenants (`RetentionService`, `internal/service/retention.go`):
once at startup, then every `retention.interval`. Each category is purged in batches until nothing is left
past its cutoff; a failing category is logged and does not stop the others; the log lines name only category,
action, row count and cutoff. Deleting an expired conversation also deletes its agent sessions that belong to
no task (sessions shared with a task are only detached).

| YAML key (`retention.`) | Environment variable                      | Default          |
|-------------------------|-------------------------------------------|------------------|
| `interval`              | `CODEFORGE_RETENTION_INTERVAL`            | `24h` (0 disables the job) |
| `sessions`              | `CODEFORGE_RETENTION_SESSIONS`            | `720h`           |
| `conversations`         | `CODEFORGE_RETENTION_CONVERSATIONS`       | `8760h`          |
| `cost_records`          | `CODEFORGE_RETENTION_COST_RECORDS`        | `8760h`          |
| `audit_entries`         | `CODEFORGE_RETENTION_AUDIT_ENTRIES`       | `61320h`         |
| `audit_ip_addresses`    | `CODEFORGE_RETENTION_AUDIT_IP_ADDRESSES`  | `4320h`          |
| `handoff_claims`        | `CODEFORGE_RETENTION_HANDOFF_CLAIMS`      | `720h` (0 or at least 720h, the NATS stream's max age) |

A period of 0 keeps that category forever; negative periods and periods under 24h are rejected at startup
(this catches unit mistakes such as `30m` meant as months).

A period of whole 365-day years counts calendar years (a period of 1 year on 15 March 2027 keeps data back to
15 March 2026). On 29 February that date does not exist that many years back; the cutoff is then 28 February
of that year, so data is kept a day longer, never shorter than configured. Only one replica sweeps at a time
(the sweep runs on a PostgreSQL advisory-lock connection). Besides the categories above, every sweep deletes
expired OAuth states of abandoned GitHub connect flows. Every sweep also deletes handoff claims whose stage was done longer ago than `retention.handoff_claims` (the default
outlasts the NATS stream's 30-day max age; claims never done are kept: a redelivery may still take them over) and
webhook delivery claims (`webhook_deliveries`, body hash and delivery ID only, KI-85) older than
`webhook.delivery_retention` (default 168h, their dedup window; a webhook also prunes its own claims on its next
delivery), across tenants in batches (KI-90). `conversation_turn_completions` are deleted with their conversation and
`task_result_costs` with their task.

## Project Workspaces and Tool Caches (KI-96)

With per-tenant tool users (`workspace.tool_acls: required`, production) a deleted project's workspace is removed
asynchronously by the worker as the tenant's tool UID: the project row and a `workspace_deletions` record are written in
one transaction, the Go Core publishes `workspace.delete.request` and republishes pending deletions every 10 minutes until
the worker reports the workspace removed. Pending deletions are visible in `workspace_deletions` and in the logs; there is
no erasure report of them yet. A project with active work cannot be deleted (409). After a deletion the tenant's build
caches (`<HOME>/.cache` on the `tool_homes` volume) are removed when the tenant next has no work in that worker, because
they can hold data derived from the project; otherwise a tenant's cache is removed at that point only when it exceeds
`CODEFORGE_TOOL_CACHE_MAX_MB` (default 4096). Tenants cannot be deleted yet, so their HOMEs and tool UIDs stay
([KI-112](todo.md#known-issues)). With `workspace.tool_acls: off` (development) the Go Core removes the workspace itself.

## User Rights

- **Data Export:** `POST /api/v1/users/{id}/export` -- returns all user data as JSON
- **Data Deletion:** `DELETE /api/v1/users/{id}/data` -- cascades across all tables
- **Account Deletion:** `DELETE /api/v1/users/{id}` -- removes account and all data; it erases like the data deletion
  above (personal data in audit entries, consent records, channel messages and quarantine reviews is anonymized
  first, then the user row is deleted)
- **Self-service:** `GET /api/v1/me/export` and `DELETE /api/v1/me/data` for the signed-in user

Account data is erased immediately when erasure is requested or the account is deleted. Database backups that
still contain it are not edited; they roll off with the backup retention (physical base backups for 4 weeks and
the WAL archive up to one week longer, so about five weeks; see [disaster-recovery.md](disaster-recovery.md)).
