"""Per-tenant tool identities (KI-96, ADR-018).

With tool isolation required, every tool process of a tenant runs as that
tenant's tool UID (and GID), from 20000-29999, with no supplementary group:
the kernel then separates tenants for files (POSIX ACLs on the tenant
directories, codeforge.tool_state), signals, ptrace and
``/proc/<pid>/environ``. The Go Core allocates the UID and sends it as
``tool_uid`` on every payload that starts tool processes; only the Core may
publish those subjects (ADR-017 section 5).

A handler enters ``tool_tenant(...)`` once its heartbeat runs: it checks the
payload (UID range, the on-volume binding, the workspace path, the tenant
directory), makes sure the tenant's HOME exists and sets the identity in a
context variable. Every launcher call below it (codeforge.tool_process)
reads the variable; asyncio tasks inherit it (sub-agents, MCP sessions,
heartbeats). A tool process without an identity fails closed with
ToolIsolationError. Tenantless spawns (the isolation probe, CLI checks,
backend version checks) run as the system tool user (19999), which has no
access to any workspace.

With isolation off (development) none of this runs and tool processes start
as the worker, as before.
"""

from __future__ import annotations

import asyncio
import contextlib
import logging
import os
import secrets
from contextvars import ContextVar
from dataclasses import dataclass, field, replace
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import AsyncIterator, Mapping

logger = logging.getLogger(__name__)

# The same values as internal/domain/tenant/tooluid.go (a contract test pins them).
TOOL_UID_MIN = 20000
TOOL_UID_MAX = 29999
SYSTEM_TOOL_UID = 19999
LEGACY_TOOL_UID = 10002
WORKER_UID = 10001
WORKSPACE_GID = 10010
_RESERVED_UIDS = frozenset({WORKER_UID, LEGACY_TOOL_UID, SYSTEM_TOOL_UID})

# Tool processes of a tenant: files outside default ACLs stay the tenant's own.
TENANT_TOOL_UMASK = 0o007
DEFAULT_HOME_BASE = "/home/codeforge-tools"
DEFAULT_TOOL_PATH = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
# Directories of the tool HOME that come after the system ones on PATH: a
# run cannot shadow git for the next run of the same tenant.
_HOME_PATH_DIRS = (".local/bin", "go/bin", ".cargo/bin", ".npm-global/bin")


class ToolIsolationError(PermissionError):
    """Tool isolation is required, but this tool process must not or cannot start.

    An OSError, so callers that report a process that could not start as a
    failed tool call report this one too.
    """


@dataclass(frozen=True)
class ToolIdentity:
    """Who a tool process runs as and what it may use.

    ``workspace`` is the work item's workspace (None for the system
    identity); ``work_id`` names the work item's TMPDIR below the HOME;
    ``read_paths``/``write_paths`` are per-call extras (the Claude Code run
    directory, the hook file).
    """

    tenant_id: str
    uid: int
    home: str
    work_id: str
    workspace: str | None = None
    read_paths: tuple[str, ...] = field(default=())
    write_paths: tuple[str, ...] = field(default=())
    claude_config: bool = False
    # Supplementary groups: none for tenant UIDs; the workspace group only for
    # the retired shared tool user in the migration (codeforge.tool_migration).
    groups: tuple[int, ...] = field(default=())

    @property
    def gid(self) -> int:
        return self.uid

    @property
    def is_system(self) -> bool:
        return not self.tenant_id

    @property
    def tmpdir(self) -> str:
        return f"{self.home}/tmp/{self.work_id}"  # noqa: S108 - below the tool HOME, not /tmp

    @property
    def claude_config_dir(self) -> str:
        return f"{self.home}/claude/{self.work_id}"

    def prepare(self) -> list[str]:
        """The per-work directories the launch helper creates below the HOME, as this identity."""
        if not self.home:
            return []
        entries = [f"tmp/{self.work_id}"]
        if self.claude_config:
            entries.append(f"claude/{self.work_id}")
        return entries

    def with_paths(
        self, *, read: tuple[str, ...] = (), write: tuple[str, ...] = (), claude_config: bool | None = None
    ) -> ToolIdentity:
        return replace(
            self,
            read_paths=self.read_paths + read,
            write_paths=self.write_paths + write,
            claude_config=self.claude_config if claude_config is None else claude_config,
        )

    def with_workspace(self, workspace: str) -> ToolIdentity:
        return replace(self, workspace=workspace)


