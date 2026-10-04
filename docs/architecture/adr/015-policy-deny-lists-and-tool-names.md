# ADR-015: Policy Deny Lists Are Blocklists; Canonical Tool Names

> **Status:** accepted, implemented 2026-09-30 (milestone S1 of the [fix plan](../../known-issues-fix-plan.md#s1---policy-and-security-enforcement), KI-4 to KI-10)
> **Date:** 2026-09-30
> **Deciders:** Project owner (accepted the principles "deny lists win" and "fail closed", decision D6 of the earlier
> fix plan, carried over as D-S1/D-S2)
> **Amends:** [ADR-007](007-policy-layer.md) (first-match-wins stays for allow/ask decisions)

## Context

ADR-007 defines policy profiles as ordered permission rules with optional `path_allow` / `path_deny` /
`command_allow` / `command_deny` lists and a mode fallback, and states that a `path_deny` match means the path is
denied. The Safety Layer lists a **Path Blocklist** and a **Command Safety Evaluator**. The code on `staging`
(2026-09-29) does not deliver that ([Known Issues](../../todo.md#known-issues) KI-4, KI-5, KI-6, KI-10):

- A matching deny list only makes its rule skip; the next rule or the mode default decides, so `.env` edits are
  allowed under `acceptEdits` profiles (KI-5).
- Paths are only `filepath.Clean`ed; absolute paths and workspace escapes are matched as sent (KI-5).
- Command lists are prefix matches on a `bash -c` string, so `go test ./... ; curl x | sh` passes an allow list
  and `/usr/bin/curl` passes a `curl` deny (KI-6).
- The worker sends snake_case tool names (`bash`, `read_file`, ...) with the raw JSON arguments as `command` and no
  path, the Claude Code executor sends `file:read`-style names, while presets and modes use `Bash`, `Read`, ...;
  no rule ever matches, so every decision comes from the mode default (KI-4, KI-10).

## Decision

1. **Canonical tool names.** The Go policy domain owns one mapping from worker and backend tool names to the
   ADR-007 names (`read_file` -> `Read`, `write_file` -> `Write`, `edit_file` -> `Edit`, `bash` -> `Bash`,
   `search_files` -> `Grep`, `glob_files` -> `Glob`, the listing tool, Claude Code names and legacy categories).
   Every evaluation entry point canonicalizes before matching; unknown names (MCP tools, `LLM`, `propose_goal`, ...)
   keep their own name. Workers keep their LLM-facing tool names and send the real `path` and `command` values
   extracted from the tool arguments.
2. **Deny lists are blocklists and win regardless of rule order.** Pass 1: if any rule whose specifier matches the
   call has a matching `path_deny` / `command_deny` entry, the decision is **deny**. Pass 2: the first rule whose
   specifier and allow lists match decides (ADR-007 first-match-wins). Otherwise the mode default decides.
3. **Normalize before matching.** Paths are cleaned and made relative to the project workspace; absolute paths
   outside the workspace and `..` escapes are denied.
4. **Commands are matched per segment.** A command is split on `;`, `&&`, `||`, `|` and newlines (quotes
   respected) and matched on the executable's basename. Allow lists need every segment to match; deny lists deny
   if any segment matches. Constructs that cannot be analysed (`$(...)`, backticks, process substitution,
   `bash -c` / `sh -c` / `eval`) never satisfy an allow list and are denied by any rule with a command deny list.
5. **Fail closed.** A rule with a deny list denies calls that carry no value for it (no path, no command); an
   empty value never matches a non-empty allow list; an unknown profile denies on every path (run and
   conversation).

### Implementation notes (2026-09-30)

- Canonical names: `internal/domain/policy/toolnames.go`; the list-directory tool is `ListDir`. Legacy tool globs
  (`file:*`, `*_file`) in custom rules still match the raw and the canonical name for deny and ask rules (fail
  closed); allow rules match canonical names only.
- Shell parsing (`internal/domain/policy/command.go`) models quotes, escapes, ANSI-C quoting (`$'...'`), comments and
  here-doc bodies. Opaque (fail closed): command/process substitution, backticks, `$[...]` and any `${...}` other than
  a plain `${name}`, assignment prefixes that are not plain literals (see below), `export`/`declare`/`read`, shells, `eval`/`source`/`alias`/`trap`,
  inline interpreter code, `/dev/tcp`/`/dev/udp` redirections, code-running options (`git -c`, `--output`,
  `--ext-diff`, `go test -exec`/`-toolexec`, `go generate`, `sed e`, awk, `find -exec`, make, npm, tar, rsync, ...),
  unknown wrapper options. Safe wrappers (`time`, `timeout`, `nice`, `nohup`, `command`, `env`, `xargs`) are unwrapped, but a wrapper reached through `xargs` is opaque;
  interpreters called only with `--version`/`-V`/`--help` are not inline code.
- `trust_minimum` on an allow rule requires a trust annotation that meets it; deny and ask rules apply to everyone.
- Mode tool lists are enforced through `policy.WithModeTools`; the worker reports the turn's mode as `mode_id`. Since KI-69 the worker also offers the LLM only the tools its mode allows (`ToolRegistry.restrict_to_mode`, same rule, kept equal to Go by a test); the Go check stays the enforcement.
- Request payload: `runs.toolcall.request` carries `tool` (the worker's or backend's own name), `command`, `path`, `mode_id` and, for conversation runs, `turn_id`; these and the trust annotation are what the policy evaluates. `arguments_preview` (truncated JSON of the arguments) is display-only for a human approver and never evaluated. `agui.permission_request` carries the deciding `profile`, which Allow-Always extends.
- Profile resolution (KI-69): a run uses the request's profile, then the project's (`policy_profile`, then config `policy_preset`), then the default; a conversation uses the project's profile, then the preset of its mode's autonomy, then the default.
- Allow-Always adds `{tool, allow}` (Bash: `command_allow` with every executable of the approved command) to a
  per-project clone `{profile}-custom-{projectID}` of the profile that decided the call; the clone replaces its base
  only for that project and never changes the project's profile selection. Profiles persist atomically to the file
  that defines them in `policy.custom_dir` (default `data/policies`).
- Claude Code (KI-72): `claudecode/*` runs (`workers/codeforge/claude_code_executor.py`) load no repository, user or local settings and no MCP servers (`--setting-sources ""`, `--strict-mcp-config`, never `bypassPermissions`); its tool calls reach the policy through a PreToolUse hook and a per-run socket in the worker;
  `Monitor` (background shell command) is an alias of `Bash`; the CLI is offered only tools with a canonical name
  and any other name is denied before evaluation. Worker and Claude Code send paths through one mapping
  (`workers/codeforge/policy_args.py`): relative to the real workspace, absolute when outside it.
- Redirections (KI-69): the shell parser records redirection targets (writes `>`, `>>`, `>|`, `&>`, `&>>`, `>&`,
  `<>`; reads `<`, `<&`), following static `cd`/`pushd` (up to 4 changes); Bash and Claude Code `Monitor` calls check
  write targets against the `path_deny` lists of Write/Edit rules and read targets against Read rules. A target with an
  expansion, glob, `~` or `/dev/tcp|udp` is unknown and denied when such a list exists; targets outside the workspace
  are left to the outside-workspace rule. Paths given as program arguments (`cp`, `tee`, `sed -i`) are covered by
  `command_deny`, not by this check.
- Profiles are tenant-scoped (KI-68): `<custom_dir>/<tenant_id>/`, resolved tenant first, then legacy flat files
  (default tenant only, read-only), then the global read-only presets. Allow-Always clones are snapshots of their base
  profile: later changes of the base do not reach them.
- Git metadata (KI-77): the presets deny Write/Edit on `**/.git/**`, and the worker's file tools refuse `.git`
  path components; the Go Core runs workspace git only through the hardened `internal/git` entry point.
- Workspace real path (S6-E follow-up): Bash redirection targets are checked against the workspace path and against its real path with symlinks resolved (`policy.WithWorkspaceRealPath`), so a symlink retargeted after the run started does not move the check. The policy tester (`POST /policies/{name}/evaluate`) evaluates in a synthetic workspace `/workspace`, so relative paths and redirection targets are placed like in a run.
- Claude Code Bash calls start in the workspace (`CLAUDE_BASH_MAINTAIN_PROJECT_WORKING_DIR=1`; CLIs that do not know the variable are not covered).

- Leading variable assignments (KI-128, 2026-10-04): `NAME=value cmd` and `env [-i] [-u NAME] NAME=value cmd` are
  checked as `cmd`. They stay opaque when a value is not a plain literal (parameter, command or arithmetic expansion,
  ANSI-C quoting, `~` after `=` or `:`, unquoted glob characters), for `NAME+=`, `NAME[i]=` and quoted names, without
  a command (the assignment persists in the shell), after a wrapper (after `time` it is an assignment, after `nice` or
  `timeout` a program name), and for other `env` options (`-S`, `-C`).
  They also stay opaque for every variable that is not on the allow list in `internal/domain/policy/command_env.go`.
  Names are exact and case-sensitive, as in bash. The list:
  - output and runtime switches: `CI`, `DEBUG`, `NO_COLOR`, `FORCE_COLOR`, `TERM`, `COLUMNS`, `LINES`, `TZ`,
    `NODE_ENV`, `RUST_BACKTRACE`, `RUST_LOG`, `PYTHONUNBUFFERED`, `PYTHONDONTWRITEBYTECODE`, `PYTHONHASHSEED`,
    `PYTHONIOENCODING`, `PYTHONUTF8`, `PYTHONFAULTHANDLER`, `GOOS`, `GOARCH`, `CGO_ENABLED`;
  - the locale: `LANG`, `LANGUAGE`, `LC_ALL` and the glibc `LC_*` categories, whose value must not contain `/` (glibc
    takes such a name for a path to locale data);
  - the module search paths `PYTHONPATH` and `NODE_PATH`: accepted so deny lists see the command, but a simple
    command that sets one is never matched by an allow rule that names commands (command allow list, an allow rule's
    sub-pattern, Allow-Always, which refuses it), since `PYTHONPATH=. python3 -m json.tool` runs `./sitecustomize.py`.
    It falls through to later rules and the profile's default (S7-F review, 2026-10-04).

  A deny list of names could not be complete (`CC_<target>`, `GOPACKAGESDRIVER`, `PYTEST_PLUGINS`, `PIP_INDEX_URL`,
  `UV_*`, lower-case proxies, CA overrides, ...). `PYTHONWARNINGS` is not allowed: a category `foo.Bar` imports `foo`.

  Any assignment before `make` or `gmake` keeps the command opaque: GNU make turns every environment variable into a
  make variable, which overrides built-in defaults (`RM`, `CC`) and `?=` assignments.

  A refused assignment before or apart from `cd`, or any assignment after `time`, makes later redirection targets
  unknown. Allow-Always on such a command stores only the executable. The same fix closed a bypass: an assignment after
  `time` (`time GIT_EXTERNAL_DIFF=... git diff`) hid the command from deny lists.
- xargs (KI-128 review, 2026-10-04): xargs appends its input to its command's operands, so a wrapper after it (`env`,
  `nohup`, `nice`, `timeout`, `time`, `command`, `xargs`) would run a command taken from the input; that is opaque
  (`echo x | xargs env` used to parse as `echo x` and `env`). `--eof`, `--replace` and `--max-lines` take a value
  only after `=`, as in GNU xargs (`xargs --replace curl x` runs curl). A replace string (`-I`, `-i`, `--replace`) in
  the command name is opaque (busybox replaces it there). Programs that run a command from their operands (`ionice`,
  `chrt`, `taskset`, `numactl`, `prlimit`, `setpriv`, `runuser`, `nsenter`, `setarch`, `sg`, `pkexec`, `script`,
  `unbuffer`, `chronic`, `ifne`, `systemd-run`, `fakeroot`, `faketime`, `valgrind`, `ltrace`, `firejail`, `bwrap`,
  `xvfb-run`, `dbus-run-session`, `run-parts`, `at`, `batch`, `tmux`, `screen`) are opaque, like `stdbuf`, `setsid`,
  `watch`, `flock` and `parallel`.
- GNU make (S7-F review, 2026-10-04): options are parsed like make 4.4's getopt. Short clusters are walked letter by
  letter: `-E` anywhere is code; `-C`/`-f`/`-I`/`-o`/`-W` take the rest of the cluster or the next word; `-j`/`-l`/`-O`
  take only an attached value. Long options must be exact names (getopt accepts unambiguous prefixes, so `--ev=` is
  `--eval=`); every other long option, `--temp-stdin` and the jobserver internals fail closed. A makefile named `-`,
  `/dev/...` or `/proc/...` is code in every form. An assignment after `--` still counts.
- Package runners (S7-F review, 2026-10-04): `poetry run`, `uv run`, `pipenv run`, `pdm run` and `bundle exec` are
  wrappers; the command after them is the segment, so deny lists see it and allow rules name it (`pytest`, not
  `poetry run pytest`). An option before it, no command, the run subcommand after an option or another subcommand
  (`uv tool run`), a computed word before the subcommand, and xargs into a runner fail closed; other subcommands are
  plain commands. `pipenv run` loads the project's `.env`, so its command is never allow-matched and counts as "with
  assignments" for make. An allow rule that names the inner command trusts the project environment the runner sets up
  (`.venv/bin`, the Gemfile, build hooks), as matching `./pytest` by basename does (KI-140).

## Consequences

### Positive

- Presets behave as documented: permissive presets block network commands and secret files, `plan-readonly`
  can run read-only, rule order cannot open a hole.
- Mode tool lists become enforceable because they use the same canonical names.

### Negative

- **Behavior change for custom YAML profiles** that relied on deny lists meaning "exclude from this rule" or on
  prefix matching of chained commands: such calls are now denied.
- Some previously allowed calls without a path or command are now denied.

### Neutral

- The profile YAML format does not change; only its semantics are specified. The `runs.toolcall.request`
  payload keeps its fields; only their values become meaningful.

## Alternatives Considered

| Alternative | Pros | Cons | Why Not |
|---|---|---|---|
| Keep deny lists as "skip this rule" | Matches the old implementation | Rule order decides whether secrets are protected | Contradicts ADR-007's own text and the Safety Layer |
| Rename presets to snake_case worker names | No mapping needed | Couples policies to one backend's LLM-facing names; Claude Code and MCP differ again | One mapping in the policy domain serves every backend |
| Map names in the Python worker | Go unchanged | Every backend (agent loop, Claude Code, future ones) needs its own mapping; Go stays blind to raw names | Go owns policy (ADR-006, Approach C) |
| Full shell parser for commands | Precise | Large dependency or complex code | Segment splitting plus fail-closed for non-analysable constructs covers the bypasses |

## References

- [ADR-007: Policy Layer](007-policy-layer.md)
- [Fix plan, S1](../../known-issues-fix-plan.md#s1---policy-and-security-enforcement)
- [Known Issues KI-4, KI-5, KI-6, KI-7, KI-10](../../todo.md#known-issues)
