# Autonomous Goal-to-Program Test Plan — Claude Code + Playwright-MCP

**Date:** 2026-03-19
**Type:** Interactive AI QA Test (Claude Code drives browser via playwright-mcp)
**Scope:** Core autonomous workflow: Chat -> Goals -> Roadmap -> Execution -> Working Program
**Coverage:** 9 phases, sequential, real LLM calls (no mocks)

---

## How to Use This Document

This is a **runbook for Claude Code sessions** using playwright-mcp tools. Claude Code drives the CodeForge frontend interactively, observes results via `browser_snapshot`, and adapts via Decision Trees when steps fail.

**Prerequisites:**
- Playwright-MCP connected (`/mcp` shows playwright-mcp)
- At least one LLM API key configured (ANTHROPIC_API_KEY, OPENAI_API_KEY, etc.)
- **Claude Code is responsible for starting ALL required services** — see Phase 0

**Execution:**
- Follow phases 0-8 sequentially (strict order — each gates the next)
- Each phase: execute Steps, check Validation, follow Decision Tree on failure
- Record results as PASS / PARTIAL / SKIP / FAIL per phase
- Save report to `docs/testing/YYYY-MM-DD-autonomous-goal-to-program-report.md`

**State Variables** (carried across phases):
```
SCENARIO = "S2"   # Selected scenario (S1/S2/S3/S4) — default S2 for first run
ENV = {}          # Phase 0: models, devMode, token
PROJECT_ID = ""   # Phase 1: created project UUID
WORKSPACE = ""    # Phase 1: project workspace path
BRANCH = ""       # Phase 1: test branch name
GOAL_COUNT = 0    # Phase 2-3: detected goals
ROADMAP_OK = false # Phase 4: roadmap created
EXEC_STARTED = false # Phase 5-6: execution dispatched
TOOL_CALLS = {}   # Phase 6-7: tool call counts by type
```

---

## Phase Dependency Graph

```
Phase 0: Environment Discovery & Login
    |
    v
Phase 1: Project Setup (create project via UI, adopt/clone repo)
    |
    v
Phase 2: Goal Discovery (AI Discover via chat)
    |
    v
Phase 3: Goal Validation (verify goals in GoalsPanel)
    |
    v
Phase 4: Roadmap Creation (build roadmap + milestones + features)
    |
    v
Phase 5: Autonomous Execution (agentic chat executes roadmap)
    |    \
    |     v
    |   Phase 5b: Blocker & HITL Handling
    |     |
    v     v
Phase 6: Execution Monitoring (poll progress, check commits)
    |
    v
Phase 7: Program Validation (run program in workspace)
    |
    v
Phase 8: Report & Cleanup
```

---

## Test Scenarios — Difficulty Levels

Each test run picks **one scenario**. Scenarios are ordered by difficulty and exercise progressively more tool calls. Run the easiest first to validate the pipeline, then increase difficulty.

### Scenario Selection

| ID | Difficulty | Language | Primary Tool Calls | Description |
|----|-----------|----------|-------------------|-------------|
| S1 | Easy | Python | Write, Bash | Greenfield: single-file script from scratch |
| S2 | Medium | Python | Write, Read, Bash, Glob, Search | Greenfield: multi-module CLI app with tests |
| S3 | Hard | Python | Read, Edit, Search, Glob, Bash, ListDir | Brownfield: extend existing codebase with new feature |
| S4 | Expert | TypeScript | Write, Read, Edit, Bash, Search, Glob, ListDir | Full-stack: REST API + tests + config + build |

### Tool Call Coverage Matrix

| Tool | S1 Easy | S2 Medium | S3 Hard | S4 Expert |
|------|---------|-----------|---------|-----------|
| **write_file** | Heavy (create all files) | Heavy (8+ modules, tests, config) | Light (1 new test file only) | Heavy (12+ src, tests, configs) |
| **read_file** | None | 3+ (README, failing tests) | **10+** (models, CLI, storage, tests, conftest) | 5+ (type errors, failing tests) |
| **edit_file** | Light (fix errors) | 5+ (wire entry point, fix tests/lint) | **10+** (models 3, CLI 4, storage 1, README 1) | 8+ (fix type errors 4, validation 3) |
| **bash** | Light (pytest, py_compile) | 12+ (mkdir, py_compile, pytest, ruff, git, smoke) | **15+** (pytest 5x, smoke 4x, ruff, git 3x, migration) | **20+** (npm install, tsc 6x, npm test 3x, build 2x, curl 3x, git 3x) |
| **search_files** | None | 1-2 (check imports) | **5+** (find definitions, imports, edit points) | 4+ (find types, check `any`, patterns) |
| **glob_files** | None | 2 (Python files, configs) | 3 (Python files, tests, configs) | 3 (TS files, tests, structure) |
| **list_directory** | None | 2 (workspace, verify structure) | **3** (workspace, package, tests) | 2 (workspace, src) |
| **propose_goal** | 3-4 goals | 5-7 goals | 6-8 goals | 8-12 goals |

### Minimum Tool Call Counts

| Metric | S1 | S2 | S3 | S4 |
|--------|----|----|----|----|
| **Total calls** | 20-30 | **33+** | **47+** | **54+** |
| **Unique tools** | 3-4 | 6-7 | 7 | 7 |
| **Explore-before-edit calls** | 0 | 5-8 | **8-12** | 6-8 |
| **Build-verify cycles** | 1 | 3-4 | 5-6 | **6-8** |
| **Error-recovery edits** | 0-2 | 2-4 | 3-5 | 4-6 |
| **Git operations** | 0-1 | 2-3 | 3-4 | 3-4 |

### Expected Tool Call Pipeline per Scenario

> **Real coding agents (Aider, SWE-agent, OpenHands) follow a structured pipeline,
> NOT random tool calls.** Each scenario must validate this pipeline:
>
> 1. **Explore** → list_directory, glob_files, read_file (understand workspace)
> 2. **Search** → search_files (find patterns, definitions, edit points)
> 3. **Build** → write_file, bash mkdir (create structure)
> 4. **Implement** → write_file, edit_file (create/modify code)
> 5. **Verify** → bash py_compile/tsc, bash pytest/npm test (compile + test)
> 6. **Fix** → read_file (understand error), edit_file (fix), bash (re-verify)
> 7. **Quality** → bash ruff/eslint, search_files (check patterns)
> 8. **Commit** → bash git add/commit

#### S2 Pipeline: Multi-Module Python CLI (6 phases)

```
Phase 1 — Explore (5-8 calls):
  list_directory "." → glob "**/*.py" → glob "**/*.toml" → read README.md → search "import" in *.py

Phase 2 — Project Setup (4-6 calls):
  write pyproject.toml → bash "mkdir -p task_manager tests"
  → write task_manager/__init__.py → write task_manager/__main__.py
  → bash "python -m py_compile task_manager/__init__.py"

Phase 3 — Core Implementation (8-12 calls):
  write models.py → bash "python -c 'from task_manager.models import Task'"
  → write storage.py → bash "python -c 'from task_manager.storage import load_tasks'"
  → write cli.py → bash "python -m task_manager --help"
  → edit __main__.py (wire entry) → bash "python -m task_manager add --title test"

Phase 4 — Test Suite (6-10 calls):
  write tests/__init__.py → write tests/conftest.py
  → write tests/test_models.py → write tests/test_storage.py → write tests/test_cli.py
  → bash "pytest tests/ -v" → read (failing test) → edit (fix) → bash "pytest -v"

Phase 5 — Quality (4-6 calls):
  bash "py_compile task_manager/*.py" → bash "ruff check task_manager/ tests/"
  → edit (fix lint) → write README.md → bash "python -m task_manager --help"

Phase 6 — Git (2-3 calls):
  bash "git add -A" → bash "git status" → bash "git commit -m 'feat: CLI task manager'"
```

#### S3 Pipeline: Brownfield Feature Extension (7 phases)

```
Phase 1 — Codebase Discovery (8-12 calls):
  list_directory "." recursive → glob "**/*.py" → glob "**/test_*.py" → glob "**/*.toml"
  → read README.md → read pyproject.toml → list_directory "task_manager" → list_directory "tests"

Phase 2 — Code Understanding (10-15 calls):
  read models.py → read cli.py → read storage.py
  → search "class Task" in *.py → search "def add" in *.py
  → search "argparse|add_parser|subparser" → search "json.dump|json.load" in *.py
  → read tests/test_cli.py → read tests/conftest.py → search "def test_" in test_*.py

Phase 3 — Baseline Verification (2-3 calls):
  bash "pytest tests/ -v" (all pass?) → bash "python -m task_manager --help"

Phase 4 — Data Model Extension (6-8 calls):
  read models.py → edit models.py (add tags field) → edit models.py (update to_dict)
  → edit models.py (update from_dict with default [])
  → bash "python -c 'from task_manager.models import Task; t=Task(...); print(t.tags)'"
  → bash "pytest tests/test_models.py -v" (regression check)

Phase 5 — CLI Extension (6-10 calls):
  read cli.py → search "add_parser|add_argument.*title"
  → edit cli.py (add --tags argument) → edit cli.py (add search subparser)
  → edit cli.py (implement search handler) → edit cli.py (show tags in list)
  → bash "python -m task_manager add --title test --tags a,b"
  → bash "python -m task_manager search --tag a"
  → bash "pytest tests/ -v" (ALL tests still pass)

Phase 6 — Storage Migration (4-6 calls):
  read storage.py → edit storage.py (add tags default)
  → bash (create old-format JSON, load it) → bash "pytest tests/ -v"

Phase 7 — New Tests + Docs + Git (8-12 calls):
  read tests/test_cli.py (follow existing patterns)
  → write tests/test_tags.py → bash "pytest tests/test_tags.py -v"
  → bash "pytest tests/ -v" (ALL pass) → read README.md → edit README.md (add tags docs)
  → bash "ruff check" → bash "git diff --stat" → bash "git add -A && git commit"
```

#### S4 Pipeline: TypeScript REST API (8 phases)

```
Phase 1 — Project Bootstrap (6-8 calls):
  list_directory "." → bash "npm init -y" → edit package.json (add scripts)
  → write tsconfig.json → bash "npm install typescript express @types/express"
  → bash "npm install -D vitest @types/node"
  → bash "mkdir -p src/routes src/models src/storage src/middleware tests"
  → write .gitignore

Phase 2 — Types & Models (5-7 calls):
  write src/models/bookmark.ts → write src/models/errors.ts → write src/models/validation.ts
  → bash "npx tsc --noEmit" → edit (fix type errors) → bash "npx tsc --noEmit"

Phase 3 — Storage Layer (5-7 calls):
  write src/storage/store.ts → write src/storage/memory.ts → write src/storage/file.ts
  → bash "npx tsc --noEmit" → search "BookmarkStore" in *.ts

Phase 4 — HTTP Server + Routes (8-12 calls):
  write src/server.ts → write src/routes/bookmarks.ts → write src/middleware/error-handler.ts
  → write src/index.ts → bash "npx tsc --noEmit" → read (type error file)
  → edit (fix imports/types) → bash "npx tsc --noEmit" → bash "npm run build"
  → bash "npm start & sleep 2 && curl localhost:3000/bookmarks && kill %1"

Phase 5 — Validation & Errors (6-8 calls):
  read src/routes/bookmarks.ts → edit (add URL validation) → edit (add field checks)
  → edit src/middleware/error-handler.ts → bash "npx tsc --noEmit"
  → bash (start + curl POST invalid → expect 400)
  → search ": any" in *.ts → glob "src/**/*.ts"

Phase 6 — Test Suite (8-12 calls):
  write tests/bookmarks.test.ts → write tests/validation.test.ts → write tests/storage.test.ts
  → bash "npm test" → read (failing test) → search "describe|it(" in *.test.ts
  → edit (fix test/source) → bash "npm test" → bash "npm run build"

Phase 7 — Quality (4-6 calls):
  bash "npx tsc --noEmit --strict" → search ": any|as any|<any>" in *.ts
  → bash "npx eslint src/" → glob "src/**/*.ts" → bash "wc -l src/**/*.ts"

Phase 8 — Docs + Git (4-6 calls):
  write README.md → bash (start + curl for README examples)
  → bash "git add -A && git status" → bash "git commit"
```

---

### S1: Easy — Build Your Own `wc` Tool (Greenfield, Single File)

