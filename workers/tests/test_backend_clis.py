"""The agent backend CLIs of the standard worker image (KI-118).

Owner decision (2026-10-04): the backend CLIs go into the standard worker
image. Dockerfile.worker installs each pinned and verified: Aider from
hash-locked wheels (workers/aider-requirements.txt) in its own virtual
environment, OpenCode as the native binary of its npm platform package
(sha256 of each tarball), Goose from Block's multi-arch image by digest.
Claude Code is a build option (INSTALL_CLAUDE_CODE=true), not part of the
published images (owner decision 2026-10-06). They live below /usr, which
Landlock lets tool processes read and execute, and on the tool PATH
(CODEFORGE_TOOL_PATH), where the worker's backend executors look them up;
they start, like every tool, only through codeforge.tool_process as the
tenant's tool user. Every backend the worker registers is shipped, a build
option or listed in NOT_SHIPPED with the reason. The Docker suite runs each
installed CLI as a tenant's tool user in the built image
(docker_isolation_checks "backends").
"""

from __future__ import annotations

import os
import re
from pathlib import Path
from typing import TYPE_CHECKING

import yaml

from codeforge.backends import build_default_router
from codeforge.backends.opencode import OpenCodeExecutor
from codeforge.config import WorkerSettings
from codeforge.tool_identity import DEFAULT_TOOL_PATH

if TYPE_CHECKING:
    import pytest

REPO = Path(__file__).resolve().parents[2]
DOCKERFILE = (REPO / "Dockerfile.worker").read_text()
AIDER_REQUIREMENTS = REPO / "workers" / "aider-requirements.txt"

# Backend name -> the CLI the image installs.
SHIPPED = {
    "aider": "/usr/local/bin/aider",
    "goose": "/usr/local/bin/goose",
    "opencode": "/usr/local/bin/opencode",
}
# Installed only with --build-arg INSTALL_CLAUDE_CODE=true: Claude Code's
# licence is proprietary, so the published images do not carry it (owner
# decision 2026-10-06).
BUILD_OPTION = {"claudecode": "/usr/local/bin/claude"}
NOT_SHIPPED = {
    "openhands": "an HTTP service of its own (CODEFORGE_OPENHANDS_URL), not a CLI",
    "plandex": "needs a Plandex server and an interactive sign-in per HOME; plandex 2.2.1 (upstream inactive "
    "since 2025-10) has no `tell --yes` or `--model`, which its executor passes",
    "sweagent": "no Go adapter dispatches tasks to it, and it runs its tasks in Docker",
}


def _stage(name: str) -> str:
    match = re.search(rf"^FROM [^\n]+ AS {re.escape(name)}\n(.*?)(?=^FROM |\Z)", DOCKERFILE, re.MULTILINE | re.DOTALL)
    assert match, f"Dockerfile.worker has no stage {name}"
    return match.group(1)


def _runtime() -> str:
    return DOCKERFILE.split("# --- Runtime stage ---", 1)[1]


def test_every_registered_backend_is_shipped_or_explained() -> None:
    registered = {*build_default_router().available_backends(), "claudecode"}

    assert registered == set(SHIPPED) | set(BUILD_OPTION) | set(NOT_SHIPPED)
    assert len(registered) == len(SHIPPED) + len(BUILD_OPTION) + len(NOT_SHIPPED)


def test_the_executors_find_the_shipped_clis_on_the_tool_path(monkeypatch: pytest.MonkeyPatch) -> None:
    for variable in (
        "CODEFORGE_AIDER_PATH",
        "CODEFORGE_GOOSE_PATH",
        "CODEFORGE_OPENCODE_PATH",
        "CODEFORGE_CLAUDECODE_PATH",
    ):
        monkeypatch.delenv(variable, raising=False)
    router = build_default_router()
    commands = {name: router.get(name).info.cli_command for name in SHIPPED if router.get(name)}  # type: ignore[union-attr]
    commands["claudecode"] = WorkerSettings().claudecode_path
    tool_path = DEFAULT_TOOL_PATH.split(":")

    for name, path in (SHIPPED | BUILD_OPTION).items():
        assert os.path.dirname(path) in tool_path, name
        assert os.path.basename(path) == commands[name], (
            f"{name}: the executor runs {commands[name]!r}, the image installs {path}"
        )
        assert path.startswith("/usr/"), f"{name}: Landlock lets tool processes execute below /usr only"


