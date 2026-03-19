# Tool Call Complexity Plan — S2, S3, S4 Scenarios

> **Goal:** Define which tool call patterns each scenario MUST exercise,
> based on real coding agent behavior (Aider, SWE-agent, OpenHands, Cline).
>
> **Available Tools (12):**
> `read_file`, `write_file`, `edit_file`, `bash`, `search_files`, `glob_files`,
> `list_directory`, `search_conversations`, `search_skills`, `create_skill`,
> `handoff_to`, `propose_goal`

---

## S1 Recap: What was actually tested

| Phase | Tools Used | Complexity |
|-------|-----------|------------|
| Create files | write_file x2 | Trivial (2 small files) |
| Fix errors | edit_file x15 | Simple (1-3 line replacements) |
| Run tests | bash x5 | Trivial (pytest, py_compile) |
| Read file | read_file x1 | Trivial (single file) |

**Missing:** search_files, glob_files, list_directory, search_conversations, git operations, multi-file coordination, dependency management, build pipelines

---

## S2: Medium — Multi-Module CLI App

### Expected Tool Call Pipeline (6 phases, ~40-60 calls)

#### Phase 1: Exploration (5-8 calls)
The agent MUST explore the workspace before writing anything.

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 1.1 | `list_directory` | `path=".", recursive=false` | Understand workspace root |
| 1.2 | `glob_files` | `pattern="**/*.py"` | Find existing Python files |
| 1.3 | `glob_files` | `pattern="**/*.toml"` | Check for existing config |
| 1.4 | `read_file` | README.md | Understand project context |
| 1.5 | `search_files` | `pattern="import", include="*.py"` | Check existing imports |

**Validates:** Agent doesn't blindly overwrite existing files.

#### Phase 2: Project Setup (4-6 calls)
Create package structure with proper configuration.

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 2.1 | `write_file` | `pyproject.toml` | Project metadata, dependencies (pytest) |
| 2.2 | `bash` | `mkdir -p task_manager tests` | Create package directories |
| 2.3 | `write_file` | `task_manager/__init__.py` | Package init |
| 2.4 | `write_file` | `task_manager/__main__.py` | Entry point (`python -m task_manager`) |
| 2.5 | `bash` | `python -m py_compile task_manager/__init__.py` | Verify syntax |

**Validates:** Proper Python package structure, not just loose scripts.

#### Phase 3: Core Implementation (8-12 calls)
Build modules iteratively, verifying each.

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 3.1 | `write_file` | `task_manager/models.py` | Task dataclass, enums |
| 3.2 | `bash` | `python -c "from task_manager.models import Task"` | Verify import |
| 3.3 | `write_file` | `task_manager/storage.py` | JSON load/save |
| 3.4 | `bash` | `python -c "from task_manager.storage import load_tasks"` | Verify import |
| 3.5 | `write_file` | `task_manager/cli.py` | argparse subcommands |
| 3.6 | `bash` | `python -m task_manager --help` | Verify CLI works |
| 3.7 | `edit_file` | `task_manager/__main__.py` | Wire CLI entry point |
| 3.8 | `bash` | `python -m task_manager add --title "test"` | Smoke test |

**Validates:** Multi-file coordination, cross-module imports, iterative build-verify.

#### Phase 4: Test Suite (6-10 calls)
Write tests and run them, fixing failures.

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 4.1 | `write_file` | `tests/__init__.py` | Test package init |
| 4.2 | `write_file` | `tests/conftest.py` | Shared fixtures (tmp_path) |
| 4.3 | `write_file` | `tests/test_models.py` | Model tests |
| 4.4 | `write_file` | `tests/test_storage.py` | Storage tests |
| 4.5 | `write_file` | `tests/test_cli.py` | CLI integration tests |
| 4.6 | `bash` | `python -m pytest tests/ -v` | Run all tests |
| 4.7 | `read_file` | (failing test file) | Read to understand failure |
| 4.8 | `edit_file` | (fix the test) | Fix based on error output |
| 4.9 | `bash` | `python -m pytest tests/ -v` | Re-run after fix |

