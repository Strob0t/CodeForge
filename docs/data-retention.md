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

- **Data Export:** `POST /api/v1/users/{id}/export` -- returns all user data as JSON
- **Data Deletion:** `DELETE /api/v1/users/{id}/data` -- cascades across all tables
- **Account Deletion:** `DELETE /api/v1/users/{id}` -- removes account and all data
