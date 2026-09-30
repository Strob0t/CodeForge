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
| User Accounts             | Until deletion   | -              | -        | GDPR Article 17                   |
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

A period of 0 keeps that category forever; negative periods and periods under 24h are rejected at startup
(this catches unit mistakes such as `30m` meant as months).

## User Rights

- **Data Export:** `POST /api/v1/users/{id}/export` -- returns all user data as JSON
- **Data Deletion:** `DELETE /api/v1/users/{id}/data` -- cascades across all tables
- **Account Deletion:** `DELETE /api/v1/users/{id}` -- removes account and all data