def test_the_runtime_stage_installs_each_shipped_cli() -> None:
    runtime = _runtime()
    assert (
        "COPY --from=aider /usr/local/lib/codeforge-backends/aider /usr/local/lib/codeforge-backends/aider" in runtime
    )
    assert "ln -s ../lib/codeforge-backends/aider/bin/aider /usr/local/bin/aider" in runtime
    assert "COPY --from=goose /usr/local/bin/goose /usr/local/bin/goose" in runtime
    assert re.search(r"apt-get install[^&]*\blibgomp1\b", runtime), "goose links libgomp"
    assert "-C /usr/local/bin --strip-components=2 --no-same-owner package/bin/opencode" in runtime
    assert "--mount=type=bind,from=backend-clis,target=/tmp/backend-clis" in runtime
    # The tool processes' environment turns off the CLIs' self-updates and
    # analytics, and goose's keyring (none in the container).
    for setting in (
        "AIDER_ANALYTICS_DISABLE=true",
        "AIDER_CHECK_UPDATE=false",
        "OPENCODE_DISABLE_AUTOUPDATE=true",
        "GOOSE_DISABLE_KEYRING=1",
    ):
        assert setting in runtime, setting


def _checksummed_downloads(stage_prefix: str, target: str) -> None:
    """Each architecture's stage is one sha256-checked download of the same pinned npm tarball."""
    assert f"FROM {stage_prefix}-${{TARGETARCH}} AS " in DOCKERFILE
    versions = set()
    for arch, npm_arch in (("amd64", "x64"), ("arm64", "arm64")):
        stage = _stage(f"{stage_prefix}-{arch}")
        adds = re.findall(r"^ADD --checksum=sha256:([0-9a-f]{64}) (\S+) (\S+)$", stage, re.MULTILINE)
        assert [added for _, _, added in adds] == [target], stage
        assert len(stage.strip().splitlines()) == 1, "the stage is the checksummed download only"
        match = re.fullmatch(r"https://registry\.npmjs\.org/(\S+)/-/(\S+)-(\d+\.\d+\.\d+)\.tgz", adds[0][1])
        assert match, adds[0][1]
        assert npm_arch in match.group(1), f"{arch}: {adds[0][1]}"
        versions.add(match.group(3))
    assert len(versions) == 1, f"both architectures get the same version: {versions}"


def test_downloads_are_pinned_and_checksummed() -> None:
    _checksummed_downloads("backend-clis", "/opencode.tgz")
    _checksummed_downloads("claude-code", "/claude-code.tgz")
    assert re.search(
        r"^FROM ghcr\.io/block/goose:\d+\.\d+\.\d+@sha256:[0-9a-f]{64} AS goose$", DOCKERFILE, re.MULTILINE
    )
    assert not re.search(r"curl[^\n]*\|\s*(ba)?sh|npm install|:latest|releases/latest", DOCKERFILE)


def test_claude_code_is_a_build_option_off_by_default() -> None:
    """The default build neither downloads nor installs Claude Code; INSTALL_CLAUDE_CODE=true does both."""
    head = DOCKERFILE.split("\nFROM ", 1)[0]
    assert re.search(r"^ARG INSTALL_CLAUDE_CODE=false$", head, re.MULTILINE), "a global default of false"
    assert "--build-arg INSTALL_CLAUDE_CODE=true" in head, "the build command is in the header comment"
    assert "FROM claude-code-${INSTALL_CLAUDE_CODE} AS claude-code" in DOCKERFILE
    assert "FROM claude-code-${TARGETARCH} AS claude-code-true" in DOCKERFILE
    assert _stage("claude-code-false").strip() == "", "false selects an empty stage: nothing is downloaded"
    assert "claude" not in _stage("backend-clis-amd64") + _stage("backend-clis-arm64")
    runtime = _runtime()
    assert "--mount=type=bind,from=claude-code,target=/tmp/claude-code" in runtime
    install = re.search(r'if \[ "\$INSTALL_CLAUDE_CODE" = true \]; then \\\n(.*?)\n\s*fi', runtime, re.DOTALL)
    assert install, "Claude Code is extracted only with INSTALL_CLAUDE_CODE=true"
    assert "-C /usr/local/bin --strip-components=1 --no-same-owner package/claude" in install.group(1)
    assert runtime.count("package/claude") == 1
    assert re.search(r"^ARG INSTALL_CLAUDE_CODE$", runtime, re.MULTILINE)


