# CodeForge — Documentation Index

> **LLM Agents:** Start here. This file maps all project documentation.
> For open tasks and priorities, see [todo.md](todo.md); for known defects on the current code, see its [Known Issues](todo.md#known-issues) section.

### Quick Reference

| Document | Purpose |
|---|---|
| [../AGENTS.md](../AGENTS.md) | Instructions for coding agents: workflow, rules, architecture summary (AGENTS.md convention, replaces CLAUDE.md) |
| [todo.md](todo.md) | Active TODO tracker — what needs to be done next |
| [todo.md#known-issues](todo.md#known-issues) | Known Issues (KI-n) — verified defects in the current code |
| [known-issues-fix-plan.md](known-issues-fix-plan.md) | Milestone plan (S0-S7) and decisions for fixing the Known Issues |
| [project-status.md](project-status.md) | Phase tracking, milestones, completed work |
| [architecture.md](architecture.md) | System architecture, patterns, design details |
| [tech-stack.md](tech-stack.md) | Languages, tools, dependencies, infrastructure |
| [dev-setup.md](dev-setup.md) | Development environment setup guide |
| [api/openapi.yaml](api/openapi.yaml) | REST API specification (OpenAPI 3.0) |

### Feature Specifications

Each of the four core pillars has its own feature spec (01-04); 05-07 cover cross-cutting features:

| Feature | File | Status |
|---|---|---|
| Project Dashboard | [features/01-project-dashboard.md](features/01-project-dashboard.md) | Foundation implemented |
| Roadmap/Feature-Map | [features/02-roadmap-feature-map.md](features/02-roadmap-feature-map.md) | Foundation implemented |
| Multi-LLM Provider | [features/03-multi-llm-provider.md](features/03-multi-llm-provider.md) | Foundation implemented |
| Agent Orchestration | [features/04-agent-orchestration.md](features/04-agent-orchestration.md) | Core implemented |
| Chat Enhancements | [features/05-chat-enhancements.md](features/05-chat-enhancements.md) | Implemented |
| Visual Design Canvas | [features/06-visual-design-canvas.md](features/06-visual-design-canvas.md) | Implemented |
| Chat-First Orchestrator | [features/07-chat-first-orchestrator.md](features/07-chat-first-orchestrator.md) | Implemented (`spawn_subagent` is not offered until sub-agents start: [Known Issues](todo.md#known-issues) KI-25) |

### Architecture Details

| Document | Purpose |
|---|---|
| [architecture/adr/](architecture/adr/) | Architecture Decision Records (ADRs) |
| [architecture/adr/_template.md](architecture/adr/_template.md) | ADR template for new decisions |
| [architecture/project-reference.md](architecture/project-reference.md) | Catalogue of adopted patterns, implemented phases, protocols, competitors |

### Security, Compliance & Operations

| Document | Purpose |
|---|---|
| [SECURITY.md](SECURITY.md) | Security policy, vulnerability reporting, secret management |
| [security/](security/) | [Breach notification procedure](security/breach-notification-procedure.md), [data classification](security/data-classification.md) |
| [data-retention.md](data-retention.md) | GDPR data retention policy |
| [privacy-policy.md](privacy-policy.md) | Privacy & LLM data processing notice |
| [disaster-recovery.md](disaster-recovery.md) | Backup, restore and recovery runbook |

### Research

| Document | Purpose |
|---|---|
| [research/market-analysis.md](research/market-analysis.md) | Market research, competitor analysis, framework comparison |
| [research/aider-deep-analysis.md](research/aider-deep-analysis.md) | Deep dive into Aider architecture |
| [research/protocol-analysis.md](research/protocol-analysis.md) | Protocol and standards analysis (MCP, A2A, AG-UI, LSP, OTEL) |

### Design Specs, Plans & Testing

| Directory | Purpose |
|---|---|
| [specs/](specs/) | Design specifications (`*-design.md`) |
| [plans/](plans/) | Implementation plans (`*-plan.md`) |
| [testing/](testing/) | Test plans (`*-testplan.md`) and test reports (`*-report.md`) |
| [testing/e2e-setup.md](testing/e2e-setup.md) | E2E setup: full stack, LLM API tests, autonomous goal-to-program run |

### Audits

| Document | Purpose |
|---|---|
| [audits/2026-03-18-schema-audit.md](audits/2026-03-18-schema-audit.md) | Database schema audit (score, findings, remediation) |
| [audits/ux-ui-audit.md](audits/ux-ui-audit.md) | Frontend UX/UI automated audit |
| [audits/stub-tracker.md](audits/stub-tracker.md) | Stub/placeholder inventory and status |

### Prompts

| Document | Purpose |
|---|---|
| [prompts/](prompts/) | Claude Code audit/discovery prompts and their reports |
| [prompts/stub-finder.md](prompts/stub-finder.md) | Claude Code prompt for stub discovery |

### Directory Layout

```text
docs/
├── README.md                # Index (this file)
├── todo.md                  # Central TODO (single source of truth, incl. Known Issues)
├── known-issues-fix-plan.md # Milestone plan (S0-S7) for fixing the Known Issues
├── architecture.md          # System architecture
├── dev-setup.md             # Setup guide, configuration, ports, env vars, versioning
├── project-status.md        # Phase tracking
├── tech-stack.md            # Dependencies
├── SECURITY.md              # Security policy, secret management, tool isolation
├── data-retention.md        # GDPR data retention policy
├── privacy-policy.md        # Privacy & LLM data processing notice
├── disaster-recovery.md     # Backup/restore runbook
├── features/                # Feature specs (01-07)
├── api/                     # OpenAPI spec (openapi.yaml)
├── security/                # Breach notification procedure, data classification
├── specs/                   # Design specs (*-design.md)
├── plans/                   # Implementation plans (*-plan.md)
├── testing/                 # Test plans + reports, E2E setup (e2e-setup.md)
├── audits/                  # Schema, UX, code audits
├── architecture/            # ADRs 001-018 (adr/, new ones from _template.md), project-reference.md
├── research/                # Market research
└── prompts/                 # Claude Code audit/discovery prompts
```

### Documentation Rules

See [AGENTS.md](../AGENTS.md) section "Workflow" for rules about when to update which documentation files and how to track TODOs. Structure:

- **Feature docs:** one per core pillar in `features/` (01-04; 05-07 cover cross-cutting features), sub-features as sections, each with overview, design, API and TODOs; the feature TODOs are cross-referenced in [todo.md](todo.md).
- **ADRs:** `architecture/adr/NNN-<title>.md` from [the template](architecture/adr/_template.md): Context -> Decision -> Consequences -> Alternatives; a later change to a decision is added as a dated update note, and the ADR is listed in `AGENTS.md`.
- **Plans and specs:** a larger refactor starts with a brief plan in `plans/` (problem, design, affected files, tests); design specs go to `specs/`.
