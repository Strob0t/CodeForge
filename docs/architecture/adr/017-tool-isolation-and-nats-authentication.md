# ADR-017: Tool Process Isolation and NATS Authentication

> **Status:** accepted (implemented as KI-71 of the [fix plan](../../known-issues-fix-plan.md), four review rounds and a
> security-review round); decision 9 amended 2026-10-02 (S7-A: outbound policy, redaction, tenant-scoped tool upserts); note of
> 2026-10-02 (S7-B: symlink-safe in-process workspace access, KI-95)
> **Date:** 2026-10-01
> **Deciders:** Project owner (lead decisions of the KI-71 milestone: a separate tool user, a shared workspace group,
> a fail-closed isolation mode, one authenticated NATS user per service)
> **Relates to:** [ADR-006](006-agent-execution-approach-c.md) (the worker runs what an LLM chose),
> [ADR-015](015-policy-deny-lists-and-tool-names.md) (policy decisions are Go's), refines
> [ADR-016](016-nats-delivery-semantics.md) (notification consumers)

### Context

The worker runs commands an LLM chose and code an agent wrote: the Bash tool, grep, `git` in the workspace, quality
gates and workspace tests, benchmark test commands, the agent CLIs of `tasks.agent.*`, the Claude Code CLI and its
policy hook, and MCP stdio servers. Until KI-71 they ran as the worker's own user ([Known Issues](../../todo.md#known-issues)
KI-71, found in the S4 and S2-follow-up reviews):

- They could read `/run/secrets/*` (Compose mounts the files 0644) and `/proc/<worker pid>/environ`, which held
  `CODEFORGE_INTERNAL_KEY` (admin on the core API), the database, NATS and LiteLLM credentials. A scrubbed environment
  (`codeforge.subprocess_env.tool_env`) did not help, because the files and the worker's own environment were readable.
- NATS had no authentication inside the deployment. A prompt-injected agent with Bash could publish completions,
  heartbeats, cancels or tool-call responses (forging policy decisions) for any run, and the worker's own credentials
  could do the same.
- Compose ignores `uid`, `gid` and `mode` of file secrets (they are Swarm-only; verified: Compose warns "not supported,
  they will be ignored" and the files stay 0644), and Docker gives a non-root process no effective capabilities even
  with `cap_add`. The obvious fixes (0400 secrets owned by the worker, a setuid launcher) do not work as they are.
- Core, worker and tools write the same workspaces. Separate users must not break checkpoints, delivery, rewind or
  project deletion in the Go Core.

### Decision

**1. A tool user and one launcher.** The worker container starts as root with `cap_drop: ALL` and only `SETUID`,
`SETGID` and `KILL` added, plus `no-new-privileges`. `scripts/worker-entrypoint.sh` runs the worker through `setpriv`
as uid 10001 (group `codeforge`, supplementary group `codeforge-ws` 10010) and keeps those three capabilities as
ambient capabilities: the worker never runs as root. Every process an agent causes starts through
`workers/codeforge/tool_process.py`, which wraps the command in

```text
setpriv --reuid=10002 --regid=10002 --groups=10010 --inh-caps=-all --ambient-caps=-all --no-new-privs -- \
    env -i NAME=value ... command
```

so the command runs as the tool user `codeforge-tool` (uid and gid 10002) with the workspace group as its only
supplementary group, no capabilities (inheritable, permitted, effective and ambient sets empty), `no_new_privs`,
umask 002 and every other file descriptor closed. `CAP_KILL` is for the worker only: it stops tool processes of
another user (timeouts, cancels). Tool processes get `HOME=/home/codeforge-tool` (a tmpfs, mode 0700, noexec, lost on
restart), `USER` and `LOGNAME`.

*The environment reset.* An ambient capability does not set `AT_SECURE`, so the dynamic loader honours `LD_PRELOAD`
and friends for `setpriv`, which holds the worker's capabilities at that point. A backend's `extra_env` or an MCP
server's declared environment could have injected code into `setpriv`. `setpriv` therefore gets an environment of
`PATH` only, and `env -i`, already running as the tool user, sets the command's environment. Declared environments of
MCP servers pass `declared_tool_env` first, which drops the worker's credentials (`CODEFORGE_*`, `LITELLM_*`,
`DATABASE_URL`, `NATS_URL`) and variables that make the loader or an interpreter load other code (`LD_*`, `PYTHON*`,
`PERL5*`, `RUBY*`, `MALLOC_*`, `NODE_OPTIONS`, `NODE_PATH`, `BASH_ENV`, `ENV`, `GCONV_PATH`, `GLIBC_TUNABLES`,
`LOCPATH`, `NLSPATH`, `HOSTALIASES`); the server's own tokens stay.

