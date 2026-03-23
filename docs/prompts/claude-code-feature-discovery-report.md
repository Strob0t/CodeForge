# Claude Code Feature Discovery Report

**Version:** 2.1.81 | **Date:** 2026-03-23 | **Platform:** Linux (WSL2)

---

## Documented Features (from `claude --help`)

| Feature | Type | How to Access |
|---------|------|---------------|
| Interactive REPL | Core | `claude` |
| Non-interactive print mode | Core | `claude -p "prompt"` |
| Continue conversation | Core | `claude -c` / `claude --continue` |
| Resume session | Core | `claude -r` / `claude --resume [id]` |
| Resume from PR | Core | `claude --from-pr [number/URL]` |
| Model selection | Config | `claude --model <model>` |
| Permission modes | Config | `claude --permission-mode <mode>` (acceptEdits, bypassPermissions, default, dontAsk, plan, auto) |
| Effort level | Config | `claude --effort <level>` (low, medium, high, max) |
| Named sessions | UX | `claude -n "name"` / `claude --name` |
| Worktree isolation | Git | `claude -w` / `claude --worktree [name]` |
| Tmux integration | UX | `claude --tmux` (requires `--worktree`) |
| Bare mode | Performance | `claude --bare` (skip hooks, LSP, plugins, CLAUDE.md) |
| Debug mode | Debug | `claude -d` / `claude --debug [filter]` |
| MCP management | CLI Subcommand | `claude mcp add/remove/list/serve/get` |
| Auth management | CLI Subcommand | `claude auth login/logout/status` |
| Plugin management | CLI Subcommand | `claude plugin install/enable/disable/list/update/uninstall/validate` |
| Plugin marketplace | CLI Subcommand | `claude plugin marketplace add/list/remove/update` |
| Agent listing | CLI Subcommand | `claude agents` |
| Auto-mode inspection | CLI Subcommand | `claude auto-mode config/defaults/critique` |
| Health check | CLI Subcommand | `claude doctor` |
| Update/upgrade | CLI Subcommand | `claude update` / `claude upgrade` |
| Install native | CLI Subcommand | `claude install [target]` |
| Token setup | CLI Subcommand | `claude setup-token` |
| Allowed tools | Config | `--allowedTools "Bash(git:*) Edit"` |
| Disallowed tools | Config | `--disallowedTools "..."` |
| Tools override | Config | `--tools "Bash,Edit,Read"` or `--tools ""` (disable all) |
| System prompt override | Config | `--system-prompt <prompt>` |
| Append system prompt | Config | `--append-system-prompt <prompt>` |
| Add directories | Config | `--add-dir <dirs>` |
| JSON output | Output | `--output-format json/stream-json/text` (with `-p`) |
| JSON schema validation | Output | `--json-schema <schema>` (structured output) |
| Budget cap | Cost | `--max-budget-usd <amount>` (with `-p`) |
| Fallback model | Resilience | `--fallback-model <model>` (with `-p`) |
| Max turns | Config | `--max-turns` (with `-p`) |
| Custom agents | Config | `--agents <json>` |
| Agent override | Config | `--agent <agent>` |
| MCP config files | Config | `--mcp-config <files>` |
| Strict MCP config | Config | `--strict-mcp-config` |
| Fork session | Session | `--fork-session` (new ID on resume) |
| Session ID | Session | `--session-id <uuid>` |
| File download | Input | `--file <file_id:path>` |
| Input format | Input | `--input-format text/stream-json` |
| IDE auto-connect | IDE | `--ide` |
| Chrome integration | IDE | `--chrome` / `--no-chrome` |
| Plugin dir | Config | `--plugin-dir <path>` |
| Settings file | Config | `--settings <file-or-json>` |
| Setting sources | Config | `--setting-sources <sources>` |
| Beta headers | API | `--betas <betas>` |
| Brief mode | Agent | `--brief` (enables SendUserMessage) |
| Skip permissions | Security | `--dangerously-skip-permissions` / `--allow-dangerously-skip-permissions` |
| Disable slash commands | Config | `--disable-slash-commands` |
| No session persistence | Config | `--no-session-persistence` (with `-p`) |
| Verbose mode | Debug | `--verbose` |
| Include partial messages | Stream | `--include-partial-messages` (with stream-json) |
| Replay user messages | Stream | `--replay-user-messages` (with stream-json) |

---

## Undocumented Features

