# Aider — Deep Technical Analysis

> Date: 2026-02-14

## Overview

- **URL:** https://aider.chat / https://github.com/Aider-AI/aider
- **Stars:** ~40,000+ | **License:** Apache 2.0
- **Language:** Python (100%)
- **Installation:** `pip install aider-chat` / Docker / pipx
- **Created by:** Paul Gauthier
- **Status:** Active development (last stable version 0.86.1 — development pause from August 2025 triggered community discussions)
- **Self-Coding:** Aider writes ~70% of its own new code per release

---

## 1. Architecture

### 1.1 Tech Stack

| Component | Technology |
|---|---|
| Language | Python 3.9+ |
| LLM Connection | LiteLLM (127+ Providers) |
| Code Parsing | tree-sitter (via py-tree-sitter-languages / tree-sitter-language-pack) |
| Graph Ranking | NetworkX (PageRank algorithm) |
| Terminal UI | prompt_toolkit (Autocomplete, History, Color output) |
| Browser UI | Experimental (`--browser` flag) |
| Git Integration | GitPython / native git CLI |
| Speech Recognition | Whisper API (OpenAI) |
| Web Scraping | Playwright (optional) |
| Packaging | pip / pipx / Docker |

### 1.2 Internal Module Structure

```
aider/
├── main.py                    # Entry Point, CLI-Argument-Parsing
├── coders/                    # Edit format implementations (core module)
│   ├── base_coder.py         # Abstract base class (~highest complexity)
│   ├── base_prompts.py       # System prompts for base coder
│   ├── editblock_coder.py    # SEARCH/REPLACE block format ("diff")
│   ├── wholefile_coder.py    # Whole file replacement ("whole")
│   ├── udiff_coder.py        # Unified diff format ("udiff")
│   ├── architect_coder.py    # Two-model architect/editor pattern
│   ├── ask_coder.py          # Ask mode (no edits)
│   ├── help_coder.py         # Help mode (Aider documentation)
│   ├── context_coder.py      # File selection with reflection
│   └── [format]_prompts.py   # Format-specific prompts
├── models.py                  # LLM provider abstraction via LiteLLM
├── commands.py                # In-chat commands (/add, /drop, /undo, etc.)
├── io.py                      # Terminal I/O (prompt_toolkit)
├── repomap.py                 # Repository map (tree-sitter + PageRank)
├── repo.py                    # Git operations
├── linter.py                  # Lint integration (built-in + custom)
├── scrape.py                  # Web scraping (Playwright + Markdown conversion)
├── voice.py                   # Voice input (Whisper)
├── resources/                 # Configuration files, model metadata
│   ├── model-settings.yml    # Default model configurations
│   └── model-metadata.json   # Context window sizes, pricing
└── website/                   # Documentation (aider.chat)
```

### 1.3 Coder Class Hierarchy

```
Coder (base_coder.py)          # Factory: Coder.create(edit_format=...)
├── EditBlockCoder             # SEARCH/REPLACE blocks (default for GPT-4o)
├── WholeFileCoder             # Complete file (default for GPT-3.5)
├── UdiffCoder                 # Unified diff (default for GPT-4 Turbo)
├── ArchitectCoder             # Two-model pipeline (Architect → Editor)
├── AskCoder                   # Ask mode (read-only)
├── HelpCoder                  # Help mode (Aider docs)
└── ContextCoder               # File selection with reflection
```

**Factory Pattern:** `Coder.create()` dynamically selects the implementation based on `edit_format`. Each subclass defines:
1. `edit_format` attribute — Identifies the strategy
2. `get_edits()` method — Extracts code changes from the LLM response
3. `apply_edits()` method — Applies extracted changes to files

### 1.4 Message Processing Pipeline

```
run()
  → get_input()           # Read user input (terminal/watch/script)
  → Command Routing       # If "/" prefix: Commands.run()
  → run_one()             # Preprocessing
    → send_message()      # Main LLM interaction
      → format_messages()
        → format_chat_chunks()
          → get_repo_messages()    # Generate repo map
        → System Messages         # Prompts + context
        → Done Messages            # Summarized history
        → Current Messages         # Current conversation
      → LiteLLM API Call          # Streaming or batch
      → get_edits()               # Format-specific parsing
      → apply_edits()             # Modify files
      → Auto-Lint                 # Lint check of changed files
      → Auto-Test                 # Run test suite (optional)
      → Git Commit                # Auto-commit with generated messages
    → Reflection Loop             # On errors: up to max_reflections=3 attempts
```