*One entry point.* A test scans `workers/codeforge` and fails when any module other than `tool_process.py` starts a
process (`subprocess`, `asyncio.create_subprocess_*`, `os.exec*`, `stdio_client`, `StdioServerParameters`,
`anyio.open_process` and `run_process`), so a new spawn site cannot bypass the launcher.

**2. Fail closed, usable in development.** `CODEFORGE_TOOL_ISOLATION` is `required` in the worker image and in
`docker-compose.prod.yml` and `off` elsewhere (development, tests, the devcontainer); an unknown value counts as
`required`. At startup a probe starts a tool process and checks uid, gid, groups, all four capability sets,
`no_new_privs`, umask, and that the process cannot read `/proc/<worker pid>/environ` or anything in `/run/secrets`.
When the probe fails in `required` mode, every tool call fails with `ToolIsolationError` (an `OSError`) and nothing
starts; the effective mode is logged once. A tool process never silently runs as the worker user.

**3. Secrets.** The worker reads `DATABASE_URL`, `NATS_URL`, `LITELLM_MASTER_KEY` and `CODEFORGE_INTERNAL_KEY` from
`*_FILE` paths (setting both forms is an error, as in the Go Core); Compose passes paths only, so no secret is in the
worker's environment. Because Compose ignores secret modes, the worker mounts `/run/secrets` as a tmpfs with
`uid=10001,mode=0700`; the 0644 bind-mounted files sit inside it, so only uid 10001 can enter. After startup the
worker sets the directory to mode 0, so a symlink planted in a workspace cannot make the worker's own file tools read
a secret (settings keep the values they read).

**4. Workspaces shared through a group.** Both images create group `codeforge-ws` (10010) and `/data/workspaces` as
`10001:10010`, mode 2775 (setgid, so new entries join the group). The Go Core runs with umask 002 and writes files
0664 and directories 0770 (`internal/domain/project/workspace_perm.go`); the worker and the tools use umask 002. The
worker image sets `safe.directory=*` in `/etc/gitconfig` because tool processes run `git` in repositories another user
created; the KI-77 hardening of the Go Core's git calls is unchanged.

*Permission normalisation.* Agents create files with owner-only modes (`mkdtemp`, `mkdir -m 0700`, `umask 077`) that
the worker and the Go Core could neither read nor delete. `share_tool_files(root)` runs one
`find -P root -xdev` as the tool user: for the files and directories the tool user owns it moves them into the
workspace group and makes them group-readable and -writable (directories also searchable and setgid); it never follows
a symlink and changes nothing of anybody else's. It runs when a tool process started with a working directory is
waited for (`wait()`, `communicate()`, `run_tool_process`), and once more at the end of every run, conversation run,
backend task and benchmark task. Workspaces that exist before the upgrade are opened by a one-time walk of the worker
(`share_workspace_root`) before it takes work, versioned by a stamp file `.codeforge-workspace-sharing` in the root:
the walk uses directory descriptors (`os.fwalk`) and changes an entry only through a descriptor opened with
`O_NOFOLLOW` whose inode, type and owner it re-checks; the stamp counts only as a regular file of at most 64 ASCII
bytes opened without following a symlink or blocking, and is written as a new file renamed over its place. The tool
user can write the root, so forging or deleting the stamp only skips or repeats the walk. A root the worker does not
own gets one error naming the fix and is not walked.

