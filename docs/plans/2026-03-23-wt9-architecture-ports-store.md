# WT-9: Architecture Port Abstractions & Store — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create missing port interfaces (filesystem, shell, LSP), migrate 12 service files to use ports instead of direct I/O, continue store.go decomposition, and add tests for the top-5 most-called store methods.

**Architecture:** Follow the existing port pattern in `internal/port/` — define interfaces, create OS-backed adapter implementations, inject via constructors. This enables testing services with mock filesystem/shell.

**Tech Stack:** Go 1.25, existing `internal/port/` pattern, `os` stdlib, `os/exec`

**Best Practice:**
- Hexagonal Architecture: Services MUST depend on ports (interfaces), never on adapters (implementations).
- Testing: Port interfaces enable mock injection — no real filesystem needed in tests.
- Gradual migration: Move one service at a time, verify tests pass between each.

---

### Task 1: Create Filesystem Port Interface

**Files:**
- Create: `internal/port/filesystem/provider.go`
- Create: `internal/adapter/osfs/provider.go`

- [ ] **Step 1: Define filesystem port interface**

```go
// internal/port/filesystem/provider.go
package filesystem

import (
    "context"
    "io"
    "io/fs"
)

// Provider abstracts filesystem operations for service layer decoupling.
type Provider interface {
    Stat(ctx context.Context, path string) (fs.FileInfo, error)
    ReadDir(ctx context.Context, path string) ([]fs.DirEntry, error)
    ReadFile(ctx context.Context, path string) ([]byte, error)
    WriteFile(ctx context.Context, path string, data []byte, perm fs.FileMode) error
    MkdirAll(ctx context.Context, path string, perm fs.FileMode) error
    Remove(ctx context.Context, path string) error
    RemoveAll(ctx context.Context, path string) error
    Rename(ctx context.Context, oldPath, newPath string) error
    Open(ctx context.Context, path string) (io.ReadCloser, error)
    Create(ctx context.Context, path string) (io.WriteCloser, error)
    WalkDir(ctx context.Context, root string, fn fs.WalkDirFunc) error
}
```

- [ ] **Step 2: Create OS-backed implementation**

```go
// internal/adapter/osfs/provider.go
package osfs

import (
    "context"
    "io"
    "io/fs"
    "os"
    "path/filepath"
)

type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Stat(_ context.Context, path string) (fs.FileInfo, error) {
    return os.Stat(path)
}

func (p *Provider) ReadDir(_ context.Context, path string) ([]fs.DirEntry, error) {
    return os.ReadDir(path)
}

func (p *Provider) ReadFile(_ context.Context, path string) ([]byte, error) {
    return os.ReadFile(path)
}

func (p *Provider) WriteFile(_ context.Context, path string, data []byte, perm fs.FileMode) error {
    return os.WriteFile(path, data, perm)
}

func (p *Provider) MkdirAll(_ context.Context, path string, perm fs.FileMode) error {
    return os.MkdirAll(path, perm)
}

func (p *Provider) Remove(_ context.Context, path string) error {
    return os.Remove(path)
}

func (p *Provider) RemoveAll(_ context.Context, path string) error {
    return os.RemoveAll(path)
}

func (p *Provider) Rename(_ context.Context, oldPath, newPath string) error {
    return os.Rename(oldPath, newPath)
}

func (p *Provider) Open(_ context.Context, path string) (io.ReadCloser, error) {
    return os.Open(path)
}

func (p *Provider) Create(_ context.Context, path string) (io.WriteCloser, error) {
    return os.Create(path)
}

func (p *Provider) WalkDir(_ context.Context, root string, fn fs.WalkDirFunc) error {
    return filepath.WalkDir(root, fn)
}
```

- [ ] **Step 3: Commit**

```bash
git add internal/port/filesystem/ internal/adapter/osfs/
git commit -m "feat: add filesystem port interface + OS adapter (hexagonal architecture)"
```

---

### Task 2: Create Shell/Command Port Interface

**Files:**
- Create: `internal/port/shell/commander.go`
- Create: `internal/adapter/execshell/commander.go`

- [ ] **Step 1: Define shell commander interface**

```go
// internal/port/shell/commander.go
package shell

import "context"

// Result holds the output of a command execution.
type Result struct {
    Stdout   string
    Stderr   string
    ExitCode int
}

// Commander abstracts os/exec for service layer decoupling.
type Commander interface {
    Run(ctx context.Context, dir string, name string, args ...string) (*Result, error)
    RunCombined(ctx context.Context, dir string, name string, args ...string) (string, error)
}
```