### 1.5 State Management

| Attribute | Type | Purpose |
|---|---|---|
| `abs_fnames` | set | Absolute paths of editable files |
| `abs_read_only_fnames` | set | Reference files (context only) |
| `main_model` | Model | Primary LLM |
| `repo` | GitRepo | Git repository interface |
| `repo_map` | RepoMap | Codebase context generator |
| `commands` | Commands | Command handler |
| `io` | InputOutput | Terminal interaction |
| `done_messages` | list | Completed chat history |
| `cur_messages` | list | Current conversation |
| `max_reflections` | int | Max correction attempts (default: 3) |

---

## 2. Core Concepts

### 2.1 Edit Formats

Aider supports various strategies for how LLMs express code changes. Each format is a trade-off between simplicity, efficiency, and model compatibility.

#### 2.1.1 Whole Format (`--edit-format whole`)
- **Principle:** LLM returns the complete file, even if only a few lines were changed
- **Syntax:** Filename before code fence, then complete file contents
- **Default for:** GPT-3.5
- **Advantages:** Simplest format, lowest error rate during parsing
- **Disadvantages:** High token consumption, slow for large files, expensive

#### 2.1.2 Diff Format / Search-Replace Blocks (`--edit-format diff`)
- **Principle:** LLM returns SEARCH/REPLACE blocks — searches for exact text and replaces it
- **Syntax:**
  ```
  path/to/file.py
  <<<<<<< SEARCH
  original code
  =======
  replacement code
  >>>>>>> REPLACE
  ```
- **Default for:** GPT-4o, Claude Sonnet
- **Advantages:** Efficient, only changed parts are transmitted
- **Disadvantages:** Requires exact string matches, sensitive to whitespace errors

#### 2.1.3 Diff-Fenced Format (`--edit-format diff-fenced`)
- **Principle:** Like Diff, but filename inside the code fence
- **Default for:** Gemini models
- **Reason:** Gemini models have difficulty with standard diff fencing

#### 2.1.4 Unified Diff Format (`--edit-format udiff`)
- **Principle:** Based on Standard Unified Diff, but simplified
- **Syntax:** `---`/`+++` markers, `@@` hunk headers, `+`/`-` line markers
- **Default for:** GPT-4 Turbo family
- **Advantages:** Reduces "Lazy Coding" (models elided large code blocks with `# ... original code ...` comments)
- **Disadvantages:** More complex parsing, higher error rate with some models

#### 2.1.5 Udiff-Simple Format (`--edit-format udiff-simple`)
- **Variant:** Simplified version of Udiff
- **Default for:** Gemini 2.5 Pro

#### 2.1.6 Patch Format (`--edit-format patch`)
- **New format:** Specifically for OpenAI GPT-4.1

#### 2.1.7 Editor-Diff and Editor-Whole (`--editor-edit-format`)
- **Principle:** Streamlined versions of Diff/Whole for Architect Mode
- **Prompt:** Simpler, focused only on file editing (no problem solving)
- **Usage:** In combination with `--editor-edit-format` in Architect Mode

#### 2.1.8 Architect Format (`--edit-format architect` / `--architect`)
- **Principle:** Two-step process with two LLM calls (see Section 2.5)

### 2.2 Repository Map (tree-sitter + PageRank)

The Repo Map is Aider's most innovative component — a compact, token-budgeted overview of the entire codebase.

#### Technical Pipeline

**Step 1: Code Parsing with tree-sitter**
- tree-sitter parses source code into Abstract Syntax Trees (ASTs)
- Modified `tags.scm` files (from open-source tree-sitter implementations) identify:
  - **Definitions (`def`):** Functions, classes, variables, types
  - **References (`ref`):** Usages of these symbols elsewhere in the code
- Result: Tag entries like `Tag(rel_fname='app/we.py', fname='/path/app/we.py', line=6, name='we', kind='def')`
- Supported languages: 100+ (Python, JS, TS, Java, C/C++, Go, Rust, etc.)

**Step 2: Graph Construction**
- Files are represented as nodes in the graph
- Edges connect files that share dependencies (one file defines symbol X, another references it)
- Edge weighting:
  - Referenced identifiers: **10x** weight
  - Long identifiers: **10x** weight (longer names are more specific)
  - Files in the chat: **50x** weight (focus on active context)
  - Private identifiers (with underscore): **1/10** weight