current_identity: ContextVar[ToolIdentity | None] = ContextVar("codeforge_tool_identity", default=None)


def new_work_id() -> str:
    return secrets.token_hex(8)


def home_of(home_base: str, uid: int) -> str:
    return f"{home_base.rstrip('/')}/{uid}"


def check_tool_uid(tenant_id: str, tool_uid: int) -> None:
    """The payload's tool_uid must be a tenant UID; tenant_id must be set."""
    if not tenant_id:
        raise ToolIsolationError("tool work without tenant_id: refused")
    if not tool_uid:
        raise ToolIsolationError(
            f"payload without tool_uid for tenant {tenant_id}: the Go Core is older than this worker or runs "
            "with workspace.tool_acls=off (set CODEFORGE_WORKSPACE_TOOL_ACLS=required)"
        )
    if tool_uid in _RESERVED_UIDS or not TOOL_UID_MIN <= tool_uid <= TOOL_UID_MAX:
        raise ToolIsolationError(
            f"tool uid {tool_uid} of tenant {tenant_id} is outside {TOOL_UID_MIN}-{TOOL_UID_MAX}: refused"
        )


# Every variable identity_env sets: the worker's own values never reach a tool.
IDENTITY_ENV_NAMES = frozenset(
    {
        "HOME",
        "USER",
        "LOGNAME",
        "SHELL",
        "PATH",
        "TMPDIR",
        "TMP",
        "TEMP",
        "GOTMPDIR",
        "XDG_CACHE_HOME",
        "XDG_CONFIG_HOME",
        "XDG_DATA_HOME",
        "XDG_STATE_HOME",
        "GOPATH",
        "GOMODCACHE",
        "GOCACHE",
        "CARGO_HOME",
        "RUSTUP_HOME",
        "npm_config_cache",
        "npm_config_prefix",
        "PIP_CACHE_DIR",
        "UV_CACHE_DIR",
        "JAVA_TOOL_OPTIONS",
        "TMUX_TMPDIR",
    }
)


def identity_env(identity: ToolIdentity, tool_path: str) -> dict[str, str]:
    """The environment of the identity: every cache, config, data and temp location below its HOME.

    PATH is the configured tool PATH (never the worker's, which starts with
    /app/.venv/bin), then the HOME's bin directories.
    """
    home, tmp = identity.home, identity.tmpdir
    user = "codeforge-system" if identity.is_system else f"codeforge-t{identity.uid}"
    path = ":".join([tool_path, *(f"{home}/{d}" for d in _HOME_PATH_DIRS)])
    return {
        "HOME": home,
        "USER": user,
        "LOGNAME": user,
        "SHELL": "/bin/bash",
        "PATH": path,
        "TMPDIR": tmp,
        "TMP": tmp,
        "TEMP": tmp,
        "GOTMPDIR": tmp,
        "XDG_CACHE_HOME": f"{home}/.cache",
        "XDG_CONFIG_HOME": f"{home}/.config",
        "XDG_DATA_HOME": f"{home}/.local/share",
        "XDG_STATE_HOME": f"{home}/.local/state",
        "GOPATH": f"{home}/go",
        "GOMODCACHE": f"{home}/go/pkg/mod",
        "GOCACHE": f"{home}/.cache/go-build",
        "CARGO_HOME": f"{home}/.cargo",
        "RUSTUP_HOME": f"{home}/.rustup",
        "npm_config_cache": f"{home}/.npm",
        "npm_config_prefix": f"{home}/.npm-global",
        "PIP_CACHE_DIR": f"{home}/.cache/pip",
        "UV_CACHE_DIR": f"{home}/.cache/uv",
        # The JVM ignores TMPDIR; tmux would use /tmp, which tools cannot write.
        "JAVA_TOOL_OPTIONS": f"-Djava.io.tmpdir={tmp}",
        "TMUX_TMPDIR": tmp,
    }