**5. NATS users and permissions.** The deployment's NATS server (`configs/nats/nats-server.conf`, mounted through
Compose `configs:`) requires authentication with two users, `core` and `worker`, whose passwords come from the secret
`passwords.conf` the config includes. `core` publishes on every stream subject plus `$JS.API.>`, `$JS.ACK.>`,
`$JS.FC.>` and `$KV.>`; `worker` publishes only the 31 result, output and heartbeat subjects it owns and the `.dlq`
copies of the 24 subjects it consumes, and has no stream changes and no KV access, so it cannot forge what only the Go
Core sends (run starts, cancels, tool-call decisions, task dispatches). Each user subscribes only to its own inbox
space. Both services read their URL with credentials from `*_FILE` (`nats-core-url`, `nats-worker-url`; URLs are
redacted in logs); the tool environment never gets `NATS_URL` or `*_URL_FILE`, and the tools cannot read the files.
`generate-secrets.sh` creates `nats-core-pass` and `nats-worker-pass` and derives `nats-core-url`, `nats-worker-url`
and `nats-passwords.conf`; `validate-env.sh` checks that URLs and the password file agree and that the two users do
not share a password. The image is pinned to `nats:2.15-alpine` in production (see 7). The development compose stays
without authentication (zero-config startup); a deployment without credentials connects as before.

**6. Inbox and consumer narrowing.** Observed on nats-server 2.15.0 (2.10.24 behaves the same): the server checks neither
a push consumer's deliver subject nor a pull request's reply subject against the creator's publish rights. A delivery
onto a stream subject is refused as a cycle (error 10081), a pull reply onto a stream subject is not stored, and
deliveries to `$KV.*` or `$JS.API.*` are neither stored nor executed; a delivery to a plain subscription arrives. A
worker consumer addressed to the Go Core's `_INBOX.<id>` delivered real stream messages there, and with generic
consumer rights the worker could also fetch from, delete and ack the Go Core's durables. Pull-only consumers do not
close this, since reply subjects are not checked either. So:

- Each service has an inbox prefix of its own, `_INBOX_core` (Go, `nats.CustomInboxPrefix`) and `_INBOX_worker`
  (Python, `INBOX_PREFIX`), and may subscribe only to it. The worker cannot learn the Go Core's random inbox names and
  has nothing to aim a delivery at.
- The worker's JetStream rights are limited to consumers by explicit name: for each of its 24 durables
  (`codeforge.nats_subjects.consumer_name`) `CONSUMER.CREATE.CODEFORGE.<name>.>`, `CONSUMER.INFO`, `CONSUMER.DELETE`,
  `CONSUMER.MSG.NEXT` and `$JS.ACK.CODEFORGE.<name>.>`, and for each of the four notification consumers CREATE and
  INFO. The generic `CONSUMER.*`, `DURABLE.CREATE`, `$JS.ACK.>` and `$JS.FC.>` wildcards are gone, as is the unnamed
  `$JS.API.CONSUMER.CREATE.CODEFORGE`. A name is one subject token, and the name in the subject wins: a create through
  `CREATE.CODEFORGE.<worker-name>.<filter>` whose body names a core consumer configures the subject's consumer, never
  the core's (a differing `durable_name` is refused with error 10017; the form without a filter token is refused by the
  permissions). A review PoC showed that the unnamed subject lets the worker reconfigure a core durable, for
  example to route a message it may publish into the Go Core's `handoff.approved` handler (decision 8).

**7. The NotificationHub.** Cancels (`runs.cancel`, `tasks.cancel`, `conversation.run.cancel`) and tool-call decisions
(`runs.toolcall.response`) reach runs and tasks through `workers/codeforge/notifications.py`. The worker may not create
consumers by arbitrary name, so ADR-016's per-listener ephemeral consumers are replaced by one named push consumer per
subject, `codeforge-py-notify-<subject>` (ack none, deliver policy `new`, inactivity threshold 300 s), shared by all
worker instances and delivering to `_INBOX_worker.notify.<name>`; every instance subscribes there (core NATS fan-out)
and hands each message to the listeners of its runs and tasks. The `subscribe`/`next_msg`/`unsubscribe` calls are
those of a JetStream push subscription.

- *No cancel lost.* A listener that starts at a stream sequence (its start message) registers first, then reads the
  subject back from that sequence with batched direct gets (`$JS.API.DIRECT.GET.CODEFORGE`, 256 messages per request;
  the stream is created with `AllowDirect`), then gets the live messages. A message can arrive twice; every listener
  is idempotent. A read-back over 100,000 messages, or any read error, raises `NotificationReplayError` and the run or
  task fails instead of running on.
