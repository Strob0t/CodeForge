# Knowledge Pipeline Connection — Implementation Plan

> Steps use checkbox (`- [ ]`) syntax. Execute task-by-task, commit after each.

**Goal:** Connect Knowledge Base and Stack Detection to agent conversations so weak models get framework knowledge.

**Spec:** `docs/specs/2026-03-23-knowledge-pipeline-design.md`

---

## Task 1: Add EntryKnowledge constant + GetScopesForProject store method

**Files:**
- Modify: `internal/domain/context/pack.go` — add `EntryKnowledge`
- Modify: `internal/port/database/store.go` — add interface method
- Modify: `internal/adapter/postgres/store_knowledgebase.go` — implement
- Test: `internal/adapter/postgres/store_knowledgebase_test.go`

- [ ] **Step 1: Add EntryKnowledge to pack.go**

Add `EntryKnowledge EntryKind = "knowledge"` to the const block.

- [ ] **Step 2: Add GetScopesForProject to store interface**

```go
// In Store interface:
GetScopesForProject(ctx context.Context, projectID string) ([]context.RetrievalScope, error)
```

- [ ] **Step 3: Implement in PostgreSQL adapter**

```sql
SELECT rs.id, rs.name, rs.type
FROM retrieval_scopes rs
JOIN retrieval_scope_projects rsp ON rs.id = rsp.scope_id
WHERE rsp.project_id = $1
UNION
SELECT rs.id, rs.name, rs.type
FROM retrieval_scopes rs
WHERE rs.type = 'global'
```

- [ ] **Step 4: Run go vet, gofmt, goimports**
- [ ] **Step 5: Commit**

```bash
git commit -m "feat: add EntryKnowledge kind + GetScopesForProject store method"
```

---

## Task 2: fetchKnowledgeBaseEntries in context_optimizer

**Files:**
- Modify: `internal/service/context_optimizer.go`

- [ ] **Step 1: Add fetchKnowledgeBaseEntries method**

```go
func (s *ContextOptimizerService) fetchKnowledgeBaseEntries(
    ctx context.Context,
    projectID, userMessage string,
) ([]cfcontext.ContextEntry, error) {
    // 1. Get scopes for project
    scopes, err := s.store.GetScopesForProject(ctx, projectID)
    if err != nil || len(scopes) == 0 {
        return nil, err
    }

    // 2. Get KBs for each scope
    var entries []cfcontext.ContextEntry
    for _, scope := range scopes {
        kbs, kbErr := s.store.ListKnowledgeBasesByScope(ctx, scope.ID)
        if kbErr != nil || len(kbs) == 0 {
            continue
        }
        for _, kb := range kbs {
            if kb.Status != "indexed" {
                continue // skip unindexed KBs
            }
            // 3. Search KB via retrieval (using "kb:<id>" namespace)
            if s.retrieval != nil {
                hits, rErr := s.retrieval.Search(ctx, "kb:"+kb.ID, userMessage, 3)
                if rErr == nil {
                    for _, hit := range hits {
                        entries = append(entries, cfcontext.ContextEntry{
                            Kind:     cfcontext.EntryKnowledge,
                            Path:     kb.Name,
                            Content:  hit.Content,
                            Tokens:   hit.Tokens,
                            Priority: 75,
                        })
                    }
                }
            }
        }
    }
    return entries, nil
}
```

- [ ] **Step 2: Wire into assembleAndPack() after goals**

After the goals section (around line 302), add:
```go
// Knowledge base entries (matched to project scopes).
kbEntries, kbErr := s.fetchKnowledgeBaseEntries(ctx, projectID, prompt)
if kbErr == nil && len(kbEntries) > 0 {
    candidates = append(candidates, kbEntries...)
    slog.Info("knowledge base entries added", "count", len(kbEntries))
}
```

- [ ] **Step 3: Run go test, go vet**
- [ ] **Step 4: Commit**

```bash
git commit -m "feat: inject knowledge base entries into conversation context pipeline"
```

---

## Task 3: Built-in Framework Skills

**Files:**
- Create: `workers/codeforge/skills/builtins/solidjs-patterns.yaml`
- Create: `workers/codeforge/skills/builtins/fastapi-patterns.yaml`

- [ ] **Step 1: Create solidjs-patterns.yaml**

Content from the design spec — SolidJS patterns with WRONG/RIGHT comparisons, import examples, setup commands, testing setup.

- [ ] **Step 2: Create fastapi-patterns.yaml**

Content from the design spec — FastAPI patterns, caching (cachetools NOT lru_cache ttl), CORS, testing, common mistakes.

- [ ] **Step 3: Verify YAML syntax**
- [ ] **Step 4: Commit**

```bash
git commit -m "feat: built-in framework skills for SolidJS and FastAPI"
```

---

## Task 4: Stack Detection → Framework Skill Auto-Injection

**Files:**
- Modify: `workers/codeforge/consumer/_conversation.py`

- [ ] **Step 1: Add _inject_framework_skills() method**

After `_inject_skills()` in `_build_system_prompt()`, add framework-specific skill injection:

```python
def _inject_framework_skills(
    self,
    system_prompt: str,
    stack_summary: str,
    log: BoundLogger,
) -> str:
    """Inject built-in framework skills matching detected stack."""
    if not stack_summary:
        return system_prompt

    builtins_dir = Path(__file__).parent.parent / "skills" / "builtins"
    if not builtins_dir.exists():
        return system_prompt

    # Map framework keywords to skill filenames
    FRAMEWORK_SKILLS = {
        "solidjs": "solidjs-patterns.yaml",
        "react": "react-patterns.yaml",
        "fastapi": "fastapi-patterns.yaml",
        "django": "django-patterns.yaml",
        "express": "express-patterns.yaml",
    }

    injected = []
    stack_lower = stack_summary.lower()
    for framework, filename in FRAMEWORK_SKILLS.items():
        if framework in stack_lower:
            skill_path = builtins_dir / filename
            if skill_path.exists():
                content = yaml.safe_load(skill_path.read_text()).get("content", "")
                if content:
                    system_prompt += f"\n\n<framework-guide name=\"{framework}\">\n{content}\n</framework-guide>"
                    injected.append(framework)

    if injected:
        log.info("framework skills injected", frameworks=injected)
    return system_prompt
```

- [ ] **Step 2: Call from _build_system_prompt()**

After stack detection, call `_inject_framework_skills(system_prompt, stack_summary, log)`.

- [ ] **Step 3: Run ruff check/format**
- [ ] **Step 4: Commit**

```bash
git commit -m "feat: auto-inject framework skills based on stack detection"
```

---

## Task Summary

| Task | Description | Language | Files |
|---|---|---|---|
| 1 | EntryKnowledge + GetScopesForProject | Go | pack.go, store.go, store_knowledgebase.go |
| 2 | fetchKnowledgeBaseEntries in context pipeline | Go | context_optimizer.go |
| 3 | Built-in framework skills (SolidJS, FastAPI) | YAML | builtins/*.yaml |
| 4 | Stack Detection → auto-inject skills | Python | _conversation.py |
| **Total** | | | **~16 steps** |
