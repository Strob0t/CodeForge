# Feature: Project Dashboard (Pillar 1)

> Status: Foundation implemented (Phase 1-2) -- Git local provider, project CRUD, frontend dashboard
> Priority: Phase 2 (MVP) completed; Phase 9+ for GitHub/SVN/Forgejo adapters
> Architecture reference: [architecture.md](../architecture.md) -- "Core Service (Go)" section

### Purpose

Management of multiple repositories across different SCM platforms. Users can add, remove, monitor, and interact with repositories from a **unified** dashboard.

### Supported SCM Providers

| Provider | Adapter | Key Capabilities |
|---|---|---|
| GitHub (PM) | `adapter/githubpm/` | Issues, PRs, Webhooks, Actions |
| GitHub (API) | `adapter/github/` | Clone (token-auth), ListRepos, Push, PRs, Issues |
| GitLab | `adapter/gitlab/` | Clone, MR, Webhooks, Issues, CI |
| Git (local) | `adapter/gitlocal/` | Clone, Branch, Diff, Commit |
| SVN | `adapter/svn/` | Checkout, Update, Diff, Commit |
| Gitea/Forgejo | `adapter/gitea/` | Issues, PRs (via Gitea REST API) |
| Codeberg | `adapter/gitea/` (variant) | Forgejo instance, same adapter as Gitea/Forgejo |

All providers implement the `gitprovider.Provider` interface with capability declarations. See [architecture.md -- Provider Registry Pattern](../architecture.md#provider-registry-pattern).

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
- Compliance Tests give every provider adapter the same test suite automatically.

### API Endpoints (Implemented)

```text
GET    /api/v1/projects                    # List all projects
POST   /api/v1/projects                    # Add project (clone/checkout)
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
- [x] GitHub OAuth: domain model (`vcsaccount`), OAuth state store, service (`GitHubOAuthService`), HTTP handlers (`/api/v1/auth/github`, `/api/v1/auth/github/callback`).
- [x] GitHub API git provider (`adapter/github/`): token-auth clone URLs, ListRepos via REST API with pagination, self-registering as `github-api`.
- [x] Frontend OAuth: "Connect GitHub" button in Settings > VCS Accounts, redirects to GitHub OAuth flow.
- [x] Forgejo/Codeberg compatibility: Gitea adapter with variant config, `DetectForgejo()`, provider aliases (`forgejo`, `codeberg`), frontend dropdown in CreateProjectModal.
- [x] Batch operations: `POST /projects/batch/{delete,pull,status}` endpoints, concurrent fan-out, frontend multi-select with batch action bar.
- [x] Cross-repo search: `POST /search` aggregation endpoint, frontend SearchPage with debounced input, project filter, results with code snippets.

### UX/UI Improvements (2026-03-18)

- [x] **Project card hover effects:** Cards lift with shadow on hover and are fully clickable to navigate to the project detail page.
- [x] **KPI mobile abbreviations:** Dashboard KPI labels abbreviate on narrow viewports (e.g., "Success Rate" -> "Success") to prevent overflow.
- [x] **Empty state illustrations:** SVG illustrations on 6 pages (MCP, Knowledge, Benchmarks, Prompts, Activity, Costs) replace blank content when no data exists.
- [x] **Skeleton loaders:** AI Config, Costs, and Settings pages show skeleton loaders instead of "Loading..." text.
- [x] **Per-panel ErrorBoundary:** Project Detail page wraps each tab panel in an ErrorBoundary for graceful degradation -- a single broken panel does not crash the entire page.

### Open Items

> **Task tracking:** See [docs/todo.md](../todo.md) for current open items related to Project Dashboard.