**Validates:** Test-driven fix cycle, reading error output, targeted edits.

#### Phase 5: Quality & Documentation (4-6 calls)
Lint, sort imports, write docs.

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 5.1 | `bash` | `python -m py_compile task_manager/*.py` | Syntax check all |
| 5.2 | `bash` | `python -m ruff check task_manager/ tests/` | Lint check |
| 5.3 | `edit_file` | (fix lint issues) | Auto-fix lint |
| 5.4 | `write_file` | `README.md` | Usage documentation |
| 5.5 | `bash` | `python -m task_manager --help` | Verify help text for README |

**Validates:** Quality awareness, documentation generation.

#### Phase 6: Git Commit (2-3 calls)

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 6.1 | `bash` | `git add -A` | Stage all |
| 6.2 | `bash` | `git status` | Verify what's staged |
| 6.3 | `bash` | `git commit -m "feat: CLI task manager"` | Commit |

**Validates:** Git workflow integration.

### S2 Tool Coverage Target

| Tool | Min Calls | Purpose |
|------|-----------|---------|
| `list_directory` | 2 | Explore workspace, verify structure |
| `glob_files` | 2 | Find Python files, find configs |
| `read_file` | 3 | README, failing tests, existing code |
| `write_file` | 8 | pyproject.toml, 5 modules, 4 test files, README |
| `edit_file` | 5 | Wire entry point, fix tests, fix lint |
| `bash` | 12 | mkdir, py_compile, pytest, ruff, git, smoke tests |
| `search_files` | 1 | Check existing imports |
| **TOTAL** | **33+** | |

---

## S3: Hard — Brownfield Feature Extension

### Expected Tool Call Pipeline (7 phases, ~50-80 calls)

> **Key difference from S2:** The agent MUST read and understand existing code
> before modifying it. Search and Read are the PRIMARY tools, not Write.

#### Phase 1: Codebase Discovery (8-12 calls)
The agent MUST map the entire codebase structure.

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 1.1 | `list_directory` | `path=".", recursive=true` | Full directory tree |
| 1.2 | `glob_files` | `pattern="**/*.py"` | All Python files |
| 1.3 | `glob_files` | `pattern="**/test_*.py"` | All test files specifically |
| 1.4 | `glob_files` | `pattern="**/*.toml"` | Config files |
| 1.5 | `read_file` | `README.md` | Project overview |
| 1.6 | `read_file` | `pyproject.toml` | Dependencies, project config |
| 1.7 | `list_directory` | `path="task_manager"` | Package structure |
| 1.8 | `list_directory` | `path="tests"` | Test structure |

**Validates:** Systematic exploration before any modifications.

#### Phase 2: Code Understanding (10-15 calls)
The agent MUST read and search to understand the codebase deeply.

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 2.1 | `read_file` | `task_manager/models.py` | Understand Task dataclass |
| 2.2 | `read_file` | `task_manager/cli.py` | Understand CLI structure |
| 2.3 | `read_file` | `task_manager/storage.py` | Understand JSON format |
| 2.4 | `search_files` | `pattern="class Task", include="*.py"` | Find Task definition |
| 2.5 | `search_files` | `pattern="def add", include="*.py"` | Find add command impl |
| 2.6 | `search_files` | `pattern="argparse\|add_parser\|subparser"` | Find CLI registration |
| 2.7 | `search_files` | `pattern="json.dump\|json.load", include="*.py"` | Find serialization |
| 2.8 | `read_file` | `tests/test_cli.py` | Understand test patterns |
| 2.9 | `read_file` | `tests/conftest.py` | Understand fixtures |
| 2.10 | `search_files` | `pattern="def test_", include="test_*.py"` | Count existing tests |

**Validates:** Agent understands BEFORE modifying. Uses search_files heavily.

#### Phase 3: Baseline Verification (2-3 calls)
Run existing tests to establish baseline.

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 3.1 | `bash` | `python -m pytest tests/ -v` | Baseline: all tests pass |
| 3.2 | `bash` | `python -m task_manager --help` | Baseline: CLI works |