_warned_dropped: set[str] = set()


def drop_operator_values(env: Mapping[str, str]) -> None:
    """Warn once per name that an operator value of an identity variable does not reach tools."""
    for name in sorted(IDENTITY_ENV_NAMES & set(env) - {"PATH", "HOME", "USER", "LOGNAME", "SHELL"}):
        if name not in _warned_dropped:
            _warned_dropped.add(name)
            logger.warning(
                "the worker's %s does not reach tool processes: with tool isolation every cache, config and "
                "temp location is below the tenant's HOME",
                name,
            )


def tool_identity_env() -> dict[str, str]:
    """The identity variables of the current tool identity with isolation required; else nothing."""
    from codeforge.tool_process import tool_isolation

    status = tool_isolation()
    if not status.config.required:
        return {}
    identity = current_identity.get()
    if identity is None:
        return {}
    drop_operator_values(os.environ)
    return identity_env(identity, status.config.tool_path)


# ---------------------------------------------------------------------------
# Entering a tenant's tool work
# ---------------------------------------------------------------------------

# (HOME base, uid) -> (dev, inode) of the HOME the worker made.
_homes: dict[tuple[str, int], tuple[int, int]] = {}


def tenant_home(home_base: str, uid: int) -> str:
    """Create (once) or verify the HOME of *uid* below the base; its path. Raises ToolIsolationError."""
    from codeforge import tool_state

    known = _homes.get((home_base, uid))
    try:
        if known is None:
            _homes[home_base, uid] = tool_state.ensure_home(home_base, uid)
        else:
            tool_state.verify_home(home_base, uid, known)
    except ToolIsolationError:
        raise
    except OSError as exc:
        raise ToolIsolationError(f"the HOME of tool uid {uid} is not usable: {exc}") from exc
    return home_of(home_base, uid)


def check_workspace(root: str, tenant_id: str, uid: int, workspace: str | None) -> str | None:
    """The workspace path, normalized; it must lie below ``<root>/<tenant_id>/`` (or be a ready adopted one)."""
    from codeforge import tool_state

    if not workspace:
        return None
    if not os.path.isabs(workspace):
        raise ToolIsolationError(f"workspace {workspace!r} of tenant {tenant_id} is not an absolute path")
    path = os.path.normpath(workspace)
    area = f"{os.path.normpath(root)}/{tenant_id}/"
    if path.startswith(area) and len(path) > len(area):
        return path
    if path == os.path.normpath(root) or path.startswith(os.path.normpath(root) + "/"):
        raise ToolIsolationError(
            f"workspace {workspace} is not inside the directory of tenant {tenant_id} (tool uid {uid}): refused"
        )
    if reason := tool_state.adopted_workspace_ready(path, uid):
        raise ToolIsolationError(reason)
    return path


def in_tenant_area(root: str, workspace: str | None) -> bool:
    """Whether *workspace* lies below the workspace root (in a tenant directory)."""
    return bool(workspace) and os.path.normpath(workspace).startswith(os.path.normpath(root) + "/")


def verify_tenant_dir(root: str, tenant_id: str, uid: int) -> None:
    """Refuse tool work of a tenant whose directory is not exactly as it must be."""
    from codeforge import tool_state

    check = tool_state.check_tenant_dir(root, tenant_id, uid)
    if check.state is not tool_state.TenantDir.OK:
        raise ToolIsolationError(f"tool work of tenant {tenant_id} (tool uid {uid}) refused: {check.reason}")