- *Gaps.* The hub remembers the last stream sequence it handed out per subject. Consumer sequences are contiguous
  because every instance sees every delivery, so a gap (disconnect, slow-consumer drop, recreated consumer) or any
  reconnect triggers a read-back from that point, retried with backoff (1 s to 30 s) until done.
- *Never deletes.* The hub creates or reuses its consumers and never deletes one: another instance may use it. A
  consumer with settings that cannot change in place is used if it still delivers the right subject, with ack none, to
  the hub's deliver subject; otherwise the hub refuses to start (`NotificationConsumerConflictError`). A lost consumer
  is recreated on every reconnect. A future change of the settings needs new consumer names.
- *Rights.* `CONSUMER.CREATE` and `CONSUMER.INFO` of the four named consumers and `DIRECT.GET.CODEFORGE`. The read-back
  adds no read access: the worker could already filter any subject on its own durables.
- *Per-instance consumers were rejected.* Names like `codeforge-py-notify-<instance>-<kind>` can be allowed only by a
  pattern that allows every consumer name, the Go Core's included (permissions match whole tokens), and the stream's
  `MaxConsumers` is 200.
- *Version.* Batched direct get needs nats-server 2.11 or newer (2.10.24 does not answer a batch), so production pins
  `nats:2.15-alpine` and CI runs the real-server tests against the same minor version. The worker's rights depend on
  how the server checks names; re-run `workers/tests/test_nats_permissions.py` and `internal/adapter/nats/auth_test.go`
  against a new version before changing the pin (KI-102).

**8. Approved handoffs need their release.** `handoff.approved` starts a workspace-changing run. The Go Core now carries
out a message only when it is exactly the payload of an approved quarantine message of its tenant whose replay was not
yet consumed (`quarantine_messages.consumed_at`, migration 111, with a partial index); it checks this before claiming
the stage, so a forged message cannot use up a waiting handoff's ID, and marks the approval consumed once the handoff
was carried out or refused for good. A message that merely arrives on the subject starts nothing. Approvals recorded
before the upgrade stay unconsumed; handoffs already carried out stay protected by `handoff_claims`.

**9. MCP servers.**
- *Where they run.* Stdio servers start only in the worker, through `tool_process.tool_stdio_client`, as the tool user
  with the environment of decision 1. The Go Core never starts a stdio MCP server: the connection test answers 400
  "stdio servers cannot be tested from the core; they run in the worker" and leaves a saved server's status unchanged;
  `sse` and `streamable_http` are tested from the core (under the outbound policy, see the note of 2026-10-02 below). This also fixed a pre-existing defect: the worker
  passed `errlog=io.StringIO()` (no file descriptor) to the MCP SDK, so no stdio server ever started; it now gets
  `/dev/null`, and a failed connection closes its server process and log handle.
- *Secrets.* Every MCP server response (list, get, project list, create, update) shows env and header values as `***`
  (an empty value stays empty), for every role. An update that sends `***` back keeps the stored value, only while
  transport, URL, command and arguments are unchanged (otherwise the secret could be sent to another destination);
  `***` on create or for a key with nothing stored, and a changed transport, URL, command or argument list with `***`,
  are 400. `POST /mcp/servers/test` with the `id` of a saved server and `***` values connects with the stored values.
