# Knowledge Pipeline Connection — Design Spec

**Date:** 2026-03-23
**Type:** Fix disconnected knowledge systems — KB injection + Stack-Detection auto-matching
**Scope:** Connect Knowledge Base and Stack Detection to the agent conversation context pipeline

---

## 1. Problem

CodeForge has two knowledge systems that are built but disconnected from agent conversations:

1. **Knowledge Base** — Full CRUD, indexing, scope attachment, but content never reaches the agent prompt
2. **Stack Detection** — Correctly detects frameworks (SolidJS, FastAPI, etc.) but never triggers framework-specific guidance

Result: A weak model coding in SolidJS gets zero SolidJS knowledge, even though CodeForge detected the framework.

---

## 2. Solution: Two Integration Points

### 2.1 Knowledge Base → Context Pipeline

Add KB entries to the existing context assembly pipeline in `assembleAndPack()`, alongside retrieval, GraphRAG, RepoMap, diagnostics, and goals.

**Flow:**
```
assembleAndPack()
  ... existing sources ...
  + fetchKnowledgeBaseEntries(ctx, projectID, userMessage)
      -> GetScopesForProject(projectID)        [NEW store method]
      -> ListKnowledgeBasesByScope(scopeID)     [existing]
      -> Search each KB via retrieval ("kb:<id>")
      -> Return as []ContextEntry{kind: "knowledge"}
```

**New components:**
- `EntryKnowledge` constant in `context/pack.go`
- `GetScopesForProject()` store method
- `fetchKnowledgeBaseEntries()` in `context_optimizer.go`
- Call site in `assembleAndPack()` after goals, before dedup

**Priority:** 75 (higher than generic retrieval 60, lower than diagnostics/goals 85+)

### 2.2 Stack Detection → Built-in Skills Auto-Selection

When stack detection finds frameworks, auto-inject matching built-in skills.

**Flow:**
```
_build_system_prompt()
  -> detectStackSummary() returns "typescript (solidjs), python (fastapi)"
  -> For each framework, check if a built-in skill exists
  -> Inject matching skills into system prompt
```

**New components:**
- Built-in skill YAML files: `solidjs-patterns.yaml`, `fastapi-patterns.yaml`, `react-patterns.yaml`
- `_inject_framework_skills()` in `_conversation.py` — matches detected frameworks to built-in skills

---

## 3. Built-in Framework Skills (Content)

### solidjs-patterns.yaml
```yaml
name: solidjs-patterns
type: knowledge
description: SolidJS framework patterns and common mistakes
tags: [solidjs, typescript, frontend]
content: |
  # SolidJS Patterns (NOT React)

  SolidJS uses fine-grained reactivity. Do NOT use React patterns.

  ## Correct Imports
  import { createSignal, createEffect, createResource, For, Show } from "solid-js";
  import { render } from "solid-js/web";

  ## State Management
  WRONG (React): const [count, setCount] = useState(0);
  RIGHT (SolidJS): const [count, setCount] = createSignal(0);

  ## Reading Signals (MUST call as function)
  WRONG: <p>{count}</p>
  RIGHT: <p>{count()}</p>

  ## Effects
  WRONG (React): useEffect(() => { ... }, [dep]);
  RIGHT (SolidJS): createEffect(() => { console.log(count()); });
  // No dependency array — SolidJS tracks automatically

  ## Data Fetching
  const [data] = createResource(source, fetcher);
  // source is a signal, fetcher is async function

  ## Conditional Rendering
  WRONG (React): {condition && <Component />}
  RIGHT (SolidJS): <Show when={condition()}><Component /></Show>

  ## Lists
  WRONG (React): {items.map(item => <div>{item}</div>)}
  RIGHT (SolidJS): <For each={items()}>{(item) => <div>{item}</div>}</For>

  ## JSX Differences
  - Use `class` not `className`
  - Use `onclick` not `onClick` (lowercase)
  - Components render ONCE (not on every state change)

  ## Project Setup (Vite)
  npm create vite@latest my-app -- --template solid-ts
  cd my-app && npm install && npm run dev

  ## Testing (vitest)
  npm install -D vitest @solidjs/testing-library jsdom
  // vitest.config.ts:
  import { defineConfig } from 'vitest/config';
  import solidPlugin from 'vite-plugin-solid';
  export default defineConfig({
    plugins: [solidPlugin()],
    test: { environment: 'jsdom' }
  });
```

### fastapi-patterns.yaml
```yaml
name: fastapi-patterns
type: knowledge
description: Python FastAPI patterns and common mistakes
tags: [fastapi, python, backend]
content: |
  # FastAPI Patterns

  ## Project Setup
  pip install fastapi uvicorn[standard] httpx pytest

  ## Basic App
  from fastapi import FastAPI
  from fastapi.middleware.cors import CORSMiddleware

  app = FastAPI()
  app.add_middleware(CORSMiddleware, allow_origins=["*"], allow_methods=["*"], allow_headers=["*"])

  @app.get("/health")
  def health(): return {"status": "ok"}

  ## Caching (CORRECT way)
  from functools import lru_cache  # NO ttl parameter!
  # lru_cache does NOT support ttl. Use cachetools instead:
  from cachetools import TTLCache
  cache = TTLCache(maxsize=100, ttl=300)

  ## Running
  uvicorn main:app --host 0.0.0.0 --port 8000 --reload

  ## Testing
  from fastapi.testclient import TestClient
  from main import app
  client = TestClient(app)
  def test_health():
      response = client.get("/health")
      assert response.status_code == 200

  ## Common Mistakes
  - lru_cache does NOT have a ttl parameter (use cachetools.TTLCache)
  - Don't forget CORS middleware for frontend communication
  - Use httpx for async HTTP calls, requests for sync
```

---

## 4. Scope Auto-Discovery

Projects should auto-include their implicit scope. When `GetScopesForProject()` finds no explicit scopes, fall back to searching KBs attached to global scopes.

---

## 5. Files to Modify

| File | Change |
|---|---|
| `internal/domain/context/pack.go` | Add `EntryKnowledge` constant |
| `internal/port/database/store.go` | Add `GetScopesForProject()` interface method |
| `internal/adapter/postgres/store_knowledgebase.go` | Implement `GetScopesForProject()` |
| `internal/service/context_optimizer.go` | Add `fetchKnowledgeBaseEntries()`, call in `assembleAndPack()` |
| `workers/codeforge/skills/builtins/solidjs-patterns.yaml` | Create |
| `workers/codeforge/skills/builtins/fastapi-patterns.yaml` | Create |
| `workers/codeforge/consumer/_conversation.py` | Add `_inject_framework_skills()` matching stack detection |