def accept_identity(
    tenant_id: str, tool_uid: int, workspace: str | None, *, claude_config: bool = False
) -> ToolIdentity:
    """The accept checks of tool work (D3); the identity to run its tool processes as."""
    from codeforge import tool_state
    from codeforge.tool_process import tool_isolation

    config = tool_isolation().config
    check_tool_uid(tenant_id, tool_uid)
    root = config.workspace_root
    if not root:
        raise ToolIsolationError("tool isolation requires CODEFORGE_WORKSPACE_ROOT")
    try:
        tool_state.bind_tool_uid(root, tool_uid, tenant_id)
        path = check_workspace(root, tenant_id, tool_uid, workspace)
        if in_tenant_area(root, path):
            # A directory that is the worker's but lacks the tenant's ACLs is
            # migrated in tool_tenant (codeforge.tool_migration); never one of
            # another owner or a symlink.
            check = tool_state.check_tenant_dir(root, tenant_id, tool_uid)
            if check.state is tool_state.TenantDir.REFUSED:
                raise ToolIsolationError(
                    f"tool work of tenant {tenant_id} (tool uid {tool_uid}) refused: {check.reason}"
                )
    except ToolIsolationError:
        raise
    except OSError as exc:
        raise ToolIsolationError(f"tool work of tenant {tenant_id} (tool uid {tool_uid}): {exc}") from exc
    home = tenant_home(config.home_base, tool_uid)
    return ToolIdentity(
        tenant_id=tenant_id, uid=tool_uid, home=home, work_id=new_work_id(), workspace=path, claude_config=claude_config
    )


def system_identity() -> ToolIdentity:
    """The identity of tenantless spawns (probe, CLI checks): uid 19999, no workspace."""
    from codeforge.tool_process import tool_isolation

    config = tool_isolation().config
    home = tenant_home(config.home_base, SYSTEM_TOOL_UID)
    return ToolIdentity(tenant_id="", uid=SYSTEM_TOOL_UID, home=home, work_id=new_work_id())


# ---------------------------------------------------------------------------
# End of work and idle tenants (D10)
# ---------------------------------------------------------------------------


@dataclass
class _Activity:
    """A tenant's work in this worker: how many items run, which workspaces they used since it was idle."""

    count: int = 0
    workspaces: set[str] = field(default_factory=set)
    # Held while the tenant's idle steps run: no work item of the tenant enters meanwhile.
    gate: asyncio.Lock = field(default_factory=asyncio.Lock)
    # The HOME cache is removed at the next idle (a project of the tenant was deleted, D11).
    clear_cache: bool = False


_activity: dict[str, _Activity] = {}


def mark_cache_for_removal(tenant_id: str) -> None:
    """Remove the tenant's HOME cache at its next idle (build caches can hold a deleted project's data)."""
    _activity.setdefault(tenant_id, _Activity()).clear_cache = True


async def _enter(identity: ToolIdentity) -> _Activity:
    activity = _activity.setdefault(identity.tenant_id, _Activity())
    async with activity.gate:
        activity.count += 1
        if identity.workspace:
            activity.workspaces.add(identity.workspace)
    return activity


async def _end_of_work(identity: ToolIdentity) -> None:
    """Share what the work item's processes created; remove its TMPDIR and Claude Code config as its UID."""
    from codeforge import tool_process

    if identity.workspace:
        await tool_process.share_tool_files(identity.workspace, identity)
    await tool_process.remove_as_tool([identity.tmpdir, identity.claude_config_dir], identity, confine=identity.home)


async def _cache_kb(identity: ToolIdentity) -> int:
    """The size of the tenant's HOME cache in KiB, measured as its UID (0 when it has none)."""
    from codeforge import tool_process

    cache = f"{identity.home}/.cache"
    done = await asyncio.to_thread(
        tool_process.run_as_tool,
        tool_process._confined(identity, identity.home),
        ["/bin/sh", "-c", 'if [ -d "$1" ]; then du -sk --one-file-system -- "$1"; else echo 0; fi', "cf-du", cache],
        timeout=300,
    )
    try:
        return int(done.stdout.split()[0])
    except (IndexError, ValueError):
        logger.warning("could not measure the tool cache %s: %s", cache, done.stderr.strip()[-300:])
        return 0


