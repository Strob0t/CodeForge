# WT-1: Handlers Struct Decomposition — Implementation Plan (PRIORITY)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the monolithic `Handlers` struct (61 fields, 51 methods in `handlers.go`) into domain-specific handler groups. The 37 existing handler files stay — only `handlers.go` (the monolith with 51 methods) gets decomposed.

**Architecture:** Create domain-specific handler structs that each hold only their required service dependencies. The main `Handlers` struct becomes a composition of these groups. Route registration delegates to each group.

**Tech Stack:** Go 1.25, chi v5 router

**Best Practice:** Single Responsibility — each handler group owns one domain (projects, agents, tasks, runs, policies). Handler groups inject only the services they need, reducing coupling. Composition over inheritance via struct embedding or delegation.

---

## Current State

`handlers.go` contains 51 handler methods for 6 domains mixed together:
- **Projects** (18): CRUD, git ops, workspace, stack detection
- **Agents** (8): CRUD, dispatch, inbox, state
- **Tasks** (5): CRUD, claim
- **Runs** (8): start, cancel, get, list, events
- **Policies** (6): CRUD, evaluate, allow-always
- **Utility** (6): ParseRepoURL, FetchRepoInfo, ListProviders, DetectStack, GetAgentConfig, ListActiveWork

The other 36 handler files (`handlers_conversation.go`, `handlers_benchmark.go`, etc.) are already properly separated.

---

### Task 1: Create ProjectHandlers

**Files:**
- Create: `internal/adapter/http/handlers_project.go`
- Modify: `internal/adapter/http/handlers.go` (remove project methods)

- [ ] **Step 1: Create ProjectHandlers struct**

```go
// internal/adapter/http/handlers_project.go
package http

type ProjectHandlers struct {
    Projects         *service.ProjectService
    Files            *service.FileService
    BranchProtection *service.BranchProtectionService
    Checkpoint       *service.CheckpointService
    AppEnv           string
}
```

- [ ] **Step 2: Move 18 project handler methods from handlers.go**

Move these methods, changing receiver from `(h *Handlers)` to `(ph *ProjectHandlers)`:
- `ListProjects`, `GetProject`, `CreateProject`, `DeleteProject`, `UpdateProject`
- `CloneProject`, `AdoptProject`, `SetupProject`, `InitWorkspace`
- `ProjectGitStatus`, `ProjectStatus`, `PullProject`, `ListProjectBranches`, `CheckoutBranch`
- `GetWorkspaceInfo`, `DetectProjectStack`, `DetectStackByPath`
- `ParseRepoURL`, `FetchRepoInfo`

Update service references: `h.Projects` -> `ph.Projects`

- [ ] **Step 3: Verify compilation**

```bash
go build ./internal/adapter/http/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/adapter/http/handlers_project.go internal/adapter/http/handlers.go
git commit -m "refactor: extract ProjectHandlers from monolithic Handlers struct"
```

---

### Task 2: Create AgentHandlers

**Files:**
- Create: `internal/adapter/http/handlers_agent.go`
- Modify: `internal/adapter/http/handlers.go`

- [ ] **Step 1: Create struct + move 8 methods**

```go
type AgentHandlers struct {
    Agents    *service.AgentService
    Runtime   *service.RuntimeService
}
```

Methods: `ListAgents`, `CreateAgent`, `GetAgent`, `DeleteAgent`, `DispatchTask`, `StopAgentTask`, `ListAgentInbox`, `SendAgentMessage`, `MarkInboxRead`, `GetAgentState`, `UpdateAgentState`

- [ ] **Step 2: Verify + commit**

```bash
go build ./internal/adapter/http/...
git add internal/adapter/http/
git commit -m "refactor: extract AgentHandlers from monolithic Handlers struct"
```

---

### Task 3: Create TaskHandlers

**Files:**
- Create: `internal/adapter/http/handlers_task.go`
- Modify: `internal/adapter/http/handlers.go`

- [ ] **Step 1: Create struct + move 5 methods**

```go
type TaskHandlers struct {
    Tasks     *service.TaskService
    ActiveWork *service.ActiveWorkService
}
```

Methods: `ListTasks`, `CreateTask`, `GetTask`, `ClaimTask`, `ListTaskEvents`, `ListActiveWork`, `ListActiveAgents`

- [ ] **Step 2: Verify + commit**

---

### Task 4: Create RunHandlers

