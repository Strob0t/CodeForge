# Feature: Project Dashboard (Pillar 1)

> Status: Implemented -- local Git, SVN and GitHub-API git providers; GitHub/GitLab/Gitea (Forgejo, Codeberg) issue adapters; project CRUD, batch operations, dashboard
> Priority: Phase 2 (MVP) completed; GitHub API, SVN and Forgejo/Codeberg adapters implemented
> Architecture reference: [architecture.md](../architecture.md) -- "Core Service (Go)" section

### Purpose

Management of multiple repositories across different SCM platforms. Users can add, remove, monitor, and interact with repositories from a **unified** dashboard.

### Supported SCM Providers

| Provider | Adapter | Key Capabilities |
|---|---|---|
| GitHub (PM) | `adapter/githubpm/` | Issue list/get/create/update via `gh` CLI. Planned: PRs, Webhooks, Actions |
| GitHub (API) | `adapter/github/` | Token-auth clone, ListRepos (REST), status/pull/branches/checkout via git CLI. Push, PRs, Issues are declared as capabilities but not implemented |
| GitLab | `adapter/gitlab/` | Issue CRUD via REST API v4 (clone goes through the local git provider). Planned: MR, Webhooks, CI |
| Git (local) | `adapter/gitlocal/` | Clone, Status, Pull, ListBranches, Checkout. Planned: Diff, Commit |
| SVN | `adapter/svn/` | Checkout, Status, Update, ListBranches, Switch. Planned: Diff, Commit |
| Gitea/Forgejo | `adapter/gitea/` | Issue CRUD via Gitea REST API. Planned: PRs |
| Codeberg | `adapter/gitea/` (variant) | Forgejo instance, same adapter as Gitea/Forgejo |