**Step 3: Ranking with PageRank**
- NetworkX PageRank algorithm on the file graph
- Result: Sorted list of the most important code definitions
- Higher-ranked files/symbols appear first in the map

**Step 4: Token Budget Optimization (Binary Search)**
- Configurable token budget via `--map-tokens` (default: 1024 tokens)
- Aider dynamically adjusts the budget:
  - When no files in chat: Map is significantly expanded
  - With many chat files: Map is compressed
- **Binary Search** between lower and upper bounds of ranked tags:
  - Tests whether token count fits within `max_map_tokens`
  - Best fitting tree is retained
- Caching: Parsed symbols are cached to avoid repeated parsing

**Step 5: Output Format**
- List of files with their most important symbol definitions
- Shows critical code lines for each definition (signatures, class declarations)
- LLM can derive API usage, module structure, and dependencies from this

#### Map Refresh Modes
| Mode | Description |
|---|---|
| `auto` (Default) | Refresh when files change |
| `always` | Regenerate with every message |
| `files` | Only when chat files change |
| `manual` | Only on explicit request |

### 2.3 Chat Modes

| Mode | Command | Description |
|---|---|---|
| **Code** (Default) | `/code` | LLM modifies files directly |
| **Architect** | `/architect` | Two-model pipeline: Plan + Edit |
| **Ask** | `/ask` | Answer questions, no changes |
| **Help** | `/help` | Questions about Aider itself |

- **Single-Message Override:** `/ask why is this function slow?` sends one message in Ask mode, then returns to the active mode
- **Persistent Switch:** `/chat-mode architect` switches permanently
- **CLI Launch:** `aider --chat-mode architect` or `aider --architect`

**Recommended Workflow:** Ask mode for discussion and planning, then Code mode for implementation. The conversation from Ask mode flows as context into Code mode.

### 2.4 Context Window Management

Aider actively manages the LLM context window:

**Automatic Summarization:**
- When chat history exceeds the configured token limit (`--max-chat-history-tokens`)
- "Weak Model" (cheaper model) creates summaries of older messages
- Recent messages remain verbatim, older ones are compressed
- Fallback to "Strong Model" when Weak Model fails

**Manual Control:**
- `/drop` — Remove files from the chat
- `/clear` — Clear conversation history
- `/tokens` — Display token usage
- `/add` / `/read-only` — Add files (editable vs. read-only)

**Best Practice:** Only add relevant files to the chat. The Repo Map automatically provides context about the rest of the codebase. Beyond ~25k tokens of context, most models lose focus.

### 2.5 Architect/Editor Pattern

The Architect/Editor pattern is Aider's most important architectural innovation for separating reasoning and code editing.

**Problem:** LLMs must simultaneously (a) solve the coding problem and (b) formulate the solution in a precise edit format. This dual burden reduces the quality of both tasks.

**Solution: Two-Step Pipeline**

```
User Request
     |
     v
[Architect Model]  ← Strong in reasoning (e.g., o1, Claude Opus)
     |  Describes solution in natural language
     v
[Editor Model]     ← Strong in format conformance (e.g., DeepSeek, Sonnet)
     |  Translates into precise SEARCH/REPLACE blocks
     v
File Changes
```

**Benchmark Results (Aider Code Editing Benchmark):**

| Combination | Edit Format | Score |
|---|---|---|
| o1-preview + o1-mini | whole | 85.0% (SOTA) |
| o1-preview + DeepSeek | whole | 85.0% (SOTA) |
| o1-preview + Claude 3.5 Sonnet | diff | 82.7% |
| Sonnet self-paired | diff | 80.5% (vs. 77.4% solo) |
| GPT-4o self-paired | diff | 75.2% (vs. 71.4% solo) |
| GPT-4o-mini self-paired | diff | 60.2% (vs. 55.6% solo) |

**Key Insight:** DeepSeek is surprisingly effective as an editor — it can precisely translate solution descriptions into file edits without needing to understand the solution itself.

**Auto-Accept:** `--auto-accept-architect` (Default: true) — Architect suggestions are automatically forwarded to the editor without user confirmation.

---

## 3. Git Integration

Aider has the deepest Git integration of all AI coding tools.

### 3.1 Auto-Commits

- **Default:** Every LLM change is automatically committed
- **Commit Messages:** Generated by the "Weak Model", based on diffs and chat history
- **Format:** Conventional Commits standard
- **Custom Prompt:** `--commit-prompt` for custom commit message templates
- **Deactivation:** `--no-auto-commits`

