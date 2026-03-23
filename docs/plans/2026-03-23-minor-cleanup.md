# Minor Cleanup — Implementation Plan

**Date:** 2026-03-23
**Goal:** Wire missing evaluator, delete dead code, move misplaced test, remove unused frontend API methods.

---

## Task 1: Wire FilesystemStateEvaluator in benchmark consumer

**Problem:** `FilesystemStateEvaluator` exists at `workers/codeforge/evaluation/evaluators/filesystem_state.py` with tests at `workers/tests/test_filesystem_state_evaluator.py`, but the `_build_evaluators()` function in `workers/codeforge/consumer/_benchmark.py` does not recognize `"filesystem_state"` as an evaluator name. It is never instantiated.

**Files:**
- Modify: `workers/codeforge/consumer/_benchmark.py`

- [ ] **Step 1: Add filesystem_state to the evaluator builder**

In `_build_evaluators()` (line ~804), add a case for `"filesystem_state"`:

```python
elif name == "filesystem_state":
    from codeforge.evaluation.evaluators.filesystem_state import FilesystemStateEvaluator
    evaluators.append(FilesystemStateEvaluator())
```

Insert after the `logprob_verifier` case (around line 843).

- [ ] **Step 2: Add to the `_DIMENSION_TO_METRIC` mapping**

Add the identity mapping (around line 641):

```python
# FilesystemStateEvaluator -> identity
"filesystem_state": "filesystem_state",
```

- [ ] **Step 3: Update the error message valid names list**

Add `"filesystem_state"` to the valid names string in the `else` branch (line ~847).

- [ ] **Step 4: Verify**

```bash
cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/test_filesystem_state_evaluator.py -v
```

---

## Task 2: Delete dead code — `_streaming.py`

**Problem:** `workers/codeforge/backends/_streaming.py` defines `run_streaming_subprocess()` which is never imported or called anywhere in the codebase. The only references are:
- The file itself (definition)
- `workers/tests/test_backends_streaming.py` (tests for the dead code)
- `workers/tests/test_backend_config_passthrough.py` (may reference it)

**Files:**
- Delete: `workers/codeforge/backends/_streaming.py`
- Delete: `workers/tests/test_backends_streaming.py`
- Verify: `workers/tests/test_backend_config_passthrough.py` does not import from `_streaming`

- [ ] **Step 1: Verify no imports exist**

```bash
grep -r "from.*_streaming\|import.*_streaming\|run_streaming" workers/codeforge/ --include="*.py" | grep -v "_streaming.py"
```

Must return empty (only the file itself defines it).

- [ ] **Step 2: Delete the files**

```bash
rm workers/codeforge/backends/_streaming.py
rm workers/tests/test_backends_streaming.py
```

- [ ] **Step 3: Verify tests still pass**

```bash
cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/test_backend_config_passthrough.py -v
```

---

## Task 3: Move misplaced test file

**Problem:** `workers/codeforge/test_graphrag.py` is a test file inside the source package. Tests belong in `workers/tests/`. However, `workers/tests/test_graphrag.py` already exists as a different file (FIX-070 verification tests vs. Phase 6D comprehensive tests).

**Files:**
- Move: `workers/codeforge/test_graphrag.py` -> `workers/tests/test_graphrag_phase6d.py`

The two files are different:
- `workers/codeforge/test_graphrag.py` — "Tests for the GraphRAG code graph builder and searcher (Phase 6D)" — comprehensive mock-based tests
- `workers/tests/test_graphrag.py` — "Tests for GraphRAG module (FIX-070)" — import/structure verification

- [ ] **Step 1: Move and rename to avoid collision**

```bash
mv workers/codeforge/test_graphrag.py workers/tests/test_graphrag_phase6d.py
```

- [ ] **Step 2: Fix any relative imports in the moved file**

Check if the test uses relative imports (`from .graphrag import ...`). If so, change to absolute imports (`from codeforge.graphrag import ...`).

- [ ] **Step 3: Verify both test files pass**

```bash
cd /workspaces/CodeForge && .venv/bin/python -m pytest workers/tests/test_graphrag.py workers/tests/test_graphrag_phase6d.py -v
```

---

## Task 4: Remove unused frontend API methods

**Problem:** 31 API methods are defined in frontend resource files but never called anywhere in the frontend source code (no `.tsx`/`.ts` file references them outside their definition). These are dead code that increases maintenance burden and creates false expectations about feature completeness.

**Verification method:** For each method, `grep -r "api.<resource>.<method>" frontend/src/ --include="*.ts" --include="*.tsx"` returns only the definition file.

### Methods to remove (grouped by resource file):

**`frontend/src/api/resources/conversations.ts`** (4 methods):
- `get` — `conversations.get(id)` — never called
- `delete` — `conversations.delete(id)` — never called
- `fork` — `conversations.fork(id)` — never called
- `rewind` — `conversations.rewind(id)` — never called

