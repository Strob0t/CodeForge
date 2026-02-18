# Feature: Project Dashboard (Pillar 1)

> **Status:** Design phase
> **Priority:** Phase 2 (MVP)
> **Architecture reference:** [architecture.md](../architecture.md) — "Core Service (Go)" section

## Overview

Management of multiple repositories across different SCM platforms.
Users can add, remove, monitor, and interact with repositories from a unified dashboard.

## Supported SCM Providers

| Provider | Adapter | Key Capabilities |
|---|---|---|
| GitHub | `adapter/github/` | Clone, PR, Webhooks, Issues, Actions |
| GitLab | `adapter/gitlab/` | Clone, MR, Webhooks, Issues, CI |
| Git (local) | `adapter/gitlocal/` | Clone, Branch, Diff, Commit |
| SVN | `adapter/svn/` | Checkout, Update, Diff, Commit |
| Gitea/Forgejo | `adapter/github/` (compatible) | Same as GitHub with minor adjustments |
| Codeberg | `adapter/github/` (compatible) | Forgejo instance, same adapter as Gitea/Forgejo |

All providers implement the `gitprovider.Provider` interface with capability declarations.
See [architecture.md — Provider Registry Pattern](../architecture.md#provider-registry-pattern).

## Core Functionality

### Repository Management
- Add repository by URL (auto-detect provider type)
- Clone/checkout to local workspace
- Display repository status (branch, last commit, dirty state)
- Pull/fetch updates
- Switch branches

### Status Overview
- List all managed projects with health indicators
- Show agent activity per project
- Show recent changes and commits
- Quick actions (pull, branch, run agent)

### Multi-Repo Operations
- Batch operations across selected repos
- Cross-repo search (code, issues)
- Dependency graph between repos (future)

## User Stories

1. As a user, I can add a GitHub repo by pasting its URL
2. As a user, I can see all my repos in a dashboard with their current status
3. As a user, I can add a local git directory as a project
4. As a user, I can add an SVN repository and work with it like a git repo
5. As a user, I can pull updates for all repos at once
6. As a user, I can add a Forgejo or Codeberg repo by pasting its URL

## Design Decisions

- **Provider Registry Pattern** — new SCM providers are added via blank import, no core changes
- **Capability-based** — SVN doesn't support webhooks/PRs, that's declared behavior not an error
- **Compliance Tests** — every provider adapter gets the same test suite automatically

## API Endpoints (Planned)

```
GET    /api/v1/projects                    # List all projects
POST   /api/v1/projects                    # Add project (clone/checkout)
GET    /api/v1/projects/{id}               # Project details
DELETE /api/v1/projects/{id}               # Remove project
POST   /api/v1/projects/{id}/pull          # Pull/fetch updates
GET    /api/v1/projects/{id}/status        # Git/SVN status
GET    /api/v1/projects/{id}/branches      # List branches
POST   /api/v1/projects/{id}/checkout      # Switch branch
```

## TODOs

Tracked in [todo.md](../todo.md) under Phase 1 and Phase 2.

- [ ] Implement `gitprovider.Provider` interface
- [ ] Implement GitHub adapter with OAuth
- [ ] Implement Git local adapter
- [ ] Implement SVN adapter (CLI wrapper)
- [ ] Verify GitHub adapter compatibility with Forgejo/Codeberg (base URL override, API differences)
- [ ] HTTP endpoints for project CRUD
- [ ] Frontend: Project list component
- [ ] Frontend: Add project dialog (URL input, auto-detect)
- [ ] Frontend: Project status card