**Validates:** Agent verifies baseline BEFORE making changes (regression awareness).

#### Phase 4: Data Model Extension (6-8 calls)
Add tags field to Task model, update serialization.

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 4.1 | `read_file` | `task_manager/models.py` | Re-read before editing |
| 4.2 | `edit_file` | `task_manager/models.py` | Add `tags: list[str]` field |
| 4.3 | `edit_file` | `task_manager/models.py` | Update `to_dict()` for tags |
| 4.4 | `edit_file` | `task_manager/models.py` | Update `from_dict()` with default `[]` |
| 4.5 | `bash` | `python -c "from task_manager.models import Task; t=Task(...); print(t.tags)"` | Verify |
| 4.6 | `bash` | `python -m pytest tests/test_models.py -v` | Existing model tests still pass |

**Validates:** Surgical edits to existing code, backward compatibility, regression check after each change.

#### Phase 5: CLI Extension (6-10 calls)
Add --tags to add command, add search subcommand.

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 5.1 | `read_file` | `task_manager/cli.py` | Re-read current CLI |
| 5.2 | `search_files` | `pattern="add_parser\|add_argument.*title"` | Find exact edit point |
| 5.3 | `edit_file` | `task_manager/cli.py` | Add `--tags` argument to add command |
| 5.4 | `edit_file` | `task_manager/cli.py` | Add search subparser |
| 5.5 | `edit_file` | `task_manager/cli.py` | Implement search handler |
| 5.6 | `edit_file` | `task_manager/cli.py` | Show tags in list output |
| 5.7 | `bash` | `python -m task_manager add --title "test" --tags "a,b"` | Test new flag |
| 5.8 | `bash` | `python -m task_manager search --tag "a"` | Test search |
| 5.9 | `bash` | `python -m pytest tests/ -v` | ALL tests still pass |

**Validates:** Extending existing code without breaking it. search_files to find edit points.

#### Phase 6: Storage Migration (4-6 calls)
Handle old JSON format without tags.

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 6.1 | `read_file` | `task_manager/storage.py` | Understand load logic |
| 6.2 | `edit_file` | `task_manager/storage.py` | Add `tags` default in `from_dict` |
| 6.3 | `bash` | Create test JSON without tags field, load it | Migration test |
| 6.4 | `bash` | `python -m pytest tests/ -v` | Regression check |

**Validates:** Data migration awareness, backward compatibility.

#### Phase 7: New Tests + Docs + Git (8-12 calls)

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 7.1 | `read_file` | `tests/test_cli.py` | Understand test patterns to follow |
| 7.2 | `write_file` | `tests/test_tags.py` | New test file for tags feature |
| 7.3 | `bash` | `python -m pytest tests/test_tags.py -v` | New tests pass |
| 7.4 | `bash` | `python -m pytest tests/ -v` | ALL tests pass (old + new) |
| 7.5 | `read_file` | `README.md` | Read current docs |
| 7.6 | `edit_file` | `README.md` | Add tags documentation |
| 7.7 | `bash` | `python -m ruff check task_manager/ tests/` | Lint check |
| 7.8 | `bash` | `git diff --stat` | Review changes |
| 7.9 | `bash` | `git add -A && git commit -m "feat: add tags"` | Commit |

**Validates:** Following existing test patterns, documentation updates, git workflow.

### S3 Tool Coverage Target

| Tool | Min Calls | Purpose |
|------|-----------|---------|
| `list_directory` | 3 | Workspace, package, tests dirs |
| `glob_files` | 3 | Python files, test files, configs |
| `read_file` | 10 | Models, CLI, storage, tests, README, conftest |
| `write_file` | 1 | New test file only |
| `edit_file` | 10 | Models (3), CLI (4), storage (1), README (1), fixes |
| `bash` | 15 | pytest (5x), smoke tests (4x), ruff, git (3x), migration test |
| `search_files` | 5 | Find definitions, imports, patterns, edit points |
| **TOTAL** | **47+** | |