### 3.2 Dirty File Handling

- Before every LLM change: Aider first commits existing uncommitted changes
- Separate commit with descriptive message
- **Deactivation:** `--no-dirty-commits`

### 3.3 Attribution

| Option | Description |
|---|---|
| `--attribute-author` (Default: on) | Appends "(aider)" to Git author name |
| `--attribute-committer` | Appends "(aider)" to committer name |
| `--attribute-commit-message-author` | Prefixes messages with "aider: " for aider-authored changes |
| `--attribute-commit-message-committer` | Prefixes all messages with "aider: " |
| `--attribute-co-authored-by` (Default: on) | Adds Co-authored-by trailer |
| `--no-attribute-author` | Deactivates author attribution |

### 3.4 Undo/Review

- `/undo` — Instantly reverts the last LLM commit
- `/diff` — Shows changes since last message
- `/commit` — Commits dirty files with generated messages
- `/git <cmd>` — Executes arbitrary Git commands

### 3.5 .aiderignore

- Analogous to `.gitignore` — files that Aider should ignore
- Default: `.aiderignore` in the Git root
- Configurable: `--aiderignore <path>`

### 3.6 Subtree Mode

- `--subtree-only` — Restricts Aider to the current subdirectory
- Useful for monorepos or when only a portion should be edited

---

## 4. LLM Support & Model Configuration

### 4.1 Provider Connection

Aider uses **LiteLLM** as a universal abstraction layer:
- 127+ Providers: OpenAI, Anthropic, Google, AWS Bedrock, Azure, Ollama, LM Studio, vLLM, etc.
- OpenAI-compatible API as a unified interface
- Any provider that speaks OpenAI format works automatically

### 4.2 Model Selection Logic

1. Explicit: `--model <model-name>`
2. Automatic: Aider checks available API keys (environment, config, CLI)
3. Fallback: OpenRouter onboarding (Free Tier: `deepseek/deepseek-r1:free`, Paid: `anthropic/claude-sonnet-4`)

### 4.3 Model Configuration

**Three configuration levels:**

#### a) `.aider.model.settings.yml` — Behavioral Configuration
```yaml
- name: anthropic/claude-sonnet-4-20250514
  edit_format: diff
  weak_model_name: anthropic/claude-haiku-3.5
  editor_model_name: anthropic/claude-sonnet-4-20250514
  editor_edit_format: editor-diff
  use_repo_map: true
  use_temperature: true
  streaming: true
  cache_control: true
  examples_as_sys_msg: true
  lazy: false
  overeager: false
  reminder: user
  extra_params: {}
  reasoning_tag: null
  remove_reasoning: false
  accepts_settings:
    - thinking_tokens
    - reasoning_effort
```

Field details:
| Field | Description |
|---|---|
| `name` | Model identifier (with provider prefix) |
| `edit_format` | Which edit format the model uses |
| `weak_model_name` | Cheap model for commits/summarization |
| `editor_model_name` | Editor model for Architect mode |
| `editor_edit_format` | Edit format for the editor |
| `use_repo_map` | Whether Repo Map is sent |
| `use_temperature` | Whether temperature parameter is supported |
| `streaming` | Enable streaming responses |
| `cache_control` | Enable prompt caching (Anthropic/DeepSeek) |
| `lazy` | Deferred processing mode |
| `overeager` | Aggressive response generation |
| `examples_as_sys_msg` | Pack examples into system messages |
| `extra_params` | Arbitrary parameters for `litellm.completion()` |
| `reasoning_tag` | XML tag for reasoning output |
| `remove_reasoning` | Remove reasoning from output |
| `accepts_settings` | Supported extended settings (thinking_tokens, reasoning_effort) |

#### b) `.aider.model.metadata.json` — Technical Metadata
- Context window sizes
- Pricing (input/output tokens)
- Based on LiteLLM's `model_prices_and_context_window.json` (36,000+ lines)
- Can be overridden for unknown models

#### c) `.aider.conf.yml` — General Aider Configuration
- All CLI flags as YAML keys
- Load order: Home dir → Git root → CWD (later overrides earlier)

### 4.4 Benchmark Results (Polyglot Leaderboard, as of 2026)

