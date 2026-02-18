# Feature: Roadmap/Feature-Map (Pillar 2)

> **Status:** Design phase
> **Priority:** Phase 3 (Advanced)
> **Architecture reference:** [architecture.md](../architecture.md) — "Roadmap/Feature-Map: Auto-Detection & Adaptive Integration"

## Overview

Visual management of project roadmaps and feature maps. Compatible with OpenSpec and
other spec-driven development tools. Bidirectional sync with external PM platforms.
**No proprietary PM tool** — CodeForge integrates with existing tools.

## Core Principle

CodeForge automatically detects which spec tools, PM platforms, and roadmap artifacts
are used in a project, then offers appropriate integration.

## Three-Tier Auto-Detection

1. **Spec-Driven Detectors** (repo files): OpenSpec (`openspec/`), Spec Kit (`.specify/`), Autospec (`specs/spec.yaml`), ADR/RFC
2. **Platform Detectors** (API-based): GitHub Issues, GitLab Issues, Plane.so, OpenProject
3. **File-Based Detectors** (simple markers): ROADMAP.md, TASKS.md, backlog/, CHANGELOG.md

Each detector implements `specprovider.SpecProvider` or `pmprovider.PMProvider` and
self-registers via `init()`. Same pattern as git providers.

## Supported Integrations

### Spec Providers

| Provider | Adapter | Detection |
|---|---|---|
| OpenSpec | `adapter/openspec/` | `openspec/` directory |
| GitHub Spec Kit | `adapter/speckit/` | `.specify/` directory |
| Autospec | `adapter/autospec/` | `specs/spec.yaml` file |

### PM Providers

| Provider | Adapter | Sync Method |
|---|---|---|
| Plane.so | `adapter/plane/` | REST API v1, Webhooks, HMAC-SHA256 |
| OpenProject | `adapter/openproject/` | REST API v3, Optimistic Locking |
| GitHub Issues/Projects | `adapter/github_pm/` | REST + GraphQL |
| GitLab Issues/Boards | `adapter/gitlab_pm/` | REST + GraphQL |
| Forgejo/Codeberg Issues | `adapter/github_pm/` (compatible) | REST API (GitHub-compatible) |

## Bidirectional Sync

```
CodeForge Roadmap Model  <-->  External PM (Plane, GitHub, OpenProject)
         |
         <-->  Repo Specs (OpenSpec, Spec Kit, Autospec)
```

- **Import:** PM tool items become CodeForge features/tasks
- **Export:** New features created as PM issues
- **Conflict resolution:** Timestamp-based + user decision
- **Sync triggers:** Webhook (real-time), poll (periodic), manual

## Internal Data Model

- `Milestone` → contains Features → contains Tasks
- `Feature` has: Labels (for sync), SpecRef (link to spec file), ExternalIDs (PM mappings)
- Optimistic Locking (from OpenProject) for concurrent edits

## `/ai` Endpoint

LLM-optimized roadmap format for AI agents:
```
GET /api/v1/projects/{id}/roadmap/ai?format=json|yaml|markdown
```

## Adopted Patterns

- Plane: Cursor Pagination, HMAC-SHA256 webhook verification, Label-triggered Sync
- OpenProject: Optimistic Locking, Schema Endpoints
- OpenSpec: Delta Spec Format (incremental changes)
- Ploi Roadmap: `/ai` endpoint for LLM consumption

## TODOs

Tracked in [todo.md](../todo.md) under Phase 3.

- [ ] Implement `specprovider.SpecProvider` interface
- [ ] Implement `pmprovider.PMProvider` interface
- [ ] OpenSpec adapter (read/write specs)
- [ ] Plane.so adapter (REST API, webhooks)
- [ ] GitHub PM adapter (Issues, Projects)
- [ ] Forgejo/Codeberg PM adapter (Issues — reuse GitHub PM adapter with base URL override)
- [ ] Auto-Detection Engine (`service/detection.go`)
- [ ] Bidirectional Sync Service (`service/sync.go`)
- [ ] Roadmap domain model (`domain/roadmap/`)
- [ ] Frontend: Roadmap visualization component
- [ ] Frontend: Feature-Map editor
- [ ] `/ai` endpoint for LLM consumption
