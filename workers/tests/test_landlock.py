"""Landlock per tool call, the worker's side (KI-96 D6): the mode and the rule set of each launch.

The launch helper applies the rules (tests in test_tool_exec.py); the worker
decides them: the system read-only, the work item's workspace and the
tenant's HOME fully, per-call extras read-only, operator toolchains from
CODEFORGE_TOOL_READ_PATHS (validated), nothing else.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from codeforge import landlock
from codeforge.tool_identity import ToolIdentity

WORK = sorted(set(landlock.ALL_RIGHTS) - {"make-char", "make-block", "ioctl-dev"})
IDENT = ToolIdentity(
    tenant_id="tenant-a",
    uid=20000,
    home="/home/codeforge-tools/20000",
    work_id="w1",
    workspace="/data/workspaces/tenant-a/p1",
)


@pytest.mark.parametrize(
    ("raw", "isolation", "mode"),
    [
        ("", "required", "required"),
        ("", "off", "off"),
        ("required", "required", "required"),
        ("OFF", "required", "off"),
        (" off ", "off", "off"),
        ("required", "off", "required"),
        ("disabled", "required", "required"),
        ("0", "required", "required"),
    ],
)
def test_the_mode_follows_isolation_and_fails_closed(raw: str, isolation: str, mode: str) -> None:
    assert landlock.parse_mode(raw, isolation) == mode


def _rules(rules: list[list[object]]) -> dict[str, tuple[list[str], bool]]:
    return {str(path): (sorted(rights), required) for path, rights, required in rules}  # type: ignore[arg-type]


def test_the_rules_of_a_tenant_identity() -> None:
    rules = _rules(landlock.rules_for(IDENT, interpreter_prefix="/usr"))
    assert rules["/data/workspaces/tenant-a/p1"] == (WORK, True)
    assert rules["/home/codeforge-tools/20000"] == (WORK, True)
    assert rules["/usr"] == (["execute", "read-dir", "read-file"], False)
    assert rules["/etc"] == (["read-dir", "read-file"], False)
    assert rules["/dev/null"] == (["read-file", "truncate", "write-file"], False)
    assert rules["/dev/shm"] == (["make-reg", "read-file", "remove-file", "truncate", "write-file"], False)
    assert rules["/dev/pts"] == (["ioctl-dev", "read-file", "write-file"], False)
    assert rules["/proc/sys"] == (["read-dir", "read-file"], False)
    for denied in ("/", "/tmp", "/app", "/run", "/data", "/data/workspaces", "/proc", "/home/codeforge-tools"):
        assert denied not in rules, denied
    # Nothing else of the tenant: not its other projects, not its tenant directory.
    assert not [p for p in rules if p.startswith("/data/") and p != "/data/workspaces/tenant-a/p1"]


def test_per_call_extras_and_operator_paths_are_read_only() -> None:
    identity = IDENT.with_paths(read=("/tmp/cf-cc-1", "/app/workers/codeforge/hook.py"), write=("/scratch",))
    rules = _rules(landlock.rules_for(identity, interpreter_prefix="/usr", read_paths=("/opt",), files=("/a/w.py",)))
    read_exec = ["execute", "read-dir", "read-file"]
    assert rules["/tmp/cf-cc-1"] == (read_exec, True)
    assert rules["/app/workers/codeforge/hook.py"] == (read_exec, True)
    assert rules["/opt"] == (read_exec, True)
    assert rules["/a/w.py"] == (["read-file"], True)
    assert rules["/scratch"] == (WORK, True)


@pytest.mark.parametrize(
    ("prefix", "ruled"), [("/usr", False), ("/usr/local", False), ("/opt/hostedtoolcache/py", True)]
)
def test_the_interpreter_prefix_outside_usr_gets_a_rule(prefix: str, ruled: bool) -> None:
    rules = _rules(landlock.rules_for(IDENT, interpreter_prefix=prefix))
    assert rules.get(prefix, ([], False))[1] is ruled  # /usr itself is an optional system rule


def test_an_identity_without_workspace_or_home() -> None:
    system = ToolIdentity(tenant_id="", uid=19999, home="/home/codeforge-tools/19999", work_id="s")
    legacy = ToolIdentity(tenant_id="t", uid=10002, home="", work_id="l", workspace="/data/workspaces/t")
    assert "/home/codeforge-tools/19999" in _rules(landlock.rules_for(system, interpreter_prefix="/usr"))
    rules = _rules(landlock.rules_for(legacy, interpreter_prefix="/usr"))
    assert rules["/data/workspaces/t"] == (WORK, True)
    assert "" not in rules


def test_the_spec_value() -> None:
    assert landlock.spec_value("off", IDENT, min_abi=2, interpreter_prefix="/usr") == "off"
    value = landlock.spec_value("required", IDENT, min_abi=3, interpreter_prefix="/usr")
    assert isinstance(value, dict)
    assert value["min_abi"] == 3
    assert value["scope"] is True
    assert value["proc_self"] is True
    assert ["/data/workspaces/tenant-a/p1", WORK, True] in [[p, sorted(r), q] for p, r, q in value["rules"]]


@pytest.mark.parametrize(
    "path",
    ["/", "/proc", "/sys", "/run", "/tmp", "/data", "/home", "/var", "/var/lib", "/var/lib/codeforge",
     "/run/secrets", "/proc/1", "/data/workspaces", "/home/codeforge-tools", "/home/codeforge-tools/20000",
     "relative/path", ""],
)  # fmt: skip
def test_read_paths_that_could_reach_secrets_or_tenants_are_refused(path: str) -> None:
    assert landlock.read_path_problems((path,))


def test_read_paths_must_exist_without_a_symlink(tmp_path: Path) -> None:
    real = tmp_path / "toolchain"
    real.mkdir()
    (tmp_path / "link").symlink_to(real)
    assert landlock.read_path_problems((str(real),)) == []
    assert landlock.read_path_problems(("/usr/share",)) == []
    assert "symlink" in landlock.read_path_problems((str(tmp_path / "link"),))[0]
    assert "symlink" in landlock.read_path_problems((f"{tmp_path}/link/x",))[0]
    assert "does not exist" in landlock.read_path_problems((str(tmp_path / "missing"),))[0]


def test_parse_read_paths() -> None:
    assert landlock.parse_read_paths("") == ()
    assert landlock.parse_read_paths("/opt:/app/.venv/:") == ("/opt", "/app/.venv")


def test_the_worker_can_ask_the_kernels_abi() -> None:
    assert landlock.kernel_abi() >= 0
    lsm = Path("/sys/kernel/security/lsm")
    if lsm.exists() and "landlock" in lsm.read_text():
        assert landlock.kernel_abi() >= 1