| Feature | Type | How to Access | Discovery Vector | Confidence |
|---------|------|---------------|-----------------|------------|
| Auto-mode critique | CLI | `claude auto-mode critique` — get AI feedback on custom auto-mode rules | V1: Runtime | VERIFIED |
| MCP serve mode | CLI | `claude mcp serve` — exposes Claude Code itself as an MCP server | V1: Runtime | VERIFIED |
| MCP import from Desktop | CLI | `claude mcp add-from-claude-desktop` — import MCP servers from Claude Desktop | V1: Runtime | VERIFIED |
| MCP JSON add | CLI | `claude mcp add-json <name> <json>` — add MCP server via raw JSON | V1: Runtime | VERIFIED |
| MCP reset project | CLI | `claude mcp reset-project-choices` — reset approved/rejected project MCP servers | V1: Runtime | VERIFIED |
| Plugin validate | CLI | `claude plugin validate <path>` — validate plugin or marketplace manifest | V1: Runtime | VERIFIED |
| Plans directory | Filesystem | `~/.claude/plans/` — persistent plan files with whimsical names | V2: Filesystem | VERIFIED |
| Teams system | Filesystem | `~/.claude/teams/` — multi-agent team configs with lead agents and inboxes | V2: Filesystem | VERIFIED |
| File history | Filesystem | `~/.claude/file-history/` — per-session file modification tracking | V2: Filesystem | VERIFIED |
| Shell snapshots | Filesystem | `~/.claude/shell-snapshots/` — captured bash shell state snapshots | V2: Filesystem | VERIFIED |
| Session env | Filesystem | `~/.claude/session-env/` — per-session environment variable stores | V2: Filesystem | VERIFIED |
| Paste cache | Filesystem | `~/.claude/paste-cache/` — clipboard paste content cache | V2: Filesystem | VERIFIED |
| Downloads | Filesystem | `~/.claude/downloads/` — downloaded file resources (via `--file`) | V2: Filesystem | VERIFIED |
| Backups | Filesystem | `~/.claude/backups/` — automatic config file backups | V2: Filesystem | VERIFIED |
| Cache/changelog | Filesystem | `~/.claude/cache/changelog.md` — cached release changelog | V2: Filesystem | VERIFIED |
| IDE lock files | Filesystem | `~/.claude/ide/*.lock` — IDE connection lock files | V2: Filesystem | VERIFIED |
| Security warnings state | Filesystem | `~/.claude/security_warnings_state_*.json` — per-session security warning tracking | V2: Filesystem | VERIFIED |
| Plugin blocklist | Filesystem | `~/.claude/plugins/blocklist.json` — centrally maintained plugin blocklist | V2: Filesystem | VERIFIED |
| Plugin data dirs | Filesystem | `~/.claude/plugins/data/<plugin>/` — per-plugin persistent data storage | V2: Filesystem | VERIFIED |
| MCP auth cache | Filesystem | `~/.claude/mcp-needs-auth-cache.json` — tracks MCP servers needing OAuth | V2: Filesystem | VERIFIED |
| History JSONL | Filesystem | `~/.claude/history.jsonl` — global conversation history index | V2: Filesystem | VERIFIED |
| Project sessions | Filesystem | `~/.claude/projects/<path-hash>/` — per-project session transcripts + subfolders | V2: Filesystem | VERIFIED |
| Custom status line | Config | `settings.json` → `statusLine.command` — receives JSON with model/context/workspace | V3: Config | VERIFIED |
| Hooks: PreToolUse | Config | Hook fires before any tool invocation, receives JSON with tool_input | V3: Config | VERIFIED |
| Hooks: PostToolUse | Config | Hook fires after tool completes, receives JSON with tool_input | V3: Config | VERIFIED |
| Hooks: SessionStart | Config | Hook fires when session starts | V7: Web | VERIFIED |
| Hooks: SessionEnd | Config | Hook fires when session ends (1500ms timeout by default) | V7: Web | VERIFIED |
| Hooks: PreCompact | Config | Hook fires before context compaction | V7: Web | VERIFIED |
| Hooks: Stop | Config | Hook fires when Claude stops responding | V7: Web | VERIFIED |
| Plugin scopes | Config | Plugins can be user/project/local scoped (different enablement per scope) | V3: Config | VERIFIED |
| `$schema` for settings | Config | `"$schema": "https://json.schemastore.org/claude-code-settings.json"` — enables autocomplete | V7: Web | VERIFIED |
| `CLAUDE_CODE_SSE_PORT` | Env | Internal SSE port for IDE communication (auto-set) | V4: CLI | VERIFIED |
| `CLAUDE_CODE_ENTRYPOINT` | Env | Set to `cli` — identifies how Claude Code was launched | V4: CLI | VERIFIED |
| `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` | Env | `=1` enables multi-agent teams with lead/member topology | V3: Config | VERIFIED |
| Spinner verbs customization | Config | `spinnerVerbs.mode` + `spinnerVerbs.verbs` — custom action words | V7: Web | VERIFIED |
| Company announcements | Config | `companyAnnouncements` array — startup messages for teams | V7: Web | VERIFIED |
| Voice dictation | Feature | `/voice` — push-to-talk dictation (requires Claude.ai account) | V7: Web | VERIFIED |
| Vim mode | Feature | `/vim` — toggle vim keybindings in input | V7: Web | VERIFIED |
| Language setting | Config | `language` — set Claude's response language | V7: Web | VERIFIED |
| Effort level persistence | Config | `effortLevel` in settings.json — persists across sessions | V7: Web | VERIFIED |
| Auto-updates channel | Config | `autoUpdatesChannel` — `"stable"` (1 week delay) vs `"latest"` | V7: Web | VERIFIED |
| Reduced motion | Accessibility | `prefersReducedMotion` — disable UI animations | V7: Web | VERIFIED |
| Terminal progress bar | UX | `terminalProgressBarEnabled` — progress bar in Ghostty/iTerm2 | V7: Web | VERIFIED |
| Turn duration display | UX | `showTurnDuration` — "Cooked for 1m 6s" messages | V7: Web | VERIFIED |
| Cleanup period | Config | `cleanupPeriodDays` — auto-delete old sessions (default 30, `0`=disable persistence) | V7: Web | VERIFIED |
| Plan mode fast mode opt-in | Config | `fastModePerSessionOptIn` — fast mode doesn't persist across sessions | V7: Web | VERIFIED |
| Show clear context on plan | Config | `showClearContextOnPlanAccept` — show "clear context" on plan accept | V7: Web | VERIFIED |
| File suggestion override | Config | `fileSuggestion.command` — custom `@` file autocomplete script | V7: Web | VERIFIED |
| Respect gitignore | Config | `respectGitignore` — whether `@` picker respects `.gitignore` | V7: Web | VERIFIED |
| Output style | Config | `outputStyle` — adjust system prompt style | V7: Web | VERIFIED |
| Force login method | Config | `forceLoginMethod` — restrict to `claudeai` or `console` | V7: Web | VERIFIED |
| Force login org | Config | `forceLoginOrgUUID` — auto-select organization during login | V7: Web | VERIFIED |
| Attribution config | Config | `attribution.commits` / `attribution.pr` — customize co-authored-by text | V7: Web | VERIFIED |
| Worktree sparse paths | Config | `worktree.sparsePaths` — sparse checkout for large monorepos | V7: Web | VERIFIED |
| Worktree symlinks | Config | `worktree.symlinkDirectories` — symlink dirs like node_modules | V7: Web | VERIFIED |
| Sandbox config | Config | `sandbox.unsandboxedCommands` — commands that bypass sandbox | V7: Web | VERIFIED |
| claude.ai MCP servers | Feature | Gmail, Google Calendar MCP servers from claude.ai (OAuth) | V5: MCP | VERIFIED |
| Managed settings (MDM) | Enterprise | macOS plist, Windows registry, Linux `/etc/claude-code/` | V7: Web | VERIFIED |
| IS_DEMO mode | Env | `IS_DEMO=true` — hide email/org, skip onboarding (for streaming) | V7: Web | VERIFIED |
| Prompt suggestions | Feature | `CLAUDE_CODE_ENABLE_PROMPT_SUGGESTION` — predictive prompt input | V7: Web | VERIFIED |
| Task list sharing | Feature | `CLAUDE_CODE_TASK_LIST_ID` — share task list across sessions | V7: Web | VERIFIED |
| Interactive /init | Feature | `CLAUDE_CODE_NEW_INIT=true` — interactive setup flow asking which files to generate | V7: Web | VERIFIED |

