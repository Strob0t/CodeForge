# ADR-018: Per-Tenant Tool Identities and Landlock Sandboxing

> **Status:** accepted (2026-10-03; implemented as S7-H, [KI-96](../../todo.md#known-issues), of the [fix plan](../../known-issues-fix-plan.md), with one review round; [plan](../../plans/ki96-tenant-tool-isolation-plan.md))
> **Date:** 2026-10-03
> **Deciders:** Project owner. Decision of 2026-10-02: Stage 1 per-tenant tool UID with ACLs, Stage 2 Landlock per tool call, Stage 3 network deferred.
> **Relates to:** amends [ADR-017](017-tool-isolation-and-nats-authentication.md) decisions 1, 2, 4 and 10. Builds on [ADR-016](016-nats-delivery-semantics.md) (payload delivery, the new deletion subjects) and [ADR-006](006-agent-execution-approach-c.md) (Go owns state).

### Context

ADR-017 moved agent tool processes off the worker's UID onto one tool user (10002) in a shared workspace group (10010). That stopped tools from reading the worker's secrets and from forging NATS messages. Between tenants it changed nothing:

- every tool process of every tenant runs as the same UID;
- every tenant directory is writable through the shared group;
- HOME, `/tmp` and cache locations are shared;
- one tenant's agent can read, change and replace another tenant's repositories, signal and trace its processes, and read `/proc/<pid>/environ` of its MCP servers and Claude Code runs.

A review found that the KI-71 launcher passes the whole tool environment as arguments of `setpriv`/`env -i`. A process of another tenant read 150 of 150 secrets from `/proc/<pid>/cmdline`.

The owner's constraint: CodeForge acts only inside its own containers, with no Docker socket, no host runtime and no extra host privileges. The worker container has `cap_drop: ALL` plus `SETUID`, `SETGID` and `KILL`, `no-new-privileges`, a read-only root and Docker's default seccomp profile.

A security review and an operability review of the first plan drove several requirements:

- The worker, which can write every tenant's tree, must never act by path in a tree a tool can write.
- The layout must be verified at every launch, not trusted.
- Landlock cannot be optional in production, because without it command lines are readable across UIDs.
- A tmpfs HOME is `noexec` and counts against the worker's memory.
- Removing what a tool created needs the tool's UID.
- Allocating UIDs eagerly let tenant admins and tests exhaust the range.
- Failures must show in the worker's readiness, not only in a log.

How Claude Code isolates:

- Its local sandbox wraps shell commands in bubblewrap (Linux). Writes go only to the working directory and a per-user TMPDIR, and the network goes through a proxy.
- In an unprivileged container, bubblewrap cannot mount a fresh `/proc`. The documented fallback, `enableWeakerNestedSandbox`, "considerably weakens security" and is meant only where the outer container already isolates.
- bubblewrap's user namespace maps the caller's UID, so the sandbox gives no per-tenant identity.
- For many users, Claude Code's hosting guidance recommends a container or VM per session. Inside a shared container, it recommends a per-tenant working directory and `CLAUDE_CONFIG_DIR`.

### Decision

1. **A tool UID per tenant, allocated lazily.**
   - Every tenant that runs tools gets an immutable tool UID from 20000-29999 (`tenants.tool_uid`, migration 120). It is allocated the first time the Core creates the tenant's workspace directory or dispatches tool work: a sequence without cycling, unique, immutable once set, never reused in this round.
   - Exhaustion fails the dispatch with 503. Tenant creation is for platform admins only.
   - Each UID-to-tenant binding is also recorded on the workspaces volume. The worker refuses a conflicting binding, and the Core advances the sequence over the bindings after a database restore.
   - The Core sends the UID as `tool_uid` on every payload that starts tool processes: `runs.start`, `conversation.run.start`, `tasks.agent.*`, `runs.qualitygate.request`, `conversation.test.request`, `benchmark.run.request` and `workspace.delete.request`.
   - Only the Core may publish those subjects (ADR-017 decision 5). Two invariants are pinned by a contract test: the Core never republishes a worker-written payload (DLQ copies) onto them, and runs started from handoffs take the UID from the claimed tenant in Go.
   - Every tool process of the tenant runs as that UID and GID with **no** supplementary groups: Bash, tool commands, quality gates, workspace tests, backend CLIs, the Claude Code CLI and its hook, and MCP stdio servers.
   - A spawn with no tenant fails closed. Only the isolation probe, the Claude Code CLI check and backend version checks run as a system tool user (19999, `codeforge-system`), which has no workspace access.
2. **Tenant directories through POSIX ACLs, not a shared group; verified, not trusted.**
   - `/data/workspaces` is 2771.
   - `<root>/<tenant>` is 2770, with access ACL `u:T:--x` and default ACL `u:T:rwx, g:10010:rwx, other ---`. The Core sets it with `EnsureTenantDir` before any clone or init when `workspace.tool_acls: required` (production). The default `off` keeps development unchanged.
   - Project directories inherit `u:T:rwx`. The Core and the worker (uid 10001) keep full access through the owning group 10010, which tools no longer have, and through named `g:10010` entries.
   - At every launch, the worker checks the root and the tenant directory for exact owner, mode and ACLs. A foreign-owned or symlinked tenant directory is refused.
   - An ACL-aware sharing pass, run as the tenant UID, restores `g:10010` access after owner-only modes, group changes and stripped or masked ACL entries.
3. **The worker never acts by path in a tree a tool can write.** It acts relative to descriptors on directories only it can write, with owner and inode rechecks, or it lets the tenant UID act through the launcher under Landlock. This covers creating per-work directories, the sharing pass, removals and cleanup. Its own state (migration stamps, UID bindings, locks) lives in a worker-only directory, `<root>/.codeforge`.
4. **No secret on any argument.**
   - The launcher is `setpriv --reuid=T --regid=T --clear-groups --inh-caps=-all --ambient-caps=-all --no-new-privs -- <base python> -I -S tool_exec.py <fd> argv...`.
   - The environment, the sandbox rules, the per-work directories to create and the working directory travel on a memfd passed by descriptor.
   - The helper checks its own credentials and refuses a spec without a Landlock decision.
   - MCP stdio servers use CodeForge's own stdio transport, which can pass that descriptor.
5. **Per-tenant HOME on a disk volume, per-work TMP, closed `/tmp`.**
   - HOMEs live on the `tool_homes` volume, which is executable and kept out of the worker's memory.
   - Each work item has its own TMPDIR below HOME.
   - Every cache, config and data location is set under HOME, and the tool `PATH` is set too; none of them is inherited from the worker. Tools never use the worker's venv: the image installs pytest and ruff into its system interpreter (`workers/tool-requirements.txt`, hash-pinned to `poetry.lock`).
   - `/tmp` is 1771, so tools can traverse it but create nothing there.
   - With isolation required, Claude Code gets a per-run `CLAUDE_CONFIG_DIR`.
6. **Landlock on every tool call; mandatory in production.**
   - `CODEFORGE_TOOL_LANDLOCK` follows `CODEFORGE_TOOL_ISOLATION`. `off` is refused when `APP_ENV=production`. The minimum ABI defaults to 2.
   - **Rules.** The helper restricts itself before it executes the command. It may write only to the run's workspace and its tenant's HOME. It may read `/usr`, `/etc`, a fixed set of devices (null, random, tty, pty, `/dev/shm` without listing), a few global `/proc` and `/sys` files, its own pinned `/proc/<pid>`, and validated per-call read-only extras. Everything else is denied, including `/tmp`, `/app` and every other process in `/proc`.
   - **Scopes.** With ABI >= 6, signals and abstract unix sockets are scoped to the process's own Landlock domain.
   - **Failing closed and visibly.** A startup probe runs with the real tool environment. It checks UID, groups, capabilities, `no_new_privs`, umask, that `python3`, `git` and (when the tool PATH has it) `pytest` run, and that Landlock denies a world-readable canary, `/proc/<worker>/cmdline` and (ABI 6) cross-domain signals. Any failure, and any failure of the ACL, HOME-volume or state-directory checks, fails every tool call and answers `/health/ready` with 503. A preflight script checks a host before an upgrade.
7. **Migration without capabilities.**
   - Trees from before the upgrade are migrated per tenant at its first work item, in a thread after the heartbeat started. It runs under the tenant's exclusive cross-worker lock, with no process of the tenant or of 10002 alive.
   - Each change is made by the entry's owner, the retired UID 10002 (fd walker under Landlock limited to the tenant tree) or the worker, on descriptors opened without following symlinks.
   - Steps: an exact ACL rewrite that drops planted named entries, and copying any regular file whose hard links leave the tenant's tree. An inode still linked elsewhere is never rewritten.
   - A worker-only stamp bound to the tenant directory's inode records the migration. A rollback to an older worker is detected and forces a full re-migration.
   - No `CAP_CHOWN` is added. UID 10002 never runs a tool process again. All workers are stopped before the upgrade.
8. **End of work, leftover processes and deletion.**
   - Each work item ends in a fixed order: its processes stop; a sharing pass runs; its TMPDIR is removed as T. When it is the tenant's last work item in a worker, the worker first kills that tenant's processes in its container (pidfd, UID rechecked, launches blocked), then runs those steps, then shares the tenant's other workspaces and cleans its HOME. Background processes therefore do not outlive the tenant's work.
   - The worker's commands that run as a tool UID (sharing pass, removals, cache measurement) end within 600 s, the migration walks within 3600 s; then their process group is killed and the step counts as failed. Below ABI 6 a tenant's leftovers can still stop the helpers of its other running work item; this bounds the wait.
   - Project workspaces are deleted by the worker as the tenant UID, then the worker removes its own remaining entries (the Go Core's private patch directory) by descriptor (`workspace.delete.request` and `.result`, at least once, with a `workspace_deletions` record, migration 121; the Core retries pending deletions every 10 minutes). Erasure is verifiable from those records and the logs; there is no erasure report of pending deletions yet.
9. **Not in this decision.** Network and local IPC between tenants (Stage 3, KI-110), resource limits and quotas, per-run UIDs, and tenant erasure with UID reclaim (KI-112).

### Consequences

#### Positive

- **Kernel separation between tenants:** files (DAC plus ACLs), signals, ptrace, and `/proc/<pid>/environ` and `cmdline` (different UIDs plus Landlock).
- **No shared state:** HOME is per tenant, `/tmp` is closed to tools, and cache locations are per tenant. Command lines carry no CodeForge secrets.
- **Containment of each call:** Landlock confines it to its workspace and its tenant's HOME, so runs of one tenant cannot read or write each other's projects. With ABI 6, they cannot signal each other or connect to each other's abstract sockets either.
- **Robust worker:** a tool cannot redirect the worker through planted symlinks, forged stamps or swapped directories.
- **Visible failure:** a host that cannot isolate shows an unhealthy worker instead of failing runs silently.
- **No change to the deployment's privileges:** no new capability, no host privilege, no Docker socket, no change to seccomp or AppArmor.
- **Same model as Claude Code's local sandbox:** writes go only to the working directory and a per-user TMPDIR, and everything else is denied. Unlike bubblewrap, it adds a distinct identity per tenant.

#### Negative

- **Host requirements.**
  - POSIX ACLs on the workspaces and HOME volumes: ZFS needs `acltype=posixacl`; NFSv4 and CIFS are not supported.
  - Landlock ABI 2+ in the host kernel; ABI 2 has no truncate handling, and the scopes need 6.12+.
  - Docker 23.0+ (or the 20.10 backport) for the seccomp profile.
  - All workers on one host.
  - Hosts without these fail closed. Hosts below ABI 2 have no production mode. Backups must keep ACLs.
- **Compatibility limits.**
  - Tool processes see only their own `/proc` (the exec target) and a few global `/proc` and `/sys` files. `ps`, `pkill`, `df`, `ss`, psutil and `/proc/self` reads in child processes fail.
  - `/tmp` is denied: programs that ignore `TMPDIR` and the JVM/tmux settings fail.
  - Background processes do not outlive the tenant's work in a worker.
- **Cost.**
  - Each launch costs about 24 ms more (Python helper with a precompiled stdlib).
  - Each Bash call adds a sharing-pass launch and a tree walk (about 0.16 s per 100,000 entries).
  - Each process start that looks up its user pays 1 to 3 ms for the 10,001 generated passwd entries (about 1 MB).
- **More moving parts.**
  - A worker state directory with stamps, bindings and locks.
  - A second worker volume (`tool_homes`).
  - New NATS subjects and a `workspace_deletions` table.
  - A Core switch that must match the worker's mode.
- **Old files.** Files from before the upgrade stay owned by the retired UID 10002. Tenants cannot `chmod` them until they replace them.
- **API change.** Tenant creation is for platform admins only.
- **Gaps that remain:**
  - loopback TCP, abstract sockets on ABI < 6, SysV and POSIX IPC, and `/dev/shm` names between tenants (KI-110);
  - metadata changes, and truncation on ABI 2, within a tenant;
  - leftover daemons until the tenant goes idle;
  - below ABI 6, stopped end-of-work helpers of a tenant's concurrent work item, bounded by the 600 s helper timeout;
  - orphaned tool processes that exit by themselves stay zombies, because the worker is PID 1 and collects only the processes it killed (KI-113);
  - shared CPU, memory, pids and disk (no cgroups or quotas);
  - the worker remains trusted for all tenants.
- **Limited range.** At most 10,000 tenants with tool work per deployment in this range, and no UID reclaim yet (KI-112).

#### Neutral

- Development and tests keep isolation, Landlock and `workspace.tool_acls` off by default.
- CI runs the root integration tests under `sudo` and the Docker suite (`workers/tests/test_tenant_isolation_docker.py`, job `tenant-isolation-docker`) on the built worker image with the production service definition. Isolation skips fail in CI (`CODEFORGE_ISOLATION_TESTS=required`).
- `scripts/check-tool-isolation.sh` checks two tenants, Landlock and the argv leak. `scripts/check-host.sh` checks a host before an upgrade.

### Alternatives Considered

| Alternative | Pros | Cons | Why not |
|---|---|---|---|
| A container (or VM) per run or session (Claude Code's hosting guidance; KI-13 sandbox service) | Strongest isolation: own PID, mount and network namespaces, cgroups, optional gVisor or Firecracker | Needs control of a container runtime: the Docker socket (equivalent to host root) or an external sandbox controller. Also needs a tools image, volume-subpath mounts, an exec API with streaming and group kill, and an egress path. | Excluded by the owner's constraint: no Docker socket, no host runtime, no host privileges |
| bubblewrap per tool call (Claude Code's local sandbox) | Proven in Claude Code; namespaces per command | Docker's default seccomp profile allows `unshare`, `setns` and `mount` only with `CAP_SYS_ADMIN`, and blocks `clone` with namespace flags. Making it work needs a custom seccomp profile, possibly an AppArmor profile, unprivileged user namespaces on the host, and `/proc` bound from outside, which Claude Code's docs say "considerably weakens security" in nested use. bubblewrap maps the caller's UID, so it gives no per-tenant identity by itself. | Needs host-side changes and more privileges, and does not separate tenants without per-tenant UIDs anyway |
| `setpriv --landlock-*` instead of a helper | No helper; no Python startup | util-linux 2.41 handles filesystem rights up to truncate only: no scopes, net or ioctl_dev. It opens rule paths as the worker, before the UID switch, and cannot pass the environment without argv. | The helper adds scopes, opens rule paths as the tool UID, and reads the environment from a descriptor |
| Landlock optional in production (Stage 1 only on older kernels) | Runs on more hosts | Without Landlock, `/proc/<pid>/cmdline` is readable across UIDs: agent commands, prompts and credential arguments leak between tenants | Mandatory in production; the minimum ABI lowered to 2 instead |
| Root migration with a temporary `CAP_CHOWN` (+ `FOWNER`, `DAC_READ_SEARCH`) | Clean ownership after the upgrade | Three more capabilities in the bounding set for the container's lifetime. A privileged walk over agent-made trees. Does not solve hard links between tenants. | Owner-run, fd-based, Landlock-confined ACL grants plus copying of shared links |
| Path-based worker operations in tenant areas (revision 1) | Simpler code | The worker can write every tenant's tree, so a planted symlink redirects it into another tenant | fd-relative operations on worker-only directories, everything else as the tenant UID |
| Eager UID allocation at tenant creation | Simple, every tenant has a UID | Any tenant admin and every integration test run burns UIDs; the range can be exhausted for good | Lazy allocation; tenant creation for platform admins |
| Tenant HOME on a tmpfs | No disk state, cleared at restart | Docker's tmpfs is `noexec` (npx, `go test`, uv break); its pages count against the worker's memory limit, so one tenant's caches can OOM-kill the worker | A disk volume `tool_homes` with a cache cap |
| The Core deletes workspaces itself | No new subjects | Under default ACLs, a tool's 0700 directories have mask `---`; only the tool UID can remove them, so erasure stays incomplete after a crash | Deletion through the worker as the tenant UID, tracked in `workspace_deletions` |
| Per-tenant groups, with the Core and the worker in all of them | No ACLs | Supplementary groups are fixed at process start. The read-only image and a Core without `CAP_SETGID` mean a pre-built group pool or a restart per new tenant. | ACL entries per tenant UID |
| A tool UID per run | Separates runs of a tenant by the kernel too | Workspaces outlive runs: ownership and ACLs would change on every run, and the UID space and the cleanup grow with the run count | Per tenant, with Landlock separating runs |
| A worker pool per tenant | Separate containers | NATS subjects, users and consumers per tenant (stream limit of 200 consumers); Compose cannot provision workers dynamically | Suits a few dedicated tenants, not self-service tenancy |
| `hidepid` for `/proc` | Hides other processes | Remounting `/proc` needs `CAP_SYS_ADMIN`; Docker has no option for it | Landlock's narrow `/proc` rules and no secrets on argv |
| Read access to all of `/proc` for tools | Maximum compatibility | Every tool sees every process's command line (MCP credential arguments included) and `/proc/net` | Narrow rules; the Claude Code CLI is verified with them |
| Shared HOME with `CLAUDE_CONFIG_DIR` per run only | Fewer directories | Shared caches and dotfiles let one tenant poison another's tools | HOME per tenant, TMP per work item |

### References

- [KI-96 plan](../../plans/ki96-tenant-tool-isolation-plan.md) (Docker evidence E0-E21), [ADR-016](016-nats-delivery-semantics.md), [ADR-017](017-tool-isolation-and-nats-authentication.md), [Known Issues](../../todo.md#known-issues) KI-96, KI-110, KI-111, KI-112, KI-113
- Landlock: <https://docs.kernel.org/userspace-api/landlock.html>; setpriv(1); pidfd_open(2), pidfd_send_signal(2); acl(5)
- Claude Code: sandboxing (scope, filesystem defaults, "Bubblewrap fails to start inside a container", `enableWeakerNestedSandbox`), secure deployment and hosting (a container per session; a per-tenant `CLAUDE_CONFIG_DIR` in shared containers), `anthropics/sandbox-runtime` README (the user namespace maps the caller's UID)
- Docker default seccomp profile (moby `profiles/seccomp/default.json`; Landlock syscalls allowed since 23.0, backported to 20.10)