Repository operations go through `gitprovider.Provider` adapters: `gitlocal` (registered as `local`, `github`, `gitlab`, `gitea`), `github-api` and `svn`. The GitHub Issues, GitLab and Gitea/Forgejo/Codeberg adapters implement `pmprovider.Provider` (issue CRUD). Both kinds declare capabilities. Webhook ingress for GitHub/GitLab/Plane is served by the Core (`POST /api/v1/webhooks/{vcs,pm}/...`), not by these adapters. See [architecture.md -- Provider Registry Pattern](../architecture.md#provider-registry-pattern).

### Core Functionality

#### Repository Management

- Add repository by URL (auto-detect provider type).
- Clone/checkout to local workspace.
- Create empty project with auto-workspace (`git init`, no repo URL or path needed).
- Display repository status (branch, last commit, dirty state).
- Pull/fetch updates.
- Switch branches.

#### Status Overview

- List all managed projects with health indicators.
- Show agent activity per project.
- Show recent changes and commits.
- Quick actions (pull, branch, run agent).

#### Multi-Repo Operations

- Batch operations across selected repos.
- Cross-repo search (code, issues).
- Dependency graph between repos (future).

### User Stories

1. As a user, I can add a GitHub repo by pasting its URL.
2. As a user, I can see all my repos in a dashboard with their current status.
3. As a user, I can add a local git directory as a project.
3b. As a user, I can create an empty project without specifying a path or repo URL.
4. As a user, I can add an SVN repository and work with it like a git repo.
5. As a user, I can pull updates for all repos at once.
6. As a user, I can add a Forgejo or Codeberg repo by pasting its URL.

### Design Decisions

- **Provider Registry Pattern** -- new SCM providers are added via blank import, no core changes.
- Capability-based design means SVN does not support webhooks/PRs, and that is declared behavior not an error.
- Compliance Tests are intended to give every provider adapter the same test suite automatically (planned for git/PM/spec providers; today only the cache port has one, `internal/port/cache/cache_test.go`, and each provider adapter has its own `provider_test.go`).

### API Endpoints (Implemented)

```text
GET    /api/v1/projects                    # List all projects
POST   /api/v1/projects                    # Create project record (adopts local_path if given)
GET    /api/v1/projects/remote-branches    # List remote branches for a URL
POST   /api/v1/projects/{id}/clone         # Clone repo into workspace
POST   /api/v1/projects/{id}/setup         # Clone + detect stack + import specs
POST   /api/v1/projects/{id}/adopt         # Adopt existing local path
POST   /api/v1/projects/{id}/init-workspace # Empty workspace (git init)
GET    /api/v1/projects/{id}/workspace     # Workspace info
GET    /api/v1/projects/{id}               # Project details
PUT    /api/v1/projects/{id}               # Update project
DELETE /api/v1/projects/{id}               # Remove project
POST   /api/v1/projects/{id}/git/pull      # Pull/fetch updates
GET    /api/v1/projects/{id}/git/status    # Git/SVN status
GET    /api/v1/projects/{id}/git/branches  # List branches
POST   /api/v1/projects/{id}/git/checkout  # Switch branch
```

### Completed (Phase 1-2)

- [x] `gitprovider.Provider` interface with capability declarations (`internal/port/gitprovider/`).
- [x] Git local adapter (`internal/adapter/gitlocal/`) -- Clone, Status, Pull, ListBranches, Checkout via git CLI.
- [x] HTTP endpoints for project CRUD (REST API).
- [x] Frontend: Project list component, project detail page.
- [x] Frontend: Add project dialog (URL input).
- [x] Frontend: Project status card with git operations UI.
- [x] Optimistic locking (version field) on projects.
- [x] Multi-tenancy preparation (tenant_id on projects).
- [x] Dashboard Polish: KPI strip (7 stats), HealthDot (weighted composite), ChartsPanel (5 Unovis charts), ActivityTimeline (WS 5-tier), ProjectCard enhanced, CreateProjectModal extracted.
- [x] GitHub OAuth: domain model (`vcsaccount`), OAuth state store, service (`GitHubOAuthService`), HTTP handlers (`/api/v1/auth/github`, `/api/v1/auth/github/callback`). Not wired: `GitHubOAuthService` is never constructed, so both routes return 501 (see [Known Issues](../todo.md#known-issues) KI-55).
- [x] GitHub API git provider (`adapter/github/`): token-auth clone URLs, ListRepos via REST API with pagination, self-registering as `github-api`.
- [x] Frontend OAuth: "Connect GitHub" button in Settings > VCS Accounts, redirects to GitHub OAuth flow (fails until KI-55 is fixed).
- [x] Forgejo/Codeberg compatibility: Gitea adapter with variant config, `DetectForgejo()`, provider aliases (`forgejo`, `codeberg`), Forgejo/Codeberg options in Settings > VCS Accounts (`VCSSection.tsx`).
- [x] Batch operations: `POST /projects/batch/{delete,pull,status}` endpoints, concurrent fan-out, frontend multi-select with batch action bar.
- [x] Cross-repo search: `POST /search` aggregation endpoint, frontend SearchPage with debounced input, project filter, results with code snippets. Not reachable: `SearchPage` has no route since the sidebar restructure (c6831971).

### UX/UI Improvements (2026-03-18)

- [x] **Project card hover effects:** Cards lift with shadow on hover and are fully clickable to navigate to the project detail page.
- [x] **KPI mobile abbreviations:** Dashboard KPI labels abbreviate on narrow viewports (e.g., "Success Rate" -> "Success") to prevent overflow.
- [x] **Empty state illustrations:** SVG illustrations on 6 pages (MCP, Knowledge, Benchmarks, Prompts, Activity, Costs) replace blank content when no data exists.
- [x] **Skeleton loaders:** AI Config, Costs, and Settings pages show skeleton loaders instead of "Loading..." text.
- [x] **Per-panel ErrorBoundary:** Project Detail page wraps each tab panel in an ErrorBoundary for graceful degradation -- a single broken panel does not crash the entire page.

### Open Items

> **Task tracking:** See [docs/todo.md](../todo.md) for current open items related to Project Dashboard.
