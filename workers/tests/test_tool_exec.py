"""The launch helper (codeforge/tool_exec.py, KI-96): the second half of the launcher.

It reads the launch spec from a descriptor, checks its own credentials, creates
the per-work directories below the tool HOME, enters the working directory
without following a symlink and executes the command with exactly the spec's
environment; anything wrong exits 125 and runs nothing.
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
from typing import TYPE_CHECKING

import pytest

from codeforge import tool_exec
from codeforge.tool_exec import EXIT_REFUSED, LaunchRefusedError
from codeforge.tool_process import TOOL_EXEC, write_spec

if TYPE_CHECKING:
    from pathlib import Path

STATUS = {
    "Uid": "20000\t20000\t20000\t20000",
    "Gid": "20000\t20000\t20000\t20000",
    "Groups": "",
    "CapInh": "0000000000000000",
    "CapPrm": "0000000000000000",
    "CapEff": "0000000000000000",
    "CapBnd": "000001ffffffffff",
    "CapAmb": "0000000000000000",
    "NoNewPrivs": "1",
}


def _spec(**overrides: object) -> dict[str, object]:
    spec: dict[str, object] = {
        "uid": 20000,
        "gid": 20000,
        "groups": [],
        "umask": 0o007,
        "env": {"PATH": "/usr/bin:/bin"},
        "landlock": "off",
        "prepare": [],
        "home": None,
        "cwd": None,
    }
    spec.update(overrides)
    return spec


def _read(spec: object) -> dict[str, object]:
    fd = os.memfd_create("t")
    os.write(fd, json.dumps(spec).encode())
    return tool_exec.read_spec(fd)


# ---------------------------------------------------------------------------
# The spec
# ---------------------------------------------------------------------------


def test_read_spec_reads_and_closes_the_descriptor() -> None:
    fd = write_spec(_spec())
    assert tool_exec.read_spec(fd) == _spec()
    with pytest.raises(OSError):
        os.fstat(fd)


@pytest.mark.parametrize("missing", tool_exec.SPEC_FIELDS)
def test_every_spec_field_is_mandatory(missing: str) -> None:
    spec = _spec()
    del spec[missing]
    with pytest.raises(LaunchRefusedError, match=missing):
        _read(spec)


@pytest.mark.parametrize(
    "bad",
    [
        {"uid": "20000"},
        {"uid": -1},
        {"uid": True},
        {"groups": ["10010"]},
        {"umask": None},
        {"env": {"A": 1}},
        {"env": ["A=1"]},
        {"prepare": "tmp/x"},
        {"home": 3},
        {"cwd": ["/"]},
        {"prepare": ["tmp/x"], "home": None},
    ],
)
def test_spec_types_are_checked(bad: dict[str, object]) -> None:
    with pytest.raises(LaunchRefusedError):
        _read(_spec(**bad))


@pytest.mark.parametrize("raw", [b"", b"not json", b"[1]", b'"x"'])
def test_a_spec_that_is_no_object_is_refused(raw: bytes) -> None:
    fd = os.memfd_create("t")
    os.write(fd, raw)
    with pytest.raises(LaunchRefusedError):
        tool_exec.read_spec(fd)


def test_an_oversized_spec_is_refused() -> None:
    fd = os.memfd_create("t")
    os.write(fd, b" " * (5 << 20))
    with pytest.raises(LaunchRefusedError, match="too large"):
        tool_exec.read_spec(fd)


@pytest.mark.parametrize("value", ["", "on", None, {"rules": []}, 1])
def test_an_unknown_landlock_setting_is_refused(value: object) -> None:
    with pytest.raises(LaunchRefusedError, match="landlock"):
        tool_exec.apply_landlock(_spec(landlock=value))


# ---------------------------------------------------------------------------
# Credentials
# ---------------------------------------------------------------------------


def test_credentials_that_match_pass() -> None:
    tool_exec.check_credentials(_spec(), dict(STATUS))
    tool_exec.check_credentials(_spec(groups=[10010]), {**STATUS, "Groups": "10010"})


@pytest.mark.parametrize(
    ("field", "value", "reason"),
    [
        ("Uid", "10001\t10001\t10001\t10001", "uid"),
        ("Uid", "20000\t0\t20000\t20000", "uid"),
        ("Gid", "20001\t20001\t20001\t20001", "gid"),
        ("Groups", "10010", "groups"),
        ("CapEff", "0000000000000080", "capabilities"),
        ("CapPrm", "0000000000000040", "capabilities"),
        ("CapInh", "0000000000000020", "capabilities"),
        ("CapAmb", "00000000000000e0", "capabilities"),
        ("CapAmb", "garbage", "capabilities"),
        ("NoNewPrivs", "0", "no_new_privs"),
    ],
)
def test_wrong_credentials_are_refused(field: str, value: str, reason: str) -> None:
    with pytest.raises(LaunchRefusedError, match=reason):
        tool_exec.check_credentials(_spec(), {**STATUS, field: value})


def test_missing_status_fields_are_refused() -> None:
    with pytest.raises(LaunchRefusedError):
        tool_exec.check_credentials(_spec(), {})


# ---------------------------------------------------------------------------
# Paths: never through a symlink
# ---------------------------------------------------------------------------


def test_open_nofollow_refuses_a_symlink_in_any_component(tmp_path: Path) -> None:
    real = tmp_path / "real"
    (real / "inner").mkdir(parents=True)
    (tmp_path / "link").symlink_to(real)
    os.close(tool_exec.open_nofollow(str(real / "inner"), directory=True))
    for path in (tmp_path / "link", tmp_path / "link" / "inner"):
        with pytest.raises(LaunchRefusedError, match="symlink"):
            tool_exec.open_nofollow(str(path), directory=True)
    (real / "file-link").symlink_to(real / "inner")
    with pytest.raises(LaunchRefusedError, match="symlink"):
        tool_exec.open_nofollow(str(real / "file-link"), directory=False)


@pytest.mark.parametrize("path", ["relative/dir", "/a/../b", "/a/./b", ""])
def test_open_nofollow_refuses_paths_that_are_not_plain(path: str) -> None:
    with pytest.raises(LaunchRefusedError):
        tool_exec.open_nofollow(path, directory=True)


def test_prepare_creates_private_directories(tmp_path: Path) -> None:
    home = tmp_path / "home"
    home.mkdir()
    tool_exec.prepare(str(home), ["tmp/abc", "claude/abc"])
    tool_exec.prepare(str(home), ["tmp/abc"])  # again: already there
    for entry in ("tmp", "tmp/abc", "claude/abc"):
        assert (home / entry).is_dir()
        assert (home / entry).stat().st_mode & 0o777 == 0o700


@pytest.mark.parametrize("planted", ["tmp", "tmp/abc"])
def test_prepare_never_follows_a_planted_symlink(planted: str, tmp_path: Path) -> None:
    """A tool may plant a symlink to another tenant's directory in its HOME (W1, S1)."""
    home = tmp_path / "home"
    (home / "tmp").mkdir(parents=True)
    victim = tmp_path / "victim"
    victim.mkdir()
    victim.chmod(0o755)
    target = home / planted
    if target.is_dir():
        target.rmdir()
    target.symlink_to(victim)
    with pytest.raises(LaunchRefusedError, match="symlink"):
        tool_exec.prepare(str(home), ["tmp/abc"])
    assert list(victim.iterdir()) == []
    assert victim.stat().st_mode & 0o777 == 0o755