**`frontend/src/api/resources/settings.ts`** (5 methods):
- `modes.get` — `modes.get(id)` — never called
- `scopes.get` — `scopes.get(id)` — never called
- `scopes.update` — `scopes.update(id, data)` — never called
- `scopes.graphSearch` — `scopes.graphSearch(scopeId, data)` — never called
- `knowledgeBases.get` — `knowledgeBases.get(id)` — never called

**`frontend/src/api/resources/settings.ts`** (`createReviewsResource`, 8 methods):
- All 8 methods in `createReviewsResource` — `api.reviews.*` is never referenced anywhere in the frontend (only i18n keys exist). The entire resource is dead.
- Methods: `listPolicies`, `createPolicy`, `getPolicy`, `updatePolicy`, `deletePolicy`, `trigger`, `list`, `get`

**`frontend/src/api/resources/misc.ts`** (7 methods):
- `lsp.diagnostics` — never called
- `lsp.definition` — never called
- `lsp.references` — never called
- `lsp.symbols` — never called
- `lsp.hover` — never called
- `plans.planFeature` — never called
- `goals.get` — never called
- `sessions.get` — never called

**`frontend/src/api/resources/llm.ts`** (1 method):
- `costs.byToolForRun` — never called

**`frontend/src/api/resources/projects.ts`** (1 method):
- `projects.checkout` — never called

**`frontend/src/api/resources/roadmap.ts`** (2 methods):
- `roadmap.deleteMilestone` — never called
- `roadmap.deleteFeature` — never called

**`frontend/src/api/resources/benchmarks.ts`** (2 methods):
- `benchmarks.listDatasets` — never called
- `benchmarks.getSuite` — never called

**Files to modify:**
- `frontend/src/api/resources/conversations.ts`
- `frontend/src/api/resources/settings.ts`
- `frontend/src/api/resources/misc.ts`
- `frontend/src/api/resources/llm.ts`
- `frontend/src/api/resources/projects.ts`
- `frontend/src/api/resources/roadmap.ts`
- `frontend/src/api/resources/benchmarks.ts`
- `frontend/src/api/resources/index.ts` (remove `createReviewsResource` export)
- `frontend/src/api/client.ts` (remove `reviews` from api object)
- `frontend/src/api/types.ts` (remove unused types that are only used by deleted methods)

- [ ] **Step 1: Remove individual dead methods from each resource file**

For each resource file, delete the method definition. Keep the resource function itself if it still has live methods.

- [ ] **Step 2: Remove the entire reviews resource**

Since all 8 `reviews` methods are unused:
- Delete `createReviewsResource` from `settings.ts`
- Remove export from `index.ts`
- Remove `reviews: createReviewsResource(core)` from `client.ts`
- Remove associated type imports that become orphaned

- [ ] **Step 3: Clean up unused type imports**

After removing methods, some types imported in resource files will become unused. Remove them from the import statements. Types that are only used by deleted methods and nowhere else in the codebase can also be removed from `types.ts`.

Potentially orphaned types to check:
- `Review`, `ReviewPolicy` — only used by reviews resource
- `GraphSearchRequest`, `GraphSearchResult` — check if still used by `graph.search` (yes, keep)
- `PlanFeatureRequest` — only used by `plans.planFeature`
- `BenchmarkDatasetInfo` — only used by `benchmarks.listDatasets`

- [ ] **Step 4: Verify no build errors**

```bash
cd frontend && npm run build
```

- [ ] **Step 5: Verify tests still pass**

```bash
cd frontend && npm test
```

---

## Task 5: Final commit

```
chore: minor cleanup — wire evaluator, remove dead code, fix test location

- Wire FilesystemStateEvaluator in benchmark evaluator builder
- Delete unused run_streaming_subprocess (workers/codeforge/backends/_streaming.py)
- Move misplaced test_graphrag.py from source package to workers/tests/
- Remove 31 unused frontend API methods across 7 resource files
- Remove dead createReviewsResource (all 8 methods unused)
```

---

## File Reference

| File | Action |
|------|--------|
| `workers/codeforge/consumer/_benchmark.py` | Modify (wire FilesystemStateEvaluator) |
| `workers/codeforge/backends/_streaming.py` | Delete |
| `workers/tests/test_backends_streaming.py` | Delete |
| `workers/codeforge/test_graphrag.py` | Move to `workers/tests/test_graphrag_phase6d.py` |
| `frontend/src/api/resources/conversations.ts` | Modify (remove 4 methods) |
| `frontend/src/api/resources/settings.ts` | Modify (remove 6 methods + entire reviews resource) |
| `frontend/src/api/resources/misc.ts` | Modify (remove 8 methods) |
| `frontend/src/api/resources/llm.ts` | Modify (remove 1 method) |
| `frontend/src/api/resources/projects.ts` | Modify (remove 1 method) |
| `frontend/src/api/resources/roadmap.ts` | Modify (remove 2 methods) |
| `frontend/src/api/resources/benchmarks.ts` | Modify (remove 2 methods) |
| `frontend/src/api/resources/index.ts` | Modify (remove reviews export) |
| `frontend/src/api/client.ts` | Modify (remove reviews from api) |
| `frontend/src/api/types.ts` | Modify (remove orphaned types) |