- [ ] **Step 2: Create exec-backed implementation**

```go
// internal/adapter/execshell/commander.go
package execshell

import (
    "bytes"
    "context"
    "fmt"
    "os/exec"

    "github.com/Strob0t/CodeForge/internal/port/shell"
)

type Commander struct{}

func New() *Commander { return &Commander{} }

func (c *Commander) Run(ctx context.Context, dir, name string, args ...string) (*shell.Result, error) {
    cmd := exec.CommandContext(ctx, name, args...)
    cmd.Dir = dir
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    err := cmd.Run()
    result := &shell.Result{
        Stdout: stdout.String(),
        Stderr: stderr.String(),
    }
    if exitErr, ok := err.(*exec.ExitError); ok {
        result.ExitCode = exitErr.ExitCode()
        return result, nil
    }
    return result, err
}

func (c *Commander) RunCombined(ctx context.Context, dir, name string, args ...string) (string, error) {
    cmd := exec.CommandContext(ctx, name, args...)
    cmd.Dir = dir
    out, err := cmd.CombinedOutput()
    return string(out), err
}
```

- [ ] **Step 3: Commit**

```bash
git add internal/port/shell/ internal/adapter/execshell/
git commit -m "feat: add shell commander port + exec adapter (hexagonal architecture)"
```

---

### Task 3: Create LSP Port Interface

**Files:**
- Create: `internal/port/lsp/provider.go`
- Modify: `internal/service/lsp.go` (use port instead of direct adapter import)

- [ ] **Step 1: Define LSP provider interface based on current adapter usage**

```go
// internal/port/lsp/provider.go
package lsp

import "context"

type Provider interface {
    Start(ctx context.Context, workDir string, language string) error
    Stop(ctx context.Context) error
    Definition(ctx context.Context, file string, line, col int) ([]Location, error)
    References(ctx context.Context, file string, line, col int) ([]Location, error)
    DocumentSymbols(ctx context.Context, file string) ([]Symbol, error)
    Hover(ctx context.Context, file string, line, col int) (string, error)
    Diagnostics(ctx context.Context, file string) ([]Diagnostic, error)
}

type Location struct {
    File  string
    Line  int
    Col   int
}

type Symbol struct {
    Name string
    Kind string
    Line int
}

type Diagnostic struct {
    File     string
    Line     int
    Severity string
    Message  string
}
```

- [ ] **Step 2: Update service/lsp.go to use port interface**

Remove `import lspAdapter "...internal/adapter/lsp"`.
Change `clients map[string]map[string]*lspAdapter.Client` to `clients map[string]map[string]lsp.Provider`.
Inject via constructor: `NewLSPService(providerFactory func(lang string) lsp.Provider)`.

- [ ] **Step 3: Have adapter/lsp/client.go implement the interface**

Verify `adapter/lsp.Client` satisfies `port/lsp.Provider`. Add any missing methods.

- [ ] **Step 4: Commit**

```bash
git add internal/port/lsp/ internal/service/lsp.go internal/adapter/lsp/
git commit -m "refactor: LSP service uses port interface instead of direct adapter import (F-028)"
```

---

### Task 4: Migrate FileService to Filesystem Port

**Files:**
- Modify: `internal/service/files.go`
- Modify: `cmd/codeforge/main.go`

- [ ] **Step 1: Add filesystem.Provider to FileService**

```go
type FileService struct {
    store database.Store
    fs    filesystem.Provider  // NEW
}

func NewFileService(store database.Store, fs filesystem.Provider) *FileService {
    return &FileService{store: store, fs: fs}
}
```

- [ ] **Step 2: Replace all direct os/filepath calls**

Replace `os.ReadDir(path)` with `fs.fs.ReadDir(ctx, path)`.
Replace `os.Stat(path)` with `fs.fs.Stat(ctx, path)`.
Replace `os.MkdirAll(path, perm)` with `fs.fs.MkdirAll(ctx, path, perm)`.
Replace `filepath.WalkDir(root, fn)` with `fs.fs.WalkDir(ctx, root, fn)`.

- [ ] **Step 3: Update main.go**