@pytest.mark.parametrize("entry", ["/abs", "../up", "tmp/../x", "", "."])
def test_prepare_entries_must_be_relative_paths(entry: str, tmp_path: Path) -> None:
    with pytest.raises(LaunchRefusedError):
        tool_exec.prepare(str(tmp_path), [entry])


def test_change_directory_never_follows_a_symlink(tmp_path: Path) -> None:
    (tmp_path / "ws").mkdir()
    (tmp_path / "link").symlink_to(tmp_path / "ws")
    with pytest.raises(LaunchRefusedError):
        tool_exec.change_directory(str(tmp_path / "link"))


# ---------------------------------------------------------------------------
# The whole helper
# ---------------------------------------------------------------------------


def test_run_executes_the_command_with_exactly_the_spec(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    home = tmp_path / "home"
    home.mkdir()
    (tmp_path / "ws").mkdir()
    spec = _spec(env={"TOKEN": "s3cret"}, prepare=["tmp/t1"], home=str(home), cwd=str(tmp_path / "ws"))
    executed: list[tuple[str, list[str], dict[str, str], str, int]] = []

    def fake_exec(program: str, args: list[str], env: dict[str, str]) -> None:
        umask = os.umask(0o022)
        executed.append((program, args, env, os.getcwd(), umask))

    monkeypatch.setattr(tool_exec, "check_credentials", lambda _spec: None)
    monkeypatch.setattr(tool_exec.os, "execvpe", fake_exec)
    cwd = os.getcwd()
    try:
        tool_exec.run([str(write_spec(spec)), "bash", "-c", "true"])
    finally:
        os.chdir(cwd)
    assert executed == [("bash", ["bash", "-c", "true"], {"TOKEN": "s3cret"}, str(tmp_path / "ws"), 0o007)]
    assert (home / "tmp" / "t1").is_dir()


@pytest.mark.parametrize("argv", [[], ["3"], ["x", "true"]])
def test_run_needs_a_descriptor_and_a_command(argv: list[str]) -> None:
    with pytest.raises(LaunchRefusedError):
        tool_exec.run(argv)


def _helper(spec: object, *argv: str) -> subprocess.CompletedProcess[str]:
    fd = os.memfd_create("t", 0)
    try:
        os.write(fd, json.dumps(spec).encode())
        return subprocess.run(  # noqa: S603 - the helper under test
            [sys.executable, "-I", "-S", TOOL_EXEC, str(fd), *argv],
            pass_fds=(fd,),
            capture_output=True,
            text=True,
            check=False,
            timeout=30,
        )
    finally:
        os.close(fd)


def test_the_helper_fails_closed_with_125_and_runs_nothing(tmp_path: Path) -> None:
    marker = tmp_path / "ran"
    spec = _spec()
    del spec["landlock"]
    done = _helper(spec, "touch", str(marker))
    assert done.returncode == EXIT_REFUSED
    assert done.stderr.startswith("cf-tool-exec: ")
    assert "landlock" in done.stderr
    assert not marker.exists()


def test_the_helper_refuses_credentials_it_does_not_have(tmp_path: Path) -> None:
    marker = tmp_path / "ran"
    spec = _spec(uid=os.getuid() + 1, gid=os.getgid(), groups=sorted(os.getgroups()))
    done = _helper(spec, "touch", str(marker))
    assert done.returncode == EXIT_REFUSED
    assert "uid" in done.stderr
    assert not marker.exists()


# ---------------------------------------------------------------------------
# Landlock (KI-96 D6)
# ---------------------------------------------------------------------------


def _landlock(**overrides: object) -> dict[str, object]:
    value: dict[str, object] = {"rules": [], "scope": True, "min_abi": 2, "proc_self": False}
    value.update(overrides)
    return value


@pytest.mark.parametrize(("abi", "bits"), [(1, 13), (2, 14), (3, 15), (4, 15), (5, 16), (6, 16), (7, 16)])
def test_the_handled_rights_are_every_right_the_abi_knows(abi: int, bits: int) -> None:
    assert tool_exec.handled_access(abi) == (1 << bits) - 1


@pytest.mark.parametrize(("abi", "size"), [(1, 8), (2, 8), (3, 8), (4, 16), (5, 16), (6, 24), (7, 24)])
def test_the_ruleset_attribute_grows_with_the_abi(abi: int, size: int) -> None:
    attr = tool_exec.ruleset_attr(abi, tool_exec.handled_access(abi), 3)
    assert len(attr) == size
    assert int.from_bytes(attr[:8], "little") == tool_exec.handled_access(abi)
    if abi >= 6:
        assert int.from_bytes(attr[16:24], "little") == 3


@pytest.mark.parametrize("missing", ["rules", "scope", "min_abi", "proc_self"])
def test_every_landlock_field_is_mandatory(missing: str) -> None:
    value = _landlock()
    del value[missing]
    with pytest.raises(LaunchRefusedError, match="landlock"):
        tool_exec.apply_landlock(_spec(landlock=value))


@pytest.mark.parametrize(
    "rules",
    [
        [["/usr", ["fly"], True]],
        [["/usr", "read-file", True]],
        [["usr", ["read-file"], True]],
        [["/usr", ["read-file"]]],
        ["/usr"],
    ],
)
def test_malformed_rules_are_refused(rules: object, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(tool_exec, "landlock_abi", lambda: 7)
    with pytest.raises(LaunchRefusedError, match="rule"):
        tool_exec.apply_landlock(_spec(landlock=_landlock(rules=rules)))


@pytest.mark.parametrize(("abi", "reason"), [(0, "no Landlock"), (1, "ABI 1")])
def test_a_kernel_below_the_minimum_abi_runs_nothing(abi: int, reason: str, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(tool_exec, "landlock_abi", lambda: abi)
    with pytest.raises(LaunchRefusedError, match=reason):
        tool_exec.apply_landlock(_spec(landlock=_landlock()))


class _FakeLandlock:
    """Records the Landlock system calls instead of making them."""

    def __init__(self) -> None:
        self.rules: list[tuple[int, int]] = []  # (allowed, inode of the rule's descriptor)
        self.attr = b""
        self.restricted = False

    def syscall(self, number: int, *args: object) -> int:
        if number == tool_exec.SYS_LANDLOCK_CREATE_RULESET:
            self.attr = bytes(args[0])  # type: ignore[call-overload]
            return os.open("/", os.O_PATH | os.O_CLOEXEC)
        if number == tool_exec.SYS_LANDLOCK_ADD_RULE:
            raw = bytes(args[2])  # type: ignore[call-overload]
            allowed = int.from_bytes(raw[:8], "little")
            fd = int.from_bytes(raw[8:12], "little", signed=True)
            self.rules.append((allowed, os.fstat(fd).st_ino))
            return 0
        if number == tool_exec.SYS_LANDLOCK_RESTRICT_SELF:
            self.restricted = True
            return 0
        raise AssertionError(number)


@pytest.fixture
def fake_landlock(monkeypatch: pytest.MonkeyPatch) -> _FakeLandlock:
    fake = _FakeLandlock()
    monkeypatch.setattr(tool_exec, "landlock_abi", lambda: 7)
    monkeypatch.setattr(tool_exec, "_syscall", fake.syscall)
    return fake


def test_rules_are_masked_to_the_entry_type(tmp_path: Path, fake_landlock: _FakeLandlock) -> None:
    directory = tmp_path / "d"
    directory.mkdir()
    file = tmp_path / "f"
    file.write_text("x")
    rules = [
        [str(directory), ["read-file", "read-dir", "execute"], True],
        [str(file), ["read-file", "read-dir", "execute", "make-dir"], True],
        [str(tmp_path / "missing"), ["read-file"], False],
    ]
    tool_exec.apply_landlock(_spec(landlock=_landlock(rules=rules, scope=True)))
    rights = tool_exec.FS_RIGHTS
    assert fake_landlock.rules == [
        (rights["read-file"] | rights["read-dir"] | rights["execute"], directory.stat().st_ino),
        (rights["read-file"] | rights["execute"], file.stat().st_ino),
    ]
    assert fake_landlock.restricted
    # Signals and abstract sockets are scoped (ABI 6+).
    assert int.from_bytes(fake_landlock.attr[16:24], "little") == 3


def test_a_required_rule_path_must_exist_without_a_symlink(tmp_path: Path, fake_landlock: _FakeLandlock) -> None:
    real = tmp_path / "real"
    real.mkdir()
    (tmp_path / "link").symlink_to(real)
    for path in (f"{tmp_path}/link/sub", str(tmp_path / "link"), str(tmp_path / "missing")):
        with pytest.raises(LaunchRefusedError):
            tool_exec.apply_landlock(_spec(landlock=_landlock(rules=[[path, ["read-file"], True]])))
    assert not fake_landlock.restricted


def test_the_exec_targets_own_proc_entry_is_pinned_and_allowed(fake_landlock: _FakeLandlock) -> None:
    before = set(os.listdir("/proc/self/fd"))
    tool_exec.apply_landlock(_spec(landlock=_landlock(proc_self=True)))
    rights = tool_exec.FS_RIGHTS
    (allowed, inode) = fake_landlock.rules[-1]
    assert allowed == rights["read-file"] | rights["read-dir"]
    assert inode == os.stat(f"/proc/{os.getpid()}").st_ino
    # The descriptor stays open across the exec: it pins the dentry for the command.
    kept = {int(fd) for fd in set(os.listdir("/proc/self/fd")) - before}
    kept = {fd for fd in kept if os.path.exists(f"/proc/self/fd/{fd}")}
    assert len(kept) == 1
    fd = kept.pop()
    assert os.get_inheritable(fd)
    os.close(fd)


_CONFINED = """
import importlib.util, json, sys
spec = importlib.util.spec_from_file_location("tool_exec", sys.argv[1])
tool_exec = importlib.util.module_from_spec(spec)
spec.loader.exec_module(tool_exec)
tool_exec.apply_landlock({"landlock": json.loads(sys.argv[2])})
for path in sys.argv[3:]:
    try:
        with open(path) as f:
            f.read()
        print(path, "read")
    except OSError as exc:
        print(path, exc.strerror)
"""


@pytest.mark.skipif(tool_exec.landlock_abi() < 2, reason="needs Landlock ABI 2 or later")
def test_a_real_ruleset_confines_the_process(tmp_path: Path) -> None:
    allowed = tmp_path / "allowed"
    allowed.mkdir()
    (allowed / "f").write_text("x")
    (tmp_path / "denied").write_text("x")
    rules = [["/usr", ["execute", "read-file", "read-dir"], True], [str(allowed), ["read-file", "read-dir"], True]]
    if not sys.base_prefix.startswith("/usr/"):
        rules.append([sys.base_prefix, ["execute", "read-file", "read-dir"], True])
    value = {"rules": rules, "scope": True, "min_abi": 2, "proc_self": False}
    done = subprocess.run(  # noqa: S603 - the test's own interpreter
        [sys.executable, "-I", "-S", "-c", _CONFINED, TOOL_EXEC, json.dumps(value),
         str(allowed / "f"), str(tmp_path / "denied"), "/etc/hostname"],
        capture_output=True,
        text=True,
        check=False,
    )  # fmt: skip
    assert done.returncode == 0, done.stderr
    assert done.stdout.splitlines() == [
        f"{allowed / 'f'} read",
        f"{tmp_path / 'denied'} Permission denied",
        "/etc/hostname Permission denied",
    ]