- *Tenancy.* `project_mcp_servers` rows carry the tenant of their project and server (before, every row got the default
  tenant and an assignment did not check the project's tenant; migration 112 corrects the tenant and deletes
  cross-tenant links). Assign and unassign across tenants answer 404, and runs and conversations resolve their project's
  servers in their own tenant (`ResolveForRun` takes the run's context; before, non-default tenants got none).
- *Who manages them.* A tenant's admins (`RoleAdmin`) create, change, delete, test and assign MCP servers of their own
  tenant; reads stay open to the tenant's users. A stricter rule (platform admins only) was tried in the second review
  round because the servers then ran as the worker user; once they run as the tool user, a server definition grants
  what the agents' Bash tool already has in that tenant's runs, so the platform-admin rule protects nothing a tenant
  admin could not do with a run. The remaining exposure between tenants is the shared tool UID (KI-96).
- *Audit.* Assign and unassign write one `audit_log` entry (`assign`/`unassign`, resource `mcp_server`, resource ID
  the server the handler decoded, details `{"project_id": ...}`) before the change; when it cannot be written the
  request is refused with 503 and nothing changes (`middleware.AuditLogByHandler`, `RecordAudit`). A body re-read by
  the middleware could differ from what the handler decoded (trailing data, duplicate keys, padding), so the handler
  names the resource.
- *Outbound policy, redaction and tool upserts (note of 2026-10-02, S7-A: KI-97, KI-100, KI-101).*
  - *Outbound policy.* The URL of an `sse` or `streamable_http` server is tenant input, and the core (connection test) and
    the worker (runs) connect to it. Create, update (when the transport or URL changed), both test routes, the core's
    test client and the worker's connections follow one policy (`netutil.OutboundPolicy`,
    `workers/codeforge/mcp_outbound.py`, one shared case table): link-local (cloud metadata), unspecified, multicast
    and reserved addresses are never allowed; private ranges only for the hosts, IPs and CIDRs the platform operator
    lists in `mcp.allowed_private_hosts`; every address a name resolves to must pass, and the address that is dialled
    is checked again (DNS rebinding, redirects). Only the operator sets the list. The core sends it with each server on
    `runs.start` and `conversation.run.start` (a worker setting of its own could drift from the core's; only the core
    can publish these payloads), so there is one source.
  - *Operator trust.* Servers from `mcp.servers_dir` are operator config: the core never connects to them, and the
    worker trusts them (`trusted`, set by the core only, never for a stored server that reuses an operator server's ID)
    with private and loopback addresses, never with link-local or metadata addresses.
  - *Explicit loopback.* Loopback is refused by default (the core's and the worker's own services are there) and opens
    only by an explicit entry (`localhost`, `127.0.0.1`, `::1`, a loopback CIDR); a broad prefix such as `0.0.0.0/0`
    does not open it.
  - *Proxy opt-in.* No proxy is used unless `mcp.use_proxy` is on, because a proxy chooses the address the policy
    checks. With it only the URL's host is checked before connecting and the address is not pinned (rebinding is then
    the proxy's egress policy); the core logs a warning at startup.
  - *Redirect parity.* The core's test client follows redirects exactly as the worker's MCP SDK does (same method, no
    userinfo in the Location, the origin of the request just sent, at most 20), so headers never go to another host and
    a passing test predicts a run.
  - *Redaction.* The URL is redacted as one URL value (`secrets.RedactURLField`: the whole userinfo and credential
    query and fragment values; the host shown is the host connected to) and credential argument values become `***`
    like env and header values. A URL or argument list that carries `***` is kept only when it equals the value as read,
    as a whole; every kept value still needs the same transport, URL, command and arguments. Connection-test errors
    and worker connect logs never quote URL secrets.
  - *Tenancy.* `UpsertMCPServerTools` locks the server row by id and tenant and writes the rows with that tenant.

**10. Health states.** The worker's `GET /health/ready` answers `503 {"status":"starting"}` while it shares the
workspaces (the walk runs after the health server is up and before the consumer takes work) and `503
{"status":"not ready"}` while the NotificationHub has consumers to restore or a read-back pending; `200` once the
worker, its loops and the hub are ready.

**Note of 2026-10-02 (S7-B: KI-95, KI-105, KI-106, KI-107).** Workspaces are untrusted, but the Go Core and the worker
read and write them in their own processes with rights the tool user lacks. Both now open workspace files only through
one helper per language, never a plain path: `internal/workspacefs` (Go, on `os.Root`) and `codeforge.workspace_fs`
(Python, component-wise walks with `O_NOFOLLOW`).

- *Rules both share.* Absolute paths, `..` above the workspace and symlinks that leave it are refused; a relative symlink
  that stays inside is followed (8 at most); an absolute symlink is never followed in-process, even one that points
  inside (`os.Root` cannot be told not to follow, so this is the only rule both languages can share); opens never block
  and take regular files only; the workspace directory itself may not be a symlink or FIFO. A worker test scans for
  direct file access outside the helper; the Go side has no such scan.
- *The Go Core was the larger hole.* Most of its readers (goal and spec discovery, roadmap sync, context scoring, the
  file API, index seeding) had no check at all and read with the Core's own rights (`/run/secrets`, `/data`); the worker's
  file tools already refused a static symlink out, their gaps were the swap race, FIFOs and listings.
- *Operator directories, same helpers.* Knowledge-base content lives in per-tenant areas
  `<knowledge.content_root>/<tenant_id>/` (every refusal one and the same 400; create, update, delete and index need
  admin; scope attach is tenant-checked), benchmark datasets only inside `benchmark.datasets_dir`, and `detect-stack`
  only inside the caller's tenant area (KI-105, KI-106, KI-107).
- *Residual.* Hard links cannot be told apart from regular files (`fs.protected_hardlinks=1`, a host sysctl whose kernel
  default is 0; links never cross a mount, so only the workspaces volume is reachable); absolute symlinks are not
  followed in-process; subprocess readers stay outside the helpers (the Core's git calls KI-77, LSP servers KI-83, svn,
  and the agents' own tools); tenant directories are group-writable, so a tool user can rename or replace other
  workspaces (KI-96); Go escape detection relies on `os.Root`'s unexported error text (a test pins it).

### Consequences

#### Positive

- Tool processes cannot read the worker's secrets or environment, cannot signal or trace the worker (different UID),
  hold no capabilities, cannot gain any, and have no NATS credentials, so a prompt-injected agent can no longer forge
  completions, cancels or policy decisions through NATS.
- The worker cannot forge Go Core messages even if it is compromised: its user cannot publish run starts, cancels,
  tool-call decisions or task dispatches, cannot change the stream and has no KV access, and it manages only its own
  consumers.
- Isolation fails closed: a deployment that cannot drop privileges reports it at startup and refuses tool calls.
- MCP secrets never leave the API after they are stored; MCP links and runs are tenant-correct.

#### Negative

- The worker image starts as root (no `USER`); platforms that forbid root containers get failing tool calls. The worker
  keeps `SETUID`, `SETGID` and `KILL` for its lifetime; tool processes keep the bounding set `e0`, which is harmless with
  `no_new_privs` and empty permitted sets (dropping it would need `CAP_SETPCAP`).
- Operators must run `generate-secrets.sh` and `validate-env.sh` after upgrading (Compose fails until the new NATS
  files exist), deploy `configs/nats/`, and apply migrations 111 and 112 (112 deletes cross-tenant MCP links). The first
  start walks the existing workspaces once. `/run/secrets` in the worker is unreadable after startup, even with
  `docker exec`.
- A new subject or worker subscription needs entries in `configs/nats/nats-server.conf`; `test_nats_permissions` fails
  without them (it needs a nats-server binary, `NATS_SERVER_BIN`). The Go Core's permissions are broad.
- Files created by a process that outlives the run-end pass (a background server) stay private until the next pass in
  that workspace. `CLAUDE_CONFIG_DIR` and adopted workspaces outside `/data/workspaces` must be accessible to group
  10010.
- The sharing pass costs a walk of the workspace per tool process (0.17 s on 102,000 entries when nothing changes, 0.51 s
  when everything does; KI-103).
- Shared tool UID: all runs and tenants share uid 10002, so their tool processes can read, signal or
  trace each other and reach other workspaces (as before; KI-96). Workspaces are group-writable, so an agent can still
  rewrite `.git/config` and the KI-77 check-to-use window stays open. In-process readers that followed symlinks out of
  a workspace (KI-95) are fixed by S7-B (note above); hard links, subprocess readers (KI-77, KI-83) and the
  group-writable tenant directories (KI-96) remain.
- The server does not check deliver and reply subjects: a consumer can deliver stream messages to any plain
  subscription whose name its creator knows. The per-service inbox prefixes keep the Go Core's names unknowable to the
  worker (KI-99).
- A read-back depends on stream retention (30 days, 5 million messages, discard old); a gap older than that is lost. After
  a long disconnect the read-back of tool-call responses is unbounded, though streamed to the listeners. The worker can
  read any stream message through direct get (no new exposure: its own durables could filter any subject).
- The MCP UI lacks a header editor and shows admin actions to everyone (KI-98). LSP servers started by the Go Core are
  not isolated (KI-83). The MCP outbound policy needs operator upkeep (private hosts such as a compose `docs-mcp` must
  be listed), is defense in depth in the worker (stdio servers and the agents' own tools reach internal hosts), counts
  6to4 and Teredo as public, and gives up address pinning with `mcp.use_proxy`; other callers of the older SSRF filter
  lack ranges (KI-104).

#### Neutral

- Dev and tests run with isolation `off` and without NATS authentication; the Python isolation tests that need root or
  a `nats-server` binary skip with a reason when they are not available, and CI runs them (it copies `nats-server` from
  `nats:2.15-alpine`).
- `scripts/check-tool-isolation.sh` reproduces the production container settings and prints the credentials of a tool
  process (uid, gid, groups, `CapEff`, `NoNewPrivs`) and what it can reach.
- The old secrets `nats-user`, `nats-pass`, `nats-url` and `nats-auth.conf` are unused; `generate-secrets.sh` says so.

### Alternatives Considered

| Alternative | Pros | Cons | Why Not |
|---|---|---|---|
| File-capability launcher (`cap_setuid,cap_setgid+ep`) | Worker keeps no capabilities | `no-new-privileges` disables file capabilities; the worker could not signal tool processes of another user | Does not work in the prod container |
| Python `Popen(user=..., group=...)` | No launcher process | Inherits the ambient capabilities of the worker, so it would need `preexec_fn` (unsafe with threads) | `setpriv` clears them before it executes the command |
| `setpriv` with the caller's environment | Simpler command line | `LD_PRELOAD` is honoured while `setpriv` holds the worker's capabilities | Environment reset with `env -i` as the tool user |
| Compose `secrets: uid/gid/mode` (0400, owned by the worker) | Declarative | Ignored by Compose for file secrets (verified) | Tmpfs `/run/secrets` that only the worker can enter |
| A sandbox container for tools (KI-13) | Strongest isolation, per-run | Tools do not run in the container yet; large change | Future; fail-closed rejection of sandbox modes stays |
| Per-tenant or per-run tool UIDs | Tenants cannot reach each other | Needs UID allocation and workspace ownership changes | Follow-up KI-96 |
| Default ACLs for the workspace group | No walk after tool processes | The mode bits of `mkdtemp` and `mkdir -m` mask them | A tool-user `find` pass |
| More capabilities for the worker or the Go Core (`CAP_DAC_OVERRIDE`, `CAP_CHOWN`) | Could read or delete any file | Much wider rights for the services that hold the secrets | Rejected |
| An `LD_PRELOAD` shim that forces umask 002 | No pass | Defeated by static binaries and by the tool's own environment | Rejected |
| NATS accounts or nkeys instead of users | Stronger separation, no shared passwords | More setup; the user permissions give the same separation | Simple users with per-service passwords |
| Pull-only consumers for the worker | No deliver subject to misuse | Reply subjects are not checked either; notifications need push consumers | Narrowing by inbox prefix and consumer name |
| Per-instance notification consumers | No fan-out, no read-back | Names cannot be allowed by pattern without allowing the Go Core's consumers; stream limit of 200 consumers | Shared consumers plus sequence-gap read-back |
| Ephemeral consumers per listener (ADR-016 as written) | Simple | Needs the generic CREATE right | Replaced by the NotificationHub |
| Deleting and recreating notification consumers on a settings change | Always current settings | Another instance may use the consumer; a delete right is a denial-of-service lever | Never delete; new settings need new names |
| Platform admins only for MCP management | Fewer people can define a command | Protects nothing once servers run as the tool user, and blocks tenant admins from their own servers | Tenant admins manage their tenant's servers |

### References

- [ADR-006: Approach C](006-agent-execution-approach-c.md), [ADR-011: Trust and quarantine](011-trust-quarantine-system.md),
  [ADR-015: Policy deny lists](015-policy-deny-lists-and-tool-names.md), [ADR-016: NATS delivery semantics](016-nats-delivery-semantics.md)
- [Known Issues KI-71 and the follow-ups KI-95 to KI-107](../../todo.md#known-issues), [fix plan](../../known-issues-fix-plan.md)
- [SECURITY.md](../../SECURITY.md), [architecture.md, process and UID model](../../architecture.md#process-and-uid-model), [dev-setup.md](../../dev-setup.md#tool-isolation-and-nats-authentication)
- `workers/codeforge/tool_process.py`, `workers/codeforge/notifications.py`, `scripts/worker-entrypoint.sh`,
  `scripts/check-tool-isolation.sh`, `configs/nats/nats-server.conf`
- NATS permissions: <https://docs.nats.io/running-a-nats-service/configuration/securing_nats/authorization>