> Inspired by [CodingChallengesFYI #1: Build Your Own wc](https://codingchallenges.fyi/challenges/challenge-wc/)

**Workspace Setup (Claude Code does this before sending the prompt):**
```bash
rm -rf /tmp/s1-wc-tool && mkdir -p /tmp/s1-wc-tool
cd /tmp/s1-wc-tool && git init -b main
# Create test data file (Lorem Ipsum, ~7000 words)
python3 -c "
import textwrap
text = 'The quick brown fox jumps over the lazy dog. ' * 800
with open('test.txt', 'w') as f:
    for i in range(100):
        f.write(text[i*350:(i+1)*350] + '\n')
"
wc test.txt > expected_wc_output.txt
echo "# Build Your Own wc Tool" > README.md
git add . && git commit -m "initial: test data"
```

**Project Setup (API):**
```bash
# 1. Create project
PROJECT_ID=$(curl -s -X POST /api/v1/projects -d '{"name":"S1-wc-tool","config":{"autonomy_level":"4","policy_preset":"trusted-mount-autonomous","execution_mode":"mount"}}' | jq -r '.id')
# 2. Adopt workspace (MANDATORY)
curl -s -X POST /api/v1/projects/$PROJECT_ID/adopt -d '{"path":"/tmp/s1-wc-tool"}'
# 3. Create goal (MANDATORY — prevents full-auto gate redirect)
curl -s -X POST /api/v1/projects/$PROJECT_ID/goals -d '{"kind":"requirement","title":"Build wc clone","content":"Build a Python wc clone with -c -l -w -m flags","priority":1}'
# 4. Create conversation + bypass approvals
CONV_ID=$(curl -s -X POST /api/v1/projects/$PROJECT_ID/conversations -d '{"title":"S1 wc tool"}' | jq -r '.id')
curl -s -X POST /api/v1/conversations/$CONV_ID/bypass-approvals
```

**Chat Prompt (agentic message):**
```json
{
  "content": "Build a Python clone of the Unix wc (word count) tool called ccwc.py.\n\nRequirements:\n1. Count bytes in a file: ccwc -c test.txt\n2. Count lines in a file: ccwc -l test.txt\n3. Count words in a file: ccwc -w test.txt\n4. Count characters in a file: ccwc -m test.txt\n5. Default (no flags): show lines, words, and bytes (like wc with no args)\n6. Read from stdin if no filename given: cat test.txt | python ccwc.py -l\n7. Handle missing file errors gracefully\n8. Write pytest tests in test_ccwc.py covering all flags and edge cases\n9. Commit all files to git\n\nA test file test.txt already exists in the workspace. Compare your output against the real wc command.\nThe workspace is /tmp/s1-wc-tool.",
  "role": "user",
  "model": "openai/container",
  "agentic": true
}
```

**Expected Tool Calls:** 15-30

| Tool | Expected Count | Purpose |
|------|---------------|---------|
| `read_file` | 1-2 | Read test.txt, README |
| `write_file` | 2-3 | ccwc.py, test_ccwc.py, README update |
| `edit_file` | 3-8 | Fix bugs after testing |
| `bash` | 5-10 | Run ccwc.py with flags, pytest, wc comparison, git |
| `list_directory` | 0-1 | Check workspace |
| `LLM` | 5-15 | Reasoning iterations |

**Validation (Claude Code runs after agent completes):**
```bash
cd /tmp/s1-wc-tool

# 1. Files exist
test -f ccwc.py && echo "PASS: ccwc.py exists" || echo "FAIL"
test -f test_ccwc.py && echo "PASS: test_ccwc.py exists" || echo "FAIL"

# 2. Syntax valid
python3 -m py_compile ccwc.py && echo "PASS: syntax" || echo "FAIL"
python3 -m py_compile test_ccwc.py && echo "PASS: test syntax" || echo "FAIL"

# 3. Byte count (-c) matches wc
EXPECTED=$(wc -c < test.txt | tr -d ' ')
ACTUAL=$(python3 ccwc.py -c test.txt 2>/dev/null | grep -oP '\d+' | head -1)
[ "$EXPECTED" = "$ACTUAL" ] && echo "PASS: -c byte count" || echo "FAIL: expected=$EXPECTED actual=$ACTUAL"

# 4. Line count (-l) matches wc
EXPECTED=$(wc -l < test.txt | tr -d ' ')
ACTUAL=$(python3 ccwc.py -l test.txt 2>/dev/null | grep -oP '\d+' | head -1)
[ "$EXPECTED" = "$ACTUAL" ] && echo "PASS: -l line count" || echo "FAIL: expected=$EXPECTED actual=$ACTUAL"

# 5. Word count (-w) matches wc
EXPECTED=$(wc -w < test.txt | tr -d ' ')
ACTUAL=$(python3 ccwc.py -w test.txt 2>/dev/null | grep -oP '\d+' | head -1)
[ "$EXPECTED" = "$ACTUAL" ] && echo "PASS: -w word count" || echo "FAIL: expected=$EXPECTED actual=$ACTUAL"

# 6. Default (no flag) shows lines words bytes
python3 ccwc.py test.txt 2>/dev/null | grep -qP '\d+\s+\d+\s+\d+' && echo "PASS: default output" || echo "FAIL"

# 7. Stdin mode
STDIN_RESULT=$(cat test.txt | python3 ccwc.py -l 2>/dev/null | grep -oP '\d+' | head -1)
[ "$STDIN_RESULT" = "$EXPECTED" ] && echo "PASS: stdin mode" || echo "FAIL"

# 8. Missing file error
python3 ccwc.py nonexistent.txt 2>/dev/null; [ $? -ne 0 ] && echo "PASS: error handling" || echo "FAIL"

# 9. Pytest
python3 -m pytest test_ccwc.py -v 2>&1 | tail -5

# 10. Lint
python3 -m ruff check ccwc.py test_ccwc.py 2>&1 | head -10

# 11. Git
git log --oneline | head -5
```

**Success Criteria (minimum for PASS):**
- `ccwc.py` and `test_ccwc.py` exist with valid syntax
- `-c`, `-l`, `-w` flags produce correct counts matching `wc`
- Missing file returns non-zero exit code
- At least 3/5 pytest tests pass

---

### S2: Medium — Build Your Own `cut` Tool (Greenfield, Multi-Module)

> Inspired by [CodingChallengesFYI #4: Build Your Own cut](https://codingchallenges.fyi/challenges/challenge-cut/)

**Workspace Setup:**
```bash
rm -rf /tmp/s2-cut-tool && mkdir -p /tmp/s2-cut-tool
cd /tmp/s2-cut-tool && git init -b main

# Create tab-separated test data (movie ratings)
python3 -c "
lines = [
    'f0\tf1\tf2\tf3\tf4\tf5',
    '1\tJohn Smith\t35\tM\t90000\tNew York',
    '2\tJane Doe\t28\tF\t85000\tSan Francisco',
    '3\tBob Wilson\t42\tM\t110000\tChicago',
    '4\tAlice Brown\t31\tF\t95000\tBoston',
    '5\tCharlie Davis\t55\tM\t120000\tSeattle',
]
with open('sample.tsv', 'w') as f:
    f.write('\n'.join(lines) + '\n')

# CSV test data
csv_lines = [
    'id,name,age,city',
    '1,Alice,30,New York',
    '2,Bob,25,Los Angeles',
    '3,Charlie,35,Chicago',
]
with open('sample.csv', 'w') as f:
    f.write('\n'.join(csv_lines) + '\n')
"

# Generate expected outputs for validation
cut -f2 sample.tsv > expected_f2.txt
cut -d',' -f1,3 sample.csv > expected_csv_f13.txt

echo "# Build Your Own cut Tool" > README.md
git add . && git commit -m "initial: test data"
```

**Project Setup (API):** Same pattern as S1 with `name: "S2-cut-tool"`, `path: "/tmp/s2-cut-tool"`, goal: "Build cut clone".

**Chat Prompt:**
```json
{
  "content": "Build a Python clone of the Unix cut tool as a proper Python package.\n\nRequirements:\n1. Create a package structure: cccut/ with __init__.py, __main__.py, parser.py, cutter.py\n2. Field extraction: cccut -f2 sample.tsv (extract field 2, tab-delimited)\n3. Custom delimiter: cccut -f1 -d',' sample.csv\n4. Multiple fields: cccut -f1,3 sample.tsv or cccut -f '1 3' sample.tsv\n5. Read from stdin: cat sample.tsv | python -m cccut -f2\n6. Error handling: missing file, invalid field number, no -f specified\n7. Create pyproject.toml with pytest dependency\n8. Write comprehensive tests in tests/ directory:\n   - tests/__init__.py\n   - tests/test_parser.py (argument parsing)\n   - tests/test_cutter.py (field extraction logic)\n   - tests/test_cli.py (end-to-end CLI tests)\n9. Add a README.md with usage examples\n10. Run ruff for linting, fix any issues\n11. Commit everything to git\n\nTest data files sample.tsv and sample.csv already exist in the workspace.\nCompare output against the real cut command.\nThe workspace is /tmp/s2-cut-tool.",
  "role": "user",
  "model": "openai/container",
  "agentic": true
}
```

**Expected Tool Calls:** 33-60

| Tool | Expected Count | Purpose |
|------|---------------|---------|
| `list_directory` | 2 | Check workspace, verify package structure |
| `glob_files` | 2 | Find Python files, find configs |
| `read_file` | 3-5 | sample.tsv, sample.csv, README, failing tests |
| `write_file` | 8-12 | pyproject.toml, 4 package files, 3 test files, README |
| `edit_file` | 5-10 | Wire entry point, fix tests, fix lint |
| `bash` | 12-18 | mkdir, py_compile, pytest, ruff, git, smoke tests, cut comparison |
| `search_files` | 1-2 | Check imports |
| `LLM` | 10-20 | Reasoning |

**Validation:**
```bash
cd /tmp/s2-cut-tool

# 1. Package structure exists
test -d cccut && test -f cccut/__init__.py && test -f cccut/__main__.py && echo "PASS: package" || echo "FAIL"
test -f cccut/parser.py && test -f cccut/cutter.py && echo "PASS: modules" || echo "FAIL"
test -d tests && test -f tests/test_cli.py && echo "PASS: tests dir" || echo "FAIL"
test -f pyproject.toml && echo "PASS: pyproject.toml" || echo "FAIL"

# 2. Syntax valid
python3 -m py_compile cccut/__init__.py cccut/__main__.py cccut/parser.py cccut/cutter.py && echo "PASS: syntax" || echo "FAIL"

# 3. Field extraction (-f2, tab-delimited)
EXPECTED=$(cut -f2 sample.tsv)
ACTUAL=$(python3 -m cccut -f2 sample.tsv 2>/dev/null)
[ "$EXPECTED" = "$ACTUAL" ] && echo "PASS: -f2 tab" || echo "FAIL: field 2 mismatch"

# 4. Custom delimiter (-d',')
EXPECTED=$(cut -d',' -f1,3 sample.csv)
ACTUAL=$(python3 -m cccut -d',' -f1,3 sample.csv 2>/dev/null)
[ "$EXPECTED" = "$ACTUAL" ] && echo "PASS: -d, -f1,3" || echo "FAIL: csv field mismatch"

# 5. Stdin mode
STDIN_RESULT=$(cat sample.tsv | python3 -m cccut -f2 2>/dev/null)
[ "$STDIN_RESULT" = "$(cut -f2 sample.tsv)" ] && echo "PASS: stdin" || echo "FAIL"

# 6. Error handling
python3 -m cccut -f2 nonexistent.txt 2>/dev/null; [ $? -ne 0 ] && echo "PASS: missing file error" || echo "FAIL"
python3 -m cccut sample.tsv 2>/dev/null; [ $? -ne 0 ] && echo "PASS: no -f error" || echo "FAIL"

# 7. Pytest
python3 -m pytest tests/ -v 2>&1 | tail -10

# 8. Lint
python3 -m ruff check cccut/ tests/ 2>&1 | head -10

# 9. Git
git log --oneline | head -5
```

**Success Criteria:**
- Proper Python package structure (cccut/ with 4 modules)
- `-f2` with tab-delimited file matches `cut` output
- `-d','` custom delimiter works
- Stdin mode works
- Tests directory with 3+ test files
- At least 5/10 pytest tests pass
- pyproject.toml exists

---

### S3: Hard — Extend the `cut` Tool with New Features (Brownfield)

> S3 builds on the S2 output. The agent receives an **existing codebase** and must
> **read, understand, and extend** it without breaking existing functionality.

**Precondition:** S2 must have produced a working `cccut` package. If not available,
Claude Code seeds a reference implementation before starting S3.

**Workspace Setup:**
```bash
# Option A: Use S2 output (if available and working)
cp -r /tmp/s2-cut-tool /tmp/s3-cut-extended
cd /tmp/s3-cut-extended && git checkout -b feature/byte-ranges

# Option B: Seed reference implementation (if S2 output not available)
rm -rf /tmp/s3-cut-extended && mkdir -p /tmp/s3-cut-extended
cd /tmp/s3-cut-extended && git init -b main
# [Claude Code creates a minimal working cccut package with tests]
# This MUST include: cccut/{__init__,__main__,parser,cutter}.py, tests/{__init__,test_cli}.py
# All existing tests MUST pass before S3 starts
git add . && git commit -m "initial: working cut clone from S2"
```

**Project Setup (API):** Same pattern with `name: "S3-cut-extended"`, `path: "/tmp/s3-cut-extended"`.

**Chat Prompt:**
```json
{
  "content": "This project has an existing Python cut tool clone (cccut package). I want you to extend it with new features.\n\nIMPORTANT: First explore the codebase to understand the architecture. Do NOT overwrite existing code blindly.\n\nNew features to add:\n1. Byte range selection: cccut -b1-5 file.txt (extract bytes 1-5 from each line)\n2. Character range selection: cccut -c3-8 file.txt (extract characters 3-8)\n3. Complement mode: cccut --complement -f2 file.tsv (print everything EXCEPT field 2)\n4. Output delimiter: cccut --output-delimiter='|' -f1,3 file.tsv (use | between output fields)\n5. Support field ranges: cccut -f2-4 file.tsv (fields 2, 3, and 4)\n6. Add --version flag showing 'cccut 2.0'\n\nConstraints:\n- ALL existing tests must still pass (zero regressions)\n- Add NEW tests for each new feature in tests/test_ranges.py and tests/test_options.py\n- Update README.md with new feature documentation\n- Run ruff, fix any lint issues\n- Commit with a descriptive message\n\nThe workspace is /tmp/s3-cut-extended.",
  "role": "user",
  "model": "openai/container",
  "agentic": true
}
```

**Expected Tool Calls:** 47-80

| Tool | Expected Count | Purpose |
|------|---------------|---------|
| `list_directory` | 3 | Workspace, cccut/, tests/ |
| `glob_files` | 3 | All .py files, test files, configs |
| `read_file` | 10+ | All existing source files, tests, README, pyproject.toml |
| `write_file` | 2-3 | New test files (test_ranges.py, test_options.py) |
| `edit_file` | 10+ | parser.py (add args), cutter.py (byte/char/complement), cli, README |
| `bash` | 15+ | pytest (5x+), smoke tests, ruff, git, baseline verification |
| `search_files` | 5+ | Find definitions, imports, argparse setup, edit points |
| `LLM` | 15-25 | Reasoning |

**Validation:**
```bash
cd /tmp/s3-cut-extended

# 1. Existing tests still pass (CRITICAL — no regressions)
python3 -m pytest tests/test_cli.py -v 2>&1 | tail -5
EXISTING_PASS=$?
[ $EXISTING_PASS -eq 0 ] && echo "PASS: no regression" || echo "FAIL: REGRESSION!"

# 2. Byte range (-b1-5)
echo "Hello World" | python3 -m cccut -b1-5 2>/dev/null | grep -q "Hello" && echo "PASS: -b byte range" || echo "FAIL"

# 3. Character range (-c3-8)
echo "Hello World" | python3 -m cccut -c3-8 2>/dev/null | grep -q "llo Wo" && echo "PASS: -c char range" || echo "FAIL"

# 4. Complement mode
echo -e "a\tb\tc" | python3 -m cccut --complement -f2 2>/dev/null | grep -q "a" && echo "PASS: complement" || echo "FAIL"

# 5. Output delimiter
echo -e "a\tb\tc" | python3 -m cccut --output-delimiter='|' -f1,3 2>/dev/null | grep -q "a|c" && echo "PASS: output delimiter" || echo "FAIL"

# 6. Field range (-f2-4)
echo -e "a\tb\tc\td\te" | python3 -m cccut -f2-4 2>/dev/null | grep -q "b" && echo "PASS: field range" || echo "FAIL"

# 7. --version
python3 -m cccut --version 2>/dev/null | grep -q "2.0" && echo "PASS: version" || echo "FAIL"

# 8. New test files exist
test -f tests/test_ranges.py && echo "PASS: test_ranges.py" || echo "FAIL"
test -f tests/test_options.py && echo "PASS: test_options.py" || echo "FAIL"

# 9. All tests pass (old + new)
python3 -m pytest tests/ -v 2>&1 | tail -10

# 10. Lint clean
python3 -m ruff check cccut/ tests/ 2>&1 | head -5

# 11. Git commit
git log --oneline | head -5
```

**Success Criteria:**
- **Zero regressions** — all existing tests still pass
- At least 3/6 new features work correctly
- New test files exist with tests for new features
- README updated with new feature documentation
- Agent demonstrated read-before-edit pattern (10+ read_file calls)

---

### S4: Expert — Build Your Own JSON Parser (TypeScript, Strict Types)

> Inspired by [CodingChallengesFYI #2: Build Your Own JSON Parser](https://codingchallenges.fyi/challenges/challenge-json-parser/)

**Workspace Setup:**
```bash
rm -rf /tmp/s4-json-parser && mkdir -p /tmp/s4-json-parser
cd /tmp/s4-json-parser && git init -b main

# Create test JSON files for validation (from JSON.org test suite)
mkdir -p tests/fixtures
echo '{}' > tests/fixtures/step1_valid.json
echo '' > tests/fixtures/step1_invalid.json
echo '{"key": "value"}' > tests/fixtures/step2_valid.json
echo '{"key": "value",}' > tests/fixtures/step2_invalid.json  # trailing comma
echo '{"key1": true, "key2": false, "key3": null, "key4": "value", "key5": 101}' > tests/fixtures/step3_valid.json
echo '{"key": "value", "nested": {"inner": [1, 2, 3]}, "array": ["a", "b"]}' > tests/fixtures/step4_valid.json
echo '{"key": "value",, "extra_comma": true}' > tests/fixtures/step4_invalid.json

echo "# Build Your Own JSON Parser" > README.md
git add . && git commit -m "initial: test fixtures"
```

**Project Setup (API):** Same pattern with `name: "S4-json-parser"`, `path: "/tmp/s4-json-parser"`.

**Chat Prompt:**
```json
{
  "content": "Build a JSON parser in TypeScript from scratch (do NOT use JSON.parse).\n\nRequirements:\n1. Project setup:\n   - package.json with build, test, and start scripts\n   - tsconfig.json with strict mode, ES2022 target, ESM modules\n   - Source in src/, tests in tests/\n\n2. Lexer (src/lexer.ts):\n   - Tokenize JSON input into: STRING, NUMBER, TRUE, FALSE, NULL, LBRACE, RBRACE, LBRACKET, RBRACKET, COLON, COMMA\n   - Handle escape sequences in strings (\\n, \\t, \\\", \\\\)\n   - Report line/column for error messages\n\n3. Parser (src/parser.ts):\n   - Recursive descent parser consuming token stream\n   - Support: objects, arrays, strings, numbers, booleans, null\n   - Nested structures (objects in arrays, arrays in objects)\n   - Meaningful error messages with position\n\n4. CLI (src/cli.ts):\n   - Read from file: ccjson file.json (exit 0 if valid, exit 1 if invalid)\n   - Read from stdin: cat file.json | ccjson\n   - Pretty-print mode: ccjson --pretty file.json\n   - Output parsed structure: ccjson --ast file.json\n\n5. Types (src/types.ts):\n   - Token type with discriminated unions\n   - JsonValue type (recursive: string | number | boolean | null | JsonObject | JsonArray)\n   - NO 'any' type anywhere\n\n6. Tests (vitest):\n   - tests/lexer.test.ts — tokenization tests\n   - tests/parser.test.ts — parsing valid/invalid JSON\n   - tests/cli.test.ts — CLI integration tests\n   - Use test fixtures in tests/fixtures/\n\n7. Quality:\n   - npm run build must compile without errors\n   - npm test must pass all tests\n   - No 'any' or 'as any' in source code\n   - ESLint or biome for linting (optional)\n\n8. Commit everything to git\n\nTest fixture files already exist in tests/fixtures/.\nThe workspace is /tmp/s4-json-parser.",
  "role": "user",
  "model": "openai/container",
  "agentic": true
}
```

**Expected Tool Calls:** 54-100

| Tool | Expected Count | Purpose |
|------|---------------|---------|
| `list_directory` | 2-3 | Workspace, src/, tests/ |
| `glob_files` | 3 | .ts files, test files, fixtures |
| `read_file` | 5-8 | Fixture files, type errors, failing tests |
| `write_file` | 12-18 | package.json, tsconfig, 5+ src files, 3 test files, README, .gitignore |
| `edit_file` | 8-15 | Fix type errors, fix test failures, add validation |
| `bash` | 20-30 | npm install, tsc (6x+), npm test (3x+), build, smoke tests, git |
| `search_files` | 4+ | Check for 'any' types, find imports, verify interfaces |
| `LLM` | 15-25 | Reasoning |

**Validation:**
```bash
cd /tmp/s4-json-parser

# 1. Project structure
test -f package.json && echo "PASS: package.json" || echo "FAIL"
test -f tsconfig.json && echo "PASS: tsconfig.json" || echo "FAIL"
test -f src/lexer.ts && echo "PASS: lexer.ts" || echo "FAIL"
test -f src/parser.ts && echo "PASS: parser.ts" || echo "FAIL"
test -f src/types.ts && echo "PASS: types.ts" || echo "FAIL"
test -f src/cli.ts && echo "PASS: cli.ts" || echo "FAIL"

# 2. TypeScript compiles
npx tsc --noEmit 2>&1 | tail -5
TSC_EXIT=$?
[ $TSC_EXIT -eq 0 ] && echo "PASS: tsc compiles" || echo "FAIL: tsc errors"

# 3. No 'any' types
ANY_COUNT=$(grep -rn ': any\|as any\|<any>' src/ 2>/dev/null | wc -l)
[ "$ANY_COUNT" -eq 0 ] && echo "PASS: no any types" || echo "FAIL: $ANY_COUNT any types found"

# 4. Build succeeds
npm run build 2>&1 | tail -3
[ $? -eq 0 ] && echo "PASS: build" || echo "FAIL: build error"

# 5. Valid JSON accepted (exit 0)
echo '{"key": "value"}' | node dist/cli.js 2>/dev/null
[ $? -eq 0 ] && echo "PASS: valid JSON accepted" || echo "FAIL"

# 6. Invalid JSON rejected (exit 1)
echo '{"key": "value",}' | node dist/cli.js 2>/dev/null
[ $? -ne 0 ] && echo "PASS: invalid JSON rejected" || echo "FAIL"

# 7. Nested JSON
echo '{"a": [1, {"b": true}]}' | node dist/cli.js 2>/dev/null
[ $? -eq 0 ] && echo "PASS: nested JSON" || echo "FAIL"

# 8. Empty input rejected
echo '' | node dist/cli.js 2>/dev/null
[ $? -ne 0 ] && echo "PASS: empty input rejected" || echo "FAIL"

# 9. File mode
node dist/cli.js tests/fixtures/step4_valid.json 2>/dev/null
[ $? -eq 0 ] && echo "PASS: file mode" || echo "FAIL"

# 10. Tests pass
npm test 2>&1 | tail -10

# 11. Test files exist
test -f tests/lexer.test.ts && echo "PASS: lexer tests" || echo "FAIL"
test -f tests/parser.test.ts && echo "PASS: parser tests" || echo "FAIL"

# 12. Git
git log --oneline | head -5
```

**Success Criteria:**
- `npm run build` exits 0 (TypeScript compiles cleanly in strict mode)
- No `any` types in source code
- Valid JSON accepted (exit 0), invalid JSON rejected (exit 1)
- Nested structures parsed correctly
- Test files exist for lexer and parser
- `npm test` has at least 5 passing tests

---

### Alternative Scenarios (Rotation Pool)

For repeated testing, rotate through these alternatives to avoid the agent "memorizing" solutions:

| ID | Name | Difficulty | Language | Source |
|----|------|-----------|----------|--------|
| S1a | Build Your Own `cat` | Easy | Python | [CodingChallengesFYI](https://codingchallenges.fyi/challenges/challenge-cat/) |
| S1b | Build Your Own Calculator | Easy | Python | [CodingChallengesFYI](https://codingchallenges.fyi/challenges/challenge-calculator/) |
| S2a | Build Your Own `sort` Tool | Medium | Python | [CodingChallengesFYI](https://codingchallenges.fyi/challenges/challenge-sort/) |
| S2b | Build Your Own `uniq` Tool | Medium | Go | [CodingChallengesFYI](https://codingchallenges.fyi/challenges/challenge-uniq/) |
| S3a | Add `--complement` to `sort` | Hard | Python | Extend S2a output |
| S3b | Add `--count` + `--repeated` to `uniq` | Hard | Go | Extend S2b output |
| S4a | Build Your Own `grep` | Expert | Go | [CodingChallengesFYI](https://codingchallenges.fyi/challenges/challenge-grep/) |
| S4b | Build Your Own URL Shortener | Expert | TypeScript | [CodingChallengesFYI](https://codingchallenges.fyi/challenges/challenge-url-shortener/) |

Each alternative follows the same testplan structure (workspace setup, API setup, prompt, validation, criteria).

### Scenario Selection Logic

Claude Code selects the scenario based on test run parameters:

```
If this is the FIRST test run:
  -> Use S1 (Easy) to validate the pipeline works at all

If S1 passed on a previous run:
  -> Use S2 (Medium) as the primary validation scenario

If S2 passed:
  -> Use S3 (Hard) to test brownfield capabilities
     (S3 REQUIRES S2 output as input — seed if not available)

If S2 passed AND want to test TypeScript/multi-language:
  -> Use S4 (Expert)

For regression testing (scheduled/cron):
  -> Rotate: S1 -> S2 -> S3 -> S4 -> S1a -> S2a -> S4a -> ...

For comparison testing:
  -> Run same scenario with different models (local vs cloud)
```

**The scenario ID (S1-S4) must be recorded in the test report.**

### Critical Setup Checklist (ALL scenarios)

Before dispatching any agentic message, Claude Code MUST verify:

- [ ] Docker services running (postgres, nats, litellm)
- [ ] NATS stream purged
- [ ] Go backend running with `APP_ENV=development`
- [ ] Python worker running with `CODEFORGE_ROUTING_ENABLED=false`
- [ ] Frontend running
- [ ] Project created via API
- [ ] **Workspace adopted** via `POST /projects/{id}/adopt` (MANDATORY)
- [ ] **At least one goal created** via `POST /projects/{id}/goals` (MANDATORY — prevents full-auto gate)
- [ ] Conversation created and approvals bypassed
- [ ] Test data files placed in workspace (if required by scenario)
- [ ] Model `openai/container` (or target model) is healthy in LiteLLM

---

## Phase 0: Service Startup & Environment Discovery & Login

**Goal:** Ensure ALL required services are running, then login and discover models.

**CRITICAL:** Claude Code MUST start and verify every service. Do NOT assume anything is already running. Check each service and start it if missing.

### WSL2 Docker Networking — CRITICAL

> **Known issue:** In WSL2 environments, Docker container port mappings (`0.0.0.0:4000 -> container:4000`)
> are NOT reachable via `localhost` from inside the WSL2 instance. This affects ALL Docker services
> (NATS, LiteLLM, PostgreSQL). Services running directly on the host (Go backend, frontend) ARE
> reachable at localhost.
>
> **Solution:** Dynamically resolve container IPs via `docker inspect` and use those IPs for all
> Docker service URLs. The Go backend and frontend run on the host and are reachable at localhost.

### Step 0a: Start Docker Services & Resolve Container IPs

1. **Check and start Docker services:**
   ```bash
   docker compose ps --format '{{.Name}}: {{.State}}' 2>/dev/null
   docker compose up -d postgres nats litellm
   ```

2. **Resolve container IPs** (MUST be done before starting any non-Docker service):
   ```bash
   NATS_IP=$(docker inspect codeforge-nats 2>/dev/null | grep -m1 '"IPAddress"' | grep -oP '[\d.]+')
   LITELLM_IP=$(docker inspect codeforge-litellm 2>/dev/null | grep -m1 '"IPAddress"' | grep -oP '[\d.]+')
   POSTGRES_IP=$(docker inspect codeforge-postgres 2>/dev/null | grep -m1 '"IPAddress"' | grep -oP '[\d.]+')
   echo "NATS: $NATS_IP | LiteLLM: $LITELLM_IP | Postgres: $POSTGRES_IP"
   ```
   -> Store all three IPs — they are needed for worker startup

3. **Wait for healthy state** (use container IPs, NOT localhost):
   ```bash
   # Postgres
   docker compose exec postgres pg_isready -U codeforge
   # NATS
   curl -s "http://${NATS_IP}:8222/varz" | head -c 50
   # LiteLLM
   curl -s "http://${LITELLM_IP}:4000/health" -H "Authorization: Bearer sk-codeforge-dev" | head -c 50
   ```
   -> All three must respond. If any fail, check `docker compose logs <service>`.

### Step 0a2: Purge NATS JetStream (CRITICAL for fresh test runs)

> **Why:** Old unacked messages from previous sessions accumulate in the NATS JetStream
> `CODEFORGE` stream. These stale messages block new consumers — the Go backend processes
> them sequentially, and any HITL-wait message (60s timeout) blocks all subsequent messages.
> **A fresh stream is REQUIRED for reliable test execution.**

1. **Kill any running Go backend and Python worker** (they hold NATS consumer locks):
   ```bash
   pkill -f "exe/codeforge" 2>/dev/null || true
   pkill -f "codeforge.consumer" 2>/dev/null || true
   sleep 2
   ```

2. **Purge the NATS stream and delete stale consumers:**
   ```python
   import asyncio, nats
   async def purge():
       nc = await nats.connect('nats://${NATS_IP}:4222')
       js = nc.jetstream()
       await js.purge_stream('CODEFORGE')
       for name in ['codeforge-go-runs-toolcall-request',
                     'codeforge-go-runs-toolcall-result',
                     'codeforge-go-conversation-run-complete',
                     'codeforge-go-runs-complete',
                     'codeforge-go-runs-heartbeat',
                     'codeforge-go-runs-output',
                     'codeforge-go-runs-trajectory-event']:
           try: await js.delete_consumer('CODEFORGE', name)
           except: pass
       await nc.close()
       print('NATS stream purged, stale consumers deleted')
   asyncio.run(purge())
   ```

3. **Verify stream is empty:**
   ```bash
   curl -s "http://${NATS_IP}:8222/jsz" | python3 -c "
   import sys,json; d=json.load(sys.stdin)
   msgs = sum(s.get('state',{}).get('messages',0) for a in d.get('account_details',[]) for s in a.get('stream_detail',[]))
   print(f'Messages in stream: {msgs}')
   assert msgs == 0, 'Stream not empty!'
   "
   ```
   -> Expected: `Messages in stream: 0`

### Step 0b: Start Go Backend

1. **Check if backend is running:**
   ```bash
   curl -s http://localhost:8080/health 2>/dev/null || echo "NOT_RUNNING"
   ```
   Note: Go backend runs on the host, so `localhost:8080` works.

2. **If not running — start in background:**
   ```bash
   cd /workspaces/CodeForge && APP_ENV=development go run ./cmd/codeforge/ > /tmp/codeforge-backend.log 2>&1 &
   ```

3. **Wait for backend ready (poll up to 60s):**
   ```bash
   for i in $(seq 1 30); do
     curl -s http://localhost:8080/health | grep -q '"ok"' && echo "READY" && break
     sleep 2
   done
   ```
   -> Expected: `READY`

### Step 0c: Start Frontend Dev Server

1. **Check if frontend is running:**
   ```bash
   curl -s -o /dev/null -w "%{http_code}" http://localhost:3000 2>/dev/null || echo "NOT_RUNNING"
   ```
   Note: Frontend runs on the host, so `localhost:3000` works.

2. **If not running — start in background:**
   ```bash
   cd /workspaces/CodeForge/frontend && npm run dev > /tmp/codeforge-frontend.log 2>&1 &
   ```

3. **Wait for frontend ready (poll up to 30s):**
   ```bash
   for i in $(seq 1 15); do
     curl -s -o /dev/null -w "%{http_code}" http://localhost:3000 2>/dev/null | grep -q "200" && echo "READY" && break
     sleep 2
   done
   ```

### Step 0d: Start Python Worker

> **CRITICAL env vars:** The Python worker connects to Docker services (NATS, LiteLLM) that are
> NOT reachable at localhost in WSL2. You MUST use the container IPs resolved in Step 0a.
> The env var for LiteLLM is `LITELLM_BASE_URL` (NOT `LITELLM_URL`).

1. **Kill any stale worker (bytecache + old process):**
   ```bash
   pkill -f "codeforge.consumer" 2>/dev/null || true
   find /workspaces/CodeForge/workers -name "*.pyc" -delete 2>/dev/null
   find /workspaces/CodeForge/workers -name "__pycache__" -type d -exec rm -rf {} + 2>/dev/null
   ```

2. **Start worker with correct env vars:**
   ```bash
   cd /workspaces/CodeForge && \
     PYTHONPATH=/workspaces/CodeForge/workers \
     NATS_URL="nats://${NATS_IP}:4222" \
     LITELLM_BASE_URL="http://${LITELLM_IP}:4000" \
     LITELLM_MASTER_KEY="sk-codeforge-dev" \
     DATABASE_URL="postgresql://codeforge:codeforge_dev@${POSTGRES_IP}:5432/codeforge" \
     CODEFORGE_ROUTING_ENABLED=false \
     APP_ENV=development \
     .venv/bin/python -m codeforge.consumer > /tmp/codeforge-worker.log 2>&1 &
   ```

   **Env var reference:**
   | Env Var | Source | Example |
   |---------|--------|---------|
   | `NATS_URL` | Container IP from Step 0a | `nats://172.18.0.3:4222` |
   | `LITELLM_BASE_URL` | Container IP from Step 0a | `http://172.18.0.6:4000` |
   | `LITELLM_MASTER_KEY` | Static | `sk-codeforge-dev` |
   | `DATABASE_URL` | Container IP from Step 0a | `postgresql://codeforge:codeforge_dev@172.18.0.2:5432/codeforge` |
   | `CODEFORGE_ROUTING_ENABLED` | Static | `false` (use explicit model from payload, not auto-router) |
   | `APP_ENV` | Static | `development` |
   | `PYTHONPATH` | Static | `/workspaces/CodeForge/workers` |

3. **Wait for worker ready:**
   ```bash
   for i in $(seq 1 15); do
     grep -q "consumer started\|JetStream\|subscribed" /tmp/codeforge-worker.log 2>/dev/null && echo "READY" && break
     sleep 2
   done
   ```

4. **Verify worker can reach LiteLLM** (check log for model fetch):
   ```bash
   sleep 5
   grep -i "failed to fetch models\|ConnectError\|LITELLM" /tmp/codeforge-worker.log | tail -3
   ```
   -> If "failed to fetch models" or "ConnectError" appears: LITELLM_BASE_URL is wrong.
      Re-check container IP: `docker inspect codeforge-litellm | grep IPAddress`

### Step 0e: Verify All Services & Login via Browser

> **Playwright-MCP note:** The Playwright browser runs inside a Docker container.
> It cannot reach `localhost` on the host. Use `http://host.docker.internal:3000` for all
> browser navigation.

1. `browser_navigate` -> `http://host.docker.internal:3000`
2. `browser_snapshot` -> login page visible? Look for email/password fields

3. Login with seeded admin:
   - `browser_fill_form` -> email: `admin@localhost`, password: `Changeme123`
   - `browser_click` -> Login/Submit button
   - `browser_snapshot` -> Dashboard page visible? (project cards or empty state)

4. Store auth token for API calls:
   ```js
   // browser_evaluate
   fetch('/api/v1/auth/login', {
     method: 'POST',
     headers: {'Content-Type': 'application/json'},
     body: JSON.stringify({email: 'admin@localhost', password: 'Changeme123'})
   }).then(r => r.json()).then(d => d.access_token)
   ```
   -> Store as `ENV.token` (field is `access_token`, NOT `token`)

5. Model discovery:
   ```js
   // browser_evaluate
   fetch('/api/v1/llm/discover', {
     method: 'POST',
     headers: {'Authorization': 'Bearer ' + ENV.token}
   }).then(r => r.json())
   ```
   -> Store `ENV.models`, identify tool-capable models

**Validation:**
- All 6 services running: postgres, nats, litellm, go backend, frontend, python worker
- Container IPs resolved and stored
- Worker log shows no "ConnectError" for NATS or LiteLLM
- Frontend renders via `host.docker.internal:3000`
- Backend responds with `dev_mode: true`
- Login succeeds, dashboard visible
- At least 1 model available

**Decision Tree:**
```
Docker services not starting?
├─ Docker daemon not running -> ABORT: "Start Docker first"
├─ Port conflict -> docker compose down, then up again
└─ postgres unhealthy -> Check docker compose logs postgres

Container IPs empty?
├─ Container not running -> docker compose up -d <service>
├─ grep pattern wrong -> Use: docker inspect <name> | jq '.[0].NetworkSettings.Networks[].IPAddress'
└─ Network mismatch -> docker network ls, check compose network name

Go backend fails to start?
├─ Port 8080 in use -> kill existing process: lsof -ti:8080 | xargs kill
├─ Migration errors -> Check /tmp/codeforge-backend.log
├─ Missing env vars -> Ensure APP_ENV=development
└─ Build errors -> go build ./cmd/codeforge/ first to check

Frontend fails to start?
├─ Port 3000 in use -> kill existing: lsof -ti:3000 | xargs kill
├─ node_modules missing -> cd frontend && npm install
└─ Build errors -> Check /tmp/codeforge-frontend.log

Worker can't reach LiteLLM?
├─ Wrong env var name -> MUST be LITELLM_BASE_URL (not LITELLM_URL)
├─ Container IP changed -> Re-resolve: docker inspect codeforge-litellm | grep IPAddress
├─ LiteLLM not healthy -> docker compose logs litellm
└─ 401 Unauthorized -> Check LITELLM_MASTER_KEY=sk-codeforge-dev

Worker can't reach NATS?
├─ Same WSL2 issue -> Use container IP, not localhost
├─ Container IP changed -> Re-resolve: docker inspect codeforge-nats | grep IPAddress
└─ NATS not started -> docker compose up -d nats

Worker ModuleNotFoundError?
├─ PYTHONPATH not set -> MUST include /workspaces/CodeForge/workers
├─ .venv missing -> cd /workspaces/CodeForge && poetry install
└─ bytecache stale -> find workers -name "*.pyc" -delete

Playwright browser can't reach frontend?
├─ Used localhost -> MUST use http://host.docker.internal:3000
├─ Frontend not started -> Check /tmp/codeforge-frontend.log
└─ Port wrong -> Verify frontend runs on port 3000

Login fails?
├─ "Invalid credentials" -> Check DB seeding (admin@localhost / Changeme123)
├─ 502/503 -> Backend crashed -> Check /tmp/codeforge-backend.log
├─ Token field wrong -> Response uses "access_token", not "token"
└─ Stuck on login page -> browser_console_messages, check CORS

No models found?
├─ Check LiteLLM: browser_evaluate fetch('http://localhost:4000/health')
│  ├─ Down -> ABORT: "LiteLLM proxy not started"
│  └─ Up but 0 models -> WARN: "No API keys configured"
└─ Models but none tool-capable ->
   FLAG: "Simple chat only, agentic execution will be limited"
```

---

## Phase 1: Project Setup

**Goal:** Create a new project pointing to TestRepo (or local workspace), create test branch.

**Steps:**

1. From Dashboard, `browser_click` -> "New Project" button (primary button in header)
2. `browser_snapshot` -> CreateProjectModal visible? (tabs: Remote / Local / Empty)

3. **Try Remote first:**
   - `browser_click` -> "Remote" tab (if not already selected)
   - `browser_fill_form`:
     - Project Name: `Autonomous Test - {timestamp}`
     - Repository URL: `https://github.com/Strob0t/TestRepo.git`
   - `browser_snapshot` -> Check if URL is parsed (branch dropdown should populate)
   - `browser_click` -> "Create" button
   - `browser_wait_for` -> Project detail page loads (URL changes to `/projects/:id`)

4. **If Remote fails** (clone error, GitHub unreachable):
   - `browser_click` -> close modal, re-open "New Project"
   - `browser_click` -> "Local" tab
   - First, create a local workspace via `browser_evaluate`:
     ```js
     await fetch('/api/v1/projects', {
       method: 'POST',
       headers: {
         'Authorization': 'Bearer ' + ENV.token,
         'Content-Type': 'application/json'
       },
       body: JSON.stringify({
         name: 'Autonomous Test - local',
         description: 'Automated test: goal-to-program pipeline'
       })
     }).then(r => r.json())
     ```
   - Navigate to the created project

5. `browser_snapshot` -> ProjectDetailPage visible? (header with project name, left panels, chat panel)

6. Extract project info via `browser_evaluate`:
   ```js
   const url = window.location.pathname;
   const projectId = url.split('/projects/')[1];
   const project = await fetch('/api/v1/projects/' + projectId, {
     headers: {'Authorization': 'Bearer ' + ENV.token}
   }).then(r => r.json());
   [project.id, project.workspace_path]
   ```
   -> Store `PROJECT_ID`, `WORKSPACE`

7. Create test branch (via API since UI may not have branch creation):
   ```js
   const branch = 'test/autonomous-' + new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
   await fetch('/api/v1/projects/' + PROJECT_ID + '/git/checkout', {
     method: 'POST',
     headers: {
       'Authorization': 'Bearer ' + ENV.token,
       'Content-Type': 'application/json'
     },
     body: JSON.stringify({branch: branch, create: true})
   });
   branch
   ```
   -> Store `BRANCH`

8. `browser_snapshot` -> Verify git status badge in header shows new branch name

**Validation:**
- Project exists with valid `workspace_path`
- ProjectDetailPage renders (left panels + chat visible)
- Git status badge shows test branch
- `PROJECT_ID` and `WORKSPACE` are non-empty

**Decision Tree:**
```
Clone fails?
├─ "repository not found" -> Fallback to Local tab
├─ Timeout -> Check GitHub accessibility, fallback to local
└─ "permission denied" -> Fallback to local

Project page not loading?
├─ Spinner stuck -> browser_evaluate check API response
├─ Error banner -> browser_snapshot + read error text
└─ 404 -> Project creation may have failed silently, check browser_console_messages

No workspace_path?
├─ Remote clone still in progress -> browser_wait_for (up to 60s)
├─ Clone failed silently -> Check project status via API
└─ ABORT if workspace can't be established (agentic mode requires it)
```

---

## Phase 2: Goal Discovery via Chat

**Goal:** Use AI Goal Discovery to detect project goals from natural language.

**Steps:**

1. **Open Goals panel:**
   - `browser_click` -> "More panels..." dropdown in left panel
   - `browser_click` -> "Goals" option
   - `browser_snapshot` -> GoalsPanel visible? (empty state or existing goals)

2. **Trigger AI Discovery:**
   - `browser_click` -> "AI Discover" button in GoalsPanel header
   - `browser_snapshot` -> Should switch to chat panel with a new conversation in `goal-researcher` mode
   - `browser_wait_for` -> Chat input area visible, possibly with initial agent message

3. **Send program description (from selected scenario):**
   - `browser_click` -> Chat input textarea
   - `browser_type` -> the **Chat Prompt** from the selected scenario (S1/S2/S3/S4)
     - S1: CSV-to-JSON converter prompt
     - S2: CLI task manager prompt
     - S3: Add-tags-feature prompt (requires existing codebase)
     - S4: TypeScript REST API prompt
   - `browser_click` -> Send button
   - `browser_snapshot` -> User message appears in chat

4. **Wait for agent response:**
   - `browser_wait_for` -> Assistant message appears (may take 30-120s depending on model)
   - Poll with `browser_snapshot` every 15s, up to 15 minutes (local models are slow)
   - Look for: streaming text, tool call indicators, GoalProposalCards

5. **If agent asks clarifying questions** (assistant message contains `?` but no GoalProposalCards):
   - `browser_click` -> Chat input
   - `browser_type` -> Scenario-specific clarification:
     - **S1:** `Yes, just a single Python script with argparse. No dependencies beyond the standard library. Keep it simple.`
     - **S2:** `Yes, that covers everything. Please create the goals now. Simple Python CLI app with argparse, JSON file storage, and pytest tests. No web framework, no database, just files.`
     - **S3:** `Yes, extend the existing codebase. Read it first, understand the patterns, then add the tags feature following the same code style. All existing tests must still pass.`
     - **S4:** `Yes, TypeScript with strict mode, no 'any' types. Use Express or native http. Vitest or Node.js test runner. ES modules. Keep dependencies minimal.`
   - `browser_click` -> Send button
   - `browser_wait_for` -> Next agent response (up to 3 minutes)

6. **Handle GoalProposalCards:**
   - `browser_snapshot` -> Look for GoalProposalCard components in chat (bordered cards with Approve/Reject buttons)
   - For each GoalProposalCard visible:
     - `browser_snapshot` -> Read goal title, kind, content from the card
     - `browser_click` -> "Approve" button on the card
     - `browser_snapshot` -> Card changes to approved state (green border, checkmark)
   - Repeat until all proposal cards are approved

7. **Verify goals created via GoalsPanel:**
   - `browser_click` -> "More panels..." dropdown
   - `browser_click` -> "Goals"
   - `browser_snapshot` -> GoalsPanel should show the approved goals
   - Count visible goal items

**Validation:**
- Agent responded with at least one message
- At least 3 GoalProposalCards appeared
- All proposals approved (cards show green/approved state)
- GoalsPanel shows >= 3 goals with toggle ON (enabled)
- Goals cover key concepts: CLI, tasks, JSON, tests

**Decision Tree:**
```
Agent never responds?
├─ browser_console_messages -> check for WS errors
├─ browser_evaluate: check conversation messages via API
│  fetch('/api/v1/conversations/' + convId + '/messages', {headers:...})
├─ Check: is worker running? (agent loop needs Python worker)
└─ ABORT after 5 min: "Agent unresponsive, check worker logs"

Agent responds but no GoalProposalCards?
├─ Agent in interview phase (GSD methodology) -> Send clarification (step 5)
├─ Agent proposed goals via text (not tool) -> Create goals manually via GoalsPanel
│  - browser_click -> "+ Add" in GoalsPanel
│  - browser_fill_form -> kind, title, content from agent's text
│  - browser_click -> Create
└─ propose_goal tool not registered -> FLAG as implementation bug

Agent stuck / infinite loop?
├─ Check: ActiveWorkPanel shows running task?
├─ browser_click -> Stop button (red) in chat header if visible
└─ Wait for stall detector (auto-aborts after 2 escape attempts)

GoalProposalCard Approve button doesn't work?
├─ browser_console_messages -> check for API errors
├─ browser_evaluate: manually create goal via API as fallback
└─ FLAG: "GoalProposalCard approval broken"
```

### Phase 2b: Manual Goal Creation (Fallback)

If AI Discovery fails completely, create goals manually via the GoalsPanel UI.

For each goal: `browser_click` -> "+ Add" -> `browser_fill_form` (kind, title, content) -> `browser_click` "Create"

**S1 Fallback Goals:**

| Kind | Title | Content |
|------|-------|---------|
| requirement | CSV to JSON Conversion | Python script that reads CSV file and outputs JSON file |
| requirement | CLI Arguments | Accept file path via argparse command line argument |
| requirement | Error Handling | Handle missing files and invalid CSV gracefully |

**S2 Fallback Goals:**

| Kind | Title | Content |
|------|-------|---------|
| requirement | CLI Task CRUD | Python CLI with argparse: add, list, complete, delete tasks. Each task has ID, title, priority (low/medium/high), status (open/done). |
| requirement | JSON Persistence | Store tasks in a tasks.json file. Load on startup, save after each mutation. |
| requirement | Test Suite | Pytest test suite covering add, list, complete, delete operations. Use tmp_path fixture for isolated tests. |
| requirement | README Documentation | README.md with installation instructions, usage examples for each command, and project structure. |

**S3 Fallback Goals:**

| Kind | Title | Content |
|------|-------|---------|
| requirement | Codebase Understanding | Read and understand the existing Python project architecture |
| requirement | Tags Data Model | Extend Task model with optional tags field (list of strings) |
| requirement | Search Command | Add CLI search command that filters tasks by tag |
| requirement | Data Migration | Existing tasks without tags should get empty list, no crashes |
| requirement | Regression Safety | All existing tests must still pass after changes |
| requirement | Documentation Update | Update README to document the new tags feature |

**S4 Fallback Goals:**

| Kind | Title | Content |
|------|-------|---------|
| requirement | REST API Endpoints | CRUD endpoints for bookmarks (GET, POST, PUT, DELETE) |
| requirement | TypeScript Types | Strict TypeScript interfaces for Bookmark, no any types |
| requirement | Validation | URL format validation, required field checks, proper HTTP status codes |
| requirement | Storage Layer | In-memory storage with optional JSON file persistence |
| requirement | Build Pipeline | package.json with build/test scripts, tsconfig.json for ESM |
| requirement | Test Suite | API tests using vitest or Node.js test runner |
| requirement | API Documentation | README with endpoint docs and curl examples |

---

## Phase 3: Goal Validation

**Goal:** Confirm goals are correctly persisted and cover the program specification.

**Steps:**

1. **Navigate to GoalsPanel** (if not already there):
   - `browser_click` -> "More panels..." -> "Goals"
   - `browser_snapshot` -> GoalsPanel with goal list visible

2. **Count goals:**
   - `browser_evaluate`:
     ```js
     const goals = await fetch('/api/v1/projects/' + PROJECT_ID + '/goals', {
       headers: {'Authorization': 'Bearer ' + ENV.token}
     }).then(r => r.json());
     JSON.stringify({
       count: goals.length,
       kinds: goals.map(g => g.kind),
       titles: goals.map(g => g.title),
       allEnabled: goals.every(g => g.enabled),
       sources: [...new Set(goals.map(g => g.source))]
     })
     ```
   -> Store `GOAL_COUNT`

3. **Validate coverage** (scenario-specific keyword check):
   - `browser_evaluate`:
     ```js
     const goals = await fetch('/api/v1/projects/' + PROJECT_ID + '/goals', {
       headers: {'Authorization': 'Bearer ' + ENV.token}
     }).then(r => r.json());
     const text = goals.map(g => g.title + ' ' + g.content).join(' ').toLowerCase();

     // Scenario-specific keywords:
     // S1: ['csv', 'json', 'convert', 'error', 'test', 'cli']
     // S2: ['add', 'list', 'complete', 'delete', 'test', 'readme', 'json', 'cli']
     // S3: ['tag', 'search', 'existing', 'migration', 'test', 'edit', 'extend']
     // S4: ['rest', 'api', 'typescript', 'crud', 'test', 'validation', 'build']
     const features = SCENARIO_KEYWORDS;  // select based on active scenario
     const found = features.filter(f => text.includes(f));
     const missing = features.filter(f => !text.includes(f));
     JSON.stringify({found, missing, coverage: found.length + '/' + features.length})
     ```

4. `browser_take_screenshot` -> "Phase 3: Goals Validated"

**Validation (scenario-specific):**

| Check | S1 Easy | S2 Medium | S3 Hard | S4 Expert |
|-------|---------|-----------|---------|-----------|
| Min goal count | >= 3 | >= 4 | >= 5 | >= 6 |
| Requirement goals | >= 2 | >= 3 | >= 4 | >= 5 |
| Keyword coverage | >= 4/6 | >= 5/8 | >= 5/7 | >= 5/7 |
| All enabled | Yes | Yes | Yes | Yes |

**Decision Tree:**
```
0 goals found?
├─ Phase 2 failed silently -> Execute Phase 2b (manual creation)
├─ Goals created but not persisted -> Check auto-persistence in runtime.go
└─ FLAG: "Goal auto-persistence broken"

Coverage < 5/8?
├─ Accept if agent grouped features (e.g., "CRUD operations" covers add/list/complete/delete)
├─ Add missing goals manually via GoalsPanel "+ Add"
└─ Log which features were missing
```

---

## Phase 4: Roadmap Creation

**Goal:** Build a structured roadmap with milestones and atomic features from the goals.

**Steps:**

1. **Open Roadmap panel:**
   - `browser_click` -> "More panels..." dropdown
   - `browser_click` -> "Roadmap"
   - `browser_snapshot` -> RoadmapPanel visible (empty state with "Create Roadmap" form, or existing roadmap)

2. **Create roadmap:**
   - `browser_fill_form`:
     - Title: scenario-specific (see below)
     - Description: `Build from detected goals`
   - `browser_click` -> "Create Roadmap" button
   - `browser_snapshot` -> Roadmap created, empty milestone list visible

3. **Add milestones and features (scenario-specific):**

   For each milestone:
   - `browser_click` -> "Add Milestone" button at bottom of RoadmapPanel
   - `browser_fill_form` -> Title input
   - `browser_click` -> Confirm/Create button

   For each feature within a milestone:
   - `browser_click` -> "Add Feature" link within that milestone
   - `browser_fill_form` -> Feature title (atomic, single responsibility)
   - `browser_click` -> Create/Confirm button

---

#### S1 Roadmap: CSV-to-JSON Converter

**Title:** `CSV to JSON Converter`

| Milestone | Features (atomic tasks) |
|-----------|------------------------|
| **1. Core Script** | Create csv2json.py with argparse for input file path |
| | Read CSV using csv.DictReader and convert rows to list of dicts |
| | Write output as JSON with same filename but .json extension |
| **2. Error Handling & Tests** | Handle FileNotFoundError with user-friendly message |
| | Handle csv.Error for malformed CSV with error message |
| | Create test_csv2json.py with sample CSV fixture |
| | Test successful conversion (valid CSV -> correct JSON) |
| | Test error cases (missing file, invalid CSV) |

**Milestones:** 2 | **Features:** 8

---

#### S2 Roadmap: CLI Task Manager

**Title:** `Python CLI Task Manager`

| Milestone | Features (atomic tasks) |
|-----------|------------------------|
| **1. Project Setup** | Create pyproject.toml with project metadata and pytest dependency |
| | Create task_manager/__init__.py and __main__.py entry point |
| | Create .gitignore for Python projects |
| **2. Core Data Model** | Create task_manager/models.py with Task dataclass (id, title, priority, status, created_at) |
| | Add Task.to_dict() and Task.from_dict() for JSON serialization |
| | Add TaskStatus and Priority enums |
| **3. Storage Layer** | Create task_manager/storage.py with load_tasks(path) returning list of Task |
| | Add save_tasks(path, tasks) with atomic write (write to .tmp then rename) |
| | Handle missing file gracefully (return empty list) |
| **4. CLI Interface** | Create task_manager/cli.py with argparse main parser and subparsers |
| | Implement add subcommand: --title (required), --priority (default medium) |
| | Implement list subcommand: --status filter (optional, open/done/all) |
| | Implement complete subcommand: task_id positional argument |
| | Implement delete subcommand: task_id positional argument |
| | Wire CLI to storage layer and print formatted output |
| **5. Test Suite** | Create tests/test_models.py: test Task creation, serialization, enums |
| | Create tests/test_storage.py: test load/save with tmp_path, missing file |
| | Create tests/test_cli.py: test add, list, complete, delete via subprocess |
| | Create tests/conftest.py with shared fixtures (tmp storage path) |
| **6. Documentation** | Create README.md with project overview, installation, and usage examples |

**Milestones:** 6 | **Features:** 20

---

#### S3 Roadmap: Add Tags Feature (Brownfield)

**Title:** `Add Tags Feature to Existing Task Manager`

> **Precondition:** The workspace must contain an existing Python task manager (from a prior S2 run or seeded codebase). If empty, seed via Phase 2b first.

| Milestone | Features (atomic tasks) |
|-----------|------------------------|
| **1. Codebase Analysis** | Read and list all existing Python files to understand project structure |
| | Read models.py to understand Task dataclass and existing fields |
| | Read cli.py to understand existing subcommands and argparse setup |
| | Read storage.py to understand JSON serialization format |
| | Run existing tests to confirm baseline passes |
| **2. Data Model Extension** | Edit models.py: add tags field (list[str], default empty) to Task dataclass |
| | Edit models.py: update to_dict() and from_dict() to include tags |
| | Verify existing tests still pass after model change |
| **3. Storage Migration** | Edit storage.py: handle loading old JSON without tags field (default to []) |
| | Create a migration test: load old-format JSON, verify tags default to [] |
| **4. CLI Extension** | Edit cli.py: add --tags option to add subcommand (comma-separated) |
| | Edit cli.py: add search subcommand with --tag filter argument |
| | Edit cli.py: show tags in list output format |
| **5. Tests & Docs** | Create tests/test_tags.py: test add with tags, search by tag, empty tags |
| | Run full test suite (old + new tests must all pass) |
| | Edit README.md: add tags feature documentation with examples |

**Milestones:** 5 | **Features:** 15

**Key difference:** This scenario forces the agent to **Read, Search, and Edit** existing files rather than creating everything from scratch. The agent must understand existing code patterns before modifying them.

---

#### S4 Roadmap: TypeScript REST API

**Title:** `TypeScript Bookmark Manager REST API`

| Milestone | Features (atomic tasks) |
|-----------|------------------------|
| **1. Project Setup** | Create package.json with name, scripts (build, start, test, dev), dependencies |
| | Create tsconfig.json with strict mode, ES modules, outDir |
| | Create directory structure: src/, src/routes/, src/models/, tests/ |
| | Create .gitignore for Node.js/TypeScript projects |
| **2. Types & Models** | Create src/models/bookmark.ts with Bookmark interface (id, title, url, tags, created_at) |
| | Create src/models/errors.ts with AppError class and HTTP status codes |
| | Create src/models/validation.ts with URL validator and field validator |
| **3. Storage Layer** | Create src/storage/store.ts with BookmarkStore interface |
| | Create src/storage/memory.ts with InMemoryStore implementation |
| | Create src/storage/file.ts with JsonFileStore (optional persistence) |
| **4. HTTP Server** | Create src/server.ts with HTTP server setup (Express or native http) |
| | Create src/routes/bookmarks.ts with router for /bookmarks endpoints |
| | Create src/middleware/error-handler.ts for centralized error handling |
| | Create src/middleware/json-parser.ts for request body parsing |
| **5. CRUD Endpoints** | Implement GET /bookmarks (list all, optional ?tag= filter) |
| | Implement GET /bookmarks/:id (get single, 404 if not found) |
| | Implement POST /bookmarks (create, validate fields, return 201) |
| | Implement PUT /bookmarks/:id (update, validate, 404 if not found) |
| | Implement DELETE /bookmarks/:id (delete, 204 on success, 404 if not found) |
| **6. Validation & Errors** | Add URL format validation (reject invalid URLs with 400) |
| | Add required field validation (title, url required; 400 if missing) |
| | Return proper error JSON format: { error: string, status: number } |
| **7. Test Suite** | Create tests/bookmarks.test.ts: test all CRUD operations |
| | Create tests/validation.test.ts: test URL and field validation |
| | Create tests/storage.test.ts: test in-memory and file stores |
| **8. Documentation** | Create README.md with API endpoints, curl examples, setup instructions |

**Milestones:** 8 | **Features:** 26

**Key difference:** This scenario exercises a **different language** (TypeScript), requires **build compilation** (tsc), uses **npm** for dependency management, and introduces **HTTP server testing** (start server, curl, verify responses). The agent needs Bash heavily for npm commands and must handle the compile-fix-test cycle.

---

5. **Verify roadmap structure:**
   - `browser_snapshot` -> Full roadmap with milestones and features
   - `browser_evaluate`:
     ```js
     const roadmap = await fetch('/api/v1/projects/' + PROJECT_ID + '/roadmap', {
       headers: {'Authorization': 'Bearer ' + ENV.token}
     }).then(r => r.json());
     JSON.stringify({
       title: roadmap.title,
       status: roadmap.status,
       milestones: roadmap.milestones.map(m => ({
         title: m.title,
         features: m.features ? m.features.length : 0
       })),
       totalFeatures: roadmap.milestones.reduce((s, m) => s + (m.features ? m.features.length : 0), 0)
     })
     ```

6. `browser_take_screenshot` -> "Phase 4: Roadmap Created"

**Validation (scenario-specific):**

| Check | S1 | S2 | S3 | S4 |
|-------|----|----|----|----|
| Min milestones | 2 | 6 | 5 | 8 |
| Min features | 8 | 15 | 12 | 20 |
| `ROADMAP_OK` | true | true | true | true |

**Decision Tree:**
```
"Create Roadmap" button not visible?
├─ Roadmap already exists -> browser_evaluate check API
├─ Panel rendering error -> browser_console_messages
└─ Use API fallback via browser_evaluate

Milestone creation fails?
├─ UI form not appearing -> browser_snapshot, retry click
├─ API error -> browser_console_messages
└─ Create via browser_evaluate (API fallback)

Feature creation fails per milestone?
├─ "Add Feature" link not clickable -> browser_snapshot to identify correct element
├─ Form validation error -> Check required fields
└─ Create via browser_evaluate (POST /milestones/{id}/features)
```

---

## Phase 5: Autonomous Execution

**Goal:** Start an agentic chat conversation that executes the roadmap tasks.

> **CRITICAL:** The message MUST be sent via API (not browser UI) with an explicit `model` field
> to bypass the auto-router which may select an unhealthy/offline model. Use `"model": "openai/container"`
> for local LM Studio or any other healthy model from LiteLLM `/health`.
>
> The project MUST have autonomy level 4 (Full-Auto) set — either via Advanced Settings during
> project creation, or via `POST /api/v1/projects/{id}` with `{"config":{"autonomy_level":"4"}}`.
> Without this, HITL approval requests will block the agent loop.
>
> If HITL cards appear despite full-auto: use `POST /conversations/{id}/bypass-approvals` to
> auto-approve all future tool calls for that conversation.

**Steps:**

1. **Get AI-friendly roadmap:**
   - `browser_evaluate`:
     ```js
     const aiView = await fetch('/api/v1/projects/' + PROJECT_ID + '/roadmap/ai?format=markdown', {
       headers: {'Authorization': 'Bearer ' + ENV.token}
     }).then(r => r.json());
     aiView.content
     ```
   -> Store as `AI_ROADMAP`

2. **Navigate to chat panel:**
   - `browser_click` -> Chat area on the right side of ProjectDetailPage
   - If a conversation from Phase 2 is active, create a new one:
     - `browser_click` -> (+) "New Conversation" button (`data-testid="new-conversation-btn"`)
   - `browser_snapshot` -> Clean chat with input area visible

3. **Send execution instruction (scenario-specific):**
   - `browser_click` -> Chat input textarea
   - `browser_type` -> Scenario-specific execution prompt:

   **S1 prompt:**
   ```
   Create a CSV-to-JSON converter script in this workspace. Follow the roadmap below exactly.

   RULES:
   - Use only Python standard library (csv, json, argparse). Keep it simple.
   - Run the tests when done to verify they pass.
   - Before committing: verify syntax with `python -m py_compile csv2json.py`
   - Fix any syntax errors or import issues before committing.
   - Commit your changes when everything is clean.

   ROADMAP: [AI_ROADMAP]
   ```

   **S2 prompt:**
   ```
   Build a Python CLI task manager in this workspace. Follow the roadmap exactly,
   implementing each milestone in order.

   RULES:
   - Use argparse for CLI, dataclasses for models, json for storage, pytest for tests.
   - No external dependencies beyond pytest.
   - After each milestone: run `python -m py_compile` on changed files to catch syntax errors.
   - After test suite milestone: run `python -m pytest -v` and fix any failures.
   - Before final commit: verify `python -m task_manager --help` works.
   - Commit after each milestone.

   ROADMAP: [AI_ROADMAP]
   ```

   **S3 prompt:**
   ```
   This workspace contains an existing Python task manager. Your job is to ADD a tags feature.

   RULES:
   - Read and understand the existing code FIRST before making any changes.
   - Do NOT rewrite files from scratch — EDIT existing files to add the feature.
   - After each edit: run `python -m py_compile <file>` to catch syntax errors immediately.
   - Run `python -m pytest -v` after changes to ensure ALL existing tests still pass.
   - Fix any lint errors or import issues you introduce.
   - Commit after each milestone.

   ROADMAP: [AI_ROADMAP]
   ```

   **S4 prompt:**
   ```
   Build a TypeScript REST API for a bookmark manager in this workspace.
   Follow the roadmap exactly, implementing each milestone in order.

   RULES:
   - TypeScript strict mode, no 'any' types. ES modules. Express or native http.
   - After writing code: run `npm run build` to catch TypeScript errors IMMEDIATELY.
   - Fix all type errors before moving to the next milestone.
   - After test suite milestone: run `npm test` and fix failures.
   - Before final commit: verify `npx eslint src/` passes (if ESLint configured).
   - Commit after each milestone.

   ROADMAP: [AI_ROADMAP]
   ```

   - `browser_click` -> Send button

4. **Verify agentic mode activated:**
   - `browser_snapshot` -> Look for:
     - "Agentic" badge with pulsing dot in chat header
     - Streaming content appearing
     - Tool call indicators (left-bordered blocks)
   - `EXEC_STARTED = true`

5. `browser_take_screenshot` -> "Phase 5: Execution Started"

**Validation:**
- Message sent successfully (appears in chat)
- Agentic badge visible (agent is running)
- Streaming content or tool calls appearing within 30 seconds
- `EXEC_STARTED = true`

**Decision Tree:**
```
Message sent but no agentic badge?
├─ Agent mode not activated -> Project may lack workspace_path
│  browser_evaluate: check project.workspace_path
├─ Message treated as simple chat -> Check mode/autonomy settings
│  May need to set autonomy to 4+ via settings popover (gear icon)
└─ Worker not consuming -> Check NATS delivery

No streaming content after 30s?
├─ browser_console_messages -> WS errors?
├─ Check ActiveWorkPanel -> Shows queued/running task?
├─ LiteLLM model timeout -> Check /api/v1/llm/discover for available models
└─ Wait longer (some models take 60s+ for first token)

Agent immediately finishes with short response?
├─ Model may not support tool use -> Check ENV.models for tool-capable
├─ Policy blocked all tools -> Check autonomy level (need >= 3 for auto-edit)
└─ Context too short -> Check message was sent correctly
```

---

## Phase 5b: Blocker & HITL Handling

**Goal:** Handle permission requests and agent questions during execution.

This phase runs **concurrently with Phase 6** — check for these events on every monitoring poll.

**Steps:**

1. **Handle PermissionRequestCards (HITL):**
   - `browser_snapshot` -> Look for PermissionRequestCard (amber-bordered card with countdown timer)
   - If found:
     - Read tool name and command from the card
     - `browser_click` -> "Allow" button (approve the tool execution)
     - Alternatively: `browser_click` -> "Allow Always" (saves policy rule, prevents re-asking for same tool)
     - `browser_snapshot` -> Card changes to approved state (green border)
   - Repeat for every new PermissionRequestCard that appears

2. **Handle agent questions (blockers):**
   - `browser_snapshot` -> Check if latest assistant message contains a question (ends with `?`)
   - If agent is asking a question and NOT executing tools:
     - `browser_click` -> Chat input
     - `browser_type` -> Scenario-specific unblock response:
       - **S1:** `Use your best judgment. Standard library only, keep it minimal.`
       - **S2:** `Use your best judgment and proceed. Standard Python practices, no external dependencies beyond pytest. Use argparse for CLI, dataclasses for models, json module for storage.`
       - **S3:** `Follow the existing code patterns and conventions. Read the existing files for guidance. Do not change the overall architecture. All existing tests must still pass.`
       - **S4:** `Use Express for the HTTP server and vitest for tests. Keep dependencies minimal. Follow TypeScript best practices with strict mode.`
     - `browser_click` -> Send button
     - `browser_wait_for` -> Agent resumes execution

3. **Handle timeout on PermissionRequestCards:**
   - If countdown reaches < 10s without user action:
     - `browser_click` -> "Allow" IMMEDIATELY (before auto-deny kicks in)
   - If auto-denied (card turns red):
     - Agent may need to retry or work around the denied action
     - Monitor for agent adapting

**Validation:**
- All PermissionRequestCards resolved (approved or allowed-always)
- Agent questions answered and execution resumed
- No auto-denied actions (all approved in time)

---

## Phase 6: Execution Monitoring

**Goal:** Monitor agent progress until completion. Poll browser state regularly.

**Steps:**

1. **Monitoring loop** (run for up to 60 minutes — local models need more time):

   > **PATIENCE IS CRITICAL.** Weak/local models (Ollama, LM Studio) are 5-10x slower
   > than cloud models. A task that takes GPT-4 5 minutes may take a local 30b model
   > 30-60 minutes. DO NOT abort early — every tool call is a valuable data point.
   > The goal is to collect data on how different models handle the task, not to get
   > the fastest result. A model that takes 50 tool calls and makes 15 errors but
   > eventually succeeds tells us MORE than a model that gives up after 5 calls.

   Every 30 seconds:
   - `browser_snapshot` -> Observe:
     - **Streaming content**: Is new text appearing?
     - **Tool calls**: Are new tool call blocks appearing? (Read, Write, Edit, Bash indicators)
     - **Step counter**: Check "Step N" in chat header — is it incrementing?
     - **Cost display**: Check running cost in header
     - **Agentic badge**: Still pulsing? (agent still running)
     - **PermissionRequestCards**: Any pending? -> Handle per Phase 5b
     - **Agent questions**: Any new questions? -> Handle per Phase 5b
     - **Error banners**: Any red error messages?

2. **Check for completion:**
   - Agent run finishes when:
     - Agentic badge disappears (no more pulsing dot)
     - `agui.run_finished` event received (no more streaming)
     - Final assistant message appears without tool calls
   - `browser_snapshot` -> Confirm: no active streaming, no pending tool calls

3. **Verify git activity:**
   - `browser_evaluate`:
     ```js
     const gitStatus = await fetch('/api/v1/projects/' + PROJECT_ID + '/git/status', {
       headers: {'Authorization': 'Bearer ' + ENV.token}
     }).then(r => r.json());
     JSON.stringify({branch: gitStatus.branch, dirty: gitStatus.dirty, ahead: gitStatus.ahead})
     ```
   - `browser_evaluate`:
     ```js
     const log = await fetch('/api/v1/projects/' + PROJECT_ID + '/git/log?limit=20', {
       headers: {'Authorization': 'Bearer ' + ENV.token}
     }).then(r => r.json());
     log.map(c => c.message).join('\n')
     ```
   -> Store commit messages

4. **Check for agent stall:**
   - If step counter hasn't changed in 2 minutes AND streaming is idle:
     - `browser_snapshot` -> Check for stall message ("You are repeating...")
     - If stalled: wait for stall detector to abort
     - If no progress at all: consider sending a nudge message
       - `browser_click` -> Chat input
       - `browser_type` -> `Please continue with the next milestone.`
       - `browser_click` -> Send

5. **Check ActiveWorkPanel:**
   - `browser_snapshot` -> Look for ActiveWorkPanel above chat
   - Should show running task with agent mode, step count, cost

6. `browser_take_screenshot` -> "Phase 6: Execution Progress" (capture mid-execution)
7. `browser_take_screenshot` -> "Phase 6: Execution Complete" (capture after agent finishes)

**Validation:**
- Agent executed for minimum tool call count (see table below)
- At least 1 git commit in the test branch
- Agent run completed (not stuck/stalled)
- No unhandled errors
- Tool call diversity matches expected pipeline (explore → search → build → verify)

**Minimum tool call thresholds per scenario:**

| Check | S1 | S2 | S3 | S4 |
|-------|----|----|----|----|
| Total tool calls | >= 15 | >= 33 | >= 47 | >= 54 |
| Unique tools used | >= 3 | >= 6 | >= 7 | >= 7 |
| search_files calls | 0 | >= 1 | >= 5 | >= 4 |
| read_file calls | 0 | >= 3 | >= 10 | >= 5 |
| bash calls | >= 2 | >= 12 | >= 15 | >= 20 |
| Build-verify cycles | >= 1 | >= 3 | >= 5 | >= 6 |

If agent finishes with fewer calls than minimum, it likely skipped exploration
or verification steps. Record as PARTIAL and note which pipeline phases were skipped.

**Decision Tree:**
```
Agent stuck (no NEW tool calls for 10 minutes)?
├─ PermissionRequestCard pending -> Approve it (Phase 5b)
├─ Agent asking question -> Answer it (Phase 5b)
├─ Stall detected -> Wait for auto-abort, then send "Continue with next milestone"
├─ Model rate limited -> Wait 60s, agent should auto-retry
└─ Hard stuck -> browser_click Stop button, assess partial results

Agent errored out?
├─ Read error message in chat
├─ browser_console_messages -> WS/API errors?
├─ Check if partial work was committed (git log)
├─ Send follow-up: "There was an error. Please continue from where you left off."
└─ If error persists -> Proceed to Phase 7 with partial results

Agent finished too quickly (< 2 minutes)?
├─ Check: did it actually create files? (browser_evaluate: ls workspace)
├─ Check: was response just text advice, not actual tool execution?
├─ May need higher autonomy level -> Open settings (gear icon), increase autonomy
└─ Resend instruction with explicit "Use the Write tool to create files"

0 git commits?
├─ Agent may have written files but not committed
├─ Check workspace for files: browser_evaluate fetch git/status
├─ OK if files exist uncommitted (agent may commit at end)
└─ FLAG if workspace is empty: "Agent did not produce any files"
```

---

## Phase 7: Program Validation (Scenario-Specific)

**Goal:** Verify the resulting program is functional by running it in the workspace.

All validation steps are executed **via the agent chat** — ask the agent to run commands and observe the Bash tool output in `browser_snapshot`.

---

### S1 Validation: CSV-to-JSON Converter

1. **Check files exist:**
   - `browser_click` -> Chat input
   - `browser_type` -> `List all Python files in the workspace: ls -la *.py`
   - `browser_click` -> Send
   - `browser_snapshot` -> Verify `csv2json.py` and `test_csv2json.py` exist

2. **Create sample CSV and test conversion:**
   - `browser_click` -> Chat input
   - `browser_type`:
     ```
     Create a sample test.csv with this content, then run the converter:
     echo 'name,age,city\nAlice,30,Berlin\nBob,25,Munich' > test.csv
     python csv2json.py test.csv
     cat test.json
     ```
   - `browser_click` -> Send
   - `browser_snapshot` -> Verify JSON output is correct

3. **Test error handling:**
   - `browser_type` -> `python csv2json.py nonexistent.csv`
   - `browser_snapshot` -> Should show error message, not traceback

4. **Run tests:**
   - `browser_type` -> `python -m pytest test_csv2json.py -v`
   - `browser_snapshot` -> Check for "passed"

5. `browser_take_screenshot` -> "Phase 7: S1 Validation"

**S1 Validation Matrix:**

| Check | Verify | Required |
|-------|--------|----------|
| csv2json.py exists | File listing | Yes |
| Converts valid CSV | JSON output correct | Yes |
| Error on missing file | No traceback, user-friendly message | Yes |
| Tests pass | pytest output | Yes |

---

### S2 Validation: CLI Task Manager

1. **Check file structure:**
   - `browser_click` -> Chat input
   - `browser_type` -> `Show the project structure: find . -name "*.py" -o -name "*.toml" -o -name "*.md" | grep -v __pycache__ | sort`
   - `browser_snapshot` -> Verify task_manager package and test files exist

2. **Run help:**
   - `browser_type` -> `python -m task_manager --help`
   - `browser_snapshot` -> Shows "usage" and subcommand names

3. **Run full CRUD cycle:**
   - `browser_type`:
     ```
     Run these commands in sequence and show output:
     1. python -m task_manager add --title "Buy groceries" --priority high
     2. python -m task_manager add --title "Clean house" --priority low
     3. python -m task_manager list
     4. python -m task_manager list --status open
     5. python -m task_manager complete 1
     6. python -m task_manager list --status done
     7. python -m task_manager delete 2
     8. python -m task_manager list
     ```
   - `browser_snapshot` -> Verify each step

4. **Run tests:**
   - `browser_type` -> `python -m pytest -v`
   - `browser_snapshot` -> All pass

5. **Check JSON persistence:**
   - `browser_type` -> `cat tasks.json | python -m json.tool`
   - `browser_snapshot` -> Valid JSON

6. `browser_take_screenshot` -> "Phase 7: S2 Validation"

**S2 Validation Matrix:**

| Check | Verify | Required |
|-------|--------|----------|
| Package structure | task_manager/__init__.py, __main__.py | Yes |
| --help exits 0 | Shows usage and commands | Yes |
| Add command | Creates task | Yes |
| List command | Shows tasks | Yes |
| Complete command | Marks done | Yes |
| Delete command | Removes task | Yes |
| Tests pass | pytest output | Yes |
| JSON valid | json.tool parses file | Yes |
| README exists | File listing | No |

---

### S3 Validation: Tags Feature (Brownfield)

1. **Run existing tests first (regression check):**
   - `browser_click` -> Chat input
   - `browser_type` -> `Run the full test suite to verify no regressions: python -m pytest -v`
   - `browser_snapshot` -> ALL tests must pass (old + new)

2. **Test tags on add:**
   - `browser_type` -> `python -m task_manager add --title "Tagged task" --tags "urgent,work"`
   - `browser_snapshot` -> Task created with tags

3. **Test search by tag:**
   - `browser_type` -> `python -m task_manager search --tag "urgent"`
   - `browser_snapshot` -> Found the tagged task

4. **Test tags in list output:**
   - `browser_type` -> `python -m task_manager list`
   - `browser_snapshot` -> Tags visible in output

5. **Test data migration (existing tasks without tags):**
   - `browser_type` -> `python -m task_manager list`
   - `browser_snapshot` -> Existing tasks show empty tags, no crash

6. **Verify existing CRUD still works:**
   - `browser_type`:
     ```
     python -m task_manager add --title "No tags task"
     python -m task_manager complete 1
     python -m task_manager delete 1
     ```
   - `browser_snapshot` -> All work without errors

7. **Check README updated:**
   - `browser_type` -> `grep -i "tag" README.md`
   - `browser_snapshot` -> Tags feature documented

8. `browser_take_screenshot` -> "Phase 7: S3 Validation"

**S3 Validation Matrix:**

| Check | Verify | Required |
|-------|--------|----------|
| Existing tests pass | No regression | **Critical** |
| New tag tests pass | pytest shows new tests passing | Yes |
| Add with --tags | Command succeeds | Yes |
| Search --tag | Finds tagged tasks | Yes |
| Tags in list | Tags visible in output | Yes |
| Old tasks survive | No crash on tagless tasks | Yes |
| Existing CRUD works | add/complete/delete still functional | Yes |
| README updated | grep finds "tag" mention | No |

---

### S4 Validation: TypeScript REST API

1. **Check TypeScript compiles:**
   - `browser_click` -> Chat input
   - `browser_type` -> `npm run build`
   - `browser_snapshot` -> No TypeScript errors, exit 0

2. **Run tests:**
   - `browser_type` -> `npm test`
   - `browser_snapshot` -> Tests pass

3. **Start server and test endpoints:**
   - `browser_type`:
     ```
     Run these commands (start server in background, test, then stop):
     npm start &
     sleep 2
     echo "=== POST ==="
     curl -s -X POST http://localhost:3000/bookmarks -H "Content-Type: application/json" -d '{"title":"GitHub","url":"https://github.com","tags":["dev"]}'
     echo "=== GET ALL ==="
     curl -s http://localhost:3000/bookmarks
     echo "=== GET ONE ==="
     curl -s http://localhost:3000/bookmarks/1
     echo "=== DELETE ==="
     curl -s -X DELETE http://localhost:3000/bookmarks/1 -w "\nHTTP %{http_code}"
     echo "=== VALIDATION ==="
     curl -s -X POST http://localhost:3000/bookmarks -H "Content-Type: application/json" -d '{"title":"Bad"}'
     kill %1 2>/dev/null
     ```
   - `browser_snapshot` -> Verify:
     - POST returns 201 with bookmark data
     - GET ALL returns array
     - GET ONE returns single bookmark
     - DELETE returns 204
     - Missing URL returns 400

4. **Check no 'any' types:**
   - `browser_type` -> `grep -rn "any" src/ --include="*.ts" | grep -v node_modules | grep -v "// " | head -20`
   - `browser_snapshot` -> Should find 0 or minimal `any` occurrences

5. **Check project structure:**
   - `browser_type` -> `find src tests -name "*.ts" | sort`
   - `browser_snapshot` -> Proper directory structure

6. `browser_take_screenshot` -> "Phase 7: S4 Validation"

**S4 Validation Matrix:**

| Check | Verify | Required |
|-------|--------|----------|
| tsc compiles | `npm run build` exit 0 | Yes |
| Tests pass | `npm test` exit 0 | Yes |
| POST /bookmarks | Returns 201, bookmark JSON | Yes |
| GET /bookmarks | Returns array | Yes |
| GET /bookmarks/:id | Returns single bookmark | Yes |
| DELETE /bookmarks/:id | Returns 204 | Yes |
| Validation works | Missing URL -> 400 | Yes |
| No `any` types | grep finds 0 | No |
| README exists | File listing | No |

---

### General Validation Decision Tree

```
Files don't exist?
├─ Agent may have created files in wrong directory
│  Ask agent: "List all files you created. Where is the project root?"
├─ Agent may not have executed (Phase 6 issue)
└─ FAIL: "No program files produced"

Build/compile fails? (S4)
├─ TypeScript errors -> Ask agent: "Fix the TypeScript compilation errors"
├─ Missing dependencies -> Ask agent: "Run npm install"
└─ tsconfig issue -> Ask agent: "Check tsconfig.json"

Tests fail?
├─ Read test output carefully for specific failures
├─ If > 50% pass -> PARTIAL
├─ Ask agent: "The tests are failing. Here is the output: [paste]. Fix the issues."
├─ Re-run tests after fix (allow 1 fix attempt)
└─ If still failing after fix -> PARTIAL

Regression in S3?
├─ Existing tests fail -> CRITICAL FAIL (the whole point of S3 is non-regression)
├─ Ask agent: "You broke existing tests. Revert your changes and try a different approach."
└─ If agent can fix -> re-run all tests

Functional test fails?
├─ Wrong output format -> Accept if core logic works
├─ Crash/traceback -> Ask agent to fix, re-test (1 attempt)
└─ PARTIAL if >= 60% of checks pass
```

5. `browser_take_screenshot` -> "Phase 7: Program Validation Results"

---

## Phase 7b: Code Quality Checks (Linting, Import Sort, Build)

**Goal:** Verify the generated code passes language-specific quality checks.
These are run **in addition** to the functional validation in Phase 7.

> **Error feedback to agents:** CodeForge feeds tool errors (including build/lint failures)
> back to the LLM as `{"role": "tool", "content": "Error: exit code 1\n--- stderr ---\n..."}`.
> The agent sees the full error output and can decide to fix the code. Correction hints are
> auto-appended (e.g., "Hint: The file was not found. Use list_directory...").
>
> **Quality Gates:** CodeForge has a post-agent Quality Gate system (`QualityGateExecutor`)
> that can auto-run tests/lint after the agent finishes. However, this only applies to
> Run-based execution, not Conversation-based. For conversation flows, Claude Code must
> verify code quality explicitly via the steps below.
>
> **Agent self-correction:** If the agent encounters build/lint errors during execution,
> it should fix them autonomously. The quality checks below validate whether the FINAL
> output is clean — if not, send the errors back to the agent for a fix attempt.

### Quality Checks by Scenario

#### S1 (Python — single script):

1. **Syntax check:**
   ```bash
   python -m py_compile csv2json.py && echo "SYNTAX OK" || echo "SYNTAX ERROR"
   python -m py_compile test_csv2json.py && echo "SYNTAX OK" || echo "SYNTAX ERROR"
   ```
   -> Both must exit 0

2. **Import sort (isort dry-run):**
   ```bash
   python -m isort --check-only --diff csv2json.py test_csv2json.py 2>&1
   ```
   -> Exit 0 = imports sorted correctly. If isort not available, skip (not required).

3. **Linting (ruff or flake8):**
   ```bash
   python -m ruff check csv2json.py test_csv2json.py 2>&1 || \
   python -m flake8 csv2json.py test_csv2json.py 2>&1 || \
   echo "No linter available — SKIP"
   ```
   -> 0 errors preferred. Warnings acceptable. If no linter installed, SKIP.

4. **Type check (optional):**
   ```bash
   python -m mypy csv2json.py 2>&1 || echo "mypy not available — SKIP"
   ```

#### S2 (Python — multi-module):

All S1 checks PLUS:

5. **Package importable:**
   ```bash
   python -c "import task_manager" && echo "IMPORT OK" || echo "IMPORT ERROR"
   ```

6. **Build check (pip install):**
   ```bash
   pip install -e . 2>&1 && echo "BUILD OK" || echo "BUILD ERROR"
   ```
   -> Must exit 0 if pyproject.toml exists

#### S3 (Python — brownfield):

All S2 checks PLUS:

7. **Regression lint (only changed files):**
   ```bash
   git diff --name-only HEAD~1 -- '*.py' | xargs python -m ruff check 2>&1
   ```
   -> New code must not introduce lint errors

#### S4 (TypeScript):

1. **TypeScript compile:**
   ```bash
   npm run build 2>&1
   ```
   -> Must exit 0. This is the PRIMARY quality check — catches type errors, missing imports, syntax.

2. **ESLint:**
   ```bash
   npx eslint src/ 2>&1 || echo "No ESLint config — SKIP"
   ```

3. **Import sort (eslint-plugin-import or similar):**
   ```bash
   npx eslint src/ --rule '{"import/order": "error"}' 2>&1 || echo "SKIP"
   ```

4. **No `any` types (strict mode validation):**
   ```bash
   grep -rn ": any\|as any\|<any>" src/ --include="*.ts" | grep -v node_modules | head -10
   ```
   -> 0 matches = PASS

5. **Package.json scripts defined:**
   ```bash
   node -e "const p=require('./package.json'); console.log('build:',!!p.scripts?.build,'test:',!!p.scripts?.test)"
   ```
   -> Both must be `true`

### Quality Check Validation Matrix

| Check | S1 | S2 | S3 | S4 | Required |
|-------|----|----|----|----|----------|
| Syntax / Compile | `py_compile` | `py_compile` | `py_compile` | `tsc` (npm build) | **Yes** |
| Linting | ruff/flake8 | ruff/flake8 | ruff (changed only) | ESLint | No (skip if unavailable) |
| Import sort | isort | isort | isort | ESLint import/order | No |
| Type check | mypy | mypy | mypy | tsc strict | S4: Yes, others: No |
| Package build | N/A | pip install -e . | N/A | npm run build | S2/S4: Yes |

### Error Feedback Loop

If quality checks fail, send the errors back to the agent for self-correction:

1. **Collect all failing check outputs**
2. **Send in chat:**
   ```
   The code has quality issues. Please fix them:

   [SYNTAX ERROR] csv2json.py line 15: SyntaxError: unexpected indent
   [LINT] csv2json.py:8:1: F401 'os' imported but unused
   [IMPORT SORT] csv2json.py: imports not sorted (stdlib before third-party)

   Fix these issues and commit the changes.
   ```
3. **Wait for agent to execute tool calls** (Write/Edit to fix files)
4. **Re-run quality checks** (1 retry allowed)
5. **Record final result** as PASS (all clean), PARTIAL (syntax OK but lint warnings), or FAIL

### Decision Tree
```
Syntax/compile fails?
├─ Send error to agent: "Fix the syntax error: [error output]"
├─ Wait for agent fix (1 attempt)
└─ Re-check. Still fails -> FAIL

Lint errors?
├─ Critical (undefined vars, unused imports) -> Send to agent for fix
├─ Style warnings (line length, naming) -> PARTIAL (acceptable)
└─ No linter available -> SKIP

Import sort wrong?
├─ Send to agent: "Fix import order: [isort diff]"
├─ Or accept as PARTIAL (cosmetic issue)
└─ No isort available -> SKIP

Build fails (S2/S4)?
├─ Missing dependency -> Send to agent: "Add missing dependency"
├─ Type error (S4) -> Send to agent: "Fix TypeScript error"
└─ Config error -> Send to agent: "Fix pyproject.toml/tsconfig.json"
```

---

## Phase 8: Report & Cleanup

**Goal:** Collect metrics, generate report, preserve test branch.

**Steps:**

1. **Collect metrics via API:**
   - `browser_evaluate`:
     ```js
     // Goal count
     const goals = await fetch('/api/v1/projects/' + PROJECT_ID + '/goals', {
       headers: {'Authorization': 'Bearer ' + ENV.token}
     }).then(r => r.json());

     // Roadmap stats
     const roadmap = await fetch('/api/v1/projects/' + PROJECT_ID + '/roadmap', {
       headers: {'Authorization': 'Bearer ' + ENV.token}
     }).then(r => r.json());

     // Git stats
     const gitLog = await fetch('/api/v1/projects/' + PROJECT_ID + '/git/log?limit=50', {
       headers: {'Authorization': 'Bearer ' + ENV.token}
     }).then(r => r.json());

     JSON.stringify({
       goals: goals.length,
       milestones: roadmap.milestones ? roadmap.milestones.length : 0,
       features: roadmap.milestones ?
         roadmap.milestones.reduce((s, m) => s + (m.features ? m.features.length : 0), 0) : 0,
       commits: gitLog.length,
       commitMessages: gitLog.map(c => c.message)
     })
     ```

2. **Collect conversation stats (including tool call breakdown):**
   - `browser_evaluate`:
     ```js
     const convs = await fetch('/api/v1/projects/' + PROJECT_ID + '/conversations', {
       headers: {'Authorization': 'Bearer ' + ENV.token}
     }).then(r => r.json());

     let totalMessages = 0;
     let totalToolCalls = 0;
     const toolCallsByType = {};  // Track which tools were used

     for (const conv of convs) {
       const msgs = await fetch('/api/v1/conversations/' + conv.id + '/messages', {
         headers: {'Authorization': 'Bearer ' + ENV.token}
       }).then(r => r.json());
       totalMessages += msgs.length;

       for (const msg of msgs) {
         if (msg.tool_calls) {
           totalToolCalls += msg.tool_calls.length;
           for (const tc of msg.tool_calls) {
             toolCallsByType[tc.name] = (toolCallsByType[tc.name] || 0) + 1;
           }
         }
       }
     }

     JSON.stringify({
       conversations: convs.length,
       totalMessages,
       totalToolCalls,
       toolCallsByType  // e.g. {Write: 12, Read: 8, Bash: 15, Edit: 5, Search: 3, ...}
     })
     ```

3. **Take final screenshots:**
   - `browser_click` -> "More panels..." -> "Goals"
   - `browser_take_screenshot` -> "Final: GoalsPanel"
   - `browser_click` -> "More panels..." -> "Roadmap"
   - `browser_take_screenshot` -> "Final: RoadmapPanel"
   - `browser_click` -> Chat tab
   - `browser_take_screenshot` -> "Final: Chat History"

4. **Generate report summary** (Claude Code writes to report file):

   ```
   ============================================
   AUTONOMOUS GOAL-TO-PROGRAM TEST REPORT
   ============================================
   Date:          YYYY-MM-DDTHH:MM:SSZ
   Scenario:      S1/S2/S3/S4 (Easy/Medium/Hard/Expert)
   Branch:        test/autonomous-YYYYMMDD-HHmmss
   Project ID:    <uuid>

   PHASE RESULTS:
   - Phase 0 (Environment):     PASS/FAIL
   - Phase 1 (Project Setup):   PASS/FAIL
   - Phase 2 (Goal Discovery):  PASS/FAIL (N goals)
   - Phase 3 (Goal Validation): PASS/FAIL (N/M coverage)
   - Phase 4 (Roadmap):         PASS/FAIL (N milestones, N features)
   - Phase 5 (Execution):       PASS/FAIL
   - Phase 6 (Monitoring):      PASS/FAIL (N commits)
   - Phase 7 (Validation):      PASS/FAIL (see scenario-specific checks)

   SCENARIO-SPECIFIC CHECKS (Phase 7):
   [List all checks from the scenario's Validation Matrix with PASS/FAIL]

   METRICS:
   - Goals:          N
   - Milestones:     N
   - Features:       N
   - Commits:        N
   - Messages:       N
   - Conversations:  N

   TOOL CALL BREAKDOWN:
   - Write:          N calls
   - Read:           N calls
   - Edit:           N calls
   - Bash:           N calls
   - Search:         N calls
   - Glob:           N calls
   - ListDir:        N calls
   - propose_goal:   N calls
   - Other:          N calls
   - TOTAL:          N calls

   TOOL CALL DIVERSITY SCORE:
   - Tools used:     N/7 unique tools
   - Expected:       [from Tool Call Coverage Matrix for scenario]
   - Match:          PASS/PARTIAL/FAIL

   CODE QUALITY (Phase 7b):
   - Syntax/Compile:  PASS/FAIL
   - Linting:         PASS/PARTIAL/SKIP (N errors, N warnings)
   - Import Sort:     PASS/SKIP
   - Build:           PASS/FAIL/N/A
   - Agent self-fix:  N attempts (did agent fix errors when fed back?)

   OVERALL: PASS / PARTIAL / FAIL
   ============================================
   ```

5. **Preserve branch** (do NOT delete):
   - Branch `$BRANCH` remains in the workspace for manual review
   - Push to remote if available:
     ```js
     fetch('/api/v1/projects/' + PROJECT_ID + '/git/push', {
       method: 'POST',
       headers: {'Authorization': 'Bearer ' + ENV.token, 'Content-Type': 'application/json'},
       body: JSON.stringify({branch: BRANCH})
     })
     ```

6. `browser_take_screenshot` -> "Phase 8: Test Complete"

---

## Implementation Gaps Identified

Features required for full automation but potentially not yet implemented:

| Gap | Current State | Impact on Test |
|-----|---------------|---------------|
| Goals -> Roadmap auto-generation | Manual/import only | Phase 4 uses manual UI creation |
| Roadmap -> Task auto-decomposition | PlanFeature exists but not wired | Phase 5 uses chat-based execution |
| AI Discover opens new conversation | May redirect away from GoalsPanel | Phase 2 adapts to navigation change |
| File listing API | May not exist at /files endpoint | Phase 7 uses agent to check files |
| Direct command execution API | May not exist at /exec endpoint | Phase 7 uses agent Bash tool |

**Fallback strategy:** Where auto-generation is not available, the test uses the UI (milestones/features created manually in RoadmapPanel) and agent-based execution (agentic chat with full roadmap as prompt).

---

## Estimated Resources per Scenario

> **Local models (Ollama, LM Studio) take 3-10x longer than cloud models.**
> The times below show ranges for cloud (fast) and local (slow) models.
> DO NOT abort a run just because it's slow — the data from slow models
> is equally valuable for benchmarking and routing decisions.

| Metric | S1 Easy | S2 Medium | S3 Hard | S4 Expert |
|--------|---------|-----------|---------|-----------|
| **Time (cloud model)** | 5-10 min | 10-20 min | 15-30 min | 20-45 min |
| **Time (local model)** | 15-45 min | 30-90 min | 45-120 min | 60-180 min |
| **Max wait before abort** | 60 min | 120 min | 150 min | 180 min |
| **LLM Cost (cloud)** | $0.20-$1.00 | $0.50-$3.00 | $1.00-$5.00 | $2.00-$8.00 |
| **LLM Cost (local)** | $0.00 | $0.00 | $0.00 | $0.00 |
| **Expected tool calls** | 15-50 | 33-80 | 47-120 | 54-150 |
| **Expected unique tools** | 3-4 | 6-7 | 7 | 7 |

### Why Local Model Data Matters

Every tool call from a weak model is a data point that helps:
1. **Routing decisions** — which tasks need strong models vs weak ones?
2. **Quality thresholds** — at what point should the system switch models?
3. **Benchmark scores** — how do different models compare on the same task?
4. **Error patterns** — where do weak models fail? (edit_file old_text mismatches, syntax errors, stalls)
5. **Cost optimization** — can a $0 local model handle S1? S2? Where's the cutoff?

A run that takes 50 tool calls with 15 errors tells us MORE than one that takes 5 calls and gives up.

---

## Key Files Reference

| Component | File |
|-----------|------|
| GoalsPanel | `frontend/src/features/project/GoalsPanel.tsx` |
| GoalProposalCard | `frontend/src/features/project/GoalProposalCard.tsx` |
| RoadmapPanel | `frontend/src/features/project/RoadmapPanel.tsx` |
| ChatPanel | `frontend/src/features/project/ChatPanel.tsx` |
| PermissionRequestCard | `frontend/src/features/project/PermissionRequestCard.tsx` |
| ProjectDetailPage | `frontend/src/features/project/ProjectDetailPage.tsx` |
| CreateProjectModal | `frontend/src/features/dashboard/CreateProjectModal.tsx` |
| ActiveWorkPanel | `frontend/src/features/project/ActiveWorkPanel.tsx` |
| Goal domain model | `internal/domain/goal/goal.go` |
| Goal discovery service | `internal/service/goal_discovery.go` |
| Goal researcher prompt | `internal/service/prompts/modes/goal_researcher.yaml` |
| propose_goal tool | `workers/codeforge/tools/propose_goal.py` |
| Roadmap service | `internal/service/roadmap.go` |
| Orchestrator | `internal/service/orchestrator.go` |
| Agent loop | `workers/codeforge/agent_loop.py` |
| Conversation dispatch | `internal/service/conversation.go` |
| HITL approval | `internal/service/runtime_approval.go` |
