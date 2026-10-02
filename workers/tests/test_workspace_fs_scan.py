"""Only codeforge.workspace_fs touches workspace files directly (KI-95).

Like the spawn-site scan of KI-71 (test_tool_process.py): the worker reads and
writes workspace files in its own process, so every such access must go
through the symlink-safe helper. This test scans the worker's code for direct
file access (open, Path.read_text, os.walk, ...) and fails on any call site
that is not listed below with the reason it never touches a workspace.
"""

from __future__ import annotations

import ast
from pathlib import Path

SOURCE_DIR = Path(__file__).resolve().parent.parent / "codeforge"

# Calls that read, write, list or walk files by path.
_FILE_ACCESS_ATTRIBUTES = frozenset(
    {
        "open",
        "read_text",
        "read_bytes",
        "write_text",
        "write_bytes",
        "iterdir",
        "glob",
        "rglob",
        "walk",
        "fwalk",
        "scandir",
        "listdir",
        "mkdir",
        "makedirs",
        "rmtree",
        "copy",
        "copyfile",
        "copytree",
        "unlink",
        "readlink",
        "chmod",
        "chown",
    }
)

_CONFIG = "operator or packaged configuration, never a workspace"
_DATASETS = "benchmark dataset files and their download cache (operator directories), never a workspace"
_SHARING = (
    "KI-71 permission sharing: worker-created paths, and the workspace walk through directory "
    "descriptors with O_NOFOLLOW opens and inode re-checks (ADR-017 decision 4)"
)
_PRIVATE_DIR = "the run's private socket directory (mkdtemp, 0750), not a workspace"

# (module, call) -> why it never touches a workspace path, or why it is safe.
ALLOWED: dict[tuple[str, str], str] = {
    ("__init__.py", "candidate.read_text"): "the VERSION file of the installation",
    ("config.py", "open"): _CONFIG,
    ("config.py", "Path(path).read_text"): "*_FILE secrets (operator paths)",
    ("secrets.py", "file_path.read_text"): "*_FILE secrets (operator paths)",
    ("secrets.py", "directory.chmod"): "locks /run/secrets after startup (ADR-017 decision 3)",
    ("pricing.py", "open"): _CONFIG,
    ("skills/registry.py", "_BUILTINS_DIR.glob"): "packaged built-in skills",
    ("skills/registry.py", "open"): "packaged built-in skills",
    ("consumer/_conversation_prompt_builder.py", "yaml_path.open"): "packaged prompt YAML",
    ("llm.py", "_LLM_REQUEST_LOG_DIR.mkdir"): "debug request log directory (operator opt-in)",
    ("llm.py", "log_file.write_text"): "debug request log directory (operator opt-in)",
    ("mcp_workbench.py", "open"): "os.devnull as the stdio servers' error log",
    ("claude_code_executor.py", "os.open"): _PRIVATE_DIR,
    ("claude_code_executor.py", "os.chmod"): _PRIVATE_DIR,
    ("claude_code_executor.py", "shutil.rmtree"): _PRIVATE_DIR,
    ("evaluation/cache.py", "cache_dir.mkdir"): _DATASETS,
    ("evaluation/cache.py", "open"): _DATASETS,
    ("evaluation/cache.py", "tmp_path.write_bytes"): _DATASETS,
    ("evaluation/cache.py", "tmp_path.unlink"): _DATASETS,
    ("evaluation/datasets.py", "p.parent.mkdir"): "benchmark result files (operator path)",
    ("evaluation/datasets.py", "p.write_text"): "benchmark result files (operator path)",
    ("evaluation/runners/agent.py", "shutil.rmtree"): (
        "removes the benchmark workspace; shutil.rmtree works on directory descriptors and "
        "never follows a symlink (a symlinked top is refused)"
    ),
    ("tool_process.py", "Path('/proc/self/status').read_text"): "the isolation probe reads its own status",
    ("tool_process.py", "SECRETS_DIR.iterdir"): "the isolation probe lists /run/secrets",
    ("tool_process.py", "os.chown"): _SHARING,
    ("tool_process.py", "os.chmod"): _SHARING,
    ("tool_process.py", "os.fwalk"): _SHARING,
    ("tool_process.py", "os.open"): _SHARING,
    ("tool_process.py", "os.unlink"): _SHARING,
}


def file_access_calls(path: Path, base: Path = SOURCE_DIR) -> list[tuple[str, str, int]]:
    """(module, call, line) of every direct file access call in *path*."""
    tree = ast.parse(path.read_text(), filename=str(path))
    module = path.relative_to(base).as_posix()
    found: list[tuple[str, str, int]] = []
    for node in ast.walk(tree):
        if not isinstance(node, ast.Call):
            continue
        func = node.func
        if isinstance(func, ast.Name) and func.id == "open":
            found.append((module, "open", node.lineno))
        elif isinstance(func, ast.Attribute) and func.attr in _FILE_ACCESS_ATTRIBUTES:
            owner = ast.unparse(func.value)
            # A WorkspaceRoot is named root by convention: its methods are the helper.
            if owner != "root":
                found.append((module, f"{owner}.{func.attr}", node.lineno))
    return found


def _all_calls() -> list[tuple[str, str, int]]:
    calls: list[tuple[str, str, int]] = []
    for path in sorted(SOURCE_DIR.rglob("*.py")):
        if path == SOURCE_DIR / "workspace_fs.py":
            continue
        calls.extend(file_access_calls(path))
    return calls


def test_only_workspace_fs_accesses_files() -> None:
    """A new direct file access outside codeforge.workspace_fs must be routed through it or listed here."""
    offenders = [f"{module}:{line} {call}" for module, call, line in _all_calls() if (module, call) not in ALLOWED]
    assert offenders == [], (
        "read and write workspace files through codeforge.workspace_fs (KI-95), "
        "or list the call in ALLOWED with the reason it never touches a workspace:\n" + "\n".join(offenders)
    )


def test_allowlist_has_no_stale_entries() -> None:
    used = {(module, call) for module, call, _ in _all_calls()}
    assert sorted(set(ALLOWED) - used) == []


def test_the_scan_finds_file_access(tmp_path: Path) -> None:
    sample = tmp_path / "sample.py"
    sample.write_text(
        "import os\n"
        "from pathlib import Path\n"
        "def f(ws, p: Path):\n"
        "    open(os.path.join(ws, 'a'))\n"
        "    p.read_text()\n"
        "    p.write_bytes(b'')\n"
        "    os.walk(ws)\n"
        "    p.rglob('*')\n"
        "    os.scandir(ws)\n"
        "    p.open()\n"
        "    'a'.replace('a', 'b')\n"
        "    root.read_text('a')\n"
    )
    found = file_access_calls(sample, tmp_path)
    assert [call for _, call, _ in found] == [
        "open",
        "p.read_text",
        "p.write_bytes",
        "os.walk",
        "p.rglob",
        "os.scandir",
        "p.open",
    ]