---

## Environment Variables

### Officially Documented (from code.claude.com/docs/en/env-vars)

| Variable | Effect | Default |
|----------|--------|---------|
| `ANTHROPIC_API_KEY` | API key for direct API access | unset |
| `ANTHROPIC_AUTH_TOKEN` | Custom Authorization header value | unset |
| `ANTHROPIC_BASE_URL` | Override API endpoint (proxy/gateway) | unset |
| `ANTHROPIC_CUSTOM_HEADERS` | Custom request headers (newline-separated) | unset |
| `ANTHROPIC_CUSTOM_MODEL_OPTION` | Custom model in `/model` picker | unset |
| `ANTHROPIC_CUSTOM_MODEL_OPTION_DESCRIPTION` | Display description for custom model | auto |
| `ANTHROPIC_CUSTOM_MODEL_OPTION_NAME` | Display name for custom model | model ID |
| `ANTHROPIC_DEFAULT_HAIKU_MODEL` | Override default Haiku model | built-in |
| `ANTHROPIC_DEFAULT_OPUS_MODEL` | Override default Opus model | built-in |
| `ANTHROPIC_DEFAULT_SONNET_MODEL` | Override default Sonnet model | built-in |
| `ANTHROPIC_FOUNDRY_API_KEY` | Microsoft Foundry API key | unset |
| `ANTHROPIC_FOUNDRY_BASE_URL` | Foundry full base URL | unset |
| `ANTHROPIC_FOUNDRY_RESOURCE` | Foundry resource name | unset |
| `ANTHROPIC_MODEL` | Model selection | built-in |
| `ANTHROPIC_SMALL_FAST_MODEL` | [DEPRECATED] Haiku-class for background | built-in |
| `ANTHROPIC_SMALL_FAST_MODEL_AWS_REGION` | AWS region for small model on Bedrock | unset |
| `AWS_BEARER_TOKEN_BEDROCK` | Bedrock API key auth | unset |
| `BASH_DEFAULT_TIMEOUT_MS` | Default bash command timeout | 120000 |
| `BASH_MAX_OUTPUT_LENGTH` | Max chars before middle-truncation | unset |
| `BASH_MAX_TIMEOUT_MS` | Max timeout model can set | 600000 |
| `CLAUDECODE` | Auto-set to `1` in spawned shells | auto |
| `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` | Context % for auto-compaction (1-100) | 95 |
| `CLAUDE_BASH_MAINTAIN_PROJECT_WORKING_DIR` | Reset cwd after each Bash command | unset |
| `CLAUDE_CODE_ACCOUNT_UUID` | Account UUID for SDK | unset |
| `CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD` | `=1` load CLAUDE.md from `--add-dir` dirs | unset |
| `CLAUDE_CODE_API_KEY_HELPER_TTL_MS` | Credential refresh interval | unset |
| `CLAUDE_CODE_AUTO_COMPACT_WINDOW` | Override context window for compaction calc | model default |
| `CLAUDE_CODE_CLIENT_CERT` | mTLS client certificate path | unset |
| `CLAUDE_CODE_CLIENT_KEY` | mTLS client key path | unset |
| `CLAUDE_CODE_CLIENT_KEY_PASSPHRASE` | mTLS key passphrase | unset |
| `CLAUDE_CODE_DISABLE_1M_CONTEXT` | Disable 1M context | unset |
| `CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING` | Disable adaptive reasoning | unset |
| `CLAUDE_CODE_DISABLE_AUTO_MEMORY` | Disable auto memory | unset |
| `CLAUDE_CODE_DISABLE_BACKGROUND_TASKS` | Disable background tasks + Ctrl+B | unset |
| `CLAUDE_CODE_DISABLE_CRON` | Disable scheduled tasks | unset |
| `CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS` | Strip anthropic-beta headers | unset |
| `CLAUDE_CODE_DISABLE_FAST_MODE` | Disable fast mode toggle | unset |
| `CLAUDE_CODE_DISABLE_FEEDBACK_SURVEY` | Disable session quality surveys | unset |
| `CLAUDE_CODE_DISABLE_GIT_INSTRUCTIONS` | Remove git workflow from system prompt | unset |
| `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` | Disable updates+telemetry+reporting | unset |
| `CLAUDE_CODE_DISABLE_TERMINAL_TITLE` | Disable terminal title updates | unset |
| `CLAUDE_CODE_EFFORT_LEVEL` | Effort level (low/medium/high/max/auto) | auto |
| `CLAUDE_CODE_ENABLE_PROMPT_SUGGESTION` | `=false` disable prompt predictions | true |
| `CLAUDE_CODE_ENABLE_TASKS` | `=true` enable task list in `-p` mode | false (in -p) |
| `CLAUDE_CODE_ENABLE_TELEMETRY` | Enable OpenTelemetry | unset |
| `CLAUDE_CODE_EXIT_AFTER_STOP_DELAY` | Auto-exit delay in ms after idle | unset |
| `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` | `=1` enable agent teams | unset |
| `CLAUDE_CODE_FILE_READ_MAX_OUTPUT_TOKENS` | Override file read token limit | default |
| `CLAUDE_CODE_IDE_SKIP_AUTO_INSTALL` | Skip IDE extension auto-install | false |
| `CLAUDE_CODE_MAX_OUTPUT_TOKENS` | Max output tokens per request | model default |
| `CLAUDE_CODE_NEW_INIT` | `=true` interactive /init flow | false |
| `CLAUDE_CODE_ORGANIZATION_UUID` | Org UUID for SDK | unset |
| `CLAUDE_CODE_OTEL_HEADERS_HELPER_DEBOUNCE_MS` | OTel header refresh interval | 1740000 |
| `CLAUDE_CODE_PLAN_MODE_REQUIRED` | Auto-set on plan-mode teammates | auto |
| `CLAUDE_CODE_PLUGIN_GIT_TIMEOUT_MS` | Plugin git operation timeout | 120000 |
| `CLAUDE_CODE_PLUGIN_SEED_DIR` | Pre-populated plugin dirs (colon-sep) | unset |
| `CLAUDE_CODE_PROXY_RESOLVES_HOSTS` | `=true` let proxy resolve DNS | false |
| `CLAUDE_CODE_SESSIONEND_HOOKS_TIMEOUT_MS` | SessionEnd hook timeout | 1500 |
| `CLAUDE_CODE_SHELL` | Override shell detection | auto |
| `CLAUDE_CODE_SHELL_PREFIX` | Command prefix for all bash cmds | unset |
| `CLAUDE_CODE_SIMPLE` | Minimal system prompt + tools only | unset |
| `CLAUDE_CODE_SKIP_BEDROCK_AUTH` | Skip AWS auth for Bedrock | false |
| `CLAUDE_CODE_SKIP_FAST_MODE_NETWORK_ERRORS` | Allow fast mode if status check fails | false |
| `CLAUDE_CODE_SKIP_FOUNDRY_AUTH` | Skip Azure auth for Foundry | false |
| `CLAUDE_CODE_SKIP_VERTEX_AUTH` | Skip Google auth for Vertex | false |
| `CLAUDE_CODE_SUBAGENT_MODEL` | Model for subagents | parent model |
| `CLAUDE_CODE_TASK_LIST_ID` | Share task list across sessions | auto |
| `CLAUDE_CODE_TEAM_NAME` | Agent team name (auto-set) | auto |
| `CLAUDE_CODE_TMPDIR` | Override temp directory | /tmp |
| `CLAUDE_CODE_USER_EMAIL` | Email for SDK account info | unset |
| `CLAUDE_CODE_USE_BEDROCK` | Use Amazon Bedrock | false |
| `CLAUDE_CODE_USE_FOUNDRY` | Use Microsoft Foundry | false |
| `CLAUDE_CODE_USE_VERTEX` | Use Google Vertex AI | false |
| `CLAUDE_CONFIG_DIR` | Custom config directory | ~/.claude |
| `CLAUDE_ENV_FILE` | Shell script sourced before Bash cmds | unset |
| `DISABLE_AUTOUPDATER` | Disable auto-updates | false |
| `DISABLE_COST_WARNINGS` | Disable cost warnings | false |
| `DISABLE_ERROR_REPORTING` | Disable Sentry reporting | false |
| `DISABLE_FEEDBACK_COMMAND` | Disable /feedback | false |
| `DISABLE_INSTALLATION_CHECKS` | Disable install warnings | false |
| `DISABLE_PROMPT_CACHING` | Disable prompt caching (all models) | false |
| `DISABLE_PROMPT_CACHING_HAIKU/OPUS/SONNET` | Disable per-model prompt caching | false |
| `DISABLE_TELEMETRY` | Disable Statsig telemetry | false |
| `ENABLE_CLAUDEAI_MCP_SERVERS` | `=false` disable claude.ai MCP servers | true |
| `ENABLE_TOOL_SEARCH` | MCP tool search (true/auto/auto:N/false) | auto |
| `FORCE_AUTOUPDATE_PLUGINS` | Force plugin updates when updater disabled | false |
| `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY` | Network proxy settings | unset |
| `IS_DEMO` | `=true` demo mode (hide email, skip onboard) | false |
| `MAX_MCP_OUTPUT_TOKENS` | Max tokens in MCP tool responses | 25000 |
| `MAX_THINKING_TOKENS` | Override thinking token budget | model default |
| `MCP_CLIENT_SECRET` | OAuth client secret for MCP servers | unset |
| `MCP_OAUTH_CALLBACK_PORT` | Fixed OAuth redirect callback port | auto |
| `MCP_TIMEOUT` | MCP server startup timeout | default |
| `MCP_TOOL_TIMEOUT` | MCP tool execution timeout | default |
| `SLASH_COMMAND_TOOL_CHAR_BUDGET` | Skill metadata character budget | 2% of context |
| `USE_BUILTIN_RIPGREP` | `=0` use system rg instead of bundled | 1 |

