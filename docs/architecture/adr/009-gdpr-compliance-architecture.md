# ADR-009: GDPR Compliance Architecture

> **Status:** accepted
> **Date:** 2026-03-26
> **Deciders:** Project lead + Claude Code analysis

### Context

CodeForge processes personal data across multiple subsystems: user accounts, conversation histories, LLM interaction logs, agent execution trajectories, and cost tracking records. As a tool that may be deployed by EU-based organizations or used by EU residents, GDPR compliance is a practical requirement.

Key GDPR obligations that affect the architecture:

- **Right to erasure (Art. 17):** Users can request deletion of all their personal data.
- **Right to data portability (Art. 20):** Users can request an export of their data in a machine-readable format.
- **Data minimization (Art. 5(1)(c)):** Only collect and retain data that is necessary for the stated purpose.
- **Storage limitation (Art. 5(1)(e)):** Personal data must not be kept longer than necessary.
- **Privacy by design (Art. 25):** Data protection must be integrated into system design, not bolted on.

The challenge is that user data spans multiple tables (users, conversations, messages, runs, costs, API keys, audit logs, agent trajectories) across both the Go core and Python worker layers, with additional references in NATS JetStream and LiteLLM's own tables.

### Decision

Implement GDPR compliance through four architectural mechanisms:

#### 1. Cascade Deletion with Tenant-Scoped Cleanup

All user data deletion follows a deterministic cascade order to respect foreign key constraints:

1. Agent trajectories and tool call logs
2. Conversation messages and attachments (including canvas images)
3. Conversation runs and cost records
4. Conversations
5. API keys and user tokens
6. Audit log entries (anonymized, not deleted -- retained for security)
7. User account record

The existing `RetentionService` (which already iterates all tenants for time-based cleanup) is extended with a user-scoped deletion path. Audit log entries are anonymized (user ID replaced with a tombstone value) rather than deleted, preserving the security audit trail while removing PII.

#### 2. JSON Export Format

The `/me/export` endpoint returns a JSON archive containing all user-associated data:

- Profile information (name, email, preferences)
- Conversations and messages (with tool call history)
- Cost and budget records
- Project membership and roles
- API key metadata (not secrets)

The export uses a flat JSON structure with ISO 8601 timestamps, matching the existing API response format. No new serialization format is introduced.

#### 3. PII Redaction in Logs

Two-layer approach to prevent PII leakage into logs:

- **Source-level removal:** The auth service no longer includes email addresses in INFO-level log messages. PII is only logged at DEBUG level (disabled in production).
- **RedactHandler (slog):** A structured logging handler wraps the default handler and redacts known PII field patterns (`email`, `user_email`, `ip_address`) before they reach the log output. This acts as a safety net for cases where source-level removal is missed.

Production log configuration: `LOG_LEVEL=info` (default), which excludes DEBUG-level PII. The RedactHandler provides defense-in-depth.

#### 4. Self-Service Endpoints Alongside Admin Endpoints

Two access patterns for data management:

| Endpoint | Auth | Purpose |
|---|---|---|
| `GET /me/export` | User token (own data) | Data portability (Art. 20) |
| `DELETE /me/data` | User token (own data) | Right to erasure (Art. 17) |
| `DELETE /admin/users/{id}/data` | Admin role | Admin-initiated erasure |
| `GET /admin/users/{id}/export` | Admin role | Admin-initiated export |

Self-service endpoints operate only on the authenticated user's own data. Admin endpoints require the `admin` role and can target any user. Both paths use the same underlying deletion/export logic.

### Consequences

#### Positive

- Users can exercise GDPR rights without admin intervention, reducing operational burden
- Cascade deletion logic is centralized in one service, avoiding scattered deletion across handlers
- PII redaction in logs is defense-in-depth: even if a developer accidentally logs an email, the RedactHandler catches it
- JSON export format reuses existing API serialization, no new format to maintain
- Tenant-scoped cleanup ensures multi-tenant deployments do not leak data across tenants

#### Negative

- Per-table deletion logic must be maintained as the schema evolves; new tables with user references need to be added to the cascade
- The cascade deletion order is sensitive to foreign key changes; schema migrations must update the deletion service when adding FK constraints
- Audit log anonymization (tombstone) means security investigations lose the ability to identify specific users in historical entries
- Export endpoint can be expensive for users with large conversation histories; may need pagination or async generation in the future

#### Neutral

- NATS JetStream messages are ephemeral and expire via TTL; no explicit GDPR deletion is needed for the message queue layer
- LiteLLM's own tables (prefixed `LiteLLM_`) track spend by virtual key, not by user email; these are outside the GDPR cascade but do not contain PII beyond the key mapping
- The `_FILE` env var pattern for Docker secrets (used for JWT secret) is a general infrastructure improvement that happens to support GDPR's security requirements (Art. 32)

### Alternatives Considered

| Alternative | Pros | Cons | Why Not |
|---|---|---|---|
| Soft delete (mark as deleted, purge later) | Simpler implementation, recoverable | Data remains in DB, does not satisfy Art. 17 "without undue delay" | GDPR requires actual erasure, not just hiding |
| Event-sourced deletion (append "deleted" event) | Fits event-sourcing pattern | Tombstone events accumulate, compaction complexity, data still in old events | Over-engineering for current scale; hard delete is simpler and compliant |
| Third-party consent management (OneTrust, Cookiebot) | Feature-rich, auditable | External dependency, cost, overkill for a developer tool | CodeForge is self-hosted; users control their own instance |
| Per-field encryption (encrypt PII at rest, destroy key to "delete") | Cryptographic erasure, fast | Key management complexity, cannot search encrypted fields, breaks FTS | Adds significant complexity for marginal benefit over cascade delete |

### References

- [GDPR Art. 17 -- Right to Erasure](https://gdpr-info.eu/art-17-gdpr/)
- [GDPR Art. 20 -- Right to Data Portability](https://gdpr-info.eu/art-20-gdpr/)
- [GDPR Art. 5 -- Principles (Minimization, Storage Limitation)](https://gdpr-info.eu/art-5-gdpr/)
- [GDPR Art. 25 -- Data Protection by Design](https://gdpr-info.eu/art-25-gdpr/)
- [GDPR Art. 32 -- Security of Processing](https://gdpr-info.eu/art-32-gdpr/)
- CodeForge audit findings: F-COM-003 (PII in logs), F-COM-010 (GDPR endpoints), F-COM-011 (ADR documentation)
