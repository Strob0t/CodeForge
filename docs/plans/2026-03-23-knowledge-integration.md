# Knowledge System Integration — Implementation Plan

> Steps use checkbox (`- [ ]`) syntax. Execute task-by-task, commit after each.

**Goal:** Make docs-mcp-server usable from CodeForge projects via UI assignment.

**Spec:** `docs/specs/2026-03-23-knowledge-integration-design.md`

---

## Task 1: docs-mcp-server Docker-Config vervollständigen (M2)

**Files:**
- Modify: `docker-compose.yml`

- [ ] **Step 1: Update docs-mcp environment for Ollama embeddings**

Add/change environment variables:
```yaml
environment:
  OPENAI_API_KEY: "ollama"
  OPENAI_API_BASE: "http://host.docker.internal:11434/v1"
  DOCS_MCP_EMBEDDING_MODEL: "nomic-embed-text"
```

- [ ] **Step 2: Expose Web Dashboard port**

Add port 6281 for the management Web-UI:
```yaml
ports:
  - "6280:6280"   # MCP Server (SSE/HTTP transport)
  - "6281:6281"   # Web Dashboard
```

- [ ] **Step 3: Fix health check**

The current health check may not work. Use the SSE endpoint:
```yaml
healthcheck:
  test: ["CMD", "node", "-e", "fetch('http://localhost:6280/sse').then(r=>{if(!r.ok)throw r}).catch(()=>process.exit(1))"]
```

- [ ] **Step 4: Commit**

```bash
git commit -m "fix: docs-mcp-server Ollama embeddings + Web Dashboard port"
```

---

## Task 2: MCP Server Project Assignment UI (M1)

**Files:**
- Modify: `frontend/src/features/project/CompactSettingsPopover.tsx`

**Context:** Backend API + Frontend API Client already exist and work:
- `api.mcp.listServers()` — all global MCP servers
- `api.mcp.listProjectServers(projectId)` — servers assigned to this project
- `api.mcp.assignToProject(projectId, serverId)` — assign
- `api.mcp.unassignFromProject(projectId, serverId)` — unassign

Types already defined: `MCPServer` interface in `api/types.ts`

- [ ] **Step 1: Read CompactSettingsPopover.tsx to understand structure**

Find where to add the MCP section (between autonomy field and cost summary).

- [ ] **Step 2: Add MCP Server assignment section**

Add after the autonomy level field:
- Section header: "MCP Servers"
- On mount: fetch `listServers()` and `listProjectServers(projectId)`
- For each global server: checkbox (checked if assigned to project)
- On toggle: call `assignToProject()` or `unassignFromProject()`
- Show server name + status indicator (enabled/disabled)
- Show count: "N of M servers assigned"

Use SolidJS patterns (createSignal, createResource, For, Show) — NOT React.

```tsx
// Pseudo-structure:
<div class="settings-section">
  <h4>MCP Servers</h4>
  <p class="text-sm text-muted">Assign documentation and tool servers to this project.</p>
  <Show when={!serversLoading()} fallback={<p>Loading...</p>}>
    <For each={allServers()}>
      {(server) => (
        <label class="flex items-center gap-2">
          <input
            type="checkbox"
            checked={isAssigned(server.id)}
            onChange={() => toggleServer(server.id)}
          />
          <span>{server.name}</span>
          <span class={server.enabled ? "status-dot green" : "status-dot red"} />
        </label>
      )}
    </For>
  </Show>
</div>
```

- [ ] **Step 3: Test in browser — verify checkboxes render and toggle works**

Open project settings, verify:
- All global MCP servers listed
- Checkbox state matches assignment
- Toggle calls correct API
- No console errors

- [ ] **Step 4: Run eslint + prettier**

```bash
cd frontend && npx eslint src/features/project/CompactSettingsPopover.tsx
npx prettier --write src/features/project/CompactSettingsPopover.tsx
```

- [ ] **Step 5: Commit**

```bash
git commit -m "feat: MCP server project assignment UI in settings popover"
```

---

## Task 3: Documentation (M3)

**Files:**
- Modify: `docs/dev-setup.md`

- [ ] **Step 1: Add docs-mcp-server section to dev-setup.md**

Add under Infrastructure section:

```markdown
### docs-mcp-server (Documentation Grounding)

Provides AI agents with up-to-date library documentation via MCP tools.

**Start:**
```bash
docker compose up -d docs-mcp
```

**Web Dashboard:** http://localhost:6281 (manage indexed libraries)
**MCP Endpoint:** http://localhost:6280/sse (for MCP client configuration)

**Index documentation (via Web UI or CLI):**
```bash
# Example: Index SolidJS docs
docker exec codeforge-docs-mcp npx docs-mcp-server scrape solidjs https://docs.solidjs.com

# Example: Index FastAPI docs
docker exec codeforge-docs-mcp npx docs-mcp-server scrape fastapi https://fastapi.tiangolo.com
```

**Assign to project:**
1. Go to Settings > MCP Servers > register docs-mcp-server (type: SSE, URL: http://docs-mcp:6280/sse)
2. Open project > Settings (gear icon) > check "docs-mcp-server"
3. Agent now has `search_docs`, `scrape_docs`, `list_libraries` tools

**Embeddings:** Uses Ollama by default (no API key needed). Requires `nomic-embed-text` model:
```bash
ollama pull nomic-embed-text
```

**Ports:** 6280 (MCP), 6281 (Web UI)
```

- [ ] **Step 2: Commit**

```bash
git commit -m "docs: add docs-mcp-server setup guide to dev-setup.md"
```

---

## Task Summary

| Task | Description | Files | Steps |
|---|---|---|---|
| 1 | Docker config (Ollama + Web UI) | docker-compose.yml | 4 |
| 2 | MCP Project Assignment UI | CompactSettingsPopover.tsx | 5 |
| 3 | Documentation | dev-setup.md | 2 |
| **Total** | | | **11** |