```go
osFS := osfs.New()
fileSvc := service.NewFileService(store, osFS)
```

- [ ] **Step 4: Run tests + commit**

```bash
go test ./internal/service/... -count=1
git add internal/service/files.go cmd/codeforge/main.go
git commit -m "refactor: FileService uses filesystem port instead of direct os calls (F-029)"
```

---

### Task 5: Migrate Remaining Services (best-effort, prioritized)

**Files:**
- Modify: `internal/service/benchmark.go` (os.Stat for datasets)
- Modify: `internal/service/context_optimizer.go` (os.ReadDir, os.Stat)
- Modify: `internal/service/envwriter.go` (os.Open, os.CreateTemp, etc.)
- Modify: `internal/service/goal_discovery.go` (os.Stat, os.ReadDir)
- Modify: `cmd/codeforge/main.go`

- [ ] **Step 1: Add filesystem.Provider to each service constructor**

For each service, add `fs filesystem.Provider` parameter and replace direct calls.

- [ ] **Step 2: Services using exec.CommandContext — add shell.Commander**

Migrate `project.go`, `checkpoint.go`, `sandbox.go`, `deliver.go`, `autoagent.go` to use `shell.Commander`.

- [ ] **Step 3: Update main.go wiring for all migrated services**

- [ ] **Step 4: Run full test suite**

```bash
go test ./internal/... -count=1
golangci-lint run ./internal/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/service/ cmd/codeforge/main.go
git commit -m "refactor: migrate remaining services to filesystem/shell ports (F-029)"
```

---

### Task 6: Continue store.go Decomposition

**Files:**
- Create: `internal/adapter/postgres/store_project.go`
- Create: `internal/adapter/postgres/store_roadmap.go`
- Create: `internal/adapter/postgres/store_cost.go`
- Create: `internal/adapter/postgres/store_plan.go`
- Create: `internal/adapter/postgres/store_branch_protection.go`
- Create: `internal/adapter/postgres/store_context_pack.go`
- Create: `internal/adapter/postgres/store_repo_map.go`
- Create: `internal/adapter/postgres/store_run.go`
- Create: `internal/adapter/postgres/store_team.go`
- Modify: `internal/adapter/postgres/store.go` (reduce from 1487 LOC)

- [ ] **Step 1: Move project methods (5) to store_project.go**

`ListProjects`, `GetProject`, `GetProjectByRepoName`, `CreateProject`, `UpdateProject`, `DeleteProject`

- [ ] **Step 2: Move roadmap/milestone/feature methods (18) to store_roadmap.go**

- [ ] **Step 3: Move cost methods (7) to store_cost.go**

- [ ] **Step 4: Move plan methods (8) to store_plan.go**

- [ ] **Step 5: Move remaining domain groups to their own files**

Run, Team, ContextPack, SharedContext, RepoMap, BranchProtection, Session.

- [ ] **Step 6: Verify store.go is reduced to ~200 LOC (struct + constructor + helpers)**

```bash
wc -l internal/adapter/postgres/store.go
```

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/postgres/
git commit -m "refactor: decompose store.go into domain-specific store files (F-017)"
```

---

### Task 7: Add Tests for Top-5 Most-Called Store Methods

**Files:**
- Modify: `internal/adapter/postgres/store_test.go`

- [ ] **Step 1: Write integration tests for top-5 methods**

Priority by caller count:
1. `GetRun` (61 callers)
2. `GetProject` (43 callers)
3. `GetAgent` (18 callers)
4. `GetTask` (11 callers)
5. `GetConversation` (11 callers)

Use table-driven tests with edge cases:
```go
func TestStore_GetRun(t *testing.T) {
    tests := []struct {
        name    string
        setup   func(t *testing.T, s *Store) string // returns run ID
        wantErr bool
    }{
        {"existing run", func(t *testing.T, s *Store) string { /* create run */ }, false},
        {"nonexistent run", func(t *testing.T, s *Store) string { return "nonexistent" }, true},
        {"wrong tenant", func(t *testing.T, s *Store) string { /* create in different tenant */ }, true},
    }
    // ...
}
```

- [ ] **Step 2: Run tests**

```bash
go test ./internal/adapter/postgres/... -count=1 -v
```

- [ ] **Step 3: Commit**

```bash
git add internal/adapter/postgres/store_test.go
git commit -m "test: add integration tests for top-5 most-called store methods (F-016)"
```