async def _tenant_idle(identity: ToolIdentity, activity: _Activity, lock: object) -> None:
    """The tenant's last work item in this worker ended: stop its leftovers, share, clean (D10)."""
    from codeforge import tool_process, tool_reaper

    await asyncio.to_thread(tool_reaper.reap, identity.uid)
    # Leftovers may have created private files after their work items ended.
    for workspace in sorted(activity.workspaces):
        await tool_process.share_tool_files(workspace, identity.with_workspace(workspace))
    activity.workspaces.clear()
    # Only when no other worker works for the tenant (its exclusive lock, without waiting).
    if not lock.try_exclusive():  # type: ignore[attr-defined]
        return
    home = identity.home
    await tool_process.remove_as_tool([f"{home}/tmp"], identity, confine=home, contents_only=True)  # noqa: S108
    limit = tool_process.tool_isolation().config.cache_max_mb
    if activity.clear_cache or await _cache_kb(identity) > limit * 1024:
        await tool_process.remove_as_tool([f"{home}/.cache"], identity, confine=home)
    activity.clear_cache = False


async def _leave(identity: ToolIdentity, activity: _Activity, lock: object) -> None:
    activity.count -= 1
    if activity.count:
        return
    async with activity.gate:
        if activity.count:
            return  # a new work item entered meanwhile
        try:
            await _tenant_idle(identity, activity, lock)
        except (OSError, ValueError) as exc:
            logger.warning("the idle steps of tool uid %d failed: %s", identity.uid, exc)


@contextlib.asynccontextmanager
async def tool_tenant(
    tenant_id: str, tool_uid: int, workspace: str | None, *, claude_config: bool = False
) -> AsyncIterator[ToolIdentity | None]:
    """Run the enclosed tool work as the tenant's tool identity; None (no-op) with isolation off.

    Enter it once the work item's heartbeat runs. A refused payload raises
    ToolIsolationError: the handler ends the work as failed (it was acked on
    accept). The work item holds the tenant's shared lock while it runs; a
    tree from before the upgrade is migrated first, under the exclusive lock
    (codeforge.tool_migration). On exit the end-of-work steps run as the
    tenant (the sharing pass of its workspace, the removal of its TMPDIR and
    Claude Code config) before the identity is left; when it was the
    tenant's last work item in this worker, the tenant's leftover processes
    are killed, its workspaces shared again and its HOME's tmp cleaned
    (D10). Background processes an agent starts end then.
    """
    from codeforge import tool_migration
    from codeforge.tool_process import tool_isolation

    status = tool_isolation()
    if not status.config.required:
        yield None
        return
    if not status.ready:
        raise ToolIsolationError(f"tool isolation is required but not ready: {status.reason}")
    identity = accept_identity(tenant_id, tool_uid, workspace, claude_config=claude_config)
    root = status.config.workspace_root
    lock = tool_migration.TenantLock(root, tenant_id)
    try:
        try:
            await tool_migration.acquire(
                lock,
                needs_migration=lambda: tool_migration.needs_migration(root, identity),
                migrate=lambda: tool_migration.migrate(root, identity),
            )
            if in_tenant_area(root, identity.workspace):
                verify_tenant_dir(root, tenant_id, tool_uid)
        except ToolIsolationError:
            raise
        except OSError as exc:
            raise ToolIsolationError(f"tool work of tenant {tenant_id} (tool uid {tool_uid}): {exc}") from exc
        activity = await _enter(identity)
        try:
            reset = current_identity.set(identity)
            try:
                yield identity
            finally:
                try:
                    await _end_of_work(identity)
                finally:
                    current_identity.reset(reset)
        finally:
            await _leave(identity, activity, lock)
    finally:
        lock.close()


@contextlib.asynccontextmanager
async def system_work() -> AsyncIterator[ToolIdentity | None]:
    """The system tool identity for tenantless checks (None with isolation off); its TMPDIR is removed after."""
    from codeforge import tool_process

    if not tool_process.tool_isolation().config.required:
        yield None
        return
    identity = system_identity()
    try:
        yield identity
    finally:
        await tool_process.remove_as_tool(
            [identity.tmpdir, identity.claude_config_dir], identity, confine=identity.home
        )


@contextlib.contextmanager
def use_identity(identity: ToolIdentity | None) -> object:
    """Run the enclosed (synchronous or asynchronous) code with *identity* as the current one."""
    reset = current_identity.set(identity)
    try:
        yield identity
    finally:
        current_identity.reset(reset)
