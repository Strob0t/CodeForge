# Data Classification Schema

**Date:** 2026-03-27
**Ref:** GDPR Art. 30 (Records of Processing Activities)

---

## Data Categories

| Category | Fields | PII | Sensitivity | Storage | Encryption | Retention |
|----------|--------|-----|-------------|---------|------------|-----------|
| **Account** | email, name, password_hash, role | Yes | High | PostgreSQL `users` | password_hash: bcrypt; email/name: plaintext | Account lifetime + 30 days |
| **Authentication** | JWT tokens, refresh tokens, API keys | No (opaque) | Critical | PostgreSQL `revoked_tokens`, `api_keys` | API keys: SHA-256 hash | Refresh: 7 days; Revoked: 30 days cleanup |
| **Conversation** | user prompts, LLM responses, tool calls | Yes (may contain PII) | High | PostgreSQL `conversation_messages` | **Plaintext** (see Encryption Roadmap below) | 90 days after last activity (configurable) |
| **Audit Log** | admin_id, admin_email, ip_address, action | Yes | Medium | PostgreSQL `audit_log` | Plaintext | Email: anonymized on user deletion; IP: 180 days (CNIL) |
| **Cost/Usage** | token counts, model usage, cost_usd | No | Low | PostgreSQL `cost_entries` | Plaintext | 7 years (tax/accounting) |
| **VCS Credentials** | OAuth tokens, PATs | No (opaque) | Critical | PostgreSQL `vcs_accounts` | AES-256-GCM (`crypto.Encrypt`) | Account lifetime |
| **LLM Keys** | Provider API keys | No (opaque) | Critical | PostgreSQL `llm_keys` | AES-256-GCM (`crypto.Encrypt`) | Until deleted by user |
| **Agent State** | workspace paths, run logs, trajectory | No | Medium | PostgreSQL + NATS KV | Plaintext | Run: 30 days; Trajectory: 30 days |
| **Consent Records** | purpose_id, granted, ip_address | Yes (IP) | High | PostgreSQL `user_consents` | Plaintext (append-only) | Indefinite (proof-of-consent per Art. 7(1)) |
| **Message Images** | image data (screenshots, canvas exports) | Yes (may contain PII in screenshots) | High | PostgreSQL `conversation_messages.images` (JSONB) | Plaintext | Same as Conversation (90 days) |
| **NATS Stream Messages** | serialized task/agent/conversation payloads | Yes (may contain PII) | High | NATS JetStream (transient) | TLS in-transit; plaintext at rest | 30-day MaxAge (stream config) |

## Encryption Status

| Data | Current | Recommended | Priority |
|------|---------|-------------|----------|
| Password hashes | bcrypt (cost 12) | No change needed | N/A |
| VCS tokens | AES-256-GCM | No change needed | N/A |
| LLM API keys | AES-256-GCM | No change needed | N/A |
| Conversation content | **Plaintext** | AES-256-GCM envelope encryption | High |
| Audit log PII | **Plaintext** | Consider field-level encryption | Medium |

## Encryption Roadmap for Conversation Content

**Current state:** `conversation_messages.content` stored as plaintext TEXT column.

**Recommended approach:** Application-level AES-256-GCM with envelope encryption.
- One Data Encryption Key (DEK) per tenant
- DEK encrypted by Key Encryption Key (KEK) via Vault Transit or cloud KMS
- `content` column changes from TEXT to BYTEA
- `key_version` column tracks encryption key version for rotation
- Random 12-byte nonce per encryption, prepended to ciphertext
- `conversation_id` as GCM Additional Authenticated Data (AAD)

**Trade-offs:**
- PostgreSQL FTS index (migration 069) will not work on encrypted content
- Search must be metadata-based (title, date, project) or decrypted in application
- Background re-encryption job needed for key rotation

**Migration path:**
1. Add `key_version` column (nullable, default NULL = unencrypted)
2. New writes encrypt; reads check `key_version` to decide decrypt
3. Background job encrypts existing rows in batches
4. After full migration, remove plaintext fallback

## Access Control

| Data Category | Who Can Access | Mechanism |
|---------------|----------------|-----------|
| Account | Self + Admin | RBAC middleware |
| Conversation | Tenant members | `tenant_id` filter on all queries |
| Audit Log | Admin only | `RequireRole(admin)` |
| VCS Credentials | Self only | User ID match |
| LLM Keys | Self + Admin | User ID match + Admin override |
| Cost/Usage | Tenant members | `tenant_id` filter |