---

## S4: Expert — TypeScript REST API

### Expected Tool Call Pipeline (8 phases, ~70-100 calls)

> **Key differences:** Different language (TypeScript), compile step required,
> HTTP server testing, npm dependency management, strict type system.

#### Phase 1: Project Bootstrap (6-8 calls)

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 1.1 | `list_directory` | `path="."` | Check workspace |
| 1.2 | `bash` | `npm init -y` | Create package.json |
| 1.3 | `edit_file` | `package.json` | Add scripts: build, test, start, dev |
| 1.4 | `write_file` | `tsconfig.json` | Strict mode, ESM, outDir |
| 1.5 | `bash` | `npm install typescript express @types/express` | Install deps |
| 1.6 | `bash` | `npm install -D vitest @types/node` | Dev deps |
| 1.7 | `bash` | `mkdir -p src/routes src/models src/storage src/middleware tests` | Structure |
| 1.8 | `write_file` | `.gitignore` | node_modules, dist |

**Validates:** npm workflow, TypeScript config, dependency management.

#### Phase 2: Types & Models (5-7 calls)

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 2.1 | `write_file` | `src/models/bookmark.ts` | Bookmark interface |
| 2.2 | `write_file` | `src/models/errors.ts` | AppError class, HTTP codes |
| 2.3 | `write_file` | `src/models/validation.ts` | URL validator, field validator |
| 2.4 | `bash` | `npx tsc --noEmit` | **Compile check — catch type errors early** |
| 2.5 | `edit_file` | (fix any type errors) | Fix based on tsc output |
| 2.6 | `bash` | `npx tsc --noEmit` | Re-verify |

**Validates:** TypeScript compile-fix cycle. Agent MUST run tsc after each module.

#### Phase 3: Storage Layer (5-7 calls)

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 3.1 | `write_file` | `src/storage/store.ts` | BookmarkStore interface |
| 3.2 | `write_file` | `src/storage/memory.ts` | InMemoryStore |
| 3.3 | `write_file` | `src/storage/file.ts` | JsonFileStore |
| 3.4 | `bash` | `npx tsc --noEmit` | Compile check |
| 3.5 | `search_files` | `pattern="BookmarkStore", include="*.ts"` | Verify interface usage |

**Validates:** Interface-driven design, compile verification.

#### Phase 4: HTTP Server + Routes (8-12 calls)

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 4.1 | `write_file` | `src/server.ts` | Express app setup |
| 4.2 | `write_file` | `src/routes/bookmarks.ts` | CRUD endpoints |
| 4.3 | `write_file` | `src/middleware/error-handler.ts` | Error middleware |
| 4.4 | `write_file` | `src/index.ts` | Main entry, wire routes |
| 4.5 | `bash` | `npx tsc --noEmit` | Compile check |
| 4.6 | `read_file` | (file with type errors) | Read tsc error output |
| 4.7 | `edit_file` | (fix type errors) | Fix imports, types |
| 4.8 | `bash` | `npx tsc --noEmit` | Re-verify |
| 4.9 | `bash` | `npm run build` | Full build (compile to dist/) |
| 4.10 | `bash` | `npm start & sleep 2 && curl http://localhost:3000/bookmarks && kill %1` | Smoke test |

**Validates:** Full build pipeline, HTTP server startup, curl integration test.

#### Phase 5: Validation & Error Handling (6-8 calls)

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 5.1 | `read_file` | `src/routes/bookmarks.ts` | Review current endpoints |
| 5.2 | `edit_file` | `src/routes/bookmarks.ts` | Add URL validation |
| 5.3 | `edit_file` | `src/routes/bookmarks.ts` | Add required field checks |
| 5.4 | `edit_file` | `src/middleware/error-handler.ts` | Proper error JSON format |
| 5.5 | `bash` | `npx tsc --noEmit` | Compile check |
| 5.6 | `bash` | Start server + `curl -X POST ... -d '{"title":"Bad"}'` | Test 400 response |
| 5.7 | `search_files` | `pattern=": any", include="*.ts"` | Check for forbidden `any` types |
| 5.8 | `glob_files` | `pattern="src/**/*.ts"` | Verify file structure |