**Files:**
- Create: `internal/adapter/http/handlers_run.go`
- Modify: `internal/adapter/http/handlers.go`

- [ ] **Step 1: Create struct + move 8 methods**

```go
type RunHandlers struct {
    Runtime *service.RuntimeService
    Events  eventstore.Store
}
```

Methods: `StartRun`, `CancelRun`, `GetRun`, `ListRunEvents`, `ListTaskRuns`

- [ ] **Step 2: Verify + commit**

---

### Task 5: Create PolicyHandlers

**Files:**
- Create: `internal/adapter/http/handlers_policy.go`
- Modify: `internal/adapter/http/handlers.go`

- [ ] **Step 1: Create struct + move 6 methods**

```go
type PolicyHandlers struct {
    Policies  *service.PolicyService
    PolicyDir string
}
```

Methods: `ListPolicyProfiles`, `GetPolicyProfile`, `CreatePolicyProfile`, `DeletePolicyProfile`, `EvaluatePolicy`, `AllowAlwaysPolicy`

- [ ] **Step 2: Verify + commit**

---

### Task 6: Create UtilityHandlers for remaining methods

**Files:**
- Create: `internal/adapter/http/handlers_utility.go`
- Modify: `internal/adapter/http/handlers.go`

- [ ] **Step 1: Move remaining methods**

```go
type UtilityHandlers struct {
    LLM           llmFull
    AgentConfig   *config.Agent
    OllamaBaseURL string
}
```

Methods: `ListGitProviders`, `ListAgentBackends`, `GetAgentConfig` (and any remaining utility methods)

- [ ] **Step 2: Verify handlers.go is now empty of methods (only struct + constructor)**

---

### Task 7: Refactor Handlers Struct to Compose Handler Groups

**Files:**
- Modify: `internal/adapter/http/handlers.go`
- Modify: `internal/adapter/http/routes.go`
- Modify: `cmd/codeforge/main.go`

- [ ] **Step 1: Update Handlers struct to embed handler groups**

```go
type Handlers struct {
    // Embedded domain-specific handler groups
    Project  *ProjectHandlers
    Agent    *AgentHandlers
    Task     *TaskHandlers
    Run      *RunHandlers
    Policy   *PolicyHandlers
    Utility  *UtilityHandlers

    // Existing separated handlers (already in their own files)
    Conversations    *service.ConversationService
    Auth             *service.AuthService
    Benchmarks       *service.BenchmarkService
    // ... (keep all other fields that are used by existing handler files)
}
```

- [ ] **Step 2: Update routes.go**

Change route registrations from `h.CreateProject` to `h.Project.CreateProject`:
```go
r.Post("/projects", h.Project.CreateProject)
r.Get("/projects", h.Project.ListProjects)
// etc.
```

- [ ] **Step 3: Update main.go wiring**

```go
handlers := &cfhttp.Handlers{
    Project: &cfhttp.ProjectHandlers{
        Projects:         projectSvc,
        Files:            fileSvc,
        BranchProtection: branchProtSvc,
        Checkpoint:       checkpointSvc,
        AppEnv:           cfg.AppEnv,
    },
    Agent: &cfhttp.AgentHandlers{
        Agents:  agentSvc,
        Runtime: runtimeSvc,
    },
    // ... etc
}
```

- [ ] **Step 4: Run full test suite**

```bash
go test ./internal/adapter/http/... ./cmd/... -count=1
pre-commit run --all-files
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/http/ cmd/codeforge/main.go
git commit -m "refactor: Handlers struct now composes domain-specific handler groups (F-026)"
```

---

### Task 8: Consolidate Small Handler Files

**Files:**
- Modify: `internal/adapter/http/handlers_llm.go` (merge `handlers_llm_keys.go`)
- Modify: `internal/adapter/http/handlers_agent_features.go` (merge `handlers_skill_import.go`)
- Modify: `internal/adapter/http/handlers_conversation.go` (merge `handlers_commands.go`)
- Modify: `internal/adapter/http/handlers_benchmark.go` (merge `handlers_benchmark_analyze.go`)
- Delete: the merged files

- [ ] **Step 1: Merge 4 small files into their parent handler files**

- [ ] **Step 2: Verify + commit**

```bash
go build ./internal/adapter/http/...
git add internal/adapter/http/
git commit -m "refactor: consolidate small handler files into parent groups"
```
