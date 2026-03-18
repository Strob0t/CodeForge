# Docs Reorganization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reorganize all docs into a logical directory structure with consistent naming conventions, eliminating the `superpowers/` split and cleaning up the `plans/` dumping ground.

**Architecture:** Pure file reorganization — `git mv` for moves/renames, then sed-style edits for cross-reference updates. No code changes. Design specs → `specs/`, implementation plans → `plans/`, test artifacts → `testing/`, audits → `audits/`.

**Naming Convention:** `*-design.md` (specs), `*-plan.md` (plans), `*-testplan.md` (test plans), `*-report.md` (test reports)

---

## Task 1: Create New Directories + Move Design Specs to `specs/`

**Files:**
- Create: `docs/specs/` (new directory)
- Move: 15 design spec files from `docs/plans/` and `docs/superpowers/specs/`

- [ ] **Step 1: Create `docs/specs/` and move 9 specs from `docs/plans/`**

```bash
cd /workspaces/CodeForge
mkdir -p docs/specs

# Already have -design suffix (just move)
git mv docs/plans/2026-03-08-dashboard-polish-design.md docs/specs/
git mv docs/plans/2026-03-08-mobile-responsive-design.md docs/specs/
git mv docs/plans/2026-03-08-integration-testing-design.md docs/specs/
git mv docs/plans/2026-03-09-chat-enhancements-design.md docs/specs/
git mv docs/plans/2026-03-09-auto-agent-skills-design.md docs/specs/
git mv docs/plans/2026-03-09-benchmark-external-providers-design.md docs/specs/
git mv docs/plans/2026-03-10-benchmark-live-feed-design.md docs/specs/

# Need -design suffix added (move + rename)
git mv docs/plans/2026-03-09-goal-system-redesign.md docs/specs/2026-03-09-goal-system-redesign-design.md
git mv docs/plans/2026-03-09-project-workflow-redesign.md docs/specs/2026-03-09-project-workflow-redesign-design.md
```

- [ ] **Step 2: Move 6 specs from `docs/superpowers/specs/`**

```bash
git mv docs/superpowers/specs/2026-03-11-benchmark-validation-design.md docs/specs/
git mv docs/superpowers/specs/2026-03-15-contract-first-review-refactor-design.md docs/specs/
git mv docs/superpowers/specs/2026-03-16-benchmark-live-feed-density-design.md docs/specs/
git mv docs/superpowers/specs/2026-03-18-claude-code-integration-design.md docs/specs/
git mv docs/superpowers/specs/2026-03-18-quality-performance-improvements-design.md docs/specs/
git mv docs/superpowers/specs/2026-03-18-ux-ui-audit-fixes-design.md docs/specs/
```

- [ ] **Step 3: Verify**

```bash
ls docs/specs/ | wc -l
# Expected: 15
ls docs/superpowers/specs/
# Expected: empty or directory gone
```

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "docs: move 15 design specs to docs/specs/"
```

---

## Task 2: Consolidate Implementation Plans in `docs/plans/`

**Files:**
- Move: 8 plans from `docs/superpowers/plans/` → `docs/plans/` (with `-plan` suffix)
- Rename: 8 inconsistent plans already in `docs/plans/`

- [ ] **Step 1: Move 8 plans from `docs/superpowers/plans/` with `-plan` suffix**

```bash
cd /workspaces/CodeForge