### Undocumented / Community-Discovered (from TurboAI.dev, 183 total tracked)

| Variable | Effect | Source |
|----------|--------|--------|
| `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB` | Scrub env vars from subprocesses | TurboAI |
| `CLAUDE_CODE_EXTRA_METADATA` | Extra metadata for API requests | TurboAI |
| `CLAUDE_CODE_EXTRA_BODY` | Extra body params for API requests | TurboAI |
| `CLAUDE_CODE_BASE_REF` | Base git ref for diffs | TurboAI |
| `CLAUDE_CODE_TAGS` | Session tags | TurboAI |
| `CLAUDE_CODE_CONTAINER_ID` | Container identification | TurboAI |
| `CLAUDE_CODE_ATTRIBUTION_HEADER` | Custom attribution header | TurboAI |
| `CLAUDE_CODE_MAX_RETRIES` | Max API retries | TurboAI |
| `CLAUDE_CODE_BLOCKING_LIMIT_OVERRIDE` | Override blocking limit | TurboAI |
| `CLAUDE_CODE_FORCE_FULL_LOGO` | Force full logo display | TurboAI |
| `CLAUDE_CODE_STREAMING_TEXT` | Streaming text config | TurboAI |
| `CLAUDE_CODE_SYNTAX_HIGHLIGHT` | Syntax highlighting config | TurboAI |
| `CLAUDE_CODE_ACCESSIBILITY` | Accessibility mode | TurboAI |
| `CLAUDE_CODE_RESUME_INTERRUPTED_TURN` | Resume interrupted turns | TurboAI |
| `CLAUDE_CODE_HOST_PLATFORM` | Host platform identifier | TurboAI |
| `CLAUDE_CODE_ACTION` | Action identifier | TurboAI |
| `CLAUDE_CODE_ADDITIONAL_PROTECTION` | Extra safety protections | TurboAI |
| `CLAUDE_CODE_SAVE_HOOK_ADDITIONAL_CONTEXT` | Save hook context | TurboAI |
| `CLAUDE_CODE_EAGER_FLUSH` | Eager output flushing | TurboAI |
| `CLAUDE_CODE_WORKER_EPOCH` | Worker epoch tracking | TurboAI |
| `CLAUDE_CODE_BUBBLEWRAP` | Bubblewrap sandbox config | TurboAI |
| `CLAUDE_REPL_MODE` | REPL mode config | TurboAI |
| `CLAUDE_FORCE_DISPLAY_SURVEY` | Force survey display | TurboAI |
| `ENABLE_LSP_TOOL` | Enable LSP-based tools | TurboAI |
| `ENABLE_SESSION_BACKGROUNDING` | Session backgrounding | TurboAI |
| `ENABLE_BTW` | "By the way" suggestions | TurboAI |
| `ENABLE_MCP_LARGE_OUTPUT_FILES` | Large MCP output file support | TurboAI |
| `ENABLE_CLAUDE_CODE_SM_COMPACT` | Small-model compaction | TurboAI |
| `CLAUDE_CODE_ENABLE_CFC` | Context-free compaction | TurboAI |
| `CLAUDE_CODE_ENABLE_SDK_FILE_CHECKPOINTING` | SDK file checkpointing | TurboAI |
| `CLAUDE_CODE_ENABLE_TOKEN_USAGE_ATTACHMENT` | Token usage attachments | TurboAI |
| `CLAUDE_ENABLE_STREAM_WATCHDOG` | Stream watchdog | TurboAI |
| `CLAUDE_CODE_EMIT_TOOL_USE_SUMMARIES` | Tool use summaries | TurboAI |
| `CLAUDE_CODE_DISABLE_ATTACHMENTS` | Disable attachments | TurboAI |
| `CLAUDE_CODE_DISABLE_CLAUDE_MDS` | Disable CLAUDE.md loading | TurboAI |
| `CLAUDE_CODE_DISABLE_COMMAND_INJECTION_CHECK` | Disable cmd injection check | TurboAI |
| `CLAUDE_CODE_DISABLE_FILE_CHECKPOINTING` | Disable file checkpointing | TurboAI |
| `CLAUDE_CODE_DISABLE_OFFICIAL_MARKETPLACE_AUTOINSTALL` | Disable auto-install of official plugins | TurboAI |
| `CLAUDE_CODE_DISABLE_THINKING` | Completely disable thinking | TurboAI |
| `CLAUDE_CODE_DONT_INHERIT_ENV` | Don't inherit parent env | TurboAI |
| `CLAUDE_CODE_SKIP_PROMPT_HISTORY` | Skip prompt history | TurboAI |
| `DISABLE_INTERLEAVED_THINKING` | Disable interleaved thinking | TurboAI |
| `DISABLE_MICROCOMPACT` | Disable micro-compaction | TurboAI |
| `DISABLE_COMPACT` | Disable compaction entirely | TurboAI |
| `DISABLE_AUTO_COMPACT` | Disable auto-compaction | TurboAI |
| `CLAUDE_CODE_MAX_TOOL_USE_CONCURRENCY` | Max parallel tool calls | TurboAI |
| `CLAUDE_CODE_GLOB_TIMEOUT_SECONDS` | Glob operation timeout | TurboAI |
| `CLAUDE_CODE_GLOB_HIDDEN` | Include hidden files in glob | TurboAI |
| `CLAUDE_CODE_GLOB_NO_IGNORE` | Glob ignores .gitignore | TurboAI |
| `TASK_MAX_OUTPUT_LENGTH` | Max task output length | TurboAI |
| `MCP_SERVER_CONNECTION_BATCH_SIZE` | MCP server connection batch | TurboAI |
| `MCP_REMOTE_SERVER_CONNECTION_BATCH_SIZE` | Remote MCP batch size | TurboAI |
| `MCP_CONNECTION_NONBLOCKING` | Non-blocking MCP connections | TurboAI |
| `CLAUDE_CODE_PLUGIN_CACHE_DIR` | Plugin cache directory | TurboAI |
| `CLAUDE_CODE_PLUGIN_USE_ZIP_CACHE` | Use ZIP cache for plugins | TurboAI |
| `CLAUDE_CODE_SYNC_PLUGIN_INSTALL` | Synchronous plugin install | TurboAI |
| `CLAUDE_CODE_USE_COWORK_PLUGINS` | Cowork plugin support | TurboAI |
| `CLAUDE_CODE_PLAN_V2_AGENT_COUNT` | Plan V2 agent count | TurboAI |
| `CLAUDE_CODE_PLAN_V2_EXPLORE_AGENT_COUNT` | Plan V2 explore agents | TurboAI |
| `CLAUDE_CODE_IS_COWORK` | Cowork mode flag | TurboAI |
| `CLAUDE_AUTO_BACKGROUND_TASKS` | Auto background task management | TurboAI |
| `CLAUDE_CODE_REMOTE` | Remote mode | TurboAI |
| `CLAUDE_CODE_REMOTE_SESSION_ID` | Remote session ID | TurboAI |
| `CLAUDE_CODE_REMOTE_MEMORY_DIR` | Remote memory directory | TurboAI |
| `CLAUDE_CODE_USE_CCR_V2` | Use CCR V2 protocol | TurboAI |
| `CLAUDE_CODE_PROFILE_STARTUP` | Profile startup performance | TurboAI |
| `CLAUDE_CODE_PROFILE_QUERY` | Profile query performance | TurboAI |
| `CLAUDE_CODE_PERFETTO_TRACE` | Perfetto tracing | TurboAI |
| `CLAUDE_CODE_SLOW_OPERATION_THRESHOLD_MS` | Slow operation logging threshold | TurboAI |
| `CLAUDE_CODE_FRAME_TIMING_LOG` | Frame timing logs | TurboAI |
| `CLAUDE_CODE_DIAGNOSTICS_FILE` | Diagnostics output file | TurboAI |
| `DISABLE_DOCTOR_COMMAND` | Disable /doctor | TurboAI |
| `DISABLE_EXTRA_USAGE_COMMAND` | Disable extra usage command | TurboAI |
| `DISABLE_INSTALL_GITHUB_APP_COMMAND` | Disable GitHub app install cmd | TurboAI |
| `DISABLE_LOGIN_COMMAND` | Disable login command | TurboAI |
| `DISABLE_LOGOUT_COMMAND` | Disable logout command | TurboAI |
| `DISABLE_UPGRADE_COMMAND` | Disable upgrade command | TurboAI |
| `CLAUDE_CHROME_PERMISSION_MODE` | Chrome extension permission mode | TurboAI |
| `CLAUDE_CODE_BASH_SANDBOX_SHOW_INDICATOR` | Show sandbox indicator for bash | TurboAI |
| `FORCE_CODE_TERMINAL` | Force code terminal mode | TurboAI |
| `IS_SANDBOX` | Sandbox mode flag | TurboAI |
| `CLAUDE_CODE_ENABLE_FINE_GRAINED_TOOL_STREAMING` | Fine-grained tool streaming | TurboAI |
| `USE_API_CONTEXT_MANAGEMENT` | API-level context management | TurboAI |

