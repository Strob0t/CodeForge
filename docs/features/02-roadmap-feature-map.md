# Feature: Roadmap/Feature-Map (Pillar 2)

> Status: Foundation implemented (Phase 8A) -- domain, store, service, REST API, frontend
> Priority: Phase 8 (Foundation) then Phase 9+ (Advanced integrations)
> Architecture reference: [architecture.md](../architecture.md) -- "Roadmap/Feature-Map: Auto-Detection & Adaptive Integration"

### Purpose

Visual management of project roadmaps and feature maps. Compatible with OpenSpec and other spec-driven development tools. Supports **bidirectional** sync with external PM platforms. CodeForge does not build a proprietary PM tool but integrates with existing ones.

### Core Principle

CodeForge automatically detects which spec tools, PM platforms, and roadmap artifacts a project uses, then offers appropriate integration.

### Three-Tier Auto-Detection

- Spec-Driven Detectors (repo files): OpenSpec (`openspec/`), Spec Kit (`.specify/`), Autospec (`specs/spec.yaml`), ADR/RFC.
- Platform Detectors (API-based): GitHub Issues, GitLab Issues, Plane.so, OpenProject.
- **File-Based Detectors** (simple markers): ROADMAP.md, TASKS.md, backlog/, CHANGELOG.md.

Each detector implements `specprovider.SpecProvider` or `pmprovider.PMProvider` and self-registers via `init()`. This follows the same pattern as git providers.

### Supported Integrations

#### Spec Providers

| Provider | Adapter | Detection |
|---|---|---|
| OpenSpec | `adapter/openspec/` | `openspec/` directory |
| GitHub Spec Kit | `adapter/speckit/` | `.specify/` directory |
| Autospec | `adapter/autospec/` | `specs/spec.yaml` file |

#### PM Providers

| Provider | Adapter | Sync Method |
|---|---|---|
| Plane.so | `adapter/plane/` | REST API v1, Webhooks, HMAC-SHA256 |
| OpenProject | `adapter/openproject/` | REST API v3, Optimistic Locking |
| GitHub Issues/Projects | `adapter/github_pm/` | REST + GraphQL |
| GitLab Issues/Boards | `adapter/gitlab_pm/` | REST + GraphQL |
| Forgejo/Codeberg Issues | `adapter/github_pm/` (compatible) | REST API (GitHub-compatible) |

### Bidirectional Sync

```text
CodeForge Roadmap Model  <-->  External PM (Plane, GitHub, OpenProject)
         |
         <-->  Repo Specs (OpenSpec, Spec Kit, Autospec)
```

- Import: PM tool items become CodeForge features/tasks.
- Export: New features created as PM issues.
- **Conflict resolution** uses timestamp-based comparison plus user decision.
- Sync triggers: Webhook (real-time), poll (periodic), manual.

### Internal Data Model

- `Milestone` contains Features, which contain Tasks.
- `Feature` has Labels (for sync), SpecRef (link to spec file), ExternalIDs (PM mappings).
- Optimistic Locking (from OpenProject pattern) prevents concurrent edit conflicts.

### `/ai` Endpoint

LLM-optimized roadmap format for AI agents:

```text
GET /api/v1/projects/{id}/roadmap/ai?format=json|yaml|markdown
```

### Adopted Patterns

- Plane: Cursor Pagination, HMAC-SHA256 webhook verification, Label-triggered Sync.
- OpenProject: Optimistic Locking, Schema Endpoints.
- **OpenSpec**: Delta Spec Format (incremental changes).
- Ploi Roadmap: `/ai` endpoint for LLM consumption.

### Phase 8A: Foundation (Completed)

- [x] Domain models: `internal/domain/roadmap/` (Roadmap, Milestone, Feature, statuses, validation, optimistic locking).
- [x] Migration 017: `roadmaps`, `milestones`, `features` tables with indexes, triggers.
- [x] Port interfaces: `specprovider.SpecProvider` + `pmprovider.PMProvider` (interface + registry).
- [x] Store: 16 methods on `database.Store` + Postgres adapter.
- [x] RoadmapService: CRUD, AutoDetect (file markers), AIView (json/yaml/markdown).
- [x] REST API: 12 endpoints (roadmap CRUD, milestones, features, AI view, detect).
- [x] WS event: `roadmap.status` broadcast on mutations.
- [x] Frontend: RoadmapPanel.tsx (milestone/feature tree, forms, auto-detect, AI view).
- [x] `/ai` endpoint for LLM consumption (json/yaml/markdown formats).

### Phase 9A: Spec Provider Adapters + Enhanced AutoDetect + Spec Import (Completed)

- [x] OpenSpec adapter (`internal/adapter/openspec/`) -- detect `openspec/` dir, list `.yaml`/`.yml`/`.json` specs, read with path traversal protection, YAML title extraction.
- [x] Markdown spec adapter (`internal/adapter/markdownspec/`) -- detect `ROADMAP.md`/`roadmap.md`, list, read.
- [x] GitHub Issues PM adapter (`internal/adapter/githubpm/`) -- `gh` CLI integration, list/get issues, swappable execCommand for testing.
- [x] Enhanced AutoDetect -- two-phase: providers first, hardcoded `fileMarkers` fallback for uncovered formats, format alias dedup.
- [x] ImportSpecs service method -- discover specs via providers, auto-create roadmap, milestone per format, features per spec.
- [x] ImportPMItems service method -- find PM provider by name, list items, create milestone + features.
- [x] 4 new REST endpoints: `POST /projects/{id}/roadmap/import`, `POST /projects/{id}/roadmap/import/pm`, `GET /providers/spec`, `GET /providers/pm`.
- [x] Provider wiring via blank imports + main.go instantiation from registries.
- [x] Frontend: Import Specs button, Import from PM form (provider dropdown + project ref), import result display.
- [x] 24 new adapter tests (8 openspec, 7 markdownspec, 9 githubpm), all passing.

### TODOs (Phase 9+)

Tracked in [todo.md](../todo.md) under Phase 9+.

- [ ] Spec Kit adapter (`adapter/speckit/`).
- [ ] Autospec adapter (`adapter/autospec/`).
- [x] (2026-02-19) Gitea/Forgejo PM adapter (`adapter/gitea/`) -- full CRUD via REST API.
- [x] (2026-02-19) Bidirectional Sync Service (`service/sync.go`) -- pull/push/bidi directions.
- [x] (2026-02-19) PM Webhook sync (GitHub Issues, GitLab Issues, Plane.so) -- `service/pm_webhook.go`.
- [x] (2026-02-19) Slack/Discord notification adapters -- `adapter/slack/`, `adapter/discord/`, notifier registry.
- [ ] Plane.so PM adapter (REST API, full item CRUD).
- [ ] Full Auto-Detection Engine (`service/detection.go`) with platform + file detectors.
- [ ] Frontend: Feature-Map editor (visual drag-and-drop).