| Model | Score | Cost | Edit Format |
|---|---|---|---|
| GPT-5 (High) | 88.0% | $29.08 | diff |
| GPT-5 (Medium) | 86.7% | $17.69 | diff |
| o3-Pro (High) | 84.9% | $146.32 | diff |
| Refact.ai Agent + Claude 3.7 Sonnet | 92.9% | n/a | agentic |
| DeepSeek Reasoner | 74.2% | $1.30 | diff |
| DeepSeek-V3.2 (Chat) | 70.2% | $0.88 | diff |

**Benchmark Details:**
- 225 Exercism coding tasks in C++, Go, Java, JavaScript, Python, Rust
- Two attempts per problem (second attempt with test feedback)
- Tests both problem-solving and file-editing capability

### 4.5 Prompt Caching

- **Provider:** Anthropic (Claude Sonnet, Haiku), DeepSeek
- **Activation:** `--cache-prompts`
- **Cache Structure:** System Prompt → Read-Only Files → Repo Map → Editable Files
- **Cache Warming:** `--cache-keepalive-pings N` — Pings every 5 minutes, keeps cache warm for N×5 minutes
- **Cost Savings:** Cached tokens cost ~10x less than uncached
- **Limitation:** Cache statistics only visible when streaming is disabled (`--no-stream`)

### 4.6 Reasoning Support

- `--reasoning-effort VALUE` — Reasoning effort parameter (for o1/o3/Gemini)
- `--thinking-tokens VALUE` — Token budget for thinking/reasoning
- Thinking content is displayed when models return it
- Reasoning tags can be configured via `reasoning_tag` and removed with `remove_reasoning`

---

## 5. Multi-File Editing

### 5.1 File Management

- **Chat files (editable):** `/add <file>` — LLM can modify these files
- **Read-only files:** `/read-only <file>` — Context only, no changes
- **Drop:** `/drop <file>` — Removes files from the chat
- **CLI start:** `aider file1.py file2.py` — Starts with files in the chat

### 5.2 Strategy

- Aider encourages adding **only relevant files**
- Repo Map automatically provides context about the rest of the codebase
- Multi-file edits are coordinated — a single LLM response can contain SEARCH/REPLACE blocks for multiple files
- Git commit encompasses all changed files atomically

### 5.3 Watch Mode (IDE Integration)

- **Activation:** `--watch-files`
- **Principle:** Aider monitors all repo files for AI comments
- **AI comment syntax:**
  - `# AI! description` (Python/Bash) — Triggers code change
  - `// AI! description` (JavaScript) — Triggers code change
  - `-- AI? question` (SQL) — Triggers ask mode
- **Multi-file:** AI comments can be distributed across multiple files
- **Workflow:** Write AI comment in IDE → Aider detects and processes → Changes are applied
- **Context:** AI comments are sent to LLM with Repo Map and chat context
- **Limitation:** Primarily optimized for code, Markdown editing is problematic

---

## 6. Configuration

### 6.1 Configuration Levels (Ascending Priority)

1. **Default values** — Hardcoded in Aider
2. **`~/.aider.conf.yml`** — Home directory (global defaults)
3. **`<git-root>/.aider.conf.yml`** — Project-specific
4. **`<cwd>/.aider.conf.yml`** — Directory-specific
5. **`.env` file** — `AIDER_*` environment variables
6. **Shell environment variables** — `AIDER_*`
7. **CLI flags** — Highest priority

### 6.2 Example `.aider.conf.yml`

```yaml
# Model
model: anthropic/claude-sonnet-4-20250514
weak-model: anthropic/claude-haiku-3.5
editor-model: anthropic/claude-sonnet-4-20250514

# Git
auto-commits: true
dirty-commits: true
attribute-co-authored-by: true

# Editing
edit-format: diff
auto-lint: true
auto-test: false
lint-cmd: "python: ruff check --fix"
test-cmd: "pytest"

# Context
map-tokens: 2048
map-refresh: auto
subtree-only: false

# UI
dark-mode: true
stream: true
pretty: true
```

### 6.3 Environment Variables

Every CLI option has an `AIDER_*` equivalent:
- `AIDER_MODEL` → `--model`
- `AIDER_AUTO_COMMITS` → `--auto-commits`
- `AIDER_OPENAI_API_KEY` → `--openai-api-key`
- `AIDER_ANTHROPIC_API_KEY` → `--anthropic-api-key`
- etc.

---

## 7. API/Library Usage

### 7.1 CLI Scripting (Officially Supported)

