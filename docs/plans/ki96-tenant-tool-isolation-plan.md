# KI-96 Per-Tenant Tool Isolation (S7-H) - Plan

- **Status:** implemented (2026-10-03, branch commits 3809b201..0c699d9f, one review round); the departures from this plan and the review fixes are in [Implementation notes](#implementation-notes). Plan revision 2 (2026-10-02). Revision 1 had a security review and an operability review. The section "Changes after the review" maps each finding to its fix. This plan implements the owner's S7-H decision for [KI-96](../todo.md#known-issues):
  - Stage 1: a tool UID per tenant, with POSIX ACLs on the tenant directories.
  - Stage 2: Landlock on every tool call. It is mandatory in production.
  - Stage 3 (network): not in this round. It is recorded as the new follow-up **KI-110**.
- **Hard constraint:** CodeForge acts only inside its own containers. It gets no Docker socket, no host runtime and no extra host privileges. This plan also adds **no container capability**: the worker keeps `SETUID`, `SETGID` and `KILL` (ADR-017).
- **Relates to:**
  - [ADR-017](../architecture/adr/017-tool-isolation-and-nats-authentication.md) and KI-71: the tool user, the launcher, the sharing pass, and the NATS permissions that let only the Core publish run starts.
  - [ADR-016](../architecture/adr/016-nats-delivery-semantics.md): delivery of the new deletion subjects.
  - KI-95 (`workspace_fs`, `os.Root`), KI-77 (the Core's hardened git), KI-13 (sandbox execution modes), KI-83 (LSP servers in the Core).
  - The KI-25 and KI-88 plans: they also claimed ADR-018 and now take ADR-019 and ADR-020 (Open question 1, resolved).
- **New ADR:** [ADR-018 "Per-tenant tool identities and Landlock"](../architecture/adr/018-per-tenant-tool-identities-and-landlock.md).
- **Evidence:**
  - E0-E14: lab experiments of 2026-10-02.
  - E15-E21: the reviewers' experiments of the same day.
  - Environment: kernel 6.18.44, Docker 29.3.1, `python:3.12-slim` (Debian 13.7, util-linux 2.41.5).
  - Line numbers refer to HEAD `81020dc2`.
  - New mechanisms in this revision are not verified in Docker yet: the fd-based worker steps, `prepare`, the ACL-aware sharing pass, the `tool_homes` volume, the `/dev/pts` rules and the pidfd reaper. The Docker suite (Tests) covers them.

---

## Problem

Since KI-71, every tool process of every run and tenant runs as uid 10002. Its only supplementary group is the shared workspace group 10010. The launcher is `workers/codeforge/tool_process.py:167-184`, and the probe's group check is at `:347-369`. Each tenant directory is `10001:10010` with mode 2770. Because the tool user is in 10010, any agent can:

1. **Reach other tenants' files.** It can read, write, rename and delete every tenant's workspace. That allows code theft and code injection, for example planting `.git/config` or hooks in another tenant's repository (`project_workspace.go:47, :205`, `workspace_perm.go`).
2. **Reach other tenants' processes.** All tool processes share one UID. An agent can signal and ptrace them, and read `/proc/<pid>/environ` of another tenant's MCP stdio servers (their declared tokens) and Claude Code runs (API key, policy token). The test host has no Yama, so nothing limits same-UID ptrace (E0).
3. **Use shared state.**
   - One HOME (`/home/codeforge-tool`, compose `:338`) and one `/tmp` (1777) serve all tenants, and umask 002 makes tool files 0664.
   - Cache and config locations from the worker environment (`XDG_*`, `GOCACHE`, `npm_config_*`, `subprocess_env.py:32-82`) are shared as well.
   - An agent can plant `~/.gitconfig`, npm or pip config, or caches for the next tenant. It can read Claude Code transcripts when `CLAUDE_CONFIG_DIR` is unset.
4. **Read secrets on the command line (critical, already in the KI-71 code).** `IsolationStatus.command()` (`tool_process.py:167-170`) passes the whole tool environment as `NAME=value` arguments of `setpriv`/`env -i`. Every process in the container can read `/proc/<pid>/cmdline`. A tenant-B process polling `/proc` caught **150 of 150** secrets from tenant-A launches (E8).
5. **Use channels that UIDs do not separate.** Abstract unix sockets (E7 T4c), loopback TCP (E10 R1), SysV IPC (E10 R3) and `/dev/shm` connect across UIDs.

Smaller facts the design must handle:

- `/data/workspaces` is mode 2775, so every UID can list the tenant IDs (E1).
- `git clone` creates missing leading directories itself, mode 0775 under the Core's umask. A tenant directory created that way is open to every UID.
- For a UID with no passwd entry, `git commit`, `id -un`, `os.userInfo()` and ssh fail (E12).
- The worker image puts `/app/.venv/bin` first on `PATH` (`Dockerfile.worker:62`), and tools inherit it.

**Goals:**

- **G1:** The kernel separates tenants for files, signals, ptrace and `/proc/<pid>/{environ,cmdline}`.
- **G2:** No secret CodeForge hands to a tool process is ever a process argument.
- **G3:** Each tool process:
  - writes only to its run's workspace and its tenant's HOME (which holds a per-work TMPDIR);
  - reads only the system directories plus what its call is given;
  - cannot see other processes in `/proc`;
  - cannot create anything in `/tmp`.
- **G4:** It fails closed, and visibly. A missing tool UID, missing ACL support, missing Landlock (in production) or a missing HOME volume fails every tool call, starts nothing, and turns `/health/ready` to 503.
- **G5:** No new capabilities, no host privileges, no Docker socket.
- **G6:** Workspaces that exist at the upgrade keep working, without `chown`.
- **G7:** The worker never acts by path inside a tree a tool can write (D0, W1).
- **G8:** Development stays zero-config. The Core's and the worker's defaults keep today's behavior.

**Non-goals:**

- Network and local IPC between tenants (Stage 3, KI-110).
- CPU, memory, pid and disk quotas (no cgroups).
- Per-run UIDs.
- The worker's in-process readers (KI-95, done).
- LSP servers in the Core (KI-83).
- Sandbox execution modes (KI-13).
- Tenant deletion and UID reclaim (KI-112; this plan only states what they must do, D11).

---

## Changes after the review

S = security finding, O = operability finding, numbered in the order of the review.

| ID | Sev. | Finding | Fix | Where |
|---|---|---|---|---|
| S1 | blocker | The worker acts by path below tenant HOMEs (HOME/TMP setup, `.gitconfig`, `CLAUDE_CONFIG_DIR`, the CLI-check HOME, `grant_tool_access(path)`, emptying TMPDIR) and can follow a planted symlink into another tenant | Rule W1. HOME is created once, relative to the base directory's fd, and the worker never touches anything below it. The git identity moves to `/etc/gitconfig`. The per-work TMP and `CLAUDE_CONFIG_DIR` are created (`prepare`) and removed by T. The CLI check uses the system identity. `grant_tool_access` takes an fd. Every named place gets a symlink-plant test. | D0, D5, D7, D10, Tests |
| S2 | major | The stamp in the tenant directory inherits `u:T:rw-` | Stamps live in the worker-only `<root>/.codeforge/` and are bound to the tenant directory's inode | D4, D9 |
| S3 | major | Nothing checks that the root and tenant directories are worker-owned with exact ACLs, so legacy 10002 can swap a tenant directory | The worker checks exact owner, mode and ACL at startup and at every launch. A foreign-owned tenant directory is refused and never migrated. Old workers are stopped before the upgrade. A rollback is detected and forces a full re-migration. | D4, D9, Upgrade |
| S4 | major | Migration step 1 runs the path-based `find` as 10002, and re-runs can race live processes | Every owner-run step is an fd walk. The 10002 steps run under Landlock limited to the tenant tree. A link-count recheck protects inodes shared with other tenants. Migration runs only under the tenant's exclusive cross-worker lock, after reaping. | D9 |
| S5 | major | With Landlock `off`, command lines are readable across tenants. `/tmp` and `/dev/shm` stay shared. | `off` is refused in production; the minimum ABI drops to 2 so more hosts qualify. `/tmp` becomes 1771. `/dev/shm` and IPC are added to the residuals and KI-110. The ADR wording is corrected. | D6, D7, D12, Residuals |
| S6 | major | Cache and config variables pass through from the worker | The identity environment sets every cache, config, data and temp location under HOME. Operator values are dropped with a warning. | D7 |
| S7 | minor | The benchmark workspace override never takes effect | A fresh executor per benchmark task, with its own workspace | D7 |
| S8 | minor | UIDs can be reused after a database PITR | The UID-to-tenant binding is stored on the volume. The worker refuses a conflicting binding. The Core advances the sequence over the bindings. DR runbook. | D2, D3 |
| S9 | minor | Reaper PID reuse | pidfd, a UID recheck, and the per-tenant launch lock | D10 |
| S10 | minor | Fail-closed gaps in the helper, the defaults and the probe | Mandatory `landlock` spec field. An unset Landlock mode follows isolation. The probe checks a canary, `/proc/<worker>/cmdline` and the signal scope. | D5, D6 |
| S11 | minor | NATS trust caveats | Contract test (no DLQ republish onto start subjects; handoffs derive `tool_uid` in Go). Noted in ADR-018. | D3, Tests, ADR |
| O1 | blocker | Under Landlock, `python3`/`pip` resolve to `/app/.venv/bin` and fail | Tools get their own `PATH` (`CODEFORGE_TOOL_PATH`). The probe runs `python3` with the real tool environment. The battery runs on the built image. | D6, D7, Tests |
| O2 | major | The upgrade fails silently on common hosts (ABI, filesystems, old compose) | `/health/ready` answers 503 when isolation is not ready. Preflight `scripts/check-host.sh`. Default minimum ABI 2. Host requirements list. | D6, D12, Upgrade |
| O3 | major | `EnsureTenantDir` is unconditional and breaks development | Core switch `workspace.tool_acls` (off by default). xattr code only on Linux. | D4 |
| O4 | major | Trees a tool created cannot be removed by the worker or the Core; no erasure path | Every removal runs as T. Project deletion goes through the worker (new subjects, migration 115), so erasure is verifiable. Tenant erasure requirements are recorded (KI-112). | D10, D11 |
| O5 | major | The sharing pass misses ACL lock-outs | ACL-aware fd walk as T | D8 |
| O6 | major | The tmpfs HOME is noexec and counts against the worker's memory | HOMEs move to a disk volume `tool_homes`. A cache cap applies. | D7 |
| O7 | major | The allowlist breaks ptys, `/proc` tools, the JVM tmpdir and tmux | Rules for `/dev/ptmx`, `/dev/pts` and `/dev/tty`. `JAVA_TOOL_OPTIONS` and `TMUX_TMPDIR`. Limits documented. | D6, D7 |
| O8 | major | Eager allocation lets tenant admins and tests exhaust the range | Lazy allocation. Tenant creation for platform admins only. A sequence reset for test databases. Reclaim goes to KI-112. | D2 |
| O9 | major | Migration timing; old and new workers at once | Migration runs in a thread after the heartbeat starts, with progress logs and a cross-worker lock. All workers are stopped before the upgrade. | D9, Upgrade |
| O10 | major | CI and dev coverage gaps | Root tests under `sudo -E`. A Docker job on the built image with the production service definition. Skips fail in CI. `drop_caches` automated. Base interpreter derived. `acl` installed in the dev tooling. | D5, Tests |
| O11 | minor | Blue-green order | Documented order; the rejection message names the cause | D3, Upgrade |
| O12 | minor | UID uniqueness after a DB restore | Same fix as S8 | D2, D3 |
| O13 | minor | `working_dir_override` escapes the workspace; granting `/app` fails the probe | The override is accepted only inside the workspace. A dedicated canary; read-path validation. | D6, D7 |
| O14 | minor | Costs beyond the launch | Per-call sharing pass limited by ctime. Measurements added. NSS question. | D8, Costs, Open questions |
| O15 | minor | `useradd` runs out of subordinate IDs | passwd and group lines written directly; the image test counts 10,001 entries | D1 |
| O16 | minor | Reaper behavior and its order relative to the sharing pass | Order fixed (stop, share, clean, reap at idle, share, clean). The background-process lifetime is documented. | D10 |
| O17 | minor | Adopted workspaces at the upgrade; `T` invisible | The Core logs each one with T and the `setfacl` command. Errors name T. `tool_uid` is visible to platform admins. | D2, D4, D9 |
| O18 | minor | Claude Code changes alter development | Gated on isolation `required` | D7 |

---

## Design

### D0 Rules that apply to every step

- **W1: the worker never acts by path in a tree a tool can write.**
  - A tool of tenant T can create, rename and replace entries in its workspaces, its HOME and its per-work TMP. Before the upgrade, a legacy tool could do the same in every tenant directory.
  - The worker (uid 10001, group 10010) can write every tenant's tree, so a path it follows there could land in another tenant's tree.
  - In such a tree the worker therefore does one of two things:
    - It acts relative to a directory descriptor opened with `O_NOFOLLOW|O_DIRECTORY` on a directory only it can write: the root, `/home/codeforge-tools`, `/tmp`, or its state directory `<root>/.codeforge`. Before `fchmod`/`fsetxattr` it rechecks owner, type and inode with `fstat`.
    - Or it lets the tenant UID act through the launcher, under Landlock limited to that tenant's area. This covers per-work directories, the sharing pass, removals and cleanup.
  - The path-based `share_with_tools` (`tool_process.py:582-597`) and the worker-made temporary CLI-check HOME (`claude_code_executor.py:553-555`) go away.
  - Each place this plan names gets a test that plants a symlink there (Tests).
- **W2: fail closed and visibly.** A worker that cannot isolate starts no tool process and answers `/health/ready` with 503 and the reason (D12).
- **W3: nothing secret on argv.** The tool environment travels on a memfd (D5).
- **W4: development stays unchanged.** Every new behavior depends on one of two switches. On the worker it is `CODEFORGE_TOOL_ISOLATION=required`; on the Core it is `workspace.tool_acls: required`. Their defaults keep today's behavior.

### D1 Identities

| Identity | UID:GID | Supplementary groups | HOME | Runs |
|---|---|---|---|---|
| Go Core | 10001:10001 | 10010 | - | unchanged |
| Worker | 10001:10001 | 10010 (+ ambient SETUID, SETGID, KILL) | - | unchanged |
| **Tenant tool user** | T:T, T in **20000-29999** | **none** | `/home/codeforge-tools/<T>` | every tool process of that tenant |
| **System tool user** | 19999:19999 | none | `/home/codeforge-tools/19999` | Tenantless spawns only: the isolation probe, the Claude Code CLI check, and the `--version` checks of `backends.health.request` (`subprocess_utils.py:57`). It has no access to any workspace. |
| Legacy tool user | 10002 (retired) | none | - | Nothing. It still owns files from before the upgrade. Only the migration's owner-run steps (D9) run as it. |

- **The workspace group 10010** now has only the Core and the worker as members. Tool processes start with `--clear-groups`. The helper and the probe require an empty group list.
- **Constants** are kept in one place per language:
  - Go: `internal/domain/tenant/tooluid.go` (`ToolUIDMin`, `ToolUIDMax`, `SystemToolUID`, `LegacyToolUID`).
  - Python: `codeforge/tool_identity.py`.
  - A contract test pins that both hold the same values.
- **passwd and group entries** `codeforge-t<uid>` are generated at image build for 19999 and 20000-29999: home `/home/codeforge-tools/<uid>`, shell `/usr/sbin/nologin`.
  - Without them, `git commit`, `id -un`, node's `os.userInfo()` and ssh fail (E12).
  - The lines are written directly with a `printf` loop into `/etc/passwd` and `/etc/group`, with no shadow and no subordinate-ID entries. Debian's `useradd` allocates 65,536 subordinate IDs per non-system user, runs out near the 9,156th user, and costs about 14 ms per user (E21).
  - An image test asserts exactly 10,001 generated entries.
  - The cost is an `/etc/passwd` of about 1 MB. Every process start that looks up its user (bash, git) gets 1.1 to 3.2 ms slower; the worst case is the last UIDs (E20). See Open question 2.
- **Default git identity.** The worker image's `/etc/gitconfig` gets `user.name = CodeForge agent` and `user.email = agent@codeforge.invalid`, next to `safe.directory=*`. Global and repository config still win. Nobody writes a `~/.gitconfig` (W1).
  - E12 shows `git commit` failing even with a passwd entry ("unable to auto-detect email address").
  - The same applies to uid 10002 today. So `_snapshot_workspace` (`agent_loop.py:842`, `git stash push`) probably fails in production now (not tested on that path).

### D2 UID allocation and persistence (question c)

**Migration `114_tenant_tool_uid.sql`.** UIDs are allocated lazily. The only backfill is for tenants that may already have workspace directories.

```sql
-- +goose Up
CREATE SEQUENCE tenant_tool_uid_seq AS integer MINVALUE 20000 MAXVALUE 29999 START 20000 NO CYCLE;
ALTER TABLE tenants ADD COLUMN tool_uid integer
    CONSTRAINT tenants_tool_uid_range CHECK (tool_uid BETWEEN 20000 AND 29999)
    CONSTRAINT tenants_tool_uid_key UNIQUE;
-- Tenants with projects may have tenant directories to migrate: number them in creation order.
WITH numbered AS (
    SELECT t.id, 19999 + row_number() OVER (ORDER BY t.created_at, t.id) AS uid
    FROM tenants t WHERE EXISTS (SELECT 1 FROM projects p WHERE p.tenant_id = t.id))
UPDATE tenants t SET tool_uid = n.uid FROM numbered n WHERE t.id = n.id;
SELECT setval('tenant_tool_uid_seq', COALESCE((SELECT max(tool_uid) FROM tenants), 20000),
              EXISTS (SELECT 1 FROM tenants WHERE tool_uid IS NOT NULL));
ALTER SEQUENCE tenant_tool_uid_seq OWNED BY tenants.tool_uid;
-- +goose StatementBegin
CREATE FUNCTION tenants_tool_uid_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.tool_uid IS NOT NULL AND NEW.tool_uid IS DISTINCT FROM OLD.tool_uid THEN
        RAISE EXCEPTION 'tenants.tool_uid is immutable once set';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER tenants_tool_uid_immutable BEFORE UPDATE OF tool_uid ON tenants
    FOR EACH ROW EXECUTE FUNCTION tenants_tool_uid_immutable();
```

- **Lazy allocation.**
  - A UID is allocated the first time the Core needs one for a tenant: when it creates the tenant's first workspace directory, or when it first dispatches tool work.
  - This happens only with `workspace.tool_acls: required` (D4). Development databases allocate nothing.
  - Tenants that never run tools use no UID. Integration tests create and delete tenants on every run (`tests/integration/integration_test.go:131-155`).
  - `service.ToolUIDFor(ctx, tenantID)` caches set values forever (they are immutable) and allocates in one transaction:

    ```sql
    SELECT tool_uid FROM tenants WHERE id = $1 FOR UPDATE;
    -- only when NULL:
    UPDATE tenants SET tool_uid = nextval('tenant_tool_uid_seq') WHERE id = $1 RETURNING tool_uid;
    ```

    The row lock prevents a race that would burn a UID.
- **Exhaustion.** `nextval` raises SQLSTATE `2200H`, which the store maps to `tenant.ErrToolUIDRangeExhausted`. The dispatching API call answers **503** "tool UID range exhausted (10,000 tenants with tool work)", and an error is logged. Tenant creation is not affected.
- **Tenant creation is platform-admin only.**
  - `POST /api/v1/tenants` moves from `RequireRole(RoleAdmin)` (`routes.go:703-706`) to `middleware.RequirePlatformAdmin`.
  - The frontend offers tenant creation only when `is_platform_admin` is set.
  - With lazy allocation this is not strictly needed against exhaustion, but no tenant admin can create tenants for others any more.
- **Visibility (O17).** `GET /tenants` and `GET /tenants/{id}` include `tool_uid` (a number or `null`) for platform admins only. Operators then do not need SQL for the adopted-workspace `setfacl` command (D4).
- **Never reused in this round.** Reusing a UID needs tenant erasure and a reclaim check first (KI-112, D11).
- **Restores (S8, O12).**
  - The workspaces volume records every binding in `<root>/.codeforge/uids/<T>` (D4).
  - At startup with `required`, the Core reads these bindings and, if needed, advances the sequence above the highest bound UID (`setval`, logged as a warning). A database restored to a point older than the volume therefore never hands a bound UID to a new tenant.
  - The worker refuses a payload whose UID is bound to another tenant (D3).
  - `docs/disaster-recovery.md`: restore the database and the workspaces volume to the same point. Otherwise, the Core's advance applies, and the operator removes the orphaned tenant directories it logs.
- **Test databases.** Integration packages call a helper `resetToolUIDSequence` in `TestMain`. It runs `setval` to the highest `tool_uid` still in use, or to 20000. Long-lived dev and test databases never run out.
- **Go.** `tenant.Tenant` gets `ToolUID *int`; JSON shows it only to platform admins.
  - The store gains `AllocateToolUID` and `AdvanceToolUIDSequence`.
  - Every store mock with tenant methods needs updating: `middleware/teststore_test.go`, `http/handlers_test.go`, `service/runtime_test.go`, `service/project_test.go`.

### D3 How the UID reaches every spawn site (question e)

**Payload field `tool_uid` (int, `omitempty`).** The Core sets it from the tenant in the context, only with `workspace.tool_acls: required`:

| Subject | Go struct (builder) | Python model |
|---|---|---|
| `runs.start` | `RunStartPayload` (`service/runtime.go`) | `RunStartMessage` (`models.py:112`) |
| `conversation.run.start` | `ConversationRunStartPayload` (`conversation_dispatch.go`, `conversation.go`) | `ConversationRunStartMessage` (`:529`) |
| `tasks.agent.*` | `TaskAgentPayload` (`NewTaskAgentPayload`, `schemas_run.go:31`; `agentbackend.Execution` gains `ToolUID`) | `TaskMessage` (`:25`) |
| `runs.qualitygate.request` | `QualityGateRequestPayload` (`runtime_gate.go`) | `QualityGateRequest` (`:188`) |
| `conversation.test.request` | `WorkspaceTestRequestPayload` (`autoagent.go`) | `WorkspaceTestRequest` (`:206`) |
| `benchmark.run.request` | `BenchmarkRunRequestPayload` (`benchmark_run.go`) | `BenchmarkRunRequest` (`:619`) |
| `workspace.delete.request` (new, D11) | `WorkspaceDeleteRequestPayload` (`schemas_workspace.go`) | `WorkspaceDeleteRequest` |

- **No field is needed elsewhere.**
  - Retrieval, repo map, GraphRAG and context requests run in the worker's own process and start no tool process.
  - Handoffs and approved handoffs start runs through `runs.start`.
  - Sub-agents (KI-25 plan) run inside their parent's run context.
- **Contract tests.** The contract testdata (`internal/port/messagequeue/testdata`) and the round-trip tests are updated.
- **Trust (S11).**
  - Only the Core may publish these subjects (ADR-017 §5). Tools have no NATS credentials. The worker's NATS user may publish only results, `handoff.request` and `*.dlq` copies.
  - Neither a tool nor a compromised worker can therefore pick another tenant's UID through NATS. Two invariants keep it that way, and a contract test pins both:
    - **No Core code republishes a worker-originated payload onto the seven subjects above.** This covers DLQ copies, which the worker writes. The Core's `.dlq` subscribers only end or record work.
    - **A run started from a handoff computes `tool_uid` in Go,** from the tenant of the claimed handoff (`handoff_claims`), never from worker-sent metadata.

**Checks on accept (isolation required).** The work item is acked on accept (ADR-016). A failed check ends it as a failed completion, never as a NAK. The checks:

- **`tool_uid` present.** A missing one is refused with "payload without tool_uid: the Go Core is older than this worker or runs with workspace.tool_acls=off" (O3, O11). It must be in range and not 10001, 10002 or 19999. `tenant_id` must be present.
- **On-volume binding `<root>/.codeforge/uids/<T>`.**
  - If absent, it is created with `O_CREAT|O_EXCL|O_NOFOLLOW` relative to the directory's fd. A concurrent worker that loses the race re-reads it.
  - If it names another tenant, the work is refused, and the log names both tenants and the restore remedy (D2).
  - This replaces revision 1's in-memory guard and survives restarts.
- **Workspace path.**
  - It must lie below `<root>/<tenant_id>/`. For an adopted workspace, the directory must carry its access ACL for T and an adopted-migration stamp (D9).
  - A backend's `working_dir_override` must resolve inside the workspace (D7).
- **Tenant directory verification (D4).** It passes, needs migration (D9), or is refused.
- Every refusal message names the tenant and T (O17).

**Context variable.** `codeforge.tool_identity.current: ContextVar[ToolIdentity | None]`. `ToolIdentity` holds `tenant_id`, `uid`, `workspace`, `home`, the work token (TMPDIR = `<home>/tmp/<token>`), and the per-call `read_paths`/`write_paths`.

- **Setting it.** The handlers set it with `async with tool_tenant(payload):`, entered **after the handler's heartbeat started** (`consumer/_runs.py:110` and equivalents). The handlers are in `consumer/_runs.py`, `_conversation.py`, `_tasks.py`, `_quality_gate.py`, `_workspace_test.py`, `_benchmark.py`, `_benchmark_runners.py` and `_workspace_delete.py`. The context manager:
  - runs the accept checks;
  - verifies HOME (D7);
  - takes the tenant's shared lock (D9), and migrates in a thread if needed (D9);
  - registers the work item for the end-of-work steps (D10).

  It never creates or writes anything below the HOME (W1).
- **Propagation.** asyncio tasks inherit the context: sub-agents, MCP sessions and heartbeat tasks.
- **Overrides.** `start_tool_process`, `run_tool_process`, `start_tool_shell` and `tool_stdio_client` read the variable. A keyword-only `identity=` override exists only for the probe and the system calls.

**Work without a tenant (isolation required).** A tool process with no identity raises `ToolIsolationError`, and nothing starts. Only the explicitly marked call sites of D1 use the system identity. With isolation off (development), the identity is optional, and processes run as the worker, as today.

### D4 Filesystem layout and ACLs (questions a, g)

| Path | Owner:group, mode | Access ACL | Default ACL | Who creates entries inside |
|---|---|---|---|---|
| `/data/workspaces` (root) | 10001:10010, **2771** | none (verified) | none | Core, worker |
| `<root>/.codeforge` (worker state) | 10001:10010, 2700 | none (verified) | none | worker only |
| `<root>/<tenant>` | 10001:10010, 2770 | `u::rwx, u:T:--x, g::rwx, m::rwx, o::---` | `u::rwx, u:T:rwx, g::rwx, g:10010:rwx, m::rwx, o::---` | Core, worker |
| `<root>/<tenant>/<project>` and below | creator:10010 (setgid inherited) | inherited, masked by the create mode | inherited | Core, worker, T |
| `/home/codeforge-tools` (volume `tool_homes`) | 10001:10001, 0711 | none (verified) | none | worker only |
| `/home/codeforge-tools/<T>` (HOME) | 10001:10001, 0700 | `u::rwx, u:T:rwx, g::---, m::rwx, o::---` | none | T only (W1) |
| `<HOME>/tmp/<work token>`, `<HOME>/claude/<work token>` | T:T, 0700 | none | none | T |
| `/tmp` (tmpfs) | 10001:10010, **1771** | none | none | worker only |
| `/tmp/cf-cc-*` (Claude Code run directory) | 10001, 0700 | `u:T:r-x` (socket `u:T:rw-`, system prompt `u:T:r--`) | none | worker |
| `/tmp/cf-bench-*` (benchmark task workspace) | 10001:10001, 0700 | `u:T:rwx, g:10010:rwx` | `u::rwx, u:T:rwx, g::---, g:10010:rwx, m::rwx, o::---` | T, worker |

- **Why search-only on the tenant directory.**
  - The tool UID gets `--x` there, a refinement of "rwx" in the owner decision. Tools can reach their projects but cannot list, create, rename or delete entries at tenant level (E9: "A renames its project dir: Permission denied").
  - Project directories get `rwx` from the default ACL.
  - The migration stamp no longer lives in the tenant directory. In revision 1 it would have inherited `u:T:rw-` (S2).
- **Kernel semantics (E1).**
  - With a default ACL, the process umask is ignored, and the group bits of the create mode become the mask.
    - Core files created 0664 get mask `rw-`.
    - Directories created 0770 get mask `rwx`.
    - `mkdtemp` 0700 gives mask `---` (the sharing pass fixes that, D8).
  - The `g:10010` entry keeps the Core and the worker in, even after a tool moves a file to its own group. A tool cannot `chgrp` to 10010 (EPERM, E1).
  - The Core and the worker keep full access through the owning group 10010, the named `g:10010` entries and ownership.
- **Worker state directory `<root>/.codeforge`.**
  - The worker creates it at startup, relative to the root's fd, and verifies it: owner 10001, no permission bits for group or other, no ACL. Subdirectories:
    - `tenants/<tenant_id>`: migration stamps (D9);
    - `uids/<T>`: UID bindings (D2, D3);
    - `locks/<tenant_id>`: cross-worker `flock` files (D9);
    - `adopted/<sha256 of path>`: stamps of adopted workspaces (D9).
  - Tools can traverse the root (`o::--x`), but they cannot enter `.codeforge`.
  - Tenant IDs are UUIDs, so `.codeforge` never collides with a tenant directory.
- **Verification (S3).** Without it, cross-tenant file isolation would rest on nothing but trust in the layout.
  - **At worker startup**, it checks the root, `.codeforge`, the HOME base, and the shape of every tenant directory: worker-owned, mode 2770, named entries only one `u:<uid>` and `g:10010`, and that uid matching the stamp if there is one.
  - **At every accept and every launch**, for the payload's tenant: open the root fd, `openat` the tenant name with `O_NOFOLLOW|O_DIRECTORY`, `fstat` (directory, owner 10001, mode 2770), `fgetxattr` the access and default ACLs, and compare them exactly with the shared vectors for T. The root must be owner 10001, mode 2771, with no named ACL entries. The cost is about five syscalls.
  - **Results:**
    - match: go;
    - worker-owned, but mode or ACL differ, or the stamp is missing or stale: migrate (D9);
    - not a directory, a symlink, or another owner: refuse that tenant's tool calls and log the remedy. The work is never migrated over such a directory. The worker cannot `chown`, and `docker exec -u 0` into the worker has no `CAP_CHOWN` either. The remedy is an operator-run, one-off root container on the volume.
- **ACL tooling.**
  - `python:3.12-slim` has `libacl1` but no `setfacl`/`getfacl` (E0).
  - The worker sets ACLs only with `os.setxattr(fd, ...)` through a small codec, `codeforge/posix_acl.py`.
  - The Debian `acl` package is added for operators and the preflight; the sharing pass does not need it (D8).
- **Core switch (O3).** `workspace.tool_acls` takes `off` or `required` (env `CODEFORGE_WORKSPACE_TOOL_ACLS`).
  - It is `required` in the Core image and in `docker-compose.prod.yml`. The default is `off`, so `go run ./cmd/codeforge` keeps today's `MkdirAll` on macOS, on WSL2 drvfs and on devcontainer bind mounts.
  - An unknown value counts as `required` and is logged as an error.
  - With `APP_ENV=production` and `off`, the Core logs a warning at startup.
  - A worker that requires isolation refuses the Core's payloads without `tool_uid` with a message naming this switch (D3).
- **Core package `internal/workspaceacl`.**
  - Codec: `acl.go`.
  - `acl_linux.go` (`//go:build linux`, `golang.org/x/sys/unix`, already in the module graph at `go.mod:55` and now a direct dependency).
  - `acl_other.go` returns `ErrUnsupported`. `required` on a non-Linux build fails at startup.
  - `EnsureTenantDir(root, tenantID string, toolUID int) error`:
    - runs `Mkdirat` 0770 relative to the root's fd (an existing directory is fine);
    - opens it with `O_DIRECTORY|O_NOFOLLOW`, and checks that it is a directory owned by the Core;
    - runs `Fchmod 02770`;
    - sets the exact access and default ACLs with `Fsetxattr`.
  - With `required`, it is called before `gp.Clone` (`project_workspace.go:47`) and before `MkdirAll` in `InitWorkspace` (`:205-206`), so git never creates the tenant directory itself.
  - Both languages test against one vector file (`internal/workspaceacl/testdata/acl_vectors.json`).
- **Adopted workspaces** (`project_workspace.go:60-118`, platform admins only).
  - At adoption, the Core sets the exact ACLs on the adopted directory when it owns it. Otherwise it refuses, and the message names the command.
  - At Core startup with `required`, the Core checks every adopted project outside the root:
    - if the Core owns the directory, it sets the top directory's ACL (the worker migrates the content, D9);
    - otherwise it logs the tenant, T and the exact command `setfacl -R -m u:<T>:rwX -m d:u:<T>:rwX -m g:10010:rwX -m d:g:10010:rwX <dir>` (O17).
- **Root mode.** The root is 2771 in `Dockerfile` (`:39-42`) and `Dockerfile.worker` (`:45-52`). The worker, which owns the root, fixes an existing root at startup.
- **ACL support check.** In required mode, at startup the worker sets and reads back a default ACL on a temporary directory under the root and under `/home/codeforge-tools`. A failure, such as ZFS without `acltype=posixacl` (EOPNOTSUPP) or NFSv4, makes isolation not ready (D12).

### D5 The launch path: environment off the command line (critical)

New command line. Nothing secret appears in any argument:

```text
setpriv --reuid=T --regid=T --clear-groups --inh-caps=-all --ambient-caps=-all --no-new-privs -- \
    <base python> -I -S /app/workers/codeforge/tool_exec.py <spec-fd> <argv...>
```

**The base interpreter (O10)** is `sys._base_executable`, the interpreter the worker's venv is built on (`/usr/local/bin/python3` in the image). When `sys.base_prefix` is not below `/usr`, as on CI runners (`/opt/hostedtoolcache/...`) and dev hosts, it is added as a read-and-execute Landlock rule.

**The spec.** The worker writes it to an `os.memfd_create(..., MFD_CLOEXEC)` and hands it over with `pass_fds`. It closes its copy right after the spawn.

```json
{"uid": 20000, "gid": 20000, "umask": 7, "env": {"...": "..."},
 "landlock": {"rules": [["/usr", ["execute", "read-file", "read-dir"]], "..."], "scope": true, "min_abi": 2},
 "prepare": ["tmp/3f9c...", "claude/3f9c..."], "home": "/home/codeforge-tools/20000",
 "cwd": "/data/workspaces/<tenant>/<project>"}
```

- Every field is mandatory. A missing or unknown `landlock` value makes the helper exit 125 (S10). The string `"off"` is written only when the worker's Landlock mode is `off`.
- Opening `/proc/<pid>/fd` of the holder needs ptrace access (same UID and same Landlock domain), so no other tenant can open the memfd.
- E8: the KI-71 form leaked **150/150** secrets; the helper leaked **0/150** (cmdline and environ).

**`tool_exec.py`** (new, stdlib only, about 120 lines) runs as T. It:

1. reads and closes the spec fd;
2. checks its own credentials against the spec: real and effective uid and gid equal the spec's, no supplementary groups, all four capability sets empty, `NoNewPrivs 1`;
3. detects the Landlock ABI (`landlock_create_ruleset(NULL, 0, VERSION)`);
4. builds the ruleset:
   - the handled rights are all filesystem rights the ABI knows (ABI 1: 13, 2: +refer, 3: +truncate, 5: +ioctl_dev);
   - with ABI >= 6 it adds `scoped = SIGNAL | ABSTRACT_UNIX_SOCKET`;
   - the attribute size is 8, 16 or 24 bytes by ABI;
5. opens every rule path **component by component** with `openat(O_PATH|O_NOFOLLOW)` from `/` as the tool UID. It **refuses a symlink in any component**, because `O_PATH|O_NOFOLLOW` would open the link itself (E2 T1b);
6. masks file rules to file rights, then calls `landlock_add_rule` and `landlock_restrict_self`;
7. creates each `prepare` entry below HOME, one component at a time: `mkdirat` relative to the HOME fd, mode 0700, then `openat(O_NOFOLLOW|O_DIRECTORY)`. If a component exists as a symlink or a non-directory, it exits 125;
8. `fchdir`s to `cwd`. The directory is opened component by component with `O_NOFOLLOW`. The worker no longer passes `cwd=` to Popen, which would `chdir` by path as the worker (W1);
9. sets the umask and runs `os.execvpe` with exactly the spec's environment.

Any failure prints `cf-tool-exec: ...` and exits 125 without running the command (E2 T1c, E5).

**Rule paths are opened as the tool UID, after the UID switch.** setpriv's own `--landlock-rule` opens them as the worker, before the switch. It also cannot scope signals or abstract sockets, and has no `ioctl_dev` or `net` rights (E0).

**Other launch details:**

- **Loader safety.** setpriv keeps the fixed environment `{PATH}`; ADR-017's `LD_PRELOAD` reasoning is unchanged. `env -i` goes away.
- **Cost.** The stock image has **no `.pyc` files** (E0), and on a read-only root `import ctypes, json` costs about 80 ms per launch. `Dockerfile.worker` therefore runs `python -m compileall -q /usr/local/lib/python3.12`. A launch then costs about 24 ms, against 3.4 ms for setpriv plus `env -i` (E8).
- **Umask.** Tool processes get **umask 007** (`TOOL_UMASK`). Under default ACLs the umask does not apply. Elsewhere, files are private to the tenant.
- **Pipes.** `start_tool_process` creates the stdin, stdout and stderr pipes itself (`os.pipe2`) and calls `fchmod(0o666)` before the spawn. It wraps the read ends with `loop.connect_read_pipe` and the write end with `connect_write_pipe`.
  - Since KI-71, a tool cannot reopen the worker's pipe through `/dev/stdout`, `/dev/stderr` or `/dev/fd/N`: the pipe inode belongs to 10001 with mode 0600. E11 shows `echo x > /dev/stderr` failing with and without Landlock, and working after the `fchmod`.
  - Reopening still requires access to the holder's `/proc/<pid>/fd`, so this exposes nothing.
- **MCP stdio servers.** The MCP SDK's `stdio_client` cannot pass fds (mcp 1.30.0, `_create_platform_compatible_process`).
  - `tool_stdio_client` therefore gets its own transport on `anyio.open_process(..., pass_fds=..., start_new_session=True)`. It reuses the SDK's `SessionMessage`/`JSONRPCMessage` types (JSON lines).
  - The server's declared environment goes into the spec, and its cwd is the run's workspace.
- **API change.** `IsolationStatus.command(argv, env)` becomes `launch(argv, env, identity) -> Launch(argv, pass_fds, close_after_spawn, umask)`.
- **Single entry point.** The scan (`tests/test_tool_process.py:749`) allowlists `tool_exec.py`, the second half of the launcher, for `os.exec*`.
- **Command lines of the commands themselves.** The Bash command (`bash -c <command>`, `tools/bash.py:127-135`) and backend prompts (`aider --message`, `backends/aider.py:39`) stay arguments of the command. Landlock's `/proc` confinement keeps them from other tenants: production allows no Landlock-off mode (D6, S5). Moving them off argv is Open question 7.

### D6 Landlock per tool call (Stage 2, question b)

**Mode (S5, S10).**

- `CODEFORGE_TOOL_LANDLOCK` takes `required` or `off`.
  - **Unset:** it follows `CODEFORGE_TOOL_ISOLATION`, so it is `required` whenever isolation is required.
  - **Unknown value:** counts as `required`.
- **`off` is refused in production.** With isolation required and `APP_ENV=production`, `off` makes isolation not ready: every tool call fails, `/health/ready` answers 503, and the log names the variable.
  - Without Landlock, `/proc/<pid>/cmdline` of every process is readable across UIDs. That includes agent commands, backend prompts, MCP server arguments and whatever the agents' child processes carry on their command lines (E7).
  - Outside production, `off` is allowed with a startup warning ("tool command lines are readable across tenants"). It suits development and tests.
- The worker image sets nothing beyond `CODEFORGE_TOOL_ISOLATION=required`, so Landlock follows it.

**Minimum ABI (O2).** `CODEFORGE_TOOL_LANDLOCK_MIN_ABI` defaults to **2**.

- ABI 1 has no `refer`, so every cross-directory rename or link is denied, which breaks `mv` and git.
- ABI 2 is the kernel 6.1 of Debian 12 and Amazon Linux 2023. It does not handle truncate, and the worker logs that once.
  - Truncating files outside the rules is then decided by DAC alone. That means the tenant's other projects; other tenants stay separated by DAC.
- Scopes are applied whenever ABI >= 6. With ABI < 6, the worker logs once that abstract sockets and signals between a tenant's runs are not scoped, and `/health/ready` reports it in its details.
- A minimum of 3 or 6 makes those gaps fail closed instead (Open question 3).

**Rule set**, verified as a whole in E3, E4 and E11, plus the review additions (not yet verified, Tests):

| Path | Rights | Why |
|---|---|---|
| `/usr` | execute, read-file, read-dir | Binaries and libraries (merged /usr covers `/bin` and `/lib`). |
| base interpreter prefix, when not below `/usr` | execute, read-file, read-dir | The helper's and the hook's interpreter on CI runners and dev hosts (O10). |
| `/etc` | read-file, read-dir | gitconfig, CA certificates, passwd, resolv.conf. |
| `/dev/null` | read-file, write-file, truncate | |
| `/dev/zero`, `/dev/random`, `/dev/urandom` | read-file | |
| `/dev/tty` | read-file, write-file | ENXIO without a controlling terminal instead of EACCES (E19). |
| `/dev/ptmx`, `/dev/pts` (no read-dir) | read-file, write-file, ioctl-dev | Pseudo-terminals: `os.openpty`, `script`, pexpect, tmux, node-pty, expect, OpenHands' terminal (E19). Other UIDs' pts devices stay protected by DAC. |
| `/dev/shm` | read-file, write-file, make-reg, remove-file, truncate (**no read-dir**) | POSIX semaphores: `multiprocessing.Lock()` fails without them (E11). No listing. |
| `/proc/<own pid>`, opened by the helper; the **O_PATH fd is kept open across exec** | read-file, read-dir | The exec target's own `/proc`. Claude Code (Bun) aborts without `/proc/self/{maps,cgroup,statm}` (E4). |
| `/proc/cpuinfo`, `/proc/stat`, `/proc/meminfo`, `/proc/loadavg`, `/proc/uptime` | read-file | node's `os.cpus()` is empty without them (E11). |
| `/proc/sys` | read-file, read-dir | Bun reads `vm/overcommit_memory` and `vm/mmap_min_addr` (E4). |
| `/sys/devices/system`, `/sys/fs/cgroup`, `/sys/kernel/mm/transparent_hugepage` | read-file, read-dir | CPU, NUMA and cgroup limits (E4). |
| The run's workspace (a directory; no symlink in any component) | all filesystem rights except make-char, make-block, ioctl-dev | |
| The tenant HOME (holds the per-work TMPDIR, caches and `CLAUDE_CONFIG_DIR`) | the same, execute included | `go test` binaries, npx shims and `pip --user` scripts run from here (O6). |
| Per-call read-only extras | read-file (+ read-dir, execute as needed) | The Claude Code run directory (system prompt), the hook script (one file), and operator toolchains from `CODEFORGE_TOOL_READ_PATHS` (for example `/opt`, or `/app/.venv` for backend CLIs). |

- **Everything else is denied:** `/tmp`, `/var/tmp`, `/app`, `/var/lib/codeforge` (the probe canary), `/data/knowledge`, `/run`, other tenants' HOMEs, the tenant's other projects, `/proc/<other pid>`, the `/proc` listing, `/proc/net`, and `/proc/self` of child processes.
- **`CODEFORGE_TOOL_READ_PATHS` validation (O13).** Each entry must be absolute and existing, with no symlink in any component. These entries are refused:
  - `/`, `/proc`, `/sys`, `/run`, `/tmp`, `/data`, `/home`, `/var/lib/codeforge`, and any ancestor of them;
  - anything below `/run`, `/proc`, `/data`, `/home/codeforge-tools` or `/var/lib/codeforge`.

  A refused entry makes isolation not ready, and the message names it. `/app` and `/app/.venv` are allowed (they hold no secrets), and they no longer interfere with the probe, which uses a dedicated canary.
- **What does not work under Landlock (O7, E19), documented for users and in the Bash tool description:**
  - `ps`, `pgrep`/`pkill`, `top` and psutil (no `/proc` listing);
  - `df` and `mount` (no `mountinfo`);
  - `ss` and `netstat` (no `/proc/net`);
  - `/proc/self/*` in child processes. The JVM's container detection then falls back to host values.

  Agents stop their own background processes with `kill <pid>` or `kill %1`, since signals need no `/proc`. Leftovers end with the work (D10). `JAVA_TOOL_OPTIONS` and `TMUX_TMPDIR` move the JVM's and tmux's temporary files into TMPDIR (D7).
- **Pinning `/proc/<pid>`.**
  - procfs makes a new inode when a `/proc/<pid>` dentry is evicted, and a rule on the old inode stops matching. E6: denied from check 24 on, after `drop_caches`.
  - Keeping the `O_PATH` fd open pins the dentry (E6: 40/40).
  - Rules on the ext4 workspace and on the HOME survive eviction (E6b: 40/40). E6b tested a tmpfs HOME; the disk volume is ext4 like the workspace.
  - The inherited fd only lets descendants read the exec target's own `/proc`, which holds the environment they already inherit.
- **The Claude Code hook** runs as `<base python> -I -S <hook>`, with a read rule for the hook file only.
  - The hook imports only the stdlib (json, os, socket, sys, time).
  - The venv interpreter cannot be used without `/app/.venv/pyvenv.cfg` (E11, E15).
- **The policy socket.** Landlock ABI <= 7 does not control `connect()` to a pathname socket (E14); DAC decides. The tool cannot replace the socket, because the directory is not writable for it (E14).
- **Startup probe** (required mode; S10, O1, O13). It runs with the system identity through the same launcher and rule set, and a `prepare`d probe TMP.
  - The probe process **must**:
    - run as uid and gid 19999, with no groups, all four capability sets empty, `NoNewPrivs 1` and umask 007;
    - be able to write its HOME and TMP;
    - run `python3 -c pass` and `git --version` **with the real tool environment** (the tool PATH, D7). That catches E15.
  - It **must fail** to:
    - read the world-readable canary `/var/lib/codeforge/landlock-canary` (0644, from the image; never grantable);
    - list `/dev/shm` (DAC allows it, Landlock does not);
    - read `/proc/<worker pid>/cmdline`. DAC allows that for every UID, so a denial proves `/proc` is confined;
    - read `/proc/<worker pid>/environ` and `/run/secrets/*`;
    - create a file in `/tmp`.
  - With ABI >= 6, a second probe process with the same UID in a sibling domain must get EPERM for `kill(sibling, 0)` (scope check).
  - The probe TMP is removed as 19999.
  - Any failure makes isolation not ready (D12). Every tool call then raises `ToolIsolationError` (E5: ENOSYS, exit 125). The effective mode, the ABI and the scope state are logged once.

### D7 Per-tenant HOME, per-work TMP, environment, Claude Code, MCP, backends, benchmarks (question f)

- **HOME volume (O6).** A named volume `tool_homes` is mounted at `/home/codeforge-tools`. It replaces the `/home/codeforge-tool` tmpfs (compose `:338`).
  - On disk, caches stay out of the worker's memory cgroup. A tmpfs HOME filled by one tenant got the worker OOM-killed (E18).
  - Caches survive restarts.
  - The mount is not `noexec`. Docker's tmpfs is, which breaks npx shims, `go test`, uv and `pip --user` binaries (E18). Landlock decides what may execute.
  - The image creates `/home/codeforge-tools` as 10001:10001, mode 0711, so a fresh volume starts with that owner and mode.
  - At startup the worker checks the owner, the mode, ACL support, and that the mount is not `noexec` (`statvfs` `ST_NOEXEC`). Otherwise it reports not ready, which catches a compose file that still mounts the old tmpfs.
  - Worker replicas share the volume.
- **HOME creation (W1, S1).** `<base>/<T>` is created once per volume:
  - `mkdirat(base_fd, "<T>", 0o700)` (EEXIST is fine);
  - `openat(base_fd, "<T>", O_RDONLY|O_DIRECTORY|O_NOFOLLOW)`;
  - `fstat`: a directory owned by 10001;
  - `fsetxattr` with the exact access ACL `u:T:rwx`.

  Tools cannot rename or replace the HOME: the base is the worker's and 0711. Each work item re-opens it the same way and compares owner, inode and ACL. A mismatch refuses the tenant; it cannot happen without the worker's UID. **The worker never creates, writes, lists or removes anything below a HOME.** A HOME owned by the worker is unusual; tools that insist on owning HOME itself are not known among the supported toolchains, and the battery checks this.
- **Per-work TMP.** There is one per run, conversation turn, task, gate, workspace test and benchmark task: `<HOME>/tmp/<work token>`.
  - The launcher's `prepare` creates it as T (D5). The end of the work item removes it as T (D10).
  - Concurrent work of a tenant does not share a TMPDIR, also in another worker on the same volume.
- **Identity environment (S6, O1, O7).** In required mode, `tool_identity_env()` sets each of these variables itself; none is passed through:
  - `HOME`, `USER`/`LOGNAME=codeforge-t<T>`, `SHELL=/bin/bash`;
  - `PATH` = `CODEFORGE_TOOL_PATH` (default `/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`), followed by `<HOME>/.local/bin:<HOME>/go/bin:<HOME>/.cargo/bin:<HOME>/.npm-global/bin`.
    - The worker's own `PATH` (`/app/.venv/bin` first, `Dockerfile.worker:62`) is never used, so `python3`, `pip` and `pytest` resolve to the system interpreter (E15).
    - The HOME directories come after the system ones, so one run cannot shadow `git` for the next run of the same tenant.
  - `TMPDIR`, `TMP`, `TEMP` and `GOTMPDIR` = the per-work TMP;
  - `XDG_CACHE_HOME=<HOME>/.cache`, `XDG_CONFIG_HOME=<HOME>/.config`, `XDG_DATA_HOME=<HOME>/.local/share`, `XDG_STATE_HOME=<HOME>/.local/state`;
  - `GOPATH=<HOME>/go`, `GOMODCACHE=<HOME>/go/pkg/mod`, `GOCACHE=<HOME>/.cache/go-build`;
  - `CARGO_HOME=<HOME>/.cargo`, `RUSTUP_HOME=<HOME>/.rustup`;
  - `npm_config_cache=<HOME>/.npm`, `npm_config_prefix=<HOME>/.npm-global`;
  - `PIP_CACHE_DIR=<HOME>/.cache/pip`, `UV_CACHE_DIR=<HOME>/.cache/uv`;
  - `JAVA_TOOL_OPTIONS=-Djava.io.tmpdir=<TMPDIR>` (the JVM ignores `TMPDIR`, E19) and `TMUX_TMPDIR=<TMPDIR>`.

  An operator value for any of these names in the worker environment is dropped, with a one-time warning naming it. Read-only toolchain roots (`GOROOT`, `JAVA_HOME`) still pass through. They need `CODEFORGE_TOOL_READ_PATHS` when outside `/usr`. Operators with toolchains elsewhere, such as a pre-installed rustup toolchain, add their `bin` directory to `CODEFORGE_TOOL_PATH`. With isolation off (development), the environment is today's.
- **Cache size.** When the tenant goes idle (D10) and the worker gets the tenant's lock exclusively without waiting, it measures `<HOME>/.cache` as T. If it is larger than `CODEFORGE_TOOL_CACHE_MAX_MB` (default 4096), it removes it as T. The volume's free space is a shared limit without quotas (Residual 4).
- **Claude Code** (`claude_code_executor.py`). All of this applies **only with isolation required** (O18). Development keeps today's environment, including the `CLAUDE_CONFIG_DIR` passthrough and `claude login` credentials.
  - **Config directory.**
    - `CLAUDE_CONFIG_DIR=<HOME>/claude/<work token>`, made by `prepare` and removed as T at the run's end.
    - `CLAUDE_CODE_DISABLE_AUTO_MEMORY=1`. Claude Code's hosting guidance for a shared container is a per-tenant working directory and config directory.
    - In required mode, `CLAUDE_CONFIG_DIR` is dropped from `_CLAUDE_CLI_ENV` (`:65-71`): an operator directory would be shared by all tenants (Open question 5).
  - **Policy directory.** `/tmp/cf-cc-*` is made by the worker, the only user that can create entries in `/tmp`. `PolicySocketServer.__aenter__` (`:321-339`) and `_write_private_file` (`:488-496`) set the run's tenant ACLs through the fd (`grant_tool_access(fd, identity, writable)`, E14).
  - **Hook command.** `_hook_command` (`:435-441`) uses the base interpreter with `-I -S`.
  - **CLI check.** `_check_hidden_options` (`:546-557`) runs with the system identity. Its HOME and `CLAUDE_CONFIG_DIR` are a per-check directory that `prepare` creates and the system user removes. There is no worker-made temporary directory and no tenant HOME (S1, O4).
- **MCP stdio servers.** They start in the run's workspace (cwd through the helper). `npx`, `uvx` and pip caches go to the tenant HOME and can execute there. No tenant can poison another's cache.
- **Backend CLIs** (`backends/_cli_base.py:123`) and quality gates (`qualitygate.py:166`) run in the run's workspace with the run's identity.
  - In required mode, backend CLIs are looked up on the tool PATH, for `backends.health.request` and the availability checks too. A CLI found only in `/app/.venv/bin` is then reported missing instead of failing at run time.
  - Backends installed in `/app/.venv` need `CODEFORGE_TOOL_READ_PATHS=/app/.venv`, and `/app/.venv/bin` on `CODEFORGE_TOOL_PATH`.
  - `working_dir_override` (`_cli_base.py:114`: aider, goose, opencode, plandex, sweagent) is accepted only when it resolves inside the run's workspace, and then it is the cwd. Otherwise the task fails with a clear error (O13).
- **Benchmarks (S7).**
  - `run_agent_benchmark` builds a fresh `AgentLoopExecutor`, and so a fresh `ToolExecutor`, per task, with the task's workspace. It replaces the `_workspace_path` override, which never took effect (`_benchmark_runners.py:141-149`, `evaluation/runners/agent.py:186-189`, `agent_loop.py:225-226`).
  - The task workspace is `/tmp/cf-bench-*`: `mkdtemp` by the worker in `/tmp`, which only it can write, with ACLs set through the fd (D4). It is the Landlock workspace rule of the benchmark identity.
  - Cleanup runs as T, then the worker removes the empty directory (D11).
  - `functional_test.py:87` and `codeforge_synthetic.py:92,123` run with the benchmark identity.
  - Benchmarks stay development-only (`DevModeOnly`, `routes.go:593-600`).

### D8 The sharing pass, the versioned walk, KI-95, the Core's modes, git (question g)

- **Why.** The Core and the worker (group 10010) must be able to read, checkpoint, deliver and delete what T creates. A tool can lock them out:
  - with an owner-only mode (`mkdtemp`, `mkdir -m 0700`, `chmod 600`);
  - with its own group;
  - or, under ACLs, by stripping or masking the `g:10010` entry. Revision 1's `find` predicates missed that last case (E17).
- **`share_tool_files(root)` v3.** It runs as the work item's tenant UID through the launcher, with Landlock limited to the workspace. The walker is `tool_walk.py share` (stdlib plus `posix_acl.py`):
  - `os.fwalk(follow_symlinks=False)`, `openat(O_NOFOLLOW)`, and an `fstat` recheck of inode and owner.
  - For each regular file and directory owned by T:
    - the access ACL must contain `g:10010` with `rw`, plus `x` for directories and for files the owner may execute, and the mask must include those bits;
    - directories also need the default ACL `u::rwx, u:T:rwx, g::rwx, g:10010:rwx, m::rwx, o::---`.
  - It writes with `fsetxattr` only when something differs.
  - Entries T cannot open (mode 0000) are opened `O_PATH` and changed through `/proc/self/fd/<n>`, which names exactly the checked inode. The walker is the exec target, so its own `/proc` is in the rule set.
  - Running as T under Landlock, a symlink swap can reach only T's own files in that workspace.
- **Two modes (O14):**
  - **per call**, after a tool process with a cwd exits (`_share_after_exit`, `tool_process.py:424-441`; `run_tool_process`):
    - only entries whose `st_ctime` is at or after the call's start minus 1 s. Creating, `chmod`, `chgrp` and `setfacl` all set ctime;
    - the walk still visits the tree with `lstat` (about 0.16 s per 100,000 entries, E13), but reads ACLs only of changed entries;
  - **full**, at the end of every run, turn, task and benchmark task, and before a deletion (D11): every T-owned entry. About 0.33 s per 100,000 entries (E13); it writes only on change.
- **No `setfacl` binary is needed.** E1 shows the codec does the same as `setfacl -m g:10010:rwX`.
- **`share_with_tools(path, writable)`** (`:582-597`) becomes `grant_tool_access(fd, identity, writable)`: an exact ACL for the tenant UID on a descriptor of an entry the worker made in a directory only it can write (W1).
- **`WORKSPACE_SHARING_VERSION` becomes "3"** (`:653`). The startup walk keeps only the root work: mode 2771 and the owner check. The per-tenant work is the migration of D9, with its own stamps.
- **The KI-95 readers** (Go `os.Root`, Python `workspace_fs`) run as 10001 with group 10010. They reach the files through the owning group and `g:10010`, and are unchanged. The worker's in-process file tools are still not separated by UID; KI-95 path rules confine them.
- **The Core's modes** are unchanged (0770/0664, which become the masks `rwx`/`rw-`). The Core's own 0600/0700 writes (`deliver.go:169,184`) stay closed to tools, as today.
- **git.** `safe.directory=*` in `/etc/gitconfig` stays: repositories belong to 10001, 10002 or T. KI-77 (the Core's own `safe.directory` per repository) is unchanged.

### D9 Migrating trees from before the upgrade (question d)

**Options:**

- **(A)** A temporary `CAP_CHOWN` in the entrypoint, plus `CAP_FOWNER` and `CAP_DAC_READ_SEARCH` to set ACLs and enter other users' 0700 directories. A root walk then changes the owner from 10002 to T.
- **(B)** No new capability. Every change is made by the entry's owner: 10002 through `setpriv` (the worker keeps `CAP_SETUID`), and the worker as 10001. Access is granted through ACLs, and hard links into other trees are broken by copying.

**Decision: (B).**

**Why not (A):**

1. It needs three more capabilities in `cap_add`. They stay in the bounding set for the container's lifetime, and `docker exec` gets them. They contradict both the owner's constraint and ADR-017, which rejected wider capabilities for the services that hold secrets.
2. A root walk with `CAP_CHOWN` and `CAP_FOWNER` over agent-made trees is a privileged primitive. Any gap in the no-follow and recheck logic becomes owner-change power over the whole volume.
3. Changing the owner does not solve hard links anyway. An inode linked into two tenants' trees would be given to one of them (E9), so the census and the unsharing are needed with (A) as well.

**When (O9).**

- At a tenant's first work item after the upgrade, inside `tool_tenant()`, after the handler's heartbeat started, in a worker thread (`asyncio.to_thread`).
  - The event loop, heartbeats, NATS and `/health` keep running.
  - Progress is logged every 100,000 entries and at the end, with counts and time.
  - The hot-cache cost is about 0.7 s per 100,000 entries (E13). Cold caches and million-entry trees (`node_modules`) take longer; the work item's own timeout covers that.
- Again (self-healing) when the verification (D4) finds a worker-owned tenant directory whose mode or ACL differ, or a stamp that is missing or stale (another T or inode).
- **Never** over a tenant directory that is a symlink or belongs to another user (D4: refused).

**Preconditions (S4).**

- **Locks.**
  - The worker holds the tenant's in-process lock, so no launch for that tenant starts in this worker.
  - It also holds the tenant's exclusive cross-worker lock `<root>/.codeforge/locks/<tenant_id>` (`flock(LOCK_EX)`). Every worker holds `LOCK_SH` for a tenant while it has work of that tenant (D10). With all workers on one host (one volume), this excludes concurrent work of the tenant in every worker.
  - A work item that needs migration releases its shared lock, takes the exclusive one, checks again (another worker may have migrated meanwhile), migrates, and converts back to shared.
  - The wait is bounded at 10 minutes. After that, the work item fails with "tenant workspace migration is waiting for the tenant's other work".
- **No processes.** The pidfd scan (D10) finds no process of T or of 10002 in this container. A 10002 process means an old tool survived, so the migration is refused and logged.
- **Old workers stopped** (Upgrade, step 3). An old worker takes no lock and runs 10002 tools in group 10010, which can reach every tenant tree.

**Steps.**

Every step is fd-based. The walk is `os.fwalk` without following symlinks. Each change is made on a descriptor opened with `O_NOFOLLOW` after an `fstat` recheck of inode, type and owner, as `_share_entry` does today (`tool_process.py:610-647`).

The 10002 steps run through the same helper (`setpriv` to 10002 with group 10010, then `tool_exec.py`, then `tool_walk.py`), under Landlock limited to `<root>/<tenant>` (plus `/usr` and `/etc` for reading the interpreter and the walker file).

- Landlock does not stop metadata changes by path (E10 R2). The walker changes nothing by path: it opens every entry, and Landlock denies opening anything outside the tenant tree.
- Hard links are path-independent. A link inside the tree to another tenant's inode is reachable, and so step 4 rechecks link counts.

1. **As 10002 (`tool_walk.py legacy-open`):** on its own entries, add `g:10010:rwX`, including the mask, so the worker can walk every entry. T has no path into the tree yet.
2. **As the worker: census.** For every regular file with `st_nlink > 1`, count its links inside the tree.
3. **As the worker: unshare.** Every inode with links outside the tree is copied (`O_EXCL|O_NOFOLLOW` in the same directory fd), and the copy is renamed over the name.
   - Content from before the upgrade stays as the tenant's own copy; the tenant had access to it already.
   - The other tenant's live file is no longer reachable (E9: B's `.git/config` and `planted` unchanged after A wrote through its old links).
4. **As 10002 (`tool_walk.py legacy-exact`):** an exact access ACL (plus the default ACL on directories) on its own entries.
   - A regular file whose `st_nlink` is greater than the number of its links counted inside the tree is skipped and logged. That is a link step 3 could not replace.
   - So the walk never rewrites the ACL of an inode that also lives in another tenant's tree. Revision 1 risked a cross-tenant denial of service by dropping `u:T_B` there.
5. **As the worker:** exact ACLs on its own entries. The tenant directory becomes 2770 with `u:T:--x` and the default ACL. This is the last step: only now does T get a path into the tree.
6. **Stamp.** `<root>/.codeforge/tenants/<tenant_id>` = `1 <T> <st_dev> <st_ino>` of the tenant directory, written to a temporary file with `O_EXCL` and renamed into place, relative to the directory's fd.
   - Tools cannot reach the directory (S2).
   - A stamp with another T or another inode, for example after a restore or a swapped directory, is stale.
   - It is read like `_read_stamp` (`:703-737`): `O_NOFOLLOW`, non-blocking, a regular file of at most 64 bytes, owner 10001.

**Adopted workspaces (O17).** When the Core owns the top directory, the same steps run on the adopted workspace. The census counts links inside that workspace. The stamp is `<root>/.codeforge/adopted/<sha256 of the path>`.

**Rollback detection (S3).**

- An old worker walks the root at its start (`WORKSPACE_SHARING_VERSION` "2") and opens its own entries to group 10010. `.codeforge` is included, and legacy tools could then write it.
- At startup, the new worker treats any of these as a rollback:
  - the root stamp is not "3";
  - `.codeforge` has group or other bits, belongs to another UID, or carries an ACL.
- It then renames `.codeforge` aside (`.codeforge.rollback-<time>`, kept for inspection) and creates a fresh one. Every tenant migrates again at its next work item. That is a full re-check of every entry, including ACL entries that legacy tools planted for future UIDs (E9).

**More details:**

- **Unknown owners.** Entries of other users, for example root from an operator's copy, are logged and left alone. T may lack access to them.
- **Cost.** About 0.68 s per 100,000 entries for the setting walk, 0.33 s for an xattr-reading walk and 0.16 s for `find` (E13). Data is copied only for links into other trees.
- **What remains.** Old files stay owned by the retired UID 10002. Tenant tools can read, write, delete and rename them, but cannot `chmod` them (E9: EPERM). Replacing a file makes it the tenant's (E9); editors, `git checkout` and `cp` + `mv` all replace files.
- **Concurrent project creation.** The Core may create projects while a tenant migrates. The tenant directory already has its default ACL from `EnsureTenantDir`, so this is harmless.

### D10 End of work and leftover processes

- **At the end of each work item, in this order (O16; the order was changed in the review round, see Implementation notes):**
  1. its own processes are stopped as today (process-group kill);
  2. it leaves the tenant's in-flight set;
  3. when it was the tenant's last work item in this worker (the tenant goes idle), under the worker's per-tenant lock, so that no new launch for T starts meanwhile (S9): **reap.** For every PID whose `/proc/<pid>/status` shows `Uid:` T:
     - `os.pidfd_open(pid)`;
     - re-read `/proc/<pid>/status` and check that the UID is still T;
     - `signal.pidfd_send_signal(pidfd, SIGKILL)`.

     A PID reused between the scan and the kill gets no signal: the pidfd names the scanned process, and if that one exited, the signal fails with ESRCH. Afterwards the worker waits until the scan finds no T process (bounded, logged).
  4. a full sharing pass as T (D8);
  5. its TMPDIR and `CLAUDE_CONFIG_DIR` are removed as T: `rm -rf --one-file-system` through the launcher, with Landlock limited to the HOME (W1, O4);
  6. when it was the last one:
     - **share:** a full sharing pass on the tenant's other workspaces used since it was last idle (leftovers may have created private files there);
     - **clean:** if `LOCK_EX` on the tenant lock can be taken without waiting (no other worker works for the tenant), everything below `<HOME>/tmp` is removed as T, and the cache cap applies (D7);
     - release `LOCK_SH`.
- **Bounded helpers.** Every worker command that runs as T (the sharing pass, removals, the cache measurement) ends within 600 s, and the migration walks (D9) within 3600 s. After that the worker kills the helper's process group, logs it, and the step counts as failed. Below Landlock ABI 6 signals are not scoped, so a tenant's leftover process can stop the helpers of the tenant's other running work item in that worker; the timeout bounds that wait. A work item that is not the tenant's last cannot reap, because the tenant's other work is running.
- **Behavior change (O16).** A background process an agent starts (a dev server, a watcher) lives at most until the tenant has no work left in that worker.
  - Conversation turns are separate work items, so a dev server started in one turn usually does not survive into the next. Today such `setsid` daemons survive (E10 R4).
  - This is documented for users and in the Bash tool description. A grace period or per-conversation scoping is Open question 9.
- Concurrent runs of the same tenant still share leftovers until the tenant goes idle (Residual risks).

### D11 Deleting workspaces and erasing tenants (O4)

- **Problem.** Under default ACLs, a tool's `mkdtemp` or `mkdir -m 0700` gets mask `---`, so neither the owning group nor `g:10010` grants anything. Only T can remove such an entry (E16).
  - The Core's `os.RemoveAll` (`project.go:226-238`, which only logs a warning) leaves files behind whenever the run-end sharing pass did not run (worker crash, OOM kill, SIGKILL).
  - GDPR project erasure is then silently incomplete.
- **Rule.** Every removal of a tree a tool wrote runs as T through the launcher: `find <dir> -xdev -mindepth 1 -delete`, under Landlock limited to that directory. It comes after the work is stopped and a full sharing pass. The worker then removes the empty top directory with `unlinkat` relative to its parent's fd. The same rule covers TMPDIRs, `CLAUDE_CONFIG_DIR`s, benchmark workspaces and the CLI-check directory.
- **Project deletion** with `workspace.tool_acls: required`:
  1. `ProjectService.Delete` refuses with 409 while the project has an active run, conversation turn or backend task (the check is added where the handler lacks one).
  2. In one transaction, it deletes the project row and inserts a row into `workspace_deletions` (migration 115): `id`, `tenant_id`, `project_id`, `workspace_path`, `tool_uid`, `requested_at`, `done_at`, `attempts`, `last_error`.
  3. It publishes `workspace.delete.request` {`deletion_id`, `tenant_id`, `tool_uid`, `project_id`, `workspace_path`}, Core to worker. Delivery is at least once (ADR-016), and the handler is idempotent: a missing directory counts as success.
  4. The worker runs the accept checks (D3) and takes the tenant's `LOCK_SH`. It then runs, as T:
     - the full sharing pass;
     - the removal of the contents.

     It removes the empty project directory relative to the tenant directory's fd. It marks the tenant's `<HOME>/.cache` for removal at the tenant's next idle (D10 step 3), because build caches can hold data derived from the project and are rebuilt on demand. It publishes `workspace.delete.result` {`deletion_id`, `ok`, `error`}.
  5. The Core marks the row done, or records the error. A periodic job (every 10 minutes) republishes pending deletions. Go treats `workspace.delete.request.dlq` as a failed attempt. GDPR erasure reports list pending deletions, so erasure is verifiable.
  - **NATS wiring:**
    - the subjects go into `queue.go` and `_subjects.py`;
    - the stream gets `workspace.>` (`nats.go:139`);
    - `configs/nats/nats-server.conf` lets the core publish the request and the worker subscribe (durable `codeforge-py-workspace-delete-request`) and publish the result and the `.dlq` copy;
    - contract testdata.
  - With `off` (development): today's `os.RemoveAll`, with failures logged as errors.
- **Tenant erasure.** There is no tenant deletion today; this is KI-112. When it is added, it must, in this order:
  1. refuse while the tenant has work;
  2. delete every project workspace as above;
  3. stop T's processes in every worker (a broadcast through the NotificationHub);
  4. remove the tenant HOME's content as T, then `<base>/<T>` as the worker;
  5. remove `<root>/<tenant>`;
  6. record `retired_tool_uids (uid, tenant_id, retired_at)`, and keep `.codeforge/uids/<T>`.

  A retired UID may be reused only after a reclaim check: no file with that UID, no ACL entry naming it on the volumes, and no process with it. Not in this round.

### D12 Readiness, preflight and host requirements (O2)

- **Readiness.** With isolation required, `/health/ready` answers 503 `{"status": "tool isolation not ready: <reason>"}` until all of these pass: the probe, the ACL checks, the HOME volume checks, the state directory checks and the read-path validation.
  - Today `setup_tool_isolation` only logs (`consumer/__init__.py:526-541`), and the worker shows healthy while every run fails.
  - With 503, the Docker healthcheck fails, and `docker compose up --wait` reports the worker unhealthy.
  - The reason names the remedy.
- **Preflight `scripts/check-host.sh` (new).** It runs the new worker image with the production service definition (`docker compose -f docker-compose.prod.yml run --rm --no-deps --entrypoint ... worker`) against the real `workspaces` and `tool_homes` volumes. It prints:
  - the kernel version, `/sys/kernel/security/lsm` and the Landlock ABI;
  - whether seccomp returns ENOSYS for the Landlock syscalls;
  - the ACL set-and-read-back result on both volumes;
  - the filesystem types and mount flags (`noexec`);
  - the `/tmp` mode;
  - the probe result.

  It exits non-zero on any failure. This is step 2 of the upgrade.
- **Host requirements:**

| Requirement | Works | Does not work -> remedy |
|---|---|---|
| Landlock ABI >= 2, and `landlock` in the LSM list | Debian 12 (6.1, ABI 2, no truncate handling), Amazon Linux 2023 (6.1, ABI 2; 6.12 available), Ubuntu 22.04 HWE and 24.04 (6.8, ABI 4), Debian 13 (6.12, ABI 6), GitHub ubuntu-24.04 runners (6.17, ABI 7) | Ubuntu 22.04 GA (5.15, ABI 1): HWE kernel. Kernels without Landlock in `lsm=`: enable it. There is no production mode without Landlock. |
| Docker's seccomp profile allows `landlock_*` | Docker >= 23.0 (20.10 with the backport; moby PR 43199, backport 43991) | Older engines return ENOSYS, so the worker is not ready: upgrade Docker |
| POSIX ACLs on `workspaces` and `tool_homes` | ext4, xfs, btrfs; ZFS with `acltype=posixacl` | ZFS default (`acltype=off`), NFSv4 (no `system.posix_acl_*`), CIFS: not supported. Use a local filesystem or set `acltype=posixacl`. |
| `tool_homes` not mounted `noexec` | named local volume | a tmpfs (Docker's default is `noexec`) |
| All workers on one host | `flock` across containers on one volume | workers on several hosts sharing storage: not supported |

ABI by kernel version, from the kernel documentation (only ABI 7 on 6.18 was verified here):

| ABI | Kernel | Adds |
|---|---|---|
| 1 | 5.13 | filesystem rules |
| 2 | 5.19 | refer |
| 3 | 6.2 | truncate |
| 4 | 6.7 | TCP |
| 5 | 6.10 | ioctl_dev |
| 6 | 6.12 | signal and abstract-socket scopes |
| 7 | 6.15 | logging |

### Costs

| What | Cost | Source |
|---|---|---|
| Launch (setpriv + helper, compiled stdlib) | about 24 ms (was 3.4 ms) | E8 |
| Each Bash call | the command's launch + the sharing pass's launch (about 24 ms each) + the ctime walk (about 0.16 s per 100,000 entries) | E8, E13 |
| End of each work item | full sharing pass, about 0.33 s per 100,000 entries | E13 |
| `getpwuid` against 10,001 generated entries | +1.1 to 3.2 ms per bash or git start | E20 |
| Migration, once per tenant | about 0.7 s per 100,000 entries, hot cache | E13 |

---

## Docker evidence

**Lab image.** Run on 2026-10-02; the image was removed afterwards. It was `python:3.12-slim` plus git, acl, ripgrep, nodejs 20.19.2, npm 9.2.0, strace and procps, plus `@anthropic-ai/claude-code` 2.1.287. One variant added `openssh-client`.

**Run flags.** Every run used the production worker flags on a fresh named volume, running as the worker the way the entrypoint does (`$LAB/t` held the scripts):

```sh
docker volume create ki96e
docker run --rm -v ki96e:/data/workspaces ki96lab sh -c 'chown 10001:10010 /data/workspaces && chmod 2775 /data/workspaces'
docker run --rm --read-only --cap-drop ALL --cap-add SETUID --cap-add SETGID --cap-add KILL \
  --security-opt no-new-privileges --user 0:0 --tmpfs /tmp \
  --tmpfs /home/codeforge-tools:uid=10001,gid=10001,mode=0711 -v ki96e:/data/workspaces -v $LAB/t:/t:ro ki96lab \
  sh -c 'setpriv --reuid=10001 --regid=10001 --groups=10010 --inh-caps=-all,+setuid,+setgid,+kill \
         --ambient-caps=-all,+setuid,+setgid,+kill -- python3 /t/<script>.py'
```

**Lab scripts.** They are in the session scratchpad: `ki96lab/t/` for E0-E14 and `ki96ops/t/` for the review's E15-E21. `tool_exec.py` is the prototype of D5. Note that it applied Landlock only when the spec had the key; revision 2 makes the key mandatory (S10). The Tests section turns the scripts into permanent tests.

### E0 Environment

```text
$ docker version; docker info
server 29.3.1 api 1.54; driver=overlayfs kernel=6.18.44-fc-v51 os=Ubuntu 24.04.4 LTS seccomp=builtin
$ cat /proc/sys/kernel/yama/ptrace_scope        -> No such file or directory (no Yama)
$ cat /proc/sys/fs/protected_hardlinks /proc/sys/fs/protected_symlinks  -> 1 / 0
$ docker run --rm python:3.12-slim sh -c '...'
13.7 | setpriv from util-linux 2.41.5 | no setfacl/getfacl | /usr/lib/x86_64-linux-gnu/libacl.so.1
libacl1 2.3.2-2+b1 | util-linux 2.41.5-0+deb13u1 | Python 3.12.14, os.memfd_create and os.setxattr present
$ find /usr/local/lib/python3.12 -name '*.pyc' | wc -l   -> 0
$ grep ' /data/workspaces ' /proc/self/mountinfo        -> ... - ext4 /dev/vda rw,...   (stat -f: ext2/ext3)
$ python3 abi.py (landlock_create_ruleset(NULL,0,VERSION), default seccomp)  -> landlock ABI: 7
$ setpriv --help | grep Rules
 Rules: execute,write-file,read-file,read-dir,remove-dir,remove-file,make-char,make-dir,make-reg,make-sock,make-fifo,make-block,make-sym,refer,truncate
   (no scope, net or ioctl-dev)
```

### E1 POSIX ACLs on the named volume (question a; `acl_test.py`, codec on `os.setxattr`)

```text
whoami 10001 10001 [10010]          root 0o42775 10001 10010
getfacl -n /data/workspaces/tA:  user::rwx user:20000:rwx group::rwx group:10010:rwx mask::rwx other::---
   default:user::rwx default:user:20000:rwx default:group::rwx default:group:10010:rwx default:mask::rwx default:other::---
core-created file:  user:20000:rwx #effective:rw-  mask::rw-
A write core file: (0, 'core\na')
A create files (umask 0007): a1 -rw-------+ 20000 10010 | d1 drwxrws---+ | mkdtemp dirs drwx--S---+
getfacl a1: mask::---   mkdtemp dir: mask::--- (default ACL inherited)   d1 (mkdir): mask::rwx
worker access a1 - -  | d1 R W | mkdtemp dirs False False False
B list A: Permission denied | B read A file: Permission denied | B write A dir: Permission denied
B list root: (0, 'tA\ntB')                       <- root 2775 shows tenant IDs; the plan uses 2771
A chgrp d1 to 20000 + setfacl -b + chmod 700:     worker access d1 after strip: False
A share pass chmod g+rwX only:                    worker access d1: False, a1: True
A share pass setfacl -m g:10010:rwX:              worker access d1: True
A chgrp to 10010 (not member): Operation not permitted
A adds an ACL entry for B on its own file: allowed; B reads it: Permission denied (tenant directory blocks the path)
```

### E2 The exec helper (`helper_test.py basic`)

```text
T1  ws write, /etc read, /tmp + /proc/1 denied   rc=1 ok | etc-ok | TOKEN=s3cret | /tmp/x: Permission denied
     | cat: /proc/1/status: Permission denied | /proc/self/cmdline: Permission denied   (child bash, own /proc not granted)
T1b rule path is a symlink (O_PATH|O_NOFOLLOW)  rc=0 (before the fix: the link itself was opened)
     after the type check:                      rc=125 cf-tool-exec: rule path /proc/self is a symlink
T1c missing rule path                           rc=125 cf-tool-exec: rule path /nonexistent: No such file or directory
```

### E3 The Landlock tool battery (`ll_battery.py`: workspace and tenant HOME full; /usr rx; /etc r; /dev/null rw; /dev/{zero,urandom,random} r)

The battery used `PATH=/usr/local/bin:/usr/bin:/bin`, not the worker image's `PATH` (see E15).

```text
===== R0 (no /proc)
shell      rc=0  hi | hd2 | OK              (heredocs, process substitution, mv/cp/ln)
coreutils  rc=0  99999 | ./h | OK
git        rc=0  M a | 1 | OK               (init, commit, stash, stash pop, gc)
python     rc=0  /home/codeforge-tools/20000/tmp | codeforge-tool | 1 | OK   (venv + pip install)
node       rc=0  /home/codeforge-tools/20000/tmp | userInfo ERR_SYSTEM_ERROR | pkg | OK  (npm install)
search     rc=0  ./h:1:HD | ./h:1:HD | OK   (grep, rg)
procdep    rc=0  Error, do this: mount -t proc proc /proc | cat: /proc/self/status: Permission denied
claude     rc=134  Bun v1.4.3 ... panic(main thread): abort() called
outside    rc=0  etc-read | OK | /tmp/outside: Permission denied   (no tenant list, no project rename, no /proc/1/environ)
===== R1 (+/proc read)
procdep    rc=0  PID TTY TIME CMD | 1 ? 00:00:00 sh | 1 | 4 | OK
claude     rc=0  2.1.287 (Claude Code) | OK
(all others as in R0)
```

### E4 The Claude Code CLI with a narrow /proc (`cc_strace.py`, `helper_test.py cc` against a fake Messages API)

```text
$ strace -f ... claude --version     (/proc and /sys opens)
/proc/self/maps (2) /proc/self/cgroup (2) /proc/self/statm /proc/sys/vm/overcommit_memory /proc/sys/vm/mmap_min_addr
/sys/kernel/mm/transparent_hugepage/enabled /sys/fs/cgroup/memory/... /sys/fs/cgroup/cpu/.../cpu.cfs_quota_us
/sys/devices/system/cpu/online /sys/devices/system/node/node1..5
narrow rules: /proc/{pid} r, /proc/sys/vm r, /proc/{stat,meminfo,cpuinfo} r, /sys/fs/cgroup, /sys/devices/system,
              /sys/kernel/mm/transparent_hugepage r
T3a claude --version, narrow /proc   rc=0 2.1.287 (Claude Code)
T3b claude --version, no /proc       rc=-6 ... panic(main thread): abort() called
T3c full turn, narrow /proc: rc=0 events=5 result=success
T3c Bash tool result: "cc-bash-ok\n/bin/bash: line 1: /proc/1/cmdline: Permission denied\nPROC1-DENIED\nPROC-LIST-DENIED\n
                       /bin/bash: line 1: /tmp/ccx: Permission denied\n20000"
```

### E5 Failing closed without Landlock (seccomp profile returning ENOSYS for the three Landlock syscalls)

```text
$ docker run ... --security-opt seccomp=nolandlock-seccomp.json ...
landlock ABI: -1 errno: ENOSYS
T5 helper                        rc=125 cf-tool-exec: Landlock ABI -1 (errno 38) < required 1
T5 setpriv --landlock-access fs  rc=127 setpriv: landlock_create_ruleset failed: Function not implemented
(default profile: landlock ABI: 7)
```

### E6 Pinning the /proc rule (`pin_test.py`; `echo 2 > /proc/sys/vm/drop_caches` on the host at about t=4 s and t=6 s)

```text
unpinned /proc/<pid> rule: DENIED-at-24 DENIED-at-25 DENIED-at-26 ... ok=23     (bash: /proc/8/status: Permission denied)
pinned (O_PATH fd kept):   ok=40
E6b workspace (ext4) + HOME (tmpfs) rules across drop_caches 2 and 3: ws+home-ok=40/40
```

### E7 Processes across tenants and Landlock domains (`xtenant_short.py`, `helper_test.py scope`)

```text
B reads A environ: Permission denied        B reads A cmdline (/proc granted): 'sleep 60'
B kills A: Operation not permitted          B ptraces A: ptrace(PTRACE_SEIZE): Operation not permitted
A (run 1, other domain) reads A2 environ: Permission denied
A (no Landlock) reads A2 environ (same uid): 'PATH=/usr/bin:/bin SECRET=tenant-A-run2-secret'
A (other domain, fs rules only) signals A2: SIGNAL-OK
A (other domain + ABI 6 signal scope) signals A2: Operation not permitted
A (other domain) ptraces A2: Operation not permitted
T4a same tenant, other domain, scoped:   kill: Operation not permitted | abstract connect: PermissionError
T4b same tenant, other domain, fs only:  SIGNAL-DELIVERED | ABSTRACT-CONNECTED
T4c other tenant, fs only (uid differs): kill: Operation not permitted | ABSTRACT-CONNECTED   <- UIDs do not separate abstract sockets
T4d worker (CAP_KILL) killed the scoped tool process: -9
```

### E8 Secrets on argv and launch cost (`helper_test.py leak`, `helper_test.py cost`)

```text
tenant B (Landlock, /proc read) polls /proc/*/{cmdline,environ} for 30 s during
150 KI-71 launches (TOKEN=LEAKME0150..0299 as env -i arguments) and 150 helper launches (memfd spec):
seen in others' cmdline: KI-71 form 150/150, helper 0/150; in environ: 0
launch time: setpriv+env -i 3.4 ms; setpriv + helper (memfd, 10 rules, scope) 96.8 ms
python3 -I -S -c pass: 15.3 ms; 'import ctypes, json': 96.5 ms     (stock image, no .pyc, read-only root)
after python -m compileall: pass 11 ms, import json 22 ms, import ctypes 15 ms, both 24 ms
```

### E9 Migration without CAP_CHOWN (`mig_b.py`, capabilities SETUID, SETGID and KILL only)

The steps in this experiment were path-based and had no Landlock. Revision 2 makes them fd-based and Landlock-confined (D9), and the Docker suite re-runs this scenario.

```text
legacy tool (10002 + group 10010) in A: mkdir src, mkdir -m 0700 priv, ln B/secret.txt stolen.txt, ln B/.git/config bcfg,
   ln mine B/planted, setfacl -m u:20000:rw B/btool.txt (owned by 10002), ln -s B/secret.txt sym  -> LEGACY-DONE
== migrate
(i) share pass as 10002: ok
tA: unshared 3 link(s) ['proj/stolen.txt', 'proj/mine', 'proj/bcfg']; granted as 10002: 4, as worker: 5
tB: unshared 0 link(s) []; granted as 10002: 2, as worker: 4
== check
A reads its legacy files (incl. the 0700 dir):  tool | p | WRITE-OK
A reads B's secret via its old hard link:        B-SECRET   (its own copy of what it could read before the upgrade)
A appends to its old link to B's .git/config:    WROTE -> B's .git/config afterwards: [core]   (unchanged)
A writes 'mine' (linked into B before):          WROTE -> B's planted: hook                     (unchanged)
A uses its planted ACL on B's btool.txt:         Permission denied
B reads its own files:                           B-SECRET hook b
B lists the root / A:                            Permission denied (root 2771, tenant dir u:T:--x)
A renames its project dir:                       Permission denied
A chmods a legacy (10002) file:                  Operation not permitted
A replaces it (cp + mv), then chmod +x:          20000 -rwxrwx--x
getfacl btool.txt: user::rw- user:20001:rw- group::rw- group:10010:rw- mask::rw- other::---   (planted u:20000 gone)
```

### E10 Channels that remain (`resid.py`, every process under the helper with scopes)

```text
R1 B connects to A's 127.0.0.1:9999 (scoped)  GOT b'A-server'                         -> Stage 3 (KI-110)
R2 A run of proj, on its own file in proj2: CHMOD | UTIME | READ-DENIED | WRITE-DENIED | TRUNC-DENIED
                                            -> Landlock does not cover metadata (chmod, utime, setxattr)
R3 A creates SysV shm 0666 -> B attaches A's segment: ATTACHED                         -> IPC namespace shared
R4 tenant A processes after its group was killed: 20000 20 20 sleep 300 (setsid)        -> D10
```

### E11 Compatibility details (`exp.py compat|app`, `devfd*.py`)

```text
mp R0 / +proc:  multiprocessing.Lock() -> PermissionError [Errno 13]     mp +proc+shm: lock ok | [1, 2]
+/dev/shm without read-dir: mp-lock-ok; ls /dev/shm denied
cpu R0: nproc 4 | os.cpu_count 4 | node os.cpus().length 0     cpu +proc: 4 | 4 | 4
venv noapp: PermissionError: '/app/.venv/pyvenv.cfg'           venv +app: hook ran
sock /tmp (no rule for /tmp): connected b'pong' | cat system-prompt: Permission denied
tmpfs ACL ok [(1,7,-), (2,7,10001), (4,5,-), (16,7,-), (32,0,-)]
/dev/stdout etc. with stdout = the worker's pipe: Permission denied in every variant (base, +/proc, +/dev rw)
pipes made inside the domain: inner-pipe | psub-w (both work)
worker pipe 0600 (today), no Landlock:  /dev/stdout: Permission denied | /dev/stderr: Permission denied
worker pipe 0600 (today), Landlock:     same
worker pipe fchmod 0666, Landlock:      out | err
```

### E12 UIDs without passwd entries (`passwd_test.sh`, uid 29999)

```text
without passwd entry: git: unable to auto-detect email address (got 'unknown@...(none)') | id: cannot find name for user ID 29999
                      | node userInfo ERR_SYSTEM_ERROR | ssh: No user exists for uid 29999
/etc/passwd lines: 10018, size 980839 bytes   (generated codeforge-t20000..29999)
with generated entries: git still: unable to auto-detect email address (got 'codeforge-t29999@...(none)')
                        | id -un: codeforge-t29999 | node: codeforge-t29999 | ssh: Connection refused (reaches the network)
id -un 29999 (last entry): 5473 us per call (process start included)
```

### E13 Walk costs, 101,000 entries (`exp3.py walk`, `setwalk.py`)

```text
find -user ! -group (KI-71 pass)  0.16 s     fd walk + getxattr  0.33 s     getfacl -R  1.79 s
fd walk + fsetxattr as owner: 101000 entries in 0.68 s
```

### E14 The Claude Code policy directory with tenant ACLs (`sock.py`: directory 0700 `u:A:r-x`, socket 0600 `u:A:rw-`, prompt `u:A:r--`)

```text
A, Landlock + read rule on the run dir: CONNECTED b'pong' | prompt | rm policy.sock: Permission denied | ln -s: Permission denied
A, Landlock without the run dir:        CONNECTED b'pong' | cat system-prompt: Permission denied
B (other tenant), Landlock:             PermissionError (connect) | rm: Permission denied | cat: Permission denied
```

### E15 The worker's venv PATH under Landlock (review; `ops.py venv`, worker-like image with `/app/.venv/bin` first on PATH, hardened flags, revision-1 rules)

```text
Stage 1 only:              python3 / pip / python3 -m venv   rc=0
Landlock:                  python3 -c ...   rc=1   PermissionError: [Errno 13] Permission denied: '/app/.venv/pyvenv.cfg'
                           pip --version    rc=126 bash: /app/.venv/bin/pip: Permission denied
                           python3 -m venv  rc=1   (same error)
Landlock, PATH without /app/.venv/bin:      rc=0   py-ok /usr/local
```

### E16 Removing tool-created trees as the worker (review; `ops.py tmpclean`)

```text
as T: python mkdtemp in $TMPDIR; mkdir -m 0700 -p priv/inner
worker: shutil.rmtree(TMPDIR entry) -> [Errno 13] Permission denied: '/home/codeforge-tools/20000/tmp/tmp0por6jn1'
worker: rmtree(project)            -> [Errno 13] Permission denied: '/data/workspaces/tA/proj/priv/inner'
left behind: priv, priv/inner and the tool's files
```

### E17 An ACL lock-out the revision-1 sharing pass missed (review; `ops.py sharegap`)

```text
as T: setfacl -R -m g::--- lock; setfacl -R -x g:10010 lock; (same with -d); chmod -R g+rwX lock
      -> user::rwx user:20000:rwx group::--- mask::rwx other::---
revision-1 find passes as T: share-done
worker: list lock/ DENIED | read lock/sub/f DENIED | rmtree -> [Errno 13] Permission denied: '/data/workspaces/tA/proj/lock'
after setfacl -R -m g:10010:rwX (+ -d on directories) as T: R-OK R-OK
```

### E18 A Docker tmpfs as HOME (review)

```text
docker run --tmpfs /home/codeforge-tools ...: /proc/mounts -> rw,nosuid,nodev,noexec
'#!/bin/sh' shim in /h/.npm/_npx/x/node_modules/.bin run via PATH -> sh: 1: srv: Permission denied   rc=126
(a venv python symlink into /usr works)
docker run --memory=128m --memory-swap=128m --tmpfs /home/codeforge-tools: 60 MB python "worker" + dd 120 MB into the tmpfs
      -> dd succeeded; the worker exited 137 (OOM kill)
```

### E19 Workflows under the revision-1 rule set (review; `ops.py misc|procs|java`)

```text
python3 -c 'import os; os.openpty()'  -> PermissionError [Errno 13]
script -qc ...                        -> script: failed to create pseudo-terminal: Permission denied
open('/dev/tty')                      -> [Errno 13]
sleep 30 & ps                         -> Error, do this: mount -t proc proc /proc;  pkill -f 'sleep 30' -> 1 (no match)
df -h .                               -> cannot read table of mounted file systems: Permission denied
/proc/self/cgroup, /proc/self/mountinfo in a child -> [Errno 13]
OpenJDK 21 under Landlock: java.io.tmpdir = /tmp
```

### E20 passwd lookup cost (review; `pwperf.py`, 200 launches each via setpriv, default vs generated passwd of 10,020 lines, 781 KB)

```text
bash -c true                  3.12 -> 4.15 ms (uid 20000)   3.08 -> 6.31 ms (uid 29999)
git var GIT_COMMITTER_IDENT   3.00 -> 5.20 ms (uid 29999)
true                          2.57 -> 4.84 ms
```

### E21 useradd at image build (review)

```text
docker run python:3.12-slim bash -c 'for u in $(seq 20000 20199); do useradd -M -u $u ...; done'   2.82 s
/etc/subuid: 200 lines, last 'codeforge-t20199:13141664:65536'
(65,536 subordinate IDs per non-system user; SUB_UID_MAX 600,100,000 runs out near the 9,156th user)
```

---

## Affected files

### Go

| File | Change |
|---|---|
| `internal/adapter/postgres/migrations/114_tenant_tool_uid.sql` (new) | D2 |
| `internal/adapter/postgres/migrations/115_workspace_deletions.sql` (new) | D11 |
| `internal/domain/tenant/tenant.go`, `internal/domain/tenant/tooluid.go` (new) | `ToolUID *int`, constants, `ErrToolUIDRangeExhausted`, validation |
| `internal/adapter/postgres/store_tenant.go` | Read `tool_uid`; `AllocateToolUID` (`FOR UPDATE`); `AdvanceToolUIDSequence`; map `2200H` |
| `internal/adapter/postgres/store_workspace_deletion.go` (new) | `workspace_deletions` store |
| `internal/adapter/http/routes.go` | `RequirePlatformAdmin` on `POST /tenants` |
| `internal/adapter/http/handlers_settings.go` | `tool_uid` for platform admins only; 503 on exhaustion at dispatch |
| Mocks: `internal/middleware/teststore_test.go`, `internal/adapter/http/handlers_test.go`, `internal/service/runtime_test.go`, `internal/service/project_test.go` | Interface |
| `internal/service/tenant_tooluid.go` (new) | `ToolUIDFor`: cache plus lazy allocation |
| `internal/port/messagequeue/queue.go`, `schemas_run.go`, `schemas_conversation.go`, `schemas_benchmark.go`, `schemas_workspace.go` (new), `testdata/`, `contract_test.go` | `ToolUID` field; `workspace.delete.*`; DLQ-republish and handoff contract tests |
| `internal/adapter/nats/nats.go` (`:139`) | `workspace.>` in the stream |
| `configs/nats/nats-server.conf` | Permissions and the durable for `workspace.delete.*` |
| `internal/service/runtime.go`, `runtime_gate.go`, `conversation_dispatch.go`, `conversation.go`, `autoagent.go`, `benchmark_run.go`; `agentbackend.Execution`; the handoff start path | Set `tool_uid` (handoffs: from the claimed tenant) |
| `internal/workspaceacl/` (new: `acl.go`, `acl_linux.go`, `acl_other.go`, `ensure.go`, tests, `testdata/acl_vectors.json`) | Codec, `EnsureTenantDir`, Linux only |
| `internal/config/config.go` (`Workspace`, `:372`), `codeforge.example.yaml` | `workspace.tool_acls` + env + validation |
| `internal/service/project_workspace.go` | `EnsureTenantDir` before clone (`:47`) and init (`:205`) with `required`; adopt ACL (`:60-118`) |
| `internal/service/project.go` (`:226-238`), `internal/service/workspace_deletion.go` (new) | Deletion through the worker, active-work check, retry job, result handler |
| `internal/domain/project/workspace_perm.go` | Comment: ACL masks |
| `cmd/codeforge/main.go` | Startup: sequence advance from the on-volume bindings, adopted-workspace log, warning for production with `off` |
| `tests/integration/` | `resetToolUIDSequence` helper |
| `Dockerfile` (`:39-42`) | Root 2771; `CODEFORGE_WORKSPACE_TOOL_ACLS=required` |
| `go.mod` | `golang.org/x/sys` becomes direct |

### Python

| File | Change |
|---|---|
| `workers/codeforge/tool_identity.py` (new) | `ToolIdentity`, context variable, `tool_tenant()`, accept checks, identity environment, system identity |
| `workers/codeforge/tool_state.py` (new) | `<root>/.codeforge`: stamps, UID bindings, `flock` locks, rollback detection, HOME base fd and verification |
| `workers/codeforge/tool_exec.py` (new) | Helper (D5, D6) |
| `workers/codeforge/tool_walk.py` (new) | Owner-run fd walker: `share`, `share-changed`, `legacy-open`, `legacy-exact` |
| `workers/codeforge/tool_migration.py` (new) | D9 orchestration and the worker's steps |
| `workers/codeforge/posix_acl.py` (new) | Codec; shared vectors |
| `workers/codeforge/tool_process.py` | `launch()` with the memfd spec, `pass_fds` and `prepare`; pipes with `fchmod`; `--clear-groups`; umask 007; base interpreter; Landlock rule builder and ABI detection; read-path validation; probe v2; `grant_tool_access(fd)`; removal as T; own MCP stdio transport; pidfd reaper; sharing stamp "3" |
| `workers/codeforge/subprocess_env.py` (`:32-82`, `:166`) | Identity environment, set variables, dropped operator values, tool PATH |
| `workers/codeforge/config.py` (`:172-178`, `:272-278`) | `tool_landlock`, `tool_landlock_min_abi`, `tool_read_paths`, `tool_path`, `tool_home_base`, `tool_cache_max_mb`; drop `tool_uid`/`tool_gid`/`tool_home` |
| `workers/codeforge/models.py` | `tool_uid` on six models; `WorkspaceDeleteRequest`/`Result` |
| `workers/codeforge/consumer/__init__.py` (`:516-556`), `_runs.py`, `_conversation.py`, `_tasks.py`, `_quality_gate.py`, `_workspace_test.py`, `_benchmark.py`, `_benchmark_runners.py`, `_backend_health.py`, `_workspace_delete.py` (new), `_subjects.py` | Identity per message after the heartbeat; accept checks; end-of-work order; readiness reasons; deletion handler; fresh executor per benchmark task |
| `workers/codeforge/health.py` | 503 reason "tool isolation not ready: ..." |
| `workers/codeforge/claude_code_executor.py` | Gated: run directory ACLs through the fd, hook interpreter, per-run config directory, CLI check as the system identity |
| `workers/codeforge/mcp_workbench.py` (`:72`) | Workspace as cwd |
| `workers/codeforge/subprocess_utils.py` (`:57`) | System identity for version checks; lookups on the tool PATH |
| `workers/codeforge/backends/_cli_base.py` (`:114`, `:123`) | `working_dir_override` only inside the workspace; CLI lookup on the tool PATH |
| `workers/codeforge/tools/bash.py` | Description: `ps`/`pkill` unavailable; background processes end with the work |
| `workers/codeforge/evaluation/runners/agent.py` (`:72-107`, `:186-189`), `evaluators/functional_test.py`, `providers/codeforge_synthetic.py` | Benchmark identity and per-task workspace |
| `tools/search_files.py`, `agent_loop.py`, `qualitygate.py` | No signature change (context variable); verify cwd and identity |

**Tests:** `tests/test_tool_process.py` (scan allowlist, launch), `tests/tool_isolation_check.py`, `tests/test_tool_isolation_integration.py`, `tests/test_deployment_isolation.py`, `tests/test_nats_permissions.py`, and the new files under Tests.

### Images, deployment, scripts, CI

- **`Dockerfile.worker`:**
  - add the `acl` package;
  - run `python -m compileall -q /usr/local/lib/python3.12`;
  - write passwd and group lines for 19999 and 20000-29999 with a `printf` loop (no `useradd`, no subordinate IDs);
  - rename 10002 to `codeforge-tool-legacy` and remove it from `codeforge-ws`;
  - set the root to 2771;
  - create `/home/codeforge-tools` (10001:10001, 0711) and `/var/lib/codeforge/landlock-canary` (0644);
  - add the git identity to `/etc/gitconfig`;
  - remove `CODEFORGE_TOOL_UID`, `CODEFORGE_TOOL_GID` and `CODEFORGE_TOOL_HOME`. `CODEFORGE_TOOL_ISOLATION=required` stays, and Landlock follows it.
- **`docker-compose.prod.yml`** (`:300-338`):
  - a volume `tool_homes:/home/codeforge-tools` replaces the `/home/codeforge-tool` tmpfs;
  - `/tmp:uid=10001,gid=10010,mode=1771`;
  - the Core gets `CODEFORGE_WORKSPACE_TOOL_ACLS: required`;
  - the capabilities stay unchanged.

  The blue-green overlay gets the same Core variable.
- **`scripts/worker-entrypoint.sh`:** comments only.
- **`scripts/check-tool-isolation.sh`:** defaults to the built worker image; root 2771; the HOME volume; two tenants; Landlock; the argv check.
- **`scripts/check-host.sh` (new):** preflight (D12).
- **`scripts/deploy-blue-green.sh`:** comment on the upgrade order.
- **`.github/workflows/ci.yml`:**
  - root integration tests under `sudo -E`;
  - a Docker job on the built image;
  - print the LSM list and the ABI;
  - `CI=true` makes isolation skips fail.
- **`.claude/hooks/session-start.sh`, `.devcontainer/setup.sh`:** install `acl`.

### Docs (by the lead)

- ADR-018, plus a note in ADR-017.
- `AGENTS.md`: the workspace, tool-user and HOME lines in section 3; the new subjects in section 7.
- `docs/architecture.md` (process and UID model) and `docs/SECURITY.md`.
- `docs/dev-setup.md`: new variables and switches, upgrade steps, preflight, host requirements.
- `docs/tech-stack.md`: the `acl` package.
- `docs/disaster-recovery.md`:
  - ACL-preserving backups;
  - restoring the database and the workspaces volume to the same point;
  - `tool_homes` is not backed up (caches).
- `docs/data-retention.md`: tenant caches, deletions through the worker.
- `docs/api/openapi.yaml`: `POST /tenants` is for platform admins; `tool_uid`.
- The agent-orchestration feature doc: lifetime of background processes; tools that do not work under Landlock.
- `docs/todo.md`: KI-96, plus the new KI-110, KI-111 and KI-112.
- `docs/known-issues-fix-plan.md`: S7-H.

### Order of work (atomic commits; TDD per step)

1. `fix(worker): hand the tool environment over on a memfd, not on argv`: the helper without Landlock (mandatory spec fields, credential self-check), plus the `/proc` poll test. **Ships first; it fixes the leak of item 4 on its own.**
2. `fix(worker): tool processes can reopen their stdio pipes (/dev/stdout)`.
3. `feat(db): lazily allocated tool UID per tenant`: migration 114, domain, store, allocation, platform-admin tenant creation, `tool_uid` for platform admins, the test sequence reset.
4. `feat(nats): tool_uid on the payloads that start tool processes`: Go, Python, contract (DLQ republish, handoff).
5. `feat(workspace): POSIX ACL codec, tenant directories and the workspace.tool_acls switch`: Go (Linux and stub) and Python, shared vectors, `EnsureTenantDir`, root 2771, sequence advance, adopted-workspace log.
6. `feat(worker): per-tenant tool identity`: context variable, identity environment and tool PATH, HOME volume through the fd, `prepare`, state directory, UID bindings, launch-time verification, launcher, Claude Code (gated), MCP transport, benchmarks with an executor per task, system identity, `working_dir_override`.
7. `feat(worker): ACL-aware sharing pass as the tenant UID` (D8).
8. `feat(worker): migrate pre-upgrade workspaces per tenant` (D9: fd walkers, locks, rollback detection).
9. `feat(worker): Landlock per tool call` (D6: mode, minimum ABI 2, refused off in production, probe v2, readiness 503).
10. `feat(worker): end-of-work cleanup and tenant reaper` (D10: order, pidfd, cache cap).
11. `feat(workspace): delete project workspaces through the worker` (D11: migration 115, subjects, NATS config, Core flow).
12. `build: worker image and production compose for per-tenant tool users`.
13. `test: Docker tenant-isolation suite on the built image, host preflight and CI jobs`.
14. Docs (lead).

---

## Migration and upgrade

1. **Back up first.** Back up the workspaces volume with a tool that keeps POSIX ACLs and xattrs (`tar --acls --xattrs`, or a filesystem snapshot).
   - A restore without ACLs fails closed. The stamps' inode check then fails, and each tenant migrates again at its next work item.
   - Files a tenant tool created as private (0600) before a restore without ACLs are healed only by that migration.
2. **Check the host.** Run `scripts/check-host.sh` with the new worker image against the real volumes (D12). Do not continue while it fails.
3. **Stop every worker:** `docker compose stop worker`, with all replicas when running `--scale worker=N`. No old worker may run during or after the upgrade:
   - an old worker runs tools as 10002 in group 10010, which can reach every tenant tree and plant directories and ACL entries (S3);
   - it takes no tenant lock (D9).
4. **Deploy the Core.**
   - Compose: `docker compose up -d core`.
   - Blue-green: start the new color, switch, then stop the old color. `deploy-blue-green.sh` handles only the Core and frontend colors (`:143-171`), so the worker steps 3 and 5 are done by hand.
   - The Core applies migrations 114 and 115, advances the UID sequence over the on-volume bindings (there are none on a first upgrade), and logs adopted workspaces that need an operator's `setfacl`.
5. **Start the new worker(s).** At startup the worker:
   - sets the root to 2771;
   - creates and verifies `.codeforge` and detects a rollback;
   - checks the HOME volume and ACL support;
   - runs the probe.

   `/health/ready` answers 503 until the worker is ready.
6. **First work item per tenant.** It migrates that tenant's tree in a thread, and a log line gives counts and time.
- **Mixed versions.**
  - A new worker with an old Core: payloads without `tool_uid` are refused with "payload without tool_uid: the Go Core is older than this worker or runs with workspace.tool_acls=off".
  - An old worker with a new Core: excluded by step 3.
7. **Rollback.**
   - Stop the new workers, then start the old images.
   - Old tools (10002, group 10010) keep working through the owning group and the `g:10010` entries the sharing pass maintains.
   - Do not run the down migrations of 114 and 115 unless the workspaces are reset as well: files keep their numeric tenant UIDs.
   - Re-upgrading: the new worker detects the rollback (D9) and migrates every tenant again.
8. **Development:** nothing changes. `CODEFORGE_TOOL_ISOLATION` and `workspace.tool_acls` are off, and Landlock follows isolation. E2E setup is unchanged.

---

## Tests (question h)

### Go unit

- **Codec.** The `workspaceacl` codec against the shared vectors (encode, decode, sort order, mask computation).
- **`EnsureTenantDir`** (Linux): mode, ACLs, refusal of a symlink and of a foreign owner. On non-Linux builds the stub returns `ErrUnsupported`.
- **Config.** `workspace.tool_acls` parsing (an unknown value counts as `required`, and is logged).
- **`project_workspace`.**
  - With `off`: today's `MkdirAll`, and no ACL code runs.
  - With `required`: a fake git provider asserts that the tenant directory exists with its ACL before `Clone` runs.
- **Payloads.** Every payload builder sets `tool_uid` with `required` and omits it with `off`. Contract round trips run in both directions.
- **Contract (S11).**
  - No `.dlq` subscriber in the Core publishes onto the seven subjects. This is a recording-queue test over every DLQ handler, plus a source scan.
  - A handoff whose metadata names another tenant or UID starts a run with the claimed tenant's UID.
- **Routes.** `POST /tenants` answers 403 for a tenant admin and 201 for a platform admin. `tool_uid` appears in tenant JSON only for platform admins.
- **Project deletion.** 409 with active work. One transaction for the row delete and the `workspace_deletions` insert, then a publish. Result handling, retry and DLQ.

### Go integration (`-tags=integration`, PostgreSQL)

- **Backfill:** only tenants with projects, in `created_at` order; NULL for the others.
- **Allocation:** two goroutines allocating for the same tenant get one UID, with no gap in the sequence.
- **Constraints:** the trigger allows NULL to a value and raises on a value to another value. `UNIQUE` and the CHECK constraint hold.
- **Exhaustion:** `setval` close to 29999, two first dispatches, then `ErrToolUIDRangeExhausted` and a 503.
- **Sequence advance:** from a seeded `.codeforge/uids/` directory.
- **Test helper:** `resetToolUIDSequence`.
- **Deletions:** the `workspace_deletions` lifecycle with NATS.

### Python unit

- **Spec and launch.**
  - A property test over random environments: no environment value appears in `argv`.
  - A missing or unknown `landlock` value makes the helper exit 125. `"off"` is written only in `off` mode.
  - The credential self-check: wrong uid, a group, or a capability exits 125.
  - The cwd is reached with `fchdir`, and no `cwd=` reaches Popen.
- **Rule builder.**
  - Per identity, with the extras read-only and the hook as one file.
  - A symlink in any component of a rule path is refused.
  - The base interpreter's prefix gets a rule when it is outside `/usr`.
  - Read-path validation refuses `/`, `/run`, `/data`, ancestors of the canary and symlinks.
- **ABI and mode.**
  - Handled-rights masks and attribute sizes per ABI (1-7, injected). The minimum ABI defaults to 2.
  - An unset `CODEFORGE_TOOL_LANDLOCK` with isolation required counts as `required`.
  - `off` with `APP_ENV=production` makes the worker not ready.
- **Identity environment (S6, O1, O7).**
  - Every cache, config, data and temp variable is set below HOME.
  - Operator values are dropped, with one warning each.
  - `PATH` comes from `CODEFORGE_TOOL_PATH`, never from `os.environ`.
  - `JAVA_TOOL_OPTIONS` and `TMUX_TMPDIR` are set.
  - With isolation off, the environment is unchanged.
- **Accept checks.**
  - Range and reserved UIDs.
  - The missing-`tool_uid` message.
  - A binding that names another tenant is refused.
  - A workspace outside `<root>/<tenant>` is refused, and so is a `working_dir_override` outside the workspace.
  - Error messages name T.
- **Context.**
  - The context variable propagates into child tasks (sub-agent, MCP).
  - A tenantless spawn raises `ToolIsolationError`, and only the marked calls use the system identity.
  - `tool_tenant()` is entered after the heartbeat started.
- **W1 symlink plants (S1).** Symlinks to another tenant's directory and to the root are planted at:
  - `<HOME>/tmp` and `<HOME>/tmp/<token>`;
  - `<HOME>/claude` and `<HOME>/claude/<token>`;
  - `<HOME>/.cache`;
  - inside a benchmark workspace.

  `prepare` fails with 125, removals as T stay inside the HOME, and the target's files, modes and ACLs are unchanged. A HOME whose owner or inode changed is refused.
- **State directory (S2, S3).**
  - Stamps are not readable or writable by a tool UID.
  - A moved stamp (inode mismatch) and a stamp with another T are stale.
  - Forged stamps are ignored: foreign owner, symlink, FIFO, oversize.
  - Rollback detection: a `.codeforge` opened to the group, or a root stamp of "2", makes the worker rename `.codeforge` aside and treat every tenant as needing migration.
- **Tenant directory verification.** A foreign owner and a symlinked tenant directory are refused, never migrated. An extra named ACL entry or a mode drift triggers a migration.
- **Sharing pass v3 (O5).** The E1 and E17 cases: `chmod 600`, `chgrp`, a stripped `g:10010`, `g::---` with the mask, a stripped default ACL, a 0000 file. The ctime mode skips entries that were not touched.
- **Migration (S4).**
  - Census, and unsharing with links inside and outside the tree.
  - Step 4 skips an inode whose link count exceeds its links inside the tree.
  - Planted ACL entries are removed.
  - **Race test:** a thread swaps a directory for a symlink into another tenant's tree while the walk runs. The other tenant's modes and ACLs stay unchanged.
  - The lock waits are bounded, and a 10002 process blocks the migration.
- **Reaper (S9).**
  - The pidfd path, with a PID that exits between the scan and the kill (mocked `/proc`).
  - The per-tenant lock blocks launches while reaping.
  - The end-of-work order: stop, share, clean, and at idle reap, share, clean.
- **Claude Code (O18).** In required mode: the per-run config directory and the passthrough without `CLAUDE_CONFIG_DIR`. In `off` mode: the environment is unchanged.
- **Other units.**
  - MCP transport: a real echo server through `pass_fds`.
  - Pipes: `fchmod`.
  - The scan allows `tool_exec.py` and `tool_walk.py`.
  - Benchmarks (S7): a task's tool calls run in that task's workspace.
  - Readiness: each not-ready reason gives 503.

### Python integration (root plus setpriv, the pattern of `test_tool_isolation_integration.py`)

Two tenants:

- A cannot list, read or write B's files, and cannot rename its own project directory.
- A cannot read B's `/proc/<pid>/environ` or `/proc/<pid>/cmdline`, cannot signal B (`os.kill` gives EPERM), and cannot ptrace B (PTRACE_SEIZE through ctypes gives EPERM).
- **cmdline poll.** A poller as B reads `/proc/*/{cmdline,environ}` during 200 launches for A with unique tokens, and finds none.
- **Landlock denies:** `/tmp`, the canary, another project of the same tenant, `/proc/1/status`.
- **Landlock allows:**
  - the workspace and HOME;
  - `/dev/shm` without listing;
  - `/dev/stdout` (with the `fchmod` fix);
  - `os.openpty()` and `script`;
  - executing a script from TMPDIR.
- **Scope** (ABI >= 6): a same-tenant cross-domain `kill` and an abstract `connect` are denied; the worker can still kill.
- **CI (O10):** the suite runs with `sudo -E`. With `CI=true`, a skip for a missing root, setpriv, ACL support or Landlock fails the test instead.

### Docker integration (`scripts/check-tool-isolation.sh` extended, plus a `docker`-marked `workers/tests/test_tenant_isolation_docker.py`)

It runs on the **built worker image**, with the production service definition: `docker-compose.prod.yml` plus a test override that mounts the tests and stubs the secrets. It does not use `python:3.12-slim` or a hand-copied set of flags (O1, O10). A battery image `FROM` the worker image adds node, go and a JDK for the toolchain cases.

- **Earlier evidence as assertions:** E1, E3 (with the image's `PATH`), E7, E8, E9 and E14 to E19, with the fixes applied:
  - venv-free `python3`/`pip`;
  - npx shims and `go test` from the HOME volume;
  - a pty;
  - the java tmpdir;
  - removal as T;
  - the ACL lock-out healed.
- **Failing closed and visibly:**
  - an ENOSYS seccomp profile gives 503, a Bash tool call returns `ToolIsolationError`, and a marker file stays absent;
  - Landlock `off` with `APP_ENV=production` gives 503;
  - the old `/home/codeforge-tool` tmpfs compose gives 503;
  - an ACL-less volume gives 503.
- **Migration:**
  - a seeded legacy tree with cross-tenant hard links, planted ACL entries and a 10002-owned replacement tenant directory (refused);
  - a 1,000,000-entry tree after `drop_caches` (sudo), measuring time and checking that heartbeats keep coming.
- **Rollback:** new image, then old image (its root walk), then new image. The rollback is detected, and every tenant is migrated again.
- **Two workers on one volume:** the tenant lock keeps a migration out while the other worker runs the tenant's work.
- **Project deletion through the worker:** it removes a tree with 0700 / mask `---` directories and a stripped ACL.
- **Image:** exactly 10,001 generated passwd entries; no `/etc/subuid` growth.
- **CI job:**
  - prints `uname -r`, `/sys/kernel/security/lsm` and the Landlock ABI;
  - the `drop_caches` pinning test (E6) runs with `sudo` instead of being a manual check;
  - the hosted ubuntu-24.04 runners currently report kernel 6.17 (ABI 7), so the scope assertions run;
  - with `CI=true`, skips fail.

---

## Residual risks

1. **Network and local IPC across tenants (KI-110, Stage 3).** Not closed in this round:
   - loopback TCP (E10 R1), and every internal service on the worker's network (`core:8080`, `litellm:4000`, postgres, nats; all need credentials);
   - abstract unix sockets on kernels with ABI < 6 (E7 T4c);
   - SysV IPC and POSIX message queues, because the IPC namespace is shared (E10 R3);
   - **`/dev/shm`**: one tenant can squat on names another tenant's POSIX semaphores or shared memory will use, or fill the shared 64 MB. Files created there are 0660 and private to the creating tenant, so their contents stay unreadable to other tenants.
2. **Within a tenant, Landlock does not cover metadata.** A run can `chmod`, `utime` and `setfacl` its tenant's files in other projects (E10 R2). With ABI 2 it can also truncate them. Reading and writing there are denied.
3. **The tenant HOME is shared by the tenant's runs,** including across workers through the volume. A run can plant `~/.bashrc` or caches for the next run of the same tenant. The per-work TMPDIR separates temporary files; a per-run HOME is Open question 8.
4. **Leftover daemons and resources.**
   - Leftover daemons live until the tenant goes idle in that worker (D10).
   - There are no cgroups or quotas. CPU, memory, pids and the disk space of `tool_homes` and `workspaces` are shared, so one tenant can deny service to the others.
   - The cache cap applies only at idle.
5. **Tools under Landlock cannot see `/proc` beyond their exec target's own entry.** `ps`, `pkill`, `df`, `ss`, psutil, the JVM's container detection and `/proc/self/*` reads in child processes fail. Some test suites may fail.
6. **Files from before the upgrade stay owned by the retired UID 10002.** Tenants cannot `chmod` them until they replace them (D9).
7. **Outside production, Landlock `off` exposes command lines across tenants:** agent commands, backend prompts, and MCP stdio server arguments that carry credentials. That is acceptable for development only. The MCP UI should still advise putting credentials into the environment.
8. **The worker is trusted for every tenant.** It holds `CAP_SETUID` and can become any tenant UID, as today. A compromised worker is all tenants. Its in-process file tools are confined only by KI-95 path rules, not by UIDs.
9. **Platform Claude credentials (KI-111, proposed, not verified in a CodeForge run).**
   - The Claude Code CLI gets `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN` and `CLAUDE_CODE_OAUTH_TOKEN` (`claude_code_executor.py:65-71`), and its Bash tool inherits the environment (Claude Code's sandbox docs: "Environment variables are inherited").
   - So an agent can read the platform's Claude credential and write it into its workspace.
   - Options: a credential proxy outside the boundary (Claude Code's hosting guidance), or `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB=1`, which also forces Claude Code's own filesystem isolation on (to be tested in a container).
10. **Same-UID ptrace within one domain** is allowed: there is no Yama on the test host. A run's commands can trace the Claude Code CLI of the same run. That gives nothing beyond the Bash call that is already allowed.
11. **Host dependencies** (D12): ACL support on both volumes, Landlock in the LSM list, an ABI of at least 2, a Docker seccomp profile that allows the Landlock syscalls, and all workers on one host. Each failure fails closed and shows as 503. Hosts below ABI 2 have no production mode.
12. **Backups** that drop ACLs fail closed. Tenant files then need the re-migration.
13. **Per-call sharing-pass cost.** Every Bash call walks its workspace (about 0.16 s per 100,000 entries). On very large trees this is noticeable (Open question 10).

---

## Open questions

**Owner decisions (2026-10-03)** on the questions below:

- 2, range and passwd: the fixed range 20000-29999 with static passwd entries stays.
- 3, minimum Landlock ABI: 2 stays the production minimum. Below ABI 6 the worker logs a warning and reports it in `/health`, and a setting can require ABI 6 (follow-up in `docs/todo.md`).
- 4, old files: no re-owning; ACL access is enough.
- 7, `/proc` access: no switch.
- 8, HOME layout: one HOME per tenant with the 4 GB cache cap stays.
- 10, sharing pass: the walk per call stays; it is measured on large repositories (KI-103) before a targeted pass is considered.
- 11, tenant caches: cleared at every project deletion.
- 12, UID reclaim: a documented operator procedure until tenant deletion exists (KI-112).
- 5, operator `CLAUDE_CONFIG_DIR`: not passed through in production, as implemented; operators use `CLAUDE_CODE_OAUTH_TOKEN` (`claude setup-token`) or `ANTHROPIC_API_KEY`. Per-tenant Claude credentials are a follow-up together with KI-111.
- 6, hosts below ABI 2: no production mode, as implemented; such hosts install a newer kernel (Ubuntu 22.04: the HWE kernel).
- 9, background processes: scoped per conversation or run. A tool process belongs to the work that started it and lives while that conversation or run is active, with an idle timeout, instead of ending when the tenant is idle; a subreaper per conversation also collects orphans (KI-113). A follow-up with its own plan; until then the tenant-idle reaping stays.

1. **ADR number.** The KI-25 plan (`ki25-subagents-plan.md:5`) and the KI-88 plan (`ki88-opaque-submodules-plan.md:10`) both name ADR-018. The ADR that lands first takes 018, and the plans need renumbering. **Resolved (2026-10-03):** KI-96 landed first and takes ADR-018; the KI-25 plan now names ADR-019 (`019-subagents.md`) and the KI-88 plan ADR-020 (`020-opaque-nested-repositories.md`).
2. **Range and passwd.** Should 20000-29999 stay fixed, with a sequence, a CHECK constraint and passwd entries generated at image build? And should the passwd entries stay static? They cost 1.1 to 3.2 ms per process start (E20). Alternatives are an indexed NSS backend (libnss-db) or libnss-extrausers with a worker-maintained file holding only the active tenants.
3. **Minimum Landlock ABI in production.** The default is 2. Should production demand 3 (truncate handled) or 6 (kernel 6.12+, so that abstract sockets and same-tenant signals are always scoped)?
4. **Re-owning old files.** Should old files be re-owned by copying them as T (a later command, or part of D9), so tenants can `chmod` them? It costs I/O proportional to the tree size.
5. **Operator `CLAUDE_CONFIG_DIR` in production.** Dropping it from the passthrough in required mode breaks operators who keep `.credentials.json` there. They would move to `CLAUDE_CODE_OAUTH_TOKEN` or `ANTHROPIC_API_KEY`. Development is unchanged. Acceptable?
6. **Hosts below ABI 2** (Ubuntu 22.04 GA) have no production mode. Is that acceptable, or should there be an explicit unsafe mode (`off` plus an acknowledgement variable, logged on every start)?
7. **`/proc` access and argv.** Should operators get a switch for read access to all of `/proc` (compatibility)? It would first need the Bash command and the backend prompts moved off argv: the command in the memfd spec with a fixed `bash -c 'eval "$CF_TOOL_COMMAND"'` wrapper, and aider's `--message-file`. Agents' child processes (`curl -H ...`) would still be visible.
8. **HOME layout.** Per-run versus per-tenant HOME (Residual 3); the default cache cap; disk quotas.
9. **Background processes.** Keep the reaping at tenant idle, or give a grace period, or scope processes per conversation (a subreaper helper or pidfd tracking per work item)?
10. **Per-call sharing pass.** Should the walk per Bash call be replaced by a targeted pass (a change list from the tool), or by an on-demand pass when the worker hits EACCES? That would avoid walking large trees after every call.
11. **Tenant caches at project deletion.** Clear `<HOME>/.cache` at every project deletion (chosen: build caches can hold data derived from the project), or only at tenant erasure?
12. **UID reclaim** for deleted tenants (KI-112): automate the reclaim check, or leave it as a documented operator procedure?

### New Known Issues for `docs/todo.md`

- **KI-110 Tool processes share the network and local IPC across tenants** (medium). Stage 3 of S7-H:
  - an egress proxy with per-tenant allowlists, in the manner of Claude Code's sandbox-runtime proxy;
  - Landlock TCP rules (ABI 4) limiting `connect` to the proxy and allowed internal ports, and `bind`;
  - the abstract-socket scope required (ABI 6);
  - SysV and POSIX IPC, and name squatting and capacity in `/dev/shm`.

  Evidence: E7 T4c and E10 R1 and R3. Found in the S7-H planning (2026-10-02).
- **KI-111 Claude Code runs expose the platform's Claude credentials to agent commands** (medium, to verify). See Residual 9.
- **KI-112 No tenant erasure; tool UIDs of deleted tenants are never reclaimed** (low). There is no tenant deletion. When it is added, it must:
  - delete every project workspace through the worker;
  - stop the tenant's processes in every worker;
  - remove its HOME as T and its tenant directory;
  - record the UID in `retired_tool_uids`.

  A UID may be reused only after a reclaim check (no files, no ACL entries naming it, no processes). See D11. Found in the S7-H review (2026-10-02).

---

## Implementation notes

Implemented on 2026-10-02 and 2026-10-03 in plan commits 1 to 13 (`3809b201` to `0c53e4a1`), a follow-up test commit (`e8cacb45`) and one review round (`180c849a` to `0c699d9f`). Documentation: [ADR-018](../architecture/adr/018-per-tenant-tool-identities-and-landlock.md), [SECURITY.md](../SECURITY.md#agent-tool-isolation), [architecture.md](../architecture.md#process-and-uid-model), [dev-setup.md](../dev-setup.md#upgrading-to-per-tenant-tool-users-ki-96), [Known Issues](../todo.md#known-issues) KI-96 and KI-110 to KI-113.

**Departures from the plan:**

- **Migration numbers.** `120_tenant_tool_uid.sql` and `121_workspace_deletions.sql` instead of 114 and 115 (113 was taken by S7-C; 114 to 119 stay reserved).
- **The system tool user** 19999 is `codeforge-system` (what the code sets as `USER`), not `codeforge-t19999`. The passwd and group lines are written with `seq` and `awk` rather than a `printf` loop; the image test still counts 10,001 entries and no `/etc/subuid` growth.
- **HOME mode.** A HOME shows mode 0770, because the group bits of a directory with an access ACL are the mask; the ACL itself is the one of D4.
- **Per-call sharing pass.** It runs after every tool process whose identity has a workspace, whatever its cwd, and walks that whole workspace with the ctime filter.
- **Probe.** The worker's probes read `/proc/$$/status`: Landlock lets a process read only its own `/proc` entry.
- **Rollback.** After a rollback the new worker moves the whole state directory aside, UID bindings included; the next payloads bind their UIDs again from the database.
- **Deletion.** The `workspace_deletions` store stays outside the composite Store interface. The deletion subscribers and the retry job (at startup, then every 10 minutes; an intentionally cross-tenant listing, each deletion published in its own tenant) start only with `workspace.tool_acls: required`. The busy check covers runs that are pending, running or in their quality gate, active conversation turns and queued or running tasks (409, `project.ErrProjectBusy`), in one transaction on the locked project row.
- **GDPR erasure reports** do not list pending deletions: the code has only user-level erasure and no project erasure report. Pending deletions are in `workspace_deletions` and in the logs.
- **Tenant creation in the frontend.** The frontend has no tenant creation screen, so no frontend change was needed for the platform-admin rule.
- **The `acl` package** is in the worker image only, not in `.devcontainer/setup.sh` or `.claude/hooks/session-start.sh`: no development or test step needs the binaries (the codec replaces them).
- **CI switch.** `CODEFORGE_ISOLATION_TESTS=required` (`workers/tests/isolation_requirements.py`) makes an isolation skip fail, not `CI=true`: GitHub sets `CI` for every step, the unprivileged pytest run included. CI runs the root tests in the Python job and the Docker suite in the new job `tenant-isolation-docker`; neither has run on GitHub Actions yet.
- **Rollback test.** The Docker suite emulates the KI-71 root walk in the container instead of running the old image, which cannot build from its own commit (below) and is missing from CI's shallow checkout.
- **"Heartbeats keep coming"** during the 1,000,000-entry migration is measured as event-loop ticks in the container (longest gap 0.11 s, 78 s in total after `drop_caches`), without NATS.
- **Preflight side effect.** `scripts/check-host.sh` prepares the volumes as the new worker's start does (root 2771, `.codeforge`, root stamp "3"). A running old worker keeps working with them; when an old worker starts again afterwards, the next start of a new worker detects a rollback and migrates every tenant again.
- **Found while building:** `.dockerignore` excluded `scripts/worker-entrypoint.sh`, so `Dockerfile.worker` had not built since KI-71 (fixed in `7571da73`).
- **Readiness reasons** found by the Docker suite: on a volume without ACL support the reason now names the volume and the remedy, tells an unwritable directory apart from missing ACLs, and reports a read-only HOME base.
- **Python module names.** The subject constants live in `codeforge/nats_subjects.py` (re-exported by `consumer/_subjects.py`); the reaper, Landlock rules, preflight and deletion steps are `tool_reaper.py`, `landlock.py`, `host_check.py` and `workspace_deletion.py`. The Go ACL code is `internal/workspaceacl` (`acl.go`, `acl_linux.go`, `acl_other.go`); the Core's startup steps are `ToolUIDService.PrepareAtStartup` (`internal/service/tenant_tooluid.go`).

**Review round (four findings, each reproduced and fixed test-first):**

1. **Helpers without a timeout (medium, `180c849a`).** A tenant's leftover process could stop (SIGSTOP) the worker's helpers that run as the tenant UID below ABI 6 and hold the subject's message loop forever. Every helper (the sharing pass, removals, the cache measurement) now runs in its own session and ends within 600 s (`HELPER_TIMEOUT_SECONDS`), the migration walks within 3600 s; then its process group is killed and the step counts as failed (a killed or failed migration walk fails the migration). The tenant's last work item in a worker reaps first (D10).
2. **Project deletion failed with ENOTEMPTY (high, `0f934756`, `0c699d9f`).** The Go Core's private patch directory (`.git/codeforge/patches`, 0700/0600, written by the worker's UID 10001) is out of the tool UID's reach. After the removal as T, the worker removes its own remaining entries by descriptor (`workspace_deletion.remove_own_entries`: same file system, the listed inode, a symlink unlinked as itself, never followed, other users' entries left); a workspace that is still not empty is an error naming up to 10 entries. The KI-95 scan lists these calls with their reason.
3. **No pytest or ruff on the tool PATH (high, `3016b3b1`).** Tool processes never use the venv, so the default quality gates of Python projects and the auto-agent's workspace test failed. `workers/tool-requirements.txt` (pytest 9.1.1 with its dependencies and ruff 0.15.1, hash-pinned to `poetry.lock`; `workers/tests/test_tool_requirements.py` prints the new content when the lock changes) is installed into the image's system interpreter; the probe runs `pytest --version` when the tool PATH has pytest.
4. **PID-based temporary names on the shared volumes (low, `0859f017`).** Every replica is PID 1, so replicas could remove each other's temporary entries. Temporary names are random (`secrets.token_hex(8)`), the rollback's aside directory is `.codeforge.rollback-<time>-<random>`, and a state another replica already moved counts as moved.

**Residual risks added by the implementation** (beyond the list above): orphaned tool processes that exit by themselves stay zombies, because the worker is PID 1 of its container and collects only the processes it killed (KI-113); below ABI 6 the stopped-helper wait of review finding 1 is bounded by the 600 s timeout, not closed.
