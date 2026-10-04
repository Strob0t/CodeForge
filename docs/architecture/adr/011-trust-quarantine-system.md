# ADR-011: Trust Annotations and Message Quarantine

> **Status:** accepted
> **Date:** 2026-02-18
> **Deciders:** Project lead + Claude Code analysis

### Context

CodeForge orchestrates AI agents at varying autonomy levels, including fully autonomous (level 4) and headless CI/CD (level 5). Messages flow between agents via NATS, and with A2A federation, messages can originate from external untrusted sources. The system needed a mechanism to:

- Classify the trustworthiness of messages and their sources
- Prevent untrusted or suspicious messages from triggering privileged operations (tool execution, file writes, deployments)
- Hold questionable messages for human review without blocking the pipeline
- Maintain an audit trail of trust decisions for compliance (SOC 2 CC6.1)
- Support graduated trust where new agents earn trust over time

Without trust controls, a compromised or misconfigured agent could escalate privileges through crafted messages, and external A2A agents could inject malicious tasks.

### Decision

**4-level trust annotations** auto-stamped on all NATS payloads, combined with a **message quarantine system** for risk-scored messages that exceed thresholds.

#### Trust Levels

| Level | Name | Meaning | Auto-assigned When |
|---|---|---|---|
| 0 | `untrusted` | Unknown or external origin | A2A inbound, new agents, no identity |
| 1 | `partial` | Known but unverified | Agent has identity but < N successful runs |
| 2 | `verified` | Proven track record | Agent fingerprint matches, stats above threshold |
| 3 | `full` | Fully trusted internal | Built-in agents, admin-promoted agents |

Trust annotations are stamped on NATS message headers at publish time by the Go Core. Python workers read trust level before executing actions and refuse privileged operations from untrusted sources.

#### Quarantine System

Messages that exceed a configurable risk score are held in a quarantine table:

- **Risk scoring:** Weighted factors including trust level, action severity, pattern anomalies
- **Hold state:** Quarantined messages are persisted in PostgreSQL (migration 049) with status `held`
- **Admin review:** REST API for listing, evaluating, approving, or rejecting quarantined messages
- **Auto-release:** Messages from `full`-trust agents bypass quarantine entirely

#### Domain Model

```
internal/domain/trust/
  - level.go: TrustLevel enum (Untrusted, Partial, Verified, Full)
  - annotation.go: TrustAnnotation struct (Level, Source, Timestamp, Reason)

internal/service/quarantine.go:
  - QuarantineService: Score(), Hold(), Evaluate(), Approve(), Reject()
  - RiskScorer: configurable weights per factor

internal/domain/agent/agent.go:
  - Agent.Fingerprint: stable identity hash
  - Agent.Stats: success count, failure count, last active
```

> **Implementation status (2026-09-29):** `internal/domain/trust/trust.go` defines `Level` and `Annotation{Origin, TrustLevel, SourceID, Signature, Timestamp}`. Internal messages are always stamped `full`, external (A2A, webhook) messages default to `untrusted`; `verified` means a valid cryptographic signature and `partial` a known external source without signature. Graduated promotion by run history is not implemented; `internal/domain/agent/agent.go` tracks `TotalRuns`, `TotalCost`, `SuccessRate`, `State`, `Capabilities` and `LastActiveAt` (no fingerprint). Annotations travel in the JSON payload field `trust`, not in NATS headers, and the workers do not enforce them (`validate_incoming` in `workers/codeforge/trust/middleware.py` is never called). Quarantined messages start as `pending` (then `approved`/`rejected`/`expired`, migration 049), and messages at or above `quarantine.min_trust_bypass` (default `verified`) bypass quarantine. Scoring is `quarantine.ScoreMessage` (`internal/domain/quarantine/scorer.go`); `QuarantineService` offers `Evaluate`, `Approve`, `Reject`, `List` and `Get`. The only wired gate (`runs.start`) always carries `full` trust, and inbound A2A prompts and handoffs are never evaluated, so quarantine never holds anything (see [Known Issues](../../todo.md#known-issues) KI-15).

> **Update (2026-10-01, KI-15, KI-79):** quarantine now has three gates: `runs.start`, handoffs (`HandoffService`, which the Go Core drives from `handoff.request`) and inbound A2A prompts, so the statement above that handoffs and A2A prompts are never evaluated no longer applies. For an A2A prompt the task is created first and records the held message in its metadata (`quarantine_message_id`). Approve replays the prompt only while the task is `submitted`; otherwise the message is rejected and the call answers 409. Reject rejects the task, a cancel by the A2A caller withdraws the message, and if the task cannot record the screening the message is withdrawn instead of left held. A held handoff continues on `handoff.approved` (Go Core only). The reviewer is the logged-in user (`reviewed_by_user_id`, `reviewed_by_id` in the API); a name in the request body is ignored. Messages have `expires_at`, but nothing sets the status `expired` yet (KI-91).

### Consequences

#### Positive

- Defense in depth: Trust annotations add a layer independent of policy rules and RBAC
- Graduated trust: New agents start untrusted and earn trust through successful execution, enabling safe onboarding
- Audit trail: All quarantine decisions are logged with reviewer, timestamp, and reason
- Non-blocking: Quarantine holds suspicious messages without stopping the entire pipeline
- Compatible with A2A: External agents automatically receive `untrusted` level, preventing privilege escalation from federation

#### Negative

- Latency for new agents: First-time agents experience quarantine delays until trust is established. Mitigation: admin can manually promote agents to `full` trust
- Additional database writes: Every quarantine hold/release is persisted. Mitigation: only risk-scored messages above threshold are quarantined; `full`-trust messages skip entirely
- Trust level is per-agent, not per-action: A trusted agent could still send a malicious message. Mitigation: policy layer evaluates each tool call independently; trust is an additional signal, not the only one

#### Neutral

- Trust annotations add negligible overhead (~100 bytes per message)
- Quarantine table uses the same tenant isolation pattern as all other tables
- Agent fingerprinting (SHA-256 of agent config + version) is planned on top of the persistent identity system, which currently tracks run statistics only

### Alternatives Considered

| Alternative | Pros | Cons | Why Not |
|---|---|---|---|
| Binary trust (trusted/untrusted) | Simple mental model | Too coarse; no path from untrusted to trusted without admin intervention | Graduated trust enables safe, automatic onboarding of new agents |
| Per-message cryptographic signing (JWS) | Tamper-proof, non-repudiation | High overhead for internal NATS messages, key management complexity | Overkill for single-cluster deployment; trust annotations are sufficient. JWS reserved for A2A cross-boundary messages |
| No trust system (rely on policy layer only) | Simpler architecture | Policy evaluates tool calls, not message origins; cannot distinguish internal from external | Trust and policy are complementary: trust classifies the source, policy evaluates the action |
| Reputation system (continuous score) | More granular than levels | Complex to calibrate, hard to explain to users, gaming risk | 4 discrete levels are simple to understand, configure, and audit |

### References

- `internal/domain/trust/` -- Trust level enum and annotation types
- `internal/service/quarantine.go` -- QuarantineService with risk scoring
- `internal/domain/agent/agent.go` -- Persistent agent identity and stats
- `frontend/src/features/project/WarRoom.tsx` -- War Room multi-agent view
- Migration `049` -- Quarantine table schema
- AMP (Agent Messaging Protocol) -- Inspiration for trust patterns (Ed25519, but adapted to simpler level-based system)