def test_aider_is_hash_locked() -> None:
    stage = _stage("aider")
    assert "python -m venv /usr/local/lib/codeforge-backends/aider" in stage
    assert "--require-hashes --only-binary=:all: -r /tmp/aider-requirements.txt" in stage
    text = AIDER_REQUIREMENTS.read_text()
    requirements = re.findall(r"^([a-z0-9][a-z0-9._-]*)==(\S+) \\$", text, re.MULTILINE)
    assert ("aider-chat", "0.86.2") in requirements
    assert len(requirements) > 50, "every dependency is listed (pip --require-hashes)"
    blocks = re.split(r"^(?=[a-z0-9])", text, flags=re.MULTILINE)
    for block in blocks:
        if block.startswith("#") or not block.strip():
            continue
        assert re.search(r"--hash=sha256:[0-9a-f]{64}", block), block[:80]


def test_opencode_gets_the_prompt_as_its_message() -> None:
    """`opencode run` takes the message as positionals (there is no --prompt); after -- a prompt is never an option."""
    cmd = OpenCodeExecutor(cli_path="opencode")._build_command(
        "-fix the bug", {"model": "openai/gpt-4o", "extra_args": ["--auto"]}
    )

    assert cmd == ["opencode", "run", "--model", "openai/gpt-4o", "--auto", "--", "-fix the bug"]


def test_egress_is_an_explicit_override() -> None:
    """The production worker has no route out; docker-compose.egress.yml adds one, to the worker only."""
    prod = yaml.safe_load((REPO / "docker-compose.prod.yml").read_text())
    override = yaml.safe_load((REPO / "docker-compose.egress.yml").read_text())

    assert prod["services"]["worker"]["networks"] == ["internal"]
    assert prod["networks"]["internal"]["internal"] is True
    assert prod["networks"]["egress"].get("internal") is not True
    assert set(override) == {"services"}
    assert override["services"] == {"worker": {"networks": ["internal", "egress"]}}


def test_egress_override_says_what_it_opens() -> None:
    """Tool processes get the route unfiltered: the file names the LAN, the host and the metadata service, and how to block them."""
    raw = (REPO / "docker-compose.egress.yml").read_bytes()
    assert raw.isascii()
    text = raw.decode()
    for needle in ("host's LAN", "published", "169.254.169.254", "netutil.OutboundPolicy", "DOCKER-USER", "INPUT"):
        assert needle in text, needle
    for net in ("169.254.0.0/16", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"):
        assert net in text, net


def test_egress_rules_keep_dns_and_the_opened_hosts() -> None:
    """The example's DROPs would cut DNS (the embedded resolver's upstreams sit in those ranges) and the hosts opened on purpose."""
    lines = [line.removeprefix("#").strip() for line in (REPO / "docker-compose.egress.yml").read_text().splitlines()]
    rules = [line for line in lines if line.startswith("iptables ")]
    first_accept = min(i for i, rule in enumerate(rules) if "-j ACCEPT" in rule)
    last_drop = max(i for i, rule in enumerate(rules) if "-j DROP" in rule)
    assert all(rule.startswith("iptables -I ") for rule in rules), "-I: a later rule lands above the earlier ones"
    assert first_accept > last_drop, "inserted after the DROPs, the ACCEPTs end up above them"
    dns = [rule for rule in rules if "--dport 53" in rule and "-j ACCEPT" in rule]
    assert {chain for rule in dns for chain in ("DOCKER-USER", "INPUT") if f"-I {chain} " in rule} == {
        "DOCKER-USER",
        "INPUT",
    }
    assert any('-p "$proto"' in rule for rule in dns), "udp and tcp"
    text = "\n".join(lines)
    assert "awk '/^nameserver/" in text
    assert "/run/systemd/resolve/resolv.conf" in text
    assert "/etc/resolv.conf" in text
    assert "mcp.allowed_private_hosts" in text.split('iptables -I INPUT -i "$EGRESS_IF" -j DROP', 1)[1]
