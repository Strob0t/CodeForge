"""Host preflight of per-tenant tool isolation (KI-96 D12, ADR-018).

``scripts/check-host.sh`` runs this module in the new worker image with the
production service definition (``docker compose run`` of the worker
service: its user, capabilities, volumes and tmpfs mounts) against the real
``workspaces`` and ``tool_homes`` volumes, before an upgrade. It reports
what the host gives tool isolation:

- the kernel and its Landlock ABI, and why there is none (a seccomp profile
  that hides the system calls answers ENOSYS, a kernel that has Landlock
  but not in its ``lsm=`` list EOPNOTSUPP);
- the file system and mount options of both volumes, and whether a POSIX
  ACL set on a new directory reads back;
- whether the HOME volume is mounted ``noexec`` (an old tmpfs);
- the mode and owner of ``/tmp`` (1771, the worker's);
- the result of the worker's own isolation check (codeforge.tool_process),
  which also prepares the volumes the way the new worker's start does: the
  root's mode 2771, the state directory ``<root>/.codeforge``.

It exits 1 when anything fails; the worker would answer ``/health/ready``
with 503 on such a host.

    python -m codeforge.host_check [--json]
"""

from __future__ import annotations

import errno
import json
import os
import re
import stat
import sys
from dataclasses import dataclass

from codeforge.tool_process import IsolationConfig, configure_tool_isolation

_OFF = "CODEFORGE_TOOL_ISOLATION is not required: run the check with the production worker service"
_TMP = "/tmp"  # noqa: S108 - the mount being checked, never written
_TMP_MODE = 0o1771
_ESCAPE = re.compile(r"\\([0-7]{3})")


@dataclass(frozen=True)
class Mount:
    point: str
    fstype: str
    options: str
    super_options: str

    @property
    def noexec(self) -> bool:
        return "noexec" in self.options.split(",")


def _unescape(field: str) -> str:
    return _ESCAPE.sub(lambda m: chr(int(m.group(1), 8)), field)


def parse_mountinfo(text: str) -> list[Mount]:
    """The mounts of a /proc/<pid>/mountinfo text."""
    mounts = []
    for line in text.splitlines():
        before, sep, after = line.partition(" - ")
        fields, tail = before.split(), after.split()
        if not sep or len(fields) < 6 or len(tail) < 3:
            continue
        mounts.append(Mount(point=_unescape(fields[4]), fstype=tail[0], options=fields[5], super_options=tail[2]))
    return mounts


def mount_of(path: str, mounts: list[Mount]) -> Mount:
    """The mount *path* lies on: the last mounted of the longest matching mount points."""
    best: Mount | None = None
    for mount in mounts:
        point = mount.point.rstrip("/") or "/"
        inside = point == "/" or path == point or path.startswith(point + "/")
        if inside and (best is None or len(point) >= len(best.point.rstrip("/") or "/")):
            best = mount
    return best or Mount(point="/", fstype="unknown", options="", super_options="")


def read_mounts() -> list[Mount]:
    try:
        with open("/proc/self/mountinfo") as f:
            return parse_mountinfo(f.read())
    except OSError:
        return []


def landlock_state() -> tuple[int, int]:
    """The kernel's Landlock ABI and, without one, the errno of landlock_create_ruleset."""
    import ctypes

    from codeforge.tool_exec import _CREATE_RULESET_VERSION, SYS_LANDLOCK_CREATE_RULESET, _syscall

    try:
        version = ctypes.c_uint32(_CREATE_RULESET_VERSION)
        return _syscall(SYS_LANDLOCK_CREATE_RULESET, None, ctypes.c_size_t(0), version), 0
    except OSError as exc:
        return 0, exc.errno or 0


def landlock_explanation(abi: int, code: int) -> str:
    if abi:
        return f"ABI {abi}"
    if code == errno.ENOSYS:
        return (
            "none (ENOSYS): the kernel lacks Landlock (5.13 or later) or a seccomp profile blocks its system "
            "calls (Docker before 23.0)"
        )
    if code == errno.EOPNOTSUPP:
        return "none (EOPNOTSUPP): Landlock is built in but not enabled: add landlock to the lsm= boot parameter"
    return f"none (errno {code}: {os.strerror(code) if code else 'unknown'})"


def acl_problem(path: str) -> str:
    from codeforge import tool_state

    return tool_state.acl_support_problem(path)


def tmp_state() -> tuple[int, int]:
    info = os.stat(_TMP)
    return stat.S_IMODE(info.st_mode), info.st_uid


def tmp_problem(mode: int, uid: int, *, worker_uid: int) -> str:
    """The tmpfs at /tmp must be 1771 and the worker's: tools pass through it but create nothing there."""
    if mode != _TMP_MODE:
        return (
            f"{_TMP} has mode {mode:o}, expected 1771 (docker-compose.prod.yml: {_TMP}:uid=10001,gid=10010,mode=1771)"
        )
    if uid != worker_uid:
        return f"{_TMP} belongs to uid {uid}, expected the worker ({worker_uid})"
    return ""


def _lsm() -> str:
    try:
        with open("/sys/kernel/security/lsm") as f:
            return f.read().strip()
    except OSError:
        return "unknown (securityfs is not mounted in the container; scripts/check-host.sh prints the host's)"


def check(config: IsolationConfig) -> tuple[dict[str, str], list[str]]:
    """A report of the host and the problems that keep tool isolation from working."""
    if not config.required:
        return {}, [_OFF]
    report = {"kernel": os.uname().release, "lsm": _lsm()}
    problems: list[str] = []

    abi, code = landlock_state()
    report["landlock"] = landlock_explanation(abi, code)
    if config.confined and abi < config.landlock_min_abi:
        problems.append(f"Landlock {report['landlock']}; CODEFORGE_TOOL_LANDLOCK_MIN_ABI is {config.landlock_min_abi}")

    mounts = read_mounts()
    for path in (config.workspace_root, config.home_base):
        mount = mount_of(path, mounts)
        problem = acl_problem(path)
        report[path] = f"{mount.fstype} on {mount.point} ({mount.options}), POSIX ACLs {'missing' if problem else 'ok'}"
        if problem:
            problems.append(problem)
    home = mount_of(config.home_base, mounts)
    if home.noexec:
        problems.append(
            f"{config.home_base} is mounted noexec ({home.fstype} on {home.point}): mount the tool_homes volume "
            "there, not a tmpfs"
        )

    mode, uid = tmp_state()
    report[_TMP] = f"mode {mode:o}, owner uid {uid}"
    if problem := tmp_problem(mode, uid, worker_uid=os.getuid()):
        problems.append(problem)

    status = configure_tool_isolation(config)
    if status.ready:
        report["tool isolation"] = f"ready (Landlock ABI {status.landlock_abi})"
    else:
        report["tool isolation"] = f"not ready: {status.reason}"
        problems.append(f"tool isolation not ready: {status.reason}")
    return report, problems


def _config() -> IsolationConfig:
    from codeforge.config import get_settings

    return IsolationConfig.from_settings(get_settings())


def main(argv: list[str]) -> int:
    report, problems = check(_config())
    if "--json" in argv:
        sys.stdout.write(json.dumps({**report, "problems": problems}, indent=2) + "\n")
    else:
        lines = [f"{name}: {value}" for name, value in report.items()]
        lines += [f"problem: {problem}" for problem in problems]
        lines.append("result: FAILED" if problems else "result: OK")
        sys.stdout.write("\n".join(lines) + "\n")
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