```bash
# One-time change
aider --message "add error handling to main.py" main.py

# Batch processing
for f in *.py; do
  aider --message "add type hints" "$f"
done

# Non-interactive
aider --yes --no-auto-commits --message "refactor the function" app.py
```

**Useful Scripting Flags:**
| Flag | Description |
|---|---|
| `--message` / `-m` | Execute instruction and exit |
| `--message-file` / `-f` | Read instruction from file |
| `--yes` | Automatically confirm all prompts |
| `--no-stream` | No streaming (for pipes) |
| `--dry-run` | Preview without changes |
| `--commit` | Commit dirty files and exit |

### 7.2 Python API (Unofficial, Unstable)

```python
from aider.coders import Coder
from aider.models import Model
from aider.io import InputOutput

# Setup
io = InputOutput(yes=True, pretty=False)
model = Model("anthropic/claude-sonnet-4-20250514")
coder = Coder.create(
    main_model=model,
    fnames=["app.py", "utils.py"],
    io=io,
    auto_commits=True
)

# Execute
result = coder.run("implement the missing validate() function")
result = coder.run("add tests")
result = coder.run("/tokens")  # In-chat commands work too
```

**WARNING:** The Python API is **not officially documented** and can change without backward compatibility.

### 7.3 REST API

- **Does NOT exist** — There is no HTTP server mode
- **Feature Request:** GitHub Issue #1190 — Community requests OpenAI-compatible API server
- **Workaround:** Community solution: FastAPI wrapper with asyncio subprocess

### 7.4 Browser GUI

- **Experimental:** `aider --browser` opens web interface
- **Status:** Not feature-complete, experimental
- **Limitations:** Not all terminal features available, less stable

---

## 8. Linting & Testing Integration

### 8.1 Auto-Lint

- **Default:** Enabled (`--auto-lint`)
- **Built-in Linter:** tree-sitter-based for most languages
- **Custom Linter:** `--lint-cmd <cmd>` (must return non-zero exit on errors)
- **Per Language:** `--lint "python: ruff check" --lint "javascript: eslint"`
- **Feedback Loop:** Lint errors are automatically reported back to the LLM, which attempts to fix them

### 8.2 Auto-Test

- **Default:** Disabled (`--auto-test` to enable)
- **Configuration:** `--test-cmd <cmd>` (e.g., `pytest`, `npm test`)
- **Feedback Loop:** Test errors (stdout/stderr) are reported to the LLM
- **Manual:** `/test <cmd>` within the chat
- **Reflection:** Up to 3 automatic correction attempts on errors

### 8.3 Formatter Integration

Formatters that return non-zero exit codes on changes (e.g., `black`, `prettier`) can be used as linters, but require a shell script wrapper that runs twice (1. formatting, 2. check if errors remain).

---

## 9. Multimodal Capabilities

### 9.1 Image Support

- **Vision-capable models:** GPT-4o, Claude Sonnet, Gemini
- **Adding:** `/add screenshot.png`, `/paste` (clipboard), CLI argument
- **Use Cases:** UI screenshots, design mockups, error screenshots, diagrams
- **Limitation:** Image files are added as chat files, consuming context window

### 9.2 Voice Support

- **Backend:** OpenAI Whisper API
- **Configuration:** `--voice-format wav`, `--voice-language en`, `--voice-input-device`
- **Workflow:** Speak → Whisper transcription → Aider processes as text
- **Use Cases:** Hands-free coding instructions, verbally describing feature requests

### 9.3 Web Scraping

- **Command:** `/web <url>` — Scrapes webpage, converts to Markdown, adds to chat
- **Backend:** Playwright (optional) or simple HTTP fetch
- **Use Cases:** Current documentation beyond the model's training cutoff
- **Preview:** `python -m aider.scrape https://example.com`

---

## 10. Strengths

### 10.1 Repository Map — Gold Standard for Codebase Context
- **tree-sitter + PageRank** is the most mature approach for automatic codebase contextualization
- No other tool combines AST parsing with graph ranking and token budget optimization
- Works across languages (100+ languages) without configuration
- Dynamic adaptation to chat context (files in chat receive 50x weight)

### 10.2 Edit Formats — Empirically Optimized
- 7+ edit formats, each optimized for specific model families
- Polyglot Benchmark as an objective comparison metric
- Continuous evaluation of new models against existing formats
- New formats are added when models require them (e.g., `patch` for GPT-4.1)