*Note: TurboAI.dev tracks 183 env vars total (as of v2.1.81). Only the most notable are listed above. Full list at [turboai.dev/blog/claude-code-environment-variables-complete-list](https://www.turboai.dev/blog/claude-code-environment-variables-complete-list)*

---

## Keyboard Shortcuts

| Shortcut | Action | Documented? |
|----------|--------|-------------|
| `Ctrl+C` | Cancel current generation / clear input | Yes |
| `Ctrl+D` | Exit Claude Code | No |
| `Ctrl+T` | Toggle task list display | No |
| `Ctrl+B` | Background current task | No |
| `Ctrl+R` | Reverse history search | No (shell standard) |
| `Ctrl+L` | Clear screen | No (shell standard) |
| `Tab` | Autocomplete file paths / commands | Partial |
| `Shift+Tab` | Cycle autocomplete backwards | No |
| `Esc` | Cancel autocomplete / dismiss | No |
| `!` prefix | Shell passthrough (run command directly) | Partial |
| `@` prefix | File reference autocomplete | Partial |
| `#` prefix | Reference autocomplete | Partial |
| `//` prefix | Reference autocomplete | Partial |

---

## Configuration Keys (settings.json)

| Key | File | Purpose | Documented? |
|-----|------|---------|-------------|
| `$schema` | Any settings.json | JSON Schema for autocomplete | Yes |
| `permissions.allow` | Any | Allowed tool rules | Yes |
| `permissions.deny` | Any | Denied tool rules | Yes |
| `permissions.ask` | Any | Always-prompt rules | Yes |
| `permissions.defaultMode` | Any | Default permission mode | Yes |
| `permissions.additionalDirectories` | Any | Extra working directories | Yes |
| `permissions.disableBypassPermissionsMode` | Managed | Prevent bypass mode | Yes |
| `env` | Any | Environment variables per session | Yes |
| `hooks` | Any | Lifecycle hook commands | Yes |
| `disableAllHooks` | Any | Disable all hooks + statusLine | Yes |
| `allowManagedHooksOnly` | Managed | Only managed hooks | Yes |
| `allowedHttpHookUrls` | Any | URL allowlist for HTTP hooks | Yes |
| `httpHookAllowedEnvVars` | Any | Env vars HTTP hooks can use | Yes |
| `statusLine` | Any | Custom status line config | Yes |
| `mcpServers` | Any | MCP server definitions | Yes |
| `enableAllProjectMcpServers` | Local | Auto-approve project MCP servers | Yes |
| `enabledMcpjsonServers` | Local | Approved MCP servers from .mcp.json | Yes |
| `disabledMcpjsonServers` | Local | Rejected MCP servers from .mcp.json | Yes |
| `allowManagedMcpServersOnly` | Managed | Only managed MCP servers | Yes |
| `allowedMcpServers` | Managed | MCP server allowlist | Yes |
| `deniedMcpServers` | Managed | MCP server denylist | Yes |
| `enabledPlugins` | Any | Plugin enable/disable map | Partial |
| `model` | Any | Default model override | Yes |
| `availableModels` | Any | Restrict model selection | Yes |
| `modelOverrides` | Any | Map model IDs to provider IDs | Yes |
| `effortLevel` | Any | Persisted effort level | Yes |
| `apiKeyHelper` | User | Script to generate API key | Yes |
| `autoMemoryDirectory` | User/Local | Custom auto-memory directory | Yes |
| `cleanupPeriodDays` | Any | Session cleanup period | Yes |
| `companyAnnouncements` | Managed | Startup announcements | Yes |
| `attribution` | Any | Commit/PR attribution config | Yes |
| `includeCoAuthoredBy` | Any | [DEPRECATED] Co-authored-by | Yes |
| `includeGitInstructions` | Any | Git instructions in system prompt | Yes |
| `outputStyle` | Any | Output style adjustment | Yes |
| `agent` | Any | Run main thread as named subagent | Yes |
| `forceLoginMethod` | Any | Restrict login to claudeai/console | Yes |
| `forceLoginOrgUUID` | Any | Auto-select org at login | Yes |
| `channelsEnabled` | Managed | Enable channels for Team/Enterprise | Yes |
| `strictKnownMarketplaces` | Managed | Plugin marketplace allowlist | Yes |
| `blockedMarketplaces` | Managed | Plugin marketplace blocklist | Yes |
| `pluginTrustMessage` | Managed | Custom plugin trust warning | Yes |
| `awsAuthRefresh` | Any | AWS auth refresh script | Yes |
| `awsCredentialExport` | Any | AWS credential export script | Yes |
| `alwaysThinkingEnabled` | Any | Always-on extended thinking | Yes |
| `plansDirectory` | Any | Custom plan storage path | Yes |
| `showClearContextOnPlanAccept` | Any | Show clear context option | Yes |
| `spinnerVerbs` | Any | Custom spinner action words | Yes |
| `spinnerTipsEnabled` | Any | Show/hide spinner tips | Yes |
| `spinnerTipsOverride` | Any | Custom spinner tips | Yes |
| `language` | Any | Claude's response language | Yes |
| `voiceEnabled` | Any | Push-to-talk dictation | Yes |
| `autoUpdatesChannel` | Any | stable/latest release channel | Yes |
| `prefersReducedMotion` | Any | Reduce UI animations | Yes |
| `fastModePerSessionOptIn` | Any | Fast mode per-session only | Yes |
| `teammateMode` | Any | Team display mode (auto/in-process/tmux) | Yes |
| `feedbackSurveyRate` | Any | Survey probability 0-1 | Yes |
| `fileSuggestion` | Any | Custom @ file picker | Yes |
| `respectGitignore` | Any | @ picker respects .gitignore | Yes |
| `otelHeadersHelper` | Any | Dynamic OTel header script | Yes |
| `worktree.symlinkDirectories` | Any | Worktree symlink dirs | Yes |
| `worktree.sparsePaths` | Any | Worktree sparse checkout | Yes |
| `sandbox.unsandboxedCommands` | Any | Commands bypassing sandbox | Yes |
| `sandbox.enableWeakerNestedSandbox` | Any | Weaker sandbox for docker | Yes |
| `allowManagedPermissionRulesOnly` | Managed | Only managed permission rules | Yes |
| `skipDangerousModePermissionPrompt` | User | Skip bypass mode confirmation | No |

---

## Hook Events

| Event | When Fired | Use Case |
|-------|-----------|----------|
| `PreToolUse` | Before any tool invocation | Block dangerous commands, audit logging, custom validation |
| `PostToolUse` | After tool completion | Auto-format, lint, trigger notifications |
| `SessionStart` | When a new session begins | Environment setup, activate virtualenvs, set env vars |
| `SessionEnd` | When session ends (1500ms timeout) | Cleanup, save state, send notifications |
| `PreCompact` | Before context compaction | Save important context, log compaction events |
| `Stop` | When Claude stops responding | Post-completion actions, notifications |
| `Notification` | When Claude sends a notification | Custom notification routing |

Hook input format: JSON on stdin with `tool_input`, `tool_name`, `session_id` etc. Exit codes: `0`=proceed, `2`=block (PreToolUse).

---

## MCP Capabilities

| Server | Tools | Resources | Status |
|--------|-------|-----------|--------|
| plugin:context7:context7 | `resolve-library-id`, `query-docs` | None | Connected |
| playwright-mcp | 25+ browser tools (`navigate`, `click`, `fill_form`, `screenshot`, etc.) | None | Connected |
| claude.ai Gmail | Gmail integration tools | None | Needs Auth |
| claude.ai Google Calendar | Calendar integration tools | None | Needs Auth |
| Claude Code itself (via `mcp serve`) | `list_projects`, `get_project`, `get_run_status`, `get_cost_summary` | `codeforge://projects`, `codeforge://costs/summary` | Available |

---

## Skills (from plugins + custom commands)

### Superpowers Plugin (v5.0.5) — 14 skills

| Skill | Trigger | User-Invocable? | Auto-Trigger? |
|-------|---------|-----------------|---------------|
| `using-superpowers` | Session start | Yes (`/superpowers:using-superpowers`) | Yes (SessionStart) |
| `brainstorming` | Creative work, new features | Yes | Yes |
| `test-driven-development` | Implementing features/bugfixes | Yes | Yes |
| `systematic-debugging` | Bug/test failure/unexpected behavior | Yes | Yes |
| `writing-plans` | Multi-step task with spec | Yes | Yes |
| `executing-plans` | Written implementation plan | Yes | Yes |
| `subagent-driven-development` | Executing plans with independent tasks | Yes | Yes |
| `dispatching-parallel-agents` | 2+ independent tasks | Yes | Yes |
| `verification-before-completion` | Before claiming work complete | Yes | Yes |
| `using-git-worktrees` | Feature isolation needed | Yes | Yes |
| `writing-skills` | Creating/editing skills | Yes | Yes |
| `requesting-code-review` | Completing tasks/features | Yes | Yes |
| `receiving-code-review` | Processing review feedback | Yes | Yes |
| `finishing-a-development-branch` | Implementation complete, deciding merge/PR | Yes | Yes |

### Other Plugin Skills

| Skill | Plugin | User-Invocable? | Auto-Trigger? |
|-------|--------|-----------------|---------------|
| `code-review:code-review` | code-review | Yes | No |
| `feature-dev:feature-dev` | feature-dev | Yes | No |
| `frontend-design:frontend-design` | frontend-design | Yes | No |
| `ralph-loop:ralph-loop` | ralph-loop | Yes | No |
| `ralph-loop:help` | ralph-loop | Yes | No |
| `ralph-loop:cancel-ralph` | ralph-loop | Yes | No |
| `pr-review-toolkit:review-pr` | pr-review-toolkit | Yes | No |
| `skill-creator:skill-creator` | skill-creator | Yes | No |
| `code-simplifier:code-simplifier` | code-simplifier | Yes | No |

### Custom Commands (project-level)

| Skill | Path | User-Invocable? |
|-------|------|-----------------|
| `/agent-eval` | `.claude/commands/agent-eval.md` | Yes |
| `/benchmark-e2e` | `.claude/commands/benchmark-e2e.md` | Yes |
| `/commit` | `.claude/commands/commit.md` | Yes |
| `/db-audit` | `.claude/commands/db-audit.md` | Yes |
| `/prompt-master-main` | `.claude/commands/prompt-master-main/` | Yes |

### Plugin Agents (subagent types)

| Agent | Plugin | Purpose |
|-------|--------|---------|
| `claude-code-guide` | built-in | Answer questions about Claude Code |
| `superpowers:code-reviewer` | superpowers | Review code against plan |
| `feature-dev:code-explorer` | feature-dev | Trace codebase execution paths |
| `feature-dev:code-architect` | feature-dev | Design feature architectures |
| `feature-dev:code-reviewer` | feature-dev | Confidence-based code review |
| `pr-review-toolkit:comment-analyzer` | pr-review-toolkit | Analyze code comments |
| `pr-review-toolkit:silent-failure-hunter` | pr-review-toolkit | Find silent failures |
| `pr-review-toolkit:pr-test-analyzer` | pr-review-toolkit | Review test coverage |
| `pr-review-toolkit:code-reviewer` | pr-review-toolkit | Style/guidelines review |
| `pr-review-toolkit:code-simplifier` | pr-review-toolkit | Simplify code |
| `pr-review-toolkit:type-design-analyzer` | pr-review-toolkit | Review type design |
| `code-simplifier:code-simplifier` | code-simplifier | Simplify code |

---

## Discovery Gaps

Features mentioned in web sources that could NOT be verified locally:

| Feature | Claimed Source | Reason Unverified |
|---------|---------------|-------------------|
| `~/.claude/output-modes/` directory | petegypps.uk | Directory does NOT exist on this installation |
| `@agent-output-mode-setup` | petegypps.uk | Could not verify — may require specific interaction |
| `!state`, `!memory`, `!tokens` commands | petegypps.uk | UNCONFIRMED — source may contain speculative content |
| `claude-esp` TUI tool | paddo.dev / community | External tool, not built into Claude Code |
| `ccexp` CLI tool | community | External tool, not built into Claude Code |
| `ENABLE_EXPERIMENTAL_MCP_CLI` | paddo.dev (older post) | May have been integrated into mainline or renamed |
| 41 Statsig feature gates | TurboAI.dev | Gate names not publicly enumerated |
| `/output-style` slash command | petegypps.uk | Could be renamed to `/config` output style tab |

---

## Auto-Mode Classifier (Undocumented Depth)

The auto-mode permission system contains an extensive rule set not visible in `/help`:

- **7 ALLOW rules**: Test artifacts, local operations, read-only, declared dependencies, toolchain bootstrap, standard credentials, git push to working branch
- **27 SOFT_DENY rules**: Git destructive, code from external, production deploy, credential leakage/exploration, data exfiltration, self-modification, content impersonation, real-world transactions, and more
- **Environment scoping**: Trusted repo, source control orgs, internal domains, cloud buckets, key services

Access via: `claude auto-mode defaults` (full JSON) or `claude auto-mode critique` (AI feedback on custom rules).

---

## Filesystem Architecture Summary

```
~/.claude/
  settings.json              # User-scope settings
  .credentials.json          # OAuth/API credentials
  history.jsonl              # Global conversation index
  statusline-command.sh      # Custom status line script
  mcp-needs-auth-cache.json  # MCP OAuth state
  security_warnings_state_*.json  # Per-session security state
  backups/                   # Config file backups
  cache/changelog.md         # Cached release notes
  downloads/                 # File resources from --file
  file-history/              # Per-session file modification tracking
  ide/                       # IDE connection locks
  paste-cache/               # Clipboard paste cache
  plans/                     # Persistent plan files (whimsical names)
  plugins/
    blocklist.json           # Centrally-maintained blocklist
    installed_plugins.json   # Plugin registry
    known_marketplaces.json  # Marketplace sources
    cache/                   # Plugin code cache
    data/                    # Per-plugin persistent storage
    marketplaces/            # Marketplace metadata
  projects/<path-hash>/      # Per-project session data
    <session-id>.jsonl       # Session transcript
    <session-id>/            # Session working files
  session-env/               # Per-session environment stores
  sessions/                  # Session metadata JSON files
  shell-snapshots/           # Shell state snapshots
  tasks/                     # Persistent task list data
  teams/                     # Agent team configs
    <team-name>/
      config.json            # Team topology
      inboxes/               # Agent message queues

.claude/                     # Project-scope
  settings.json              # Shared project settings
  settings.local.json        # Local-only settings (gitignored)
  commands/                  # Custom slash commands (.md files)
  hooks/                     # Hook scripts
  memory/                    # Auto-memory files (MEMORY.md)
  agents/                    # Custom subagent definitions
```

---

## Sources

- [Official Settings Docs](https://code.claude.com/docs/en/settings)
- [Official Env Vars Docs](https://code.claude.com/docs/en/env-vars)
- [TurboAI.dev Version Tracker](https://www.turboai.dev/blog/claude-code-versions)
- [TurboAI.dev Complete Env Var List](https://www.turboai.dev/blog/claude-code-environment-variables-complete-list)
- [Claude Code Hidden Commands (Pete Gypps)](https://www.petegypps.uk/blog/claude-code-hidden-commands-complete-guide-secret-features)
- [Claude Code Has 50+ Commands (Towards AI)](https://pub.towardsai.net/claude-code-has-50-commands-most-developers-use-only-5-b675387ea2ce)
- [Claude Code Hidden MCP Flag (paddo.dev)](https://paddo.dev/blog/claude-code-hidden-mcp-flag/)
- [Awesome Claude Code (GitHub)](https://github.com/hesreallyhim/awesome-claude-code)
- [JSON Schema for settings.json](https://json.schemastore.org/claude-code-settings.json)
- [GitHub Gist: CLI Environment Variables](https://gist.github.com/unkn0wncode/f87295d055dd0f0e8082358a0b5cc467)
