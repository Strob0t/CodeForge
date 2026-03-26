# Worktree 7: refactor/layer-violations — Hexagonal-Verstösse bereinigen

**Branch:** `refactor/layer-violations`
**Priority:** Mittel
**Scope:** 3 findings (F-ARC-005, F-ARC-006, F-ARC-007)
**Estimated effort:** Medium (3-5 days)

## Research Summary

- `io/fs.FS` (Go 1.16+) + `testing/fstest.MapFS` for in-memory test filesystems
- Existing `filesystem.Provider` port already in codebase (3 services use it)
- `prompt.LoadFS()` already uses `fs.FS` — proven pattern
- Fat Service / Thin Handler (Alex Edwards, Three Dots Labs)
- Domain-specific port interfaces > generic HTTP `Doer` (better abstraction)
- [go-cleanarch](https://github.com/roblaszczak/go-cleanarch) linter for CI enforcement

## Steps (in recommended order)

### Phase 1: Domain fs.FS refactoring (lowest risk)

**1a. `pipeline/loader.go`** — Change `LoadFromFile(path)` → `LoadFromFile(fsys fs.FS, name string)`
**1b. `policy/loader.go`** — Same for reads. For `SaveToFile`, define minimal `FileWriter` interface in domain:
```go
type FileWriter interface {
    WriteFile(path string, data []byte, perm fs.FileMode) error
}
```
**1c. `microagent/loader.go`** — Change `LoadFromDirectory(dir)` → `LoadFromDirectory(fsys fs.FS)`
**1d. `project/scan.go`** — Change to accept `fs.FS` for all read operations

**Callers** pass `os.DirFS(dir)` in production. Tests use `fstest.MapFS`:
```go
testFS := fstest.MapFS{
    "pipeline.yaml": &fstest.MapFile{Data: []byte("...")},
}
```

### Phase 2: Handler → Service extraction

**2a. `autoIndexProject`** → `ProjectService.AutoIndex(ctx, projectID, workspacePath)`
- Move 4-goroutine orchestration from `handlers_project.go:161-197` to service
- Handler calls `ph.Projects.AutoIndex(r.Context(), id, p.WorkspacePath)`

**2b. `ListRemoteBranches`** → `gitprovider.Provider.ListRemoteBranches(ctx, repoURL)`
- Extend port interface
- Implement in existing git CLI adapter (`internal/git/`)
- Handler delegates entirely, URL validation moves to service

### Phase 3: Service HTTP client ports

**3a. `vcsaccount.go`** — Define `port/vcsvalidator/validator.go`:
```go
type Validator interface {
    ValidateToken(ctx context.Context, provider, serverURL, token string) error
}
```
Implement in `adapter/vcsclient/validator.go`.

**3b. `project.go` `fetchJSON`** — Define `port/repoinfo/fetcher.go`:
```go
type Fetcher interface {
    FetchRepoInfo(ctx context.Context, repoURL string) (*project.RepoInfo, error)
}
```

**3c. `a2a.go` webhook** — Define small `WebhookSender` port.

**3d. `github_oauth.go`** — Lowest priority (already uses `*http.Client` field, tested with `httptest`).

### Phase 4: CI enforcement

Add `go-cleanarch` to CI pipeline to prevent future layer violations.

## Verification

- All loaders work with `fstest.MapFS` in tests (no temp dirs needed)
- `autoIndexProject` callable from non-HTTP context (NATS, CLI)
- `vcsaccount` testable with fake validator (no HTTP mocking)
- `go-cleanarch` passes in CI

## Sources

- [Bitfield: Walking with fs.FS](https://bitfieldconsulting.com/posts/filesystems)
- [Alex Edwards: Fat Service Pattern](https://www.alexedwards.net/blog/the-fat-service-pattern)
- [Three Dots Labs: Clean Architecture in Go](https://threedots.tech/post/introducing-clean-architecture/)
- [Go fs package: Modern File System Abstraction](https://dev.to/rezmoss/gos-fs-package-modern-file-system-abstraction-19-5aad)
- [Hexagonal Architecture in Go (Rafiul Alam)](https://alamrafiul.com/posts/go-hexagonal-architecture/)