### 10.3 Architect/Editor — Elegant Reasoning Separation
- Separating problem-solving and code formatting significantly increased benchmark scores
- Enables combination of strong reasoning models with efficient code models
- Self-pairing (same model as Architect+Editor) improves almost every model

### 10.4 Git Integration — Native and Deep
- Automatic commits with semantic messages
- Undo at the push of a button (`/undo`)
- Attribution (Author, Co-authored-by)
- Dirty file handling (commits unsaved changes before LLM edits)
- Every change is traceable in the Git history

### 10.5 Feedback Loop — Lint + Test + Reflection
- Auto-Lint → Auto-Fix → Auto-Test → Auto-Fix → Commit
- Up to 3 reflection cycles on errors
- Closes the loop between code generation and quality assurance

### 10.6 Configurability — Three-Layer System
- Model settings, model metadata, and Aider config as separate files
- Project-specific overrides (`.aider.conf.yml` in the repo)
- Environment variables for CI/CD integration

### 10.7 Prompt Caching — Cost Optimization
- Strategic prompt ordering (stable → variable) for maximum cache hits
- Cache warming via keepalive pings
- ~10x cost reduction for cached tokens

---

## 11. Weaknesses

### 11.1 No Web GUI (Production-Grade)
- Terminal-first design — browser UI only experimental
- No multi-user capability
- No dashboard, no project overview
- No real-time collaboration

### 11.2 Single-User, Single-Session
- No team support, no shared context
- No notification system
- Knowledge transfer only via Git history
- No multi-project management

### 11.3 No REST API
- Not deployable as a service
- No integration into existing toolchains without subprocess hacks
- Python API unofficial and unstable

### 11.4 No Project Management
- No roadmap/feature map
- No task management, no issue integration
- No PM tool sync (Plane, OpenProject, etc.)
- No spec-driven development support

### 11.5 Limited Cost Management
- Per-session token display (`/tokens`)
- No budget limits, no auto-stop on cost overruns
- No historical cost tracking (dashboard)
- No team/project-based budget

### 11.6 No Agent Orchestration
- Only one agent (the user + Aider)
- No multi-agent patterns (supervisor, swarm, etc.)
- No DAG-based workflow
- No pipeline: Plan → Approve → Execute → Review → Deliver

### 11.7 No Sandbox/Container Isolation
- Code is executed directly in the local filesystem
- No Docker-in-Docker for secure agent execution
- No command safety evaluation
- Git as the only rollback mechanism

### 11.8 Uncertain Project Future
- Development pause since August 2025 (version 0.86.1)
- Community discussions about succession plan
- Single-maintainer risk (Paul Gauthier)
- No formal governance structure

### 11.9 Context Window Limitations
- Models lose focus with large context (~25k tokens)
- No GraphRAG or semantic retrieval
- Repo Map is token-budgeted, not semantically optimized
- No experience pool / caching of successful runs

---

## 12. Relevance for CodeForge

### 12.1 What CodeForge Should Adopt

#### A) Repository Map Concept
Aider's tree-sitter + PageRank Repo Map is the gold standard. CodeForge should:
- **tree-sitter parsing** in Python Workers for AST-based code analysis
- **Graph ranking** (but with GraphRAG instead of pure PageRank) for semantically deeper contextualization
- **Token budget optimization** with binary search for context window management
- **Dynamic weighting** based on task context (active files weighted higher)

#### B) Edit Format Architecture
CodeForge must understand edit formats when Aider is used as an agent backend:
- **Model-specific edit formats** in the model configuration (analogous to `.aider.model.settings.yml`)
- **Benchmark-based format selection** instead of guesswork
- **Architect/Editor pattern** as standard workflow for complex tasks

#### C) Feedback Loop Pattern
The Lint → Fix → Test → Fix → Commit pipeline is directly transferable:
- **Quality Layer** in Python Workers: Lint check → LLM fix → Test → LLM fix
- **Configurable reflection cycles** (max_reflections as parameter)
- **Structured error feedback** (lint/test output as context for the next LLM call)

#### D) Git Integration Patterns
- **Auto-commit with attribution** as standard feature
- **Dirty file handling** before agent execution
- **Undo mechanism** via Git history
- **Conventional Commits** as default format

#### E) Prompt Caching Strategy
- **Strategic prompt ordering** (stable → variable) for maximum cache hits
- **Cache warming** for long-running tasks
- **Model-specific cache configuration** (not every provider supports it)

### 12.2 What CodeForge Does BETTER