**Validates:** Input validation, error handling, type strictness check via search.

#### Phase 6: Test Suite (8-12 calls)

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 6.1 | `write_file` | `tests/bookmarks.test.ts` | CRUD API tests |
| 6.2 | `write_file` | `tests/validation.test.ts` | Validation tests |
| 6.3 | `write_file` | `tests/storage.test.ts` | Storage tests |
| 6.4 | `bash` | `npm test` | Run vitest |
| 6.5 | `read_file` | (failing test) | Understand failure |
| 6.6 | `search_files` | `pattern="describe\|it\(", include="*.test.ts"` | Check test coverage |
| 6.7 | `edit_file` | (fix test or source) | Fix based on error |
| 6.8 | `bash` | `npm test` | Re-run |
| 6.9 | `bash` | `npm run build` | Final build |

**Validates:** vitest workflow, test-fix cycle across TypeScript.

#### Phase 7: Quality Checks (4-6 calls)

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 7.1 | `bash` | `npx tsc --noEmit --strict` | Strict mode verify |
| 7.2 | `search_files` | `pattern=": any\|as any\|<any>", include="*.ts"` | No `any` types |
| 7.3 | `bash` | `npx eslint src/ 2>/dev/null` | ESLint (if configured) |
| 7.4 | `glob_files` | `pattern="src/**/*.ts"` | Verify all expected files exist |
| 7.5 | `bash` | `wc -l src/**/*.ts` | Code size metrics |

**Validates:** Type strictness, lint, completeness.

#### Phase 8: Documentation + Git (4-6 calls)

| Step | Tool | Call | Purpose |
|------|------|------|---------|
| 8.1 | `write_file` | `README.md` | API docs with curl examples |
| 8.2 | `bash` | `npm start & sleep 2 && curl -s ... && kill %1` | Generate real curl output for README |
| 8.3 | `bash` | `git add -A && git status` | Review |
| 8.4 | `bash` | `git commit -m "feat: bookmark REST API"` | Commit |

### S4 Tool Coverage Target

| Tool | Min Calls | Purpose |
|------|-----------|---------|
| `list_directory` | 2 | Workspace, src structure |
| `glob_files` | 3 | TS files, test files, verify structure |
| `read_file` | 5 | Type errors, failing tests, review code |
| `write_file` | 12 | tsconfig, package.json, 8 src files, 3 test files, README |
| `edit_file` | 8 | Fix type errors (4), add validation (3), fix tests (1) |
| `bash` | 20 | npm install, tsc (6x), npm test (3x), npm build (2x), curl tests (3x), git (3x) |
| `search_files` | 4 | Find types, check any, find patterns |
| **TOTAL** | **54+** | |

---

## Comparison: Tool Call Complexity Across Scenarios

| Metric | S1 (Easy) | S2 (Medium) | S3 (Hard) | S4 (Expert) |
|--------|-----------|-------------|-----------|-------------|
| **Total tool calls** | 20-30 | 40-60 | 50-80 | 70-100 |
| **Unique tools used** | 3-4 | 6-7 | 7 | 7 |
| **Explore before edit** | No | Light | Heavy | Medium |
| **Search calls** | 0 | 1-2 | 5+ | 4+ |
| **Build-verify cycles** | 1 | 3-4 | 5-6 | 6-8 |
| **Multi-file edits** | 0 | 3-5 | 10+ | 8+ |
| **Git operations** | 0-1 | 2-3 | 3-4 | 3-4 |
| **Dependency mgmt** | 0 | 1 (pyproject) | 0 | 3 (npm) |
| **Compile checks** | 0 | 0 | 0 | 6+ (tsc) |
| **Cross-file search** | 0 | 1 | 5+ | 4+ |
| **Error recovery** | Weak | Medium | Strong (regression) | Strong (type errors) |
| **Key challenge** | Write files | Package structure | Understand existing code | Type system + build |