git mv docs/superpowers/plans/2026-03-11-benchmark-findings-fixes.md docs/plans/2026-03-11-benchmark-findings-fixes-plan.md
git mv docs/superpowers/plans/2026-03-15-contract-first-review-refactor.md docs/plans/2026-03-15-contract-first-review-refactor-plan.md
git mv docs/superpowers/plans/2026-03-16-benchmark-live-feed-improvements.md docs/plans/2026-03-16-benchmark-live-feed-improvements-plan.md
git mv docs/superpowers/plans/2026-03-16-canvas-e2e-bugfixes.md docs/plans/2026-03-16-canvas-e2e-bugfixes-plan.md
git mv docs/superpowers/plans/2026-03-18-claude-code-integration.md docs/plans/2026-03-18-claude-code-integration-plan.md
git mv docs/superpowers/plans/2026-03-18-db-audit-remediation.md docs/plans/2026-03-18-db-audit-remediation-plan.md
git mv docs/superpowers/plans/2026-03-18-dead-code-cleanup.md docs/plans/2026-03-18-dead-code-cleanup-plan.md
git mv docs/superpowers/plans/2026-03-18-ux-ui-audit-fixes.md docs/plans/2026-03-18-ux-ui-audit-fixes-plan.md
```

- [ ] **Step 2: Rename 8 inconsistent plans already in `docs/plans/`**

```bash
git mv docs/plans/2026-03-07-unified-llm-path.md docs/plans/2026-03-07-unified-llm-path-plan.md
git mv docs/plans/2026-03-09-goal-system-redesign-impl.md docs/plans/2026-03-09-goal-system-redesign-plan.md
git mv docs/plans/2026-03-09-project-workflow-impl-plan.md docs/plans/2026-03-09-project-workflow-plan.md
git mv docs/plans/2026-03-09-adaptive-context-injection.md docs/plans/2026-03-09-adaptive-context-injection-plan.md
git mv docs/plans/2026-03-09-e2e-findings-fix.md docs/plans/2026-03-09-e2e-findings-fix-plan.md
git mv docs/plans/2026-03-09-feature-activation-sweep.md docs/plans/2026-03-09-feature-activation-sweep-plan.md
git mv docs/plans/2026-03-09-feature-roadmap-todos.md docs/plans/2026-03-09-feature-roadmap-todos-plan.md
git mv docs/plans/2026-03-09-agent-eval-improvements.md docs/plans/2026-03-09-agent-eval-improvements-plan.md
```

- [ ] **Step 3: Verify plans count**

```bash
ls docs/plans/*.md | wc -l
# Expected: 23 (22 plans + this plan file itself)
```

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "docs: consolidate implementation plans in docs/plans/ with -plan suffix"
```

---

## Task 3: Move Test Artifacts to `docs/testing/`

**Files:**
- Create: `docs/testing/` (new directory)
- Move: 6 files from `docs/plans/` and `docs/` root

- [ ] **Step 1: Create dir and move files**

```bash
cd /workspaces/CodeForge
mkdir -p docs/testing

# From docs/ root
git mv docs/benchmark-e2e-test-plan.md docs/testing/benchmark-e2e-testplan.md

# From docs/plans/ — test reports
git mv docs/plans/2026-03-09-e2e-playwright-test-report.md docs/testing/2026-03-09-e2e-playwright-report.md
git mv docs/plans/2026-03-17-interactive-qa-report.md docs/testing/2026-03-17-interactive-qa-report.md

# From docs/plans/ — test plans
git mv docs/plans/2026-03-09-playwright-mcp-smoke-test.md docs/testing/2026-03-09-playwright-mcp-smoke-testplan.md
git mv docs/plans/2026-03-17-interactive-qa-testplan.md docs/testing/2026-03-17-interactive-qa-testplan.md
git mv docs/plans/2026-03-18-full-service-interactive-qa-testplan.md docs/testing/2026-03-18-full-service-interactive-qa-testplan.md
```

- [ ] **Step 2: Verify**

```bash
ls docs/testing/ | wc -l
# Expected: 6
```

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "docs: move test plans and reports to docs/testing/"
```

---

## Task 4: Move Audits to `docs/audits/`

**Files:**
- Create: `docs/audits/` (new directory)
- Move: 3 files from `docs/` root and `docs/audit/`

- [ ] **Step 1: Create dir and move files**

```bash
cd /workspaces/CodeForge
mkdir -p docs/audits

git mv docs/ux-ui-audit.md docs/audits/ux-ui-audit.md
git mv docs/stub-tracker.md docs/audits/stub-tracker.md
git mv docs/audit/schema-audit-2026-03-18.md docs/audits/2026-03-18-schema-audit.md
```

- [ ] **Step 2: Remove empty old directory**

```bash
rmdir docs/audit
```

- [ ] **Step 3: Verify**

```bash
ls docs/audits/
# Expected: 2026-03-18-schema-audit.md  stub-tracker.md  ux-ui-audit.md
```

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "docs: move audits to docs/audits/"
```

---

## Task 5: Delete Empty `docs/superpowers/`

- [ ] **Step 1: Remove empty superpowers directories**

```bash
cd /workspaces/CodeForge
rmdir docs/superpowers/specs 2>/dev/null
rmdir docs/superpowers/plans 2>/dev/null
rmdir docs/superpowers
```

- [ ] **Step 2: Verify**

```bash
test -d docs/superpowers && echo "STILL EXISTS" || echo "GONE"
# Expected: GONE
```

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "docs: remove empty docs/superpowers/ directory"
```

---

## Task 6: Update Cross-References in All Affected Docs

This is the most complex task. Every file that references a moved/renamed doc must be updated.

**Files to modify:**
- `docs/todo.md` (15 references)
- `docs/project-status.md` (2 references)
- `docs/audits/stub-tracker.md` (1 self-location reference)
- `docs/features/05-chat-enhancements.md` (2 references)
- `docs/plans/2026-03-09-chat-enhancements-plan.md` (1 reference)
- `docs/plans/2026-03-08-mobile-responsive-plan.md` (1 reference)
- `docs/plans/2026-03-08-dashboard-polish-plan.md` (1 reference)
- `docs/plans/2026-03-09-benchmark-external-providers-plan.md` (1 reference)
- `docs/plans/2026-03-09-auto-agent-skills-plan.md` (2 references)
- `docs/plans/2026-03-09-project-workflow-plan.md` (1 reference, already renamed)
- `docs/plans/2026-03-09-goal-system-redesign-plan.md` (1 reference, already renamed)
- `docs/specs/2026-03-18-ux-ui-audit-fixes-design.md` (1 reference)
- `docs/plans/2026-03-18-ux-ui-audit-fixes-plan.md` (1 reference, already moved)
- `docs/plans/2026-03-15-contract-first-review-refactor-plan.md` (1 reference, already moved)
- `docs/plans/2026-03-18-claude-code-integration-plan.md` (1 reference, already moved)
- `docs/plans/2026-03-18-db-audit-remediation-plan.md` (4 references, already moved)
- `docs/plans/2026-03-16-benchmark-live-feed-improvements-plan.md` (1 reference, already moved)
- `docs/testing/2026-03-17-interactive-qa-testplan.md` (1 self-reference)

- [ ] **Step 1: Update `docs/todo.md`**

Apply these replacements (all within `docs/todo.md`):

| Old reference | New reference |
|---|---|
| `docs/superpowers/plans/2026-03-11-benchmark-findings-fixes.md` | `docs/plans/2026-03-11-benchmark-findings-fixes-plan.md` |
| `docs/plans/2026-03-10-benchmark-live-feed-design.md` | `docs/specs/2026-03-10-benchmark-live-feed-design.md` |
| `docs/superpowers/specs/2026-03-16-benchmark-live-feed-density-design.md` | `docs/specs/2026-03-16-benchmark-live-feed-density-design.md` |
| `docs/superpowers/plans/2026-03-16-benchmark-live-feed-improvements.md` | `docs/plans/2026-03-16-benchmark-live-feed-improvements-plan.md` |
| `plans/2026-03-09-e2e-playwright-test-report.md` (both text and link) | `testing/2026-03-09-e2e-playwright-report.md` |
| `plans/2026-03-09-auto-agent-skills-design.md` (both text and link) | `specs/2026-03-09-auto-agent-skills-design.md` |
| `plans/2026-03-09-benchmark-external-providers-design.md` (both text and link) | `specs/2026-03-09-benchmark-external-providers-design.md` |
| `plans/2026-03-08-integration-testing-design.md` (both text and link) | `specs/2026-03-08-integration-testing-design.md` |
| `docs/plans/2026-03-09-project-workflow-impl-plan.md` | `docs/plans/2026-03-09-project-workflow-plan.md` |
| `docs/plans/2026-03-09-project-workflow-redesign.md` | `docs/specs/2026-03-09-project-workflow-redesign-design.md` |
| `docs/stub-tracker.md` | `docs/audits/stub-tracker.md` |
| `docs/superpowers/specs/2026-03-18-quality-performance-improvements-design.md` | `docs/specs/2026-03-18-quality-performance-improvements-design.md` |

- [ ] **Step 2: Update `docs/project-status.md`**

| Old reference | New reference |
|---|---|
| `docs/superpowers/plans/2026-03-11-benchmark-findings-fixes.md` | `docs/plans/2026-03-11-benchmark-findings-fixes-plan.md` |
| `docs/audit/schema-audit-2026-03-18.md` | `docs/audits/2026-03-18-schema-audit.md` |

- [ ] **Step 3: Update `docs/features/05-chat-enhancements.md`**

| Old reference | New reference |
|---|---|
| `docs/plans/2026-03-09-chat-enhancements-design.md` | `docs/specs/2026-03-09-chat-enhancements-design.md` |
| `../plans/2026-03-09-chat-enhancements-design.md` | `../specs/2026-03-09-chat-enhancements-design.md` |

- [ ] **Step 4: Update plan files that reference their design docs**

Each plan file has a `**Design Doc:**` or `**Spec:**` header line. Update these:

| File (new name) | Old reference | New reference |
|---|---|---|
| `docs/plans/2026-03-08-dashboard-polish-plan.md` | `docs/plans/2026-03-08-dashboard-polish-design.md` | `docs/specs/2026-03-08-dashboard-polish-design.md` |
| `docs/plans/2026-03-08-mobile-responsive-plan.md` | `docs/plans/2026-03-08-mobile-responsive-design.md` | `docs/specs/2026-03-08-mobile-responsive-design.md` |
| `docs/plans/2026-03-09-chat-enhancements-plan.md` | `docs/plans/2026-03-09-chat-enhancements-design.md` | `docs/specs/2026-03-09-chat-enhancements-design.md` |
| `docs/plans/2026-03-09-benchmark-external-providers-plan.md` | `docs/plans/2026-03-09-benchmark-external-providers-design.md` | `docs/specs/2026-03-09-benchmark-external-providers-design.md` |
| `docs/plans/2026-03-09-auto-agent-skills-plan.md` | `docs/plans/2026-03-09-auto-agent-skills-design.md` | `docs/specs/2026-03-09-auto-agent-skills-design.md` |
| `docs/plans/2026-03-09-project-workflow-plan.md` | `docs/plans/2026-03-09-project-workflow-redesign.md` | `docs/specs/2026-03-09-project-workflow-redesign-design.md` |
| `docs/plans/2026-03-09-goal-system-redesign-plan.md` | `docs/plans/2026-03-09-goal-system-redesign.md` | `docs/specs/2026-03-09-goal-system-redesign-design.md` |

- [ ] **Step 5: Update moved superpowers plan files (now in `docs/plans/`)**

| File (new name) | Old reference | New reference |
|---|---|---|
| `docs/plans/2026-03-18-ux-ui-audit-fixes-plan.md` | `docs/superpowers/specs/2026-03-18-ux-ui-audit-fixes-design.md` | `docs/specs/2026-03-18-ux-ui-audit-fixes-design.md` |
| `docs/plans/2026-03-15-contract-first-review-refactor-plan.md` | `docs/superpowers/specs/2026-03-15-contract-first-review-refactor-design.md` | `docs/specs/2026-03-15-contract-first-review-refactor-design.md` |
| `docs/plans/2026-03-18-claude-code-integration-plan.md` | `docs/superpowers/specs/2026-03-18-claude-code-integration-design.md` | `docs/specs/2026-03-18-claude-code-integration-design.md` |
| `docs/plans/2026-03-16-benchmark-live-feed-improvements-plan.md` | `docs/superpowers/specs/2026-03-16-benchmark-live-feed-density-design.md` | `docs/specs/2026-03-16-benchmark-live-feed-density-design.md` |
| `docs/plans/2026-03-18-db-audit-remediation-plan.md` | `docs/audit/schema-audit-2026-03-18.md` | `docs/audits/2026-03-18-schema-audit.md` |

- [ ] **Step 6: Update moved spec file**

| File (new name) | Old reference | New reference |
|---|---|---|
| `docs/specs/2026-03-18-ux-ui-audit-fixes-design.md` | `docs/ux-ui-audit.md` | `docs/audits/ux-ui-audit.md` |

- [ ] **Step 7: Update self-reference in moved test plan**

| File (new name) | Old reference | New reference |
|---|---|---|
| `docs/testing/2026-03-17-interactive-qa-testplan.md` | `docs/plans/2026-03-17-interactive-qa-testplan.md` | `docs/testing/2026-03-17-interactive-qa-testplan.md` |

- [ ] **Step 8: Final grep to catch any remaining stale references**

```bash
cd /workspaces/CodeForge
grep -rn "superpowers/" docs/ --include="*.md"
grep -rn "docs/audit/" docs/ --include="*.md"
grep -rn "plans/.*-design\.md" docs/ --include="*.md"
grep -rn "plans/.*-redesign\.md" docs/ --include="*.md"
grep -rn "plans/.*-impl" docs/ --include="*.md"
grep -rn "docs/ux-ui-audit\.md" docs/ --include="*.md"
grep -rn "docs/stub-tracker\.md" docs/ --include="*.md"
grep -rn "docs/benchmark-e2e" docs/ --include="*.md"
# Expected: NO matches for any of these
```

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "docs: update all cross-references to match new file locations"
```

---

## Task 7: Update `docs/README.md`

**Files:**
- Modify: `docs/README.md`

- [ ] **Step 1: Rewrite `docs/README.md` with new structure**

Replace the entire file with:

```markdown
# CodeForge — Documentation Index

> **LLM Agents:** Start here. This file maps all project documentation.
> For open tasks and priorities, see [todo.md](todo.md).

### Quick Reference

| Document | Purpose |
|---|---|
| [todo.md](todo.md) | Active TODO tracker — what needs to be done next |
| [project-status.md](project-status.md) | Phase tracking, milestones, completed work |
| [architecture.md](architecture.md) | System architecture, patterns, design details |
| [tech-stack.md](tech-stack.md) | Languages, tools, dependencies, infrastructure |
| [dev-setup.md](dev-setup.md) | Development environment setup guide |

### Feature Specifications

Each of the four core pillars has its own feature spec:

| Feature | File | Status |
|---|---|---|
| Project Dashboard | [features/01-project-dashboard.md](features/01-project-dashboard.md) | Foundation implemented |
| Roadmap/Feature-Map | [features/02-roadmap-feature-map.md](features/02-roadmap-feature-map.md) | Foundation implemented |
| Multi-LLM Provider | [features/03-multi-llm-provider.md](features/03-multi-llm-provider.md) | Foundation implemented |
| Agent Orchestration | [features/04-agent-orchestration.md](features/04-agent-orchestration.md) | Core implemented |
| Chat Enhancements | [features/05-chat-enhancements.md](features/05-chat-enhancements.md) | Implemented |
| Visual Design Canvas | [features/06-visual-design-canvas.md](features/06-visual-design-canvas.md) | Implemented |

### Architecture Details

| Document | Purpose |
|---|---|
| [architecture/adr/](architecture/adr/) | Architecture Decision Records (ADRs) |
| [architecture/adr/_template.md](architecture/adr/_template.md) | ADR template for new decisions |

### Research

| Document | Purpose |
|---|---|
| [research/market-analysis.md](research/market-analysis.md) | Market research, competitor analysis, framework comparison |
| [research/aider-deep-analysis.md](research/aider-deep-analysis.md) | Deep dive into Aider architecture |
| [research/protocol-analysis.md](research/protocol-analysis.md) | Protocol and standards analysis (MCP, A2A, AG-UI, LSP, OTEL) |

### Design Specs, Plans & Testing

| Directory | Purpose |
|---|---|
| [specs/](specs/) | Design specifications (`*-design.md`) |
| [plans/](plans/) | Implementation plans (`*-plan.md`) |
| [testing/](testing/) | Test plans (`*-testplan.md`) and test reports (`*-report.md`) |

### Audits

| Document | Purpose |
|---|---|
| [audits/2026-03-18-schema-audit.md](audits/2026-03-18-schema-audit.md) | Database schema audit (score, findings, remediation) |
| [audits/ux-ui-audit.md](audits/ux-ui-audit.md) | Frontend UX/UI automated audit |
| [audits/stub-tracker.md](audits/stub-tracker.md) | Stub/placeholder inventory and status |

### Prompts

| Document | Purpose |
|---|---|
| [prompts/stub-finder.md](prompts/stub-finder.md) | Claude Code prompt for stub discovery |

### Documentation Rules

See [CLAUDE.md](../CLAUDE.md) section "Documentation Policy" for rules about when to update which documentation files, how to track TODOs, and how to create feature specs and ADRs.
```

- [ ] **Step 2: Commit**

```bash
git add docs/README.md
git commit -m "docs: rewrite README.md index for new directory structure"
```

---

## Task 8: Update `CLAUDE.md` Documentation Structure

**Files:**
- Modify: `CLAUDE.md` (Documentation Structure section and Documentation Policy table)

- [ ] **Step 1: Update the Documentation Structure tree in CLAUDE.md**

Find the `### Documentation Structure` section and replace the directory tree with:

```
docs/
├── README.md                        # Documentation index (start here)
├── todo.md                          # Central TODO tracker for LLM agents
├── architecture.md                  # System architecture overview
├── dev-setup.md                     # Development setup guide
├── project-status.md                # Phase tracking & milestones
├── tech-stack.md                    # Languages, tools, dependencies
├── features/                        # Feature specifications (one per pillar)
│   ├── 01-project-dashboard.md      # Pillar 1: Multi-repo management
│   ├── 02-roadmap-feature-map.md    # Pillar 2: Visual roadmap, specs, PM sync
│   ├── 03-multi-llm-provider.md     # Pillar 3: LiteLLM, routing, cost tracking
│   ├── 04-agent-orchestration.md    # Pillar 4: Agent modes, execution, safety
│   ├── 05-chat-enhancements.md      # Chat UI features, HITL, notifications
│   └── 06-visual-design-canvas.md   # SVG canvas, triple-output export
├── specs/                           # Design specifications (*-design.md)
├── plans/                           # Implementation plans (*-plan.md)
├── testing/                         # Test plans (*-testplan.md) and reports (*-report.md)
├── audits/                          # Schema, UX, and code audits
├── architecture/                    # Detailed architecture documents
│   └── adr/                         # Architecture Decision Records
├── research/                        # Market research & analysis
└── prompts/                         # Prompt templates
```

- [ ] **Step 2: Update the "When to Update Which Document" table**

Find the table under `### When to Update Which Document` and update these rows:

| Change Type | Update These Files |
|---|---|
| New feature work | `docs/features/*.md` (scope, design, TODOs), `docs/todo.md` |
| Architecture decision | `docs/architecture.md`, `docs/architecture/adr/`, `CLAUDE.md` |
| New dependency/tool | `docs/tech-stack.md`, `docs/dev-setup.md` |
| Completed milestone | `docs/project-status.md`, `docs/todo.md` |
| New directory/port/env var | `docs/dev-setup.md` |
| Core pillar changes | `CLAUDE.md`, relevant `docs/features/*.md` |
| New design spec | `docs/specs/*-design.md`, `docs/todo.md` |
| New implementation plan | `docs/plans/*-plan.md`, `docs/todo.md` |
| Test results/plans | `docs/testing/`, `docs/todo.md` |
| Audit findings | `docs/audits/`, `docs/todo.md` |
| Any code change | `docs/todo.md` (mark task done or add new tasks) |

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md documentation structure for new layout"
```

---

## Task 9: Final Verification

- [ ] **Step 1: Verify directory structure is clean**

```bash
cd /workspaces/CodeForge

echo "=== New directories ==="
ls -d docs/specs docs/plans docs/testing docs/audits

echo "=== Removed directories ==="
test -d docs/superpowers && echo "FAIL: superpowers still exists" || echo "OK: superpowers gone"
test -d docs/audit && echo "FAIL: audit/ still exists" || echo "OK: audit/ gone"

echo "=== File counts ==="
echo "specs: $(ls docs/specs/*.md | wc -l) (expected: 15)"
echo "plans: $(ls docs/plans/*.md | wc -l) (expected: 23)"
echo "testing: $(ls docs/testing/*.md | wc -l) (expected: 6)"
echo "audits: $(ls docs/audits/*.md | wc -l) (expected: 3)"
```

- [ ] **Step 2: Verify no stale references remain**

```bash
# These greps should ALL return empty
grep -rn "superpowers/" docs/ CLAUDE.md --include="*.md" || echo "OK: no superpowers refs"
grep -rn "docs/audit/" docs/ CLAUDE.md --include="*.md" || echo "OK: no docs/audit/ refs"
grep -rn "docs/ux-ui-audit\.md" docs/ CLAUDE.md --include="*.md" || echo "OK: no root ux-ui-audit refs"
grep -rn "docs/stub-tracker\.md" docs/ CLAUDE.md --include="*.md" || echo "OK: no root stub-tracker refs"
grep -rn "docs/benchmark-e2e-test-plan\.md" docs/ CLAUDE.md --include="*.md" || echo "OK: no root benchmark refs"
```

- [ ] **Step 3: Verify naming convention compliance**

```bash
# All specs should end in -design.md
for f in docs/specs/*.md; do
  [[ "$f" == *-design.md ]] || echo "NAMING VIOLATION: $f"
done

# All plans should end in -plan.md
for f in docs/plans/*.md; do
  [[ "$f" == *-plan.md ]] || echo "NAMING VIOLATION: $f"
done

# All test plans should end in -testplan.md, reports in -report.md
for f in docs/testing/*.md; do
  [[ "$f" == *-testplan.md || "$f" == *-report.md ]] || echo "NAMING VIOLATION: $f"
done
```

- [ ] **Step 4: Push**

```bash
git push
```

---

## Summary

| Metric | Count |
|---|---|
| Files moved | 32 |
| Files renamed | 16 |
| Cross-references updated | ~35 |
| New directories | 3 (`specs/`, `testing/`, `audits/`) |
| Removed directories | 3 (`superpowers/`, `superpowers/specs/`, `audit/`) |
| Commits | 8 |