#### A) Web GUI Instead of Terminal
- Aider: Terminal-only (experimental browser UI)
- **CodeForge:** Full web GUI with SolidJS, real-time updates via WebSocket, dashboard

#### B) Multi-Project Management
- Aider: One repo per session
- **CodeForge:** Project dashboard with multiple repos (Git, GitHub, GitLab, SVN, local)

#### C) Agent Orchestration Instead of Single-Agent
- Aider: One agent (user + Aider)
- **CodeForge:** Multi-agent with DAG orchestration, Plan → Approve → Execute → Review → Deliver

#### D) Aider AS Agent Backend
- CodeForge uses Aider not as a competitor but as an **agent backend**
- Aider via CLI scripting (`--message`) or Python API as worker
- Aider's Git integration, Repo Map, and edit formats as the execution layer
- CodeForge provides orchestration, UI, project management on top

#### E) Roadmap/Spec-Driven Development
- Aider: No project management, no spec support
- **CodeForge:** Bidirectional sync with PM tools, OpenSpec/SpecKit support, auto-detection

#### F) Sandbox Execution
- Aider: No container isolation
- **CodeForge:** Docker-in-Docker, Command Safety Evaluator, tool blocklists

#### G) Cost Management
- Aider: Basic token display
- **CodeForge:** Budget limits per task/project/user, cost dashboard, LiteLLM integration

#### H) Multi-LLM with Scenario Routing
- Aider: One model per session (optional Architect+Editor)
- **CodeForge:** Scenario-based routing (default/background/think/longContext/review/plan) via LiteLLM

### 12.3 Integration Strategy: Aider as Agent Backend

```
CodeForge Go Core
       |
       v  Task Assignment via NATS/Redis
Python AI Worker
       |
       v  Subprocess / Python API
Aider (CLI or Coder class)
       |
       ├── tree-sitter Repo Map       (Context)
       ├── Edit Format (diff/whole)    (Code Editing)
       ├── Git Auto-Commit             (Versioning)
       ├── Auto-Lint + Auto-Test       (Quality)
       └── LLM Call via LiteLLM        (AI)
```

**Integration Paths:**

| Method | Stability | Use Case |
|---|---|---|
| `aider --message "..." <files>` | Stable, official | Simple tasks, batch |
| `Coder.create()` + `coder.run()` | Unofficial, subject to change | Complex workflows, chaining |
| Subprocess with stdin/stdout | Stable, but fragile | Server integration |

**Recommendation:** CLI scripting (`--message`) for robust integration, Python API only when necessary and with version pinning.

### 12.4 Architecture Insights for CodeForge

| Aider Concept | CodeForge Adaptation |
|---|---|
| Repo Map (tree-sitter + PageRank) | GraphRAG Context Layer (deeper, semantic) |
| Edit Formats (7+ variants) | Model-specific format config in worker settings |
| Architect/Editor Pattern | Standard workflow in agent pipeline (Plan → Edit) |
| Auto-Lint + Auto-Test Loop | Quality Layer with configurable reflection cycles |
| Prompt Caching (Anthropic/DeepSeek) | Cache strategy delegated via LiteLLM Proxy |
| `.aider.conf.yml` + `.env` | YAML-based worker config + environment variables |
| Watch Mode (AI comments) | Not relevant (CodeForge has its own UI) |
| Voice/Image | Later phase, not a core feature |
| Weak Model (Commits/Summary) | Scenario routing: background tag for cheap ops |

---

## 13. Summary

### Aider in One Sentence
Terminal-based AI pair programmer with the deepest Git integration and the most mature codebase context system (tree-sitter + PageRank Repo Map) of all open-source tools, but without a web GUI, project management, or agent orchestration.

### Numbers
- 40,000+ GitHub Stars
- 100+ supported languages (tree-sitter)
- 127+ LLM providers (via LiteLLM)
- 7+ edit formats (model-specifically optimized)
- 225 Polyglot Benchmark tasks (6 languages)
- ~70% self-coded (per release)
- Apache 2.0 License

### Core Concepts for CodeForge
1. **Repo Map (tree-sitter + PageRank)** — Gold standard for code context
2. **Edit Format Architecture** — Model-specific, benchmark-based
3. **Architect/Editor Pattern** — Reasoning/editing separation
4. **Lint/Test Feedback Loop** — Auto-fix with reflection cycles
5. **Git-native Workflow** — Auto-commit, attribution, undo
6. **Aider as Agent Backend** — Integrable into workers via CLI or Python API
