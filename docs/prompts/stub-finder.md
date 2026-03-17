# Stub Finder Prompt for Claude Code

Copy the prompt block below and paste it into a Claude Code session opened at the CodeForge project root.

---

## Prompt

```
Find every stub, placeholder, and unimplemented code path in this project. Produce a structured report grouped by severity.

## Scan Scope

Search ALL files under these directories:
- `internal/` and `cmd/` (Go)
- `workers/` (Python)
- `frontend/src/` (TypeScript)
- `docs/` (Markdown, YAML)
- `*.yaml` and `*.sql` in the project root and `migrations/`

## Patterns to Find

### Go (internal/, cmd/)

1. **Comment markers:** `// TODO`, `// FIXME`, `// HACK`, `// PLACEHOLDER`, `// STUB` (case-insensitive)
2. **Phase-marker stubs:** `// STATUS: Phase` indicating incomplete implementations
3. **In-memory stub stores:** Structs or maps used as temporary replacements for real persistence (e.g., `map[string]*TaskResponse` in a2a handler)
4. **Hardcoded placeholder data:** Comments containing "hardcoded placeholder" or "placeholders" noting fake data (e.g., agentcard.go skills)
5. **Stub return nil:** Methods that `return nil` as the sole body or with only a log statement, where the function signature implies real work should happen (check surrounding comments for "stub", "todo", "placeholder")

### Python (workers/)

1. **Comment markers:** `# TODO`, `# FIXME` (case-insensitive)
2. **NotImplementedError:** `raise NotImplementedError` anywhere
3. **Pass-only bodies:** Functions/methods where `pass` is the ONLY statement in the body — but EXCLUDE these false positives:
   - Protocol/ABC method stubs (class inherits `Protocol` or `ABC`)
   - Exception class bodies (`class FooError(Exception): pass`)
   - `__init__.py` files
   - Inside `except` blocks (intentional silencing)
4. **Named stub classes:** `StubBackendExecutor`, `_BenchmarkRuntime`, or any class with "Stub" or "Fake" in its name that lives in production code (not `tests/`)
5. **No-op methods in runtime classes:** Methods in `_BenchmarkRuntime` or similar that return hardcoded values or empty dicts

### TypeScript (frontend/src/)

1. **Comment markers:** `// TODO`, `// FIXME` (case-insensitive)
2. **Empty async fallbacks:** `async () => {}` used as default callbacks or placeholder handlers
3. **Console.log placeholders:** `console.log("TODO` or `console.warn("not implemented`

### Config / Docs / SQL

1. **TODO/TBD/WIP markers** in Markdown files and YAML comments (case-insensitive)
2. **"Not yet implemented"** in feature docs
3. **`changeme`** or obvious placeholder passwords/tokens in example configs (exclude test fixtures that intentionally use test credentials)
4. **Commented-out feature sections** in YAML config files (blocks of 3+ consecutive commented lines that describe features)
5. **Unchecked items** `[ ]` in `docs/todo.md` — count only, do not list each one

## Exclusions (False Positives to Skip)

- **Protocol/ABC stubs in Python:** `...` (Ellipsis) as body in Protocol or ABC classes is correct Python — skip these
- **HTML `placeholder=` attributes** in TypeScript/JSX — these are form field hints, not code stubs
- **`nolint` directives** in Go — intentional suppressions, not stubs
- **Test files:** `*_test.go`, `tests/`, `*.spec.ts`, `*.test.ts` — report separately under "Test Stubs" category (they may contain useful stubs for tracking but are lower priority)
- **`nopCloser` / `io.NopCloser`** — standard Go pattern, not a stub
- **Intentional no-ops** with explicit comments like "intentionally empty" or "no-op by design"
- **`pass` in `except` blocks** with explicit `# silenced` or logged errors
- **Migration files** (`migrations/*.sql`) — `changeme` in seeds is expected for dev environments; only flag if in production config examples
- **`codeforge.example.yaml`** — `changeme` here is correct (it's an example); only flag if it appears in non-example config files

## Output Format

Organize results into these severity categories:

### CRITICAL — Production Stubs (code that will fail or silently no-op in production)
For each finding: `file:line` — brief description of what's stubbed and what it should do

### HIGH — Incomplete Features (partially implemented, marked for future work)
For each finding: `file:line` — the TODO/marker text and what phase/feature it belongs to

### MEDIUM — Hardcoded/Placeholder Data (works but with fake data)
For each finding: `file:line` — what data is hardcoded and what it should be replaced with

### LOW — Documentation TODOs (docs that reference unfinished work)
For each finding: `file:line` — the marker text

### INFO — Test Stubs (stubs that exist only in test code)
Summary count only, grouped by language

### Summary Table

At the end, produce a summary table:

| Category | Count | Top Files |
|----------|-------|-----------|
| CRITICAL | N | file1, file2, ... |
| HIGH | N | file1, file2, ... |
| MEDIUM | N | file1, file2, ... |
| LOW | N | file1, file2, ... |
| INFO | N | (test files) |
| docs/todo.md unchecked | N | — |
| **TOTAL** | **N** | |

## Execution Instructions

1. Use Grep tool with appropriate patterns and file type filters — do NOT use Bash grep/rg
2. For `pass`-only bodies in Python, use multiline grep or read suspicious files to verify context
3. For `return nil` stubs in Go, check surrounding comments to distinguish real stubs from legitimate nil returns
4. Cross-reference TODO comments with `docs/todo.md` to identify which are already tracked
5. When uncertain whether something is a stub or intentional, include it with a `[?]` marker
6. Work through each language systematically: Go → Python → TypeScript → Config/Docs
```
