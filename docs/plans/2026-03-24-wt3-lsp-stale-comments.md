# WT3: LSP Stale Comment Cleanup & Docker Configuration

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove stale TODO comments claiming LSP is "not wired" (it IS wired since main.go:373-383), and ensure language server binaries are available in the Docker image.

**Architecture:** LSP is already fully implemented and conditionally wired via `cfg.LSP.Enabled`. The adapter comments are outdated. This bundle is purely cleanup — no logic changes.

**Tech Stack:** Go, Docker

---

### Task 1: Update stale LSP comments

**Files:**
- Modify: `internal/adapter/lsp/client.go:4-6`
- Modify: `internal/adapter/lsp/doc.go:1-12`

- [ ] **Step 1: Update client.go package comment**

Replace lines 1-7 of `internal/adapter/lsp/client.go`:

```go
// Package lsp provides a Language Server Protocol client that manages a single
// language server process, communicating via JSON-RPC 2.0 over stdio.
//
// Activation: Set config LSP.Enabled=true (or CODEFORGE_LSP_ENABLED=true).
// The service is wired in cmd/codeforge/main.go and integrates with the
// context optimizer for diagnostic-aware agent conversations.
package lsp
```

- [ ] **Step 2: Update doc.go**

Replace all content of `internal/adapter/lsp/doc.go`:

```go
// Package lsp implements the Language Server Protocol client adapter.
//
// This adapter is fully implemented and wired into the application lifecycle
// (cmd/codeforge/main.go). It is conditionally enabled via config.LSP.Enabled.
//
// Supported language servers (internal/domain/lsp/language.go):
//   - Go: gopls serve
//   - Python: pyright-langserver --stdio
//   - TypeScript/JavaScript: typescript-language-server --stdio
//
// See also: internal/service/lsp.go (service layer), internal/port/lsp/ (port interfaces)
package lsp
```

- [ ] **Step 3: Verify compilation**

Run: `go build ./internal/adapter/lsp/...`
Expected: Success

- [ ] **Step 4: Commit**

```bash
git add internal/adapter/lsp/client.go internal/adapter/lsp/doc.go
git commit -m "docs(FIX-075): update stale LSP comments — adapter is already wired"
```

---

### Task 2: Add language server binaries to Dockerfile

**Files:**
- Modify: `Dockerfile` (or `docker/Dockerfile.core`)

- [ ] **Step 1: Find the Dockerfile**

Search for the main Dockerfile that builds the Go core service.

- [ ] **Step 2: Add language server binary installation**

Add an optional build stage or runtime install for language servers. Since these are only needed when LSP is enabled, use a conditional approach:

```dockerfile
# Optional: LSP language servers (only needed if LSP.Enabled=true)
# Install via multi-stage or at runtime via config
RUN if [ "${INSTALL_LSP_SERVERS:-false}" = "true" ]; then \
      go install golang.org/x/tools/gopls@latest && \
      pip install pyright && \
      npm install -g typescript-language-server typescript; \
    fi
```

If the Dockerfile doesn't support conditional installs, add a comment documenting how to install them:

```dockerfile
# LSP servers (optional, enable via CODEFORGE_LSP_ENABLED=true):
#   go install golang.org/x/tools/gopls@latest
#   pip install pyright
#   npm install -g typescript-language-server typescript
```

- [ ] **Step 3: Commit**

```bash
git add Dockerfile
git commit -m "docs: add LSP language server install instructions to Dockerfile"
```

---

### Task 3: Add LSP config to example config

**Files:**
- Modify: `codeforge.example.yaml`

- [ ] **Step 1: Add LSP section if missing**

Check if `codeforge.example.yaml` already has an LSP section. If not, add:

```yaml
# Language Server Protocol integration (Phase 15D)
# Provides code intelligence (go-to-def, references, diagnostics) for agent conversations.
# Requires language server binaries installed (gopls, pyright, typescript-language-server).
lsp:
  enabled: false           # Enable LSP integration
  auto_start: true         # Auto-start servers on project setup
  start_timeout: 30s       # Max time for server initialization
  shutdown_timeout: 10s    # Graceful shutdown timeout
  diagnostic_delay: 500ms  # Debounce delay for diagnostic broadcasts
  max_diagnostics: 100     # Max diagnostics cached per file
```

- [ ] **Step 2: Commit**

```bash
git add codeforge.example.yaml
git commit -m "docs: add LSP configuration section to example config"
```
