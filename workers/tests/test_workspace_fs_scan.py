"""Only codeforge.workspace_fs touches workspace files directly (KI-95).

Like the spawn-site scan of KI-71 (test_tool_process.py): the worker reads and
writes workspace files in its own process, so every such access must go
through the symlink-safe helper. This test scans the worker's code for direct
file access (open, Path.read_text, os.walk, os.replace, shutil.move, ...) and
fails on any call site that is not listed below, by module, function and call
with its exact number of calls, with the reason it never touches a workspace.
Calls on a WorkspaceRoot are the helper itself: the scan infers which names
hold one (an annotation, or a WorkspaceRoot / open_tool_workspace / subroot
value), never from a variable's name alone.
"""

from __future__ import annotations

import ast
from collections import Counter
from pathlib import Path

SOURCE_DIR = Path(__file__).resolve().parent.parent / "codeforge"

# Module functions that read, write, list, walk or change files by path.
_MODULE_CALLS: dict[str, frozenset[str]] = {
    "os": frozenset(
        {
            "open",
            "walk",
            "fwalk",
            "scandir",
            "listdir",
            "mkdir",
            "makedirs",
            "remove",
            "unlink",
            "rmdir",
            "removedirs",
            "rename",
            "renames",
            "replace",
            "symlink",
            "link",
            "readlink",
            "chmod",
            "chown",
            "lchown",
            "truncate",
            "mkfifo",
            "mknod",
        }
    ),
    "shutil": frozenset({"copy", "copy2", "copyfile", "copymode", "copystat", "copytree", "move", "rmtree", "chown"}),
    "glob": frozenset({"glob", "iglob"}),
    "io": frozenset({"open"}),
}

# Path (and file-object) methods flagged on any object but a WorkspaceRoot.
_ANY_OWNER = frozenset(
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
        "touch",
        "mkdir",
        "rmdir",
        "unlink",
        "symlink_to",
        "hardlink_to",
        "chmod",
        "chown",
        "readlink",
        "rmtree",
        "copyfile",
        "copytree",
        "scandir",
        "listdir",
        "fwalk",
        "makedirs",
    }
)
# Path methods whose names other types share (str.replace): flagged on a Path.
_PATH_ONLY = frozenset({"replace", "rename"})

_ROOT_FACTORIES = frozenset({"WorkspaceRoot", "WorkspaceRoot.operator_dir", "open_tool_workspace"})
_PATH_FACTORIES = frozenset({"Path", "PurePath", "pathlib.Path"})
_PATH_ATTRIBUTES = frozenset({"parent", "resolve", "absolute", "expanduser", "with_name", "with_suffix", "joinpath"})

_CONFIG = "operator or packaged configuration, never a workspace"
_DATASETS = "benchmark dataset download cache (operator directory), never a workspace"
_SHARING = (
    "KI-71 permission sharing: worker-created paths, and the workspace walk through directory "
    "descriptors with O_NOFOLLOW opens and inode re-checks (ADR-017 decision 4)"
)
_PRIVATE_DIR = "the run's private socket directory (mkdtemp, 0750), not a workspace"
_RESULTS = "benchmark result files (operator path)"

# (module, function, call) -> (exact number of calls, why it never touches a workspace path or why it is safe).
_STAMP = (
    "the workspace sharing stamp at the workspace root: read with O_NOFOLLOW | O_NONBLOCK as a regular "
    "file only, written as a new O_EXCL | O_NOFOLLOW file renamed over its place (KI-71)"
)
ALLOWED: dict[tuple[str, str, str], tuple[int, str]] = {
    ("__init__.py", "_read_version", "candidate.read_text"): (1, "the VERSION file of the installation"),
    ("config.py", "load_yaml_config", "open"): (1, _CONFIG),
    ("config.py", "read_secret_file", "Path(path).read_text"): (1, "*_FILE secrets (operator paths)"),
    ("secrets.py", "get_secret", "file_path.read_text"): (1, "*_FILE secrets (operator paths)"),
    ("secrets.py", "lock_secrets_dir", "directory.chmod"): (1, "locks /run/secrets after startup (ADR-017 decision 3)"),
    ("pricing.py", "PricingTable.__init__", "open"): (1, _CONFIG),
    ("skills/registry.py", "load_builtin_skills", "_BUILTINS_DIR.glob"): (1, "packaged built-in skills"),
    ("skills/registry.py", "load_builtin_skills", "open"): (1, "packaged built-in skills"),
    ("consumer/_conversation_prompt_builder.py", "load_step_by_step_prompt", "yaml_path.open"): (
        1,
        "packaged prompt YAML",
    ),
    ("llm.py", "_log_request_metadata", "_LLM_REQUEST_LOG_DIR.mkdir"): (
        1,
        "debug request log directory (operator opt-in)",
    ),
    ("llm.py", "_log_request_metadata", "log_file.write_text"): (1, "debug request log directory (operator opt-in)"),
    ("mcp_workbench.py", "McpServerConnection._open", "open"): (1, "os.devnull as the stdio servers' error log"),
    ("claude_code_executor.py", "PolicySocketServer.__aenter__", "os.chmod"): (1, _PRIVATE_DIR),
    ("claude_code_executor.py", "PolicySocketServer.__aenter__", "shutil.rmtree"): (1, _PRIVATE_DIR),
    ("claude_code_executor.py", "PolicySocketServer.__aexit__", "shutil.rmtree"): (1, _PRIVATE_DIR),
    ("claude_code_executor.py", "_write_private_file", "os.open"): (1, _PRIVATE_DIR),
    ("evaluation/cache.py", "get_cache_dir", "cache_dir.mkdir"): (1, _DATASETS),
    ("evaluation/cache.py", "download_dataset", "tmp_path.write_bytes"): (1, _DATASETS),
    ("evaluation/cache.py", "download_dataset", "tmp_path.unlink"): (2, _DATASETS),
    ("evaluation/cache.py", "_verify_checksum", "open"): (1, _DATASETS),
    ("evaluation/cache.py", "load_jsonl", "open"): (1, _DATASETS),
    ("evaluation/cache.py", "load_json", "open"): (1, _DATASETS),
    ("evaluation/cache.py", "download_hf_dataset", "open"): (1, _DATASETS),
    ("evaluation/cache.py", "download_hf_dataset", "tmp_path.unlink"): (1, _DATASETS),
    ("evaluation/cache.py", "download_hf_dataset_parquet", "open"): (1, _DATASETS),
    ("evaluation/cache.py", "download_hf_dataset_parquet", "tmp_path.unlink"): (1, _DATASETS),
    ("evaluation/datasets.py", "save_results", "p.parent.mkdir"): (1, _RESULTS),
    ("evaluation/datasets.py", "save_results", "p.write_text"): (1, _RESULTS),
    ("evaluation/runners/agent.py", "AgentBenchmarkRunner.run_task", "shutil.rmtree"): (
        1,
        "removes the benchmark workspace; shutil.rmtree works on directory descriptors and "
        "never follows a symlink (a symlinked top is refused)",
    ),
    ("tool_process.py", "_own_status", "Path('/proc/self/status').read_text"): (
        1,
        "the isolation probe reads its own status",
    ),
    ("tool_process.py", "_probe_paths", "SECRETS_DIR.iterdir"): (1, "the isolation probe lists /run/secrets"),
    ("tool_process.py", "share_with_tools", "os.chown"): (1, _SHARING),
    ("tool_process.py", "share_with_tools", "os.chmod"): (2, _SHARING),
    ("tool_process.py", "_share_entry", "os.open"): (1, _SHARING),
    ("tool_process.py", "share_workspace_root", "os.fwalk"): (1, _SHARING),
    ("tool_process.py", "_read_stamp", "os.open"): (1, _STAMP),
    ("tool_process.py", "_write_stamp", "os.open"): (1, _STAMP),
    ("tool_process.py", "_write_stamp", "os.replace"): (1, _STAMP),
    ("tool_process.py", "_write_stamp", "os.unlink"): (1, _STAMP),
}


def _target_names(target: ast.expr) -> list[str]:
    if isinstance(target, ast.Name):
        return [target.id]
    if isinstance(target, ast.Tuple | ast.List):
        return [name for element in target.elts for name in _target_names(element)]
    return []


def _bindings(node: ast.AST) -> list[tuple[list[str], ast.expr]]:
    """(names, value) of the assignments and with-items below *node*."""
    found: list[tuple[list[str], ast.expr]] = []
    for child in ast.walk(node):
        if isinstance(child, ast.Assign):
            found.append(([name for target in child.targets for name in _target_names(target)], child.value))
        elif isinstance(child, ast.AnnAssign | ast.NamedExpr) and child.value is not None:
            found.append((_target_names(child.target), child.value))
        elif isinstance(child, ast.With | ast.AsyncWith):
            found.extend(
                (_target_names(item.optional_vars), item.context_expr)
                for item in child.items
                if item.optional_vars is not None
            )
    return found


class _Scanner(ast.NodeVisitor):
    """Collects (function, call) of every direct file access in a module."""

    def __init__(self, tree: ast.Module) -> None:
        self.found: list[tuple[str, str, int]] = []
        self._scopes: list[tuple[str, set[str], set[str]]] = []  # (name, root names, path names)
        self._imported: dict[str, str] = {}  # from os import remove -> {"remove": "os.remove"}
        for node in ast.walk(tree):
            if isinstance(node, ast.ImportFrom) and node.module in _MODULE_CALLS:
                for alias in node.names:
                    if alias.name in _MODULE_CALLS[node.module]:
                        self._imported[alias.asname or alias.name] = f"{node.module}.{alias.name}"
        roots, paths = self._infer(tree, set(), set())
        self._scopes.append(("<module>", roots, paths))

    # -- inference -------------------------------------------------------------------

    def _is_root(self, expr: ast.expr, roots: set[str] | None = None) -> bool:
        roots = self._names(1) if roots is None else roots
        if isinstance(expr, ast.Name):
            return expr.id in roots
        if isinstance(expr, ast.Call):
            if ast.unparse(expr.func) in _ROOT_FACTORIES:
                return True
            if isinstance(expr.func, ast.Attribute) and expr.func.attr == "subroot":
                return self._is_root(expr.func.value, roots)
        return False

    def _is_path(self, expr: ast.expr, paths: set[str] | None = None) -> bool:
        paths = self._names(2) if paths is None else paths
        if isinstance(expr, ast.Name):
            return expr.id in paths
        if isinstance(expr, ast.Call):
            if ast.unparse(expr.func) in _PATH_FACTORIES:
                return True
            func = expr.func
            return (
                isinstance(func, ast.Attribute) and func.attr in _PATH_ATTRIBUTES and self._is_path(func.value, paths)
            )
        if isinstance(expr, ast.BinOp) and isinstance(expr.op, ast.Div):
            return self._is_path(expr.left, paths)
        if isinstance(expr, ast.Attribute) and expr.attr in _PATH_ATTRIBUTES:
            return self._is_path(expr.value, paths)
        return False

    def _infer(self, node: ast.AST, roots: set[str], paths: set[str]) -> tuple[set[str], set[str]]:
        """The names bound to a WorkspaceRoot and to a Path in *node* (a fixpoint over its assignments)."""
        roots, paths = set(roots), set(paths)
        if isinstance(node, ast.FunctionDef | ast.AsyncFunctionDef):
            arguments = node.args
            for arg in [*arguments.posonlyargs, *arguments.args, *arguments.kwonlyargs]:
                annotation = ast.unparse(arg.annotation) if arg.annotation is not None else ""
                if "WorkspaceRoot" in annotation:
                    roots.add(arg.arg)
                elif "Path" in annotation:
                    paths.add(arg.arg)
        bindings = _bindings(node)
        changed = True
        while changed:
            changed = False
            for names, value in bindings:
                for kind, known in ((self._is_root, roots), (self._is_path, paths)):
                    if kind(value, known) and not set(names) <= known:
                        known.update(names)
                        changed = True
        return roots, paths

    def _names(self, index: int) -> set[str]:
        return set().union(*(scope[index] for scope in self._scopes))

    def _function(self) -> str:
        return ".".join(name for name, _, _ in self._scopes[1:]) or "<module>"

    # -- visiting ----------------------------------------------------------------------

    def visit_ClassDef(self, node: ast.ClassDef) -> None:
        self._scopes.append((node.name, set(), set()))
        self.generic_visit(node)
        self._scopes.pop()

    def visit_FunctionDef(self, node: ast.FunctionDef | ast.AsyncFunctionDef) -> None:
        roots, paths = self._infer(node, self._names(1), self._names(2))
        self._scopes.append((node.name, roots, paths))
        self.generic_visit(node)
        self._scopes.pop()

    def visit_AsyncFunctionDef(self, node: ast.AsyncFunctionDef) -> None:
        self.visit_FunctionDef(node)

    def visit_Call(self, node: ast.Call) -> None:
        call = self._file_access(node.func)
        if call:
            self.found.append((self._function(), call, node.lineno))
        self.generic_visit(node)

    def _file_access(self, func: ast.expr) -> str:
        if isinstance(func, ast.Name):
            if func.id == "open":
                return "open"
            return self._imported.get(func.id, "")
        if not isinstance(func, ast.Attribute):
            return ""
        owner = ast.unparse(func.value)
        if owner in _MODULE_CALLS:
            return f"{owner}.{func.attr}" if func.attr in _MODULE_CALLS[owner] else ""
        if self._is_root(func.value):
            return ""
        if func.attr in _ANY_OWNER or (func.attr in _PATH_ONLY and self._is_path(func.value)):
            simple = isinstance(func.value, ast.Name | ast.Attribute | ast.Call)
            return f"{owner}.{func.attr}" if simple else f"({owner}).{func.attr}"
        return ""


def file_access_calls(path: Path, base: Path = SOURCE_DIR) -> list[tuple[str, str, str, int]]:
    """(module, function, call, line) of every direct file access call in *path*."""
    tree = ast.parse(path.read_text(), filename=str(path))
    scanner = _Scanner(tree)
    scanner.visit(tree)
    module = path.relative_to(base).as_posix()
    return [(module, function, call, line) for function, call, line in scanner.found]


def _all_calls() -> list[tuple[str, str, str, int]]:
    calls: list[tuple[str, str, str, int]] = []
    for path in sorted(SOURCE_DIR.rglob("*.py")):
        if path == SOURCE_DIR / "workspace_fs.py":
            continue
        calls.extend(file_access_calls(path))
    return calls


def _counts() -> Counter[tuple[str, str, str]]:
    return Counter((module, function, call) for module, function, call, _ in _all_calls())


def test_only_workspace_fs_accesses_files() -> None:
    """A new direct file access outside codeforge.workspace_fs must be routed through it or listed here."""
    lines: dict[tuple[str, str, str], list[int]] = {}
    for module, function, call, line in _all_calls():
        lines.setdefault((module, function, call), []).append(line)
    offenders = [
        f"{site[0]}:{','.join(map(str, lines[site]))} {site[1]}: {site[2]} ({count}x, allowed {ALLOWED.get(site, (0, ''))[0]})"
        for site, count in sorted(_counts().items())
        if count > ALLOWED.get(site, (0, ""))[0]
    ]
    assert offenders == [], (
        "read and write workspace files through codeforge.workspace_fs (KI-95), "
        "or list the call in ALLOWED with the reason it never touches a workspace:\n" + "\n".join(offenders)
    )


def test_allowlist_is_exact() -> None:
    """Every allowed site exists with exactly the allowed number of calls."""
    counts = _counts()
    stale = [
        f"{site}: {counts.get(site, 0)} call(s), allowed {count}"
        for site, (count, _) in ALLOWED.items()
        if counts.get(site, 0) != count
    ]
    assert stale == []


def test_the_scan_finds_file_access(tmp_path: Path) -> None:
    sample = tmp_path / "sample.py"
    sample.write_text(
        "import glob\n"
        "import os\n"
        "import shutil\n"
        "from pathlib import Path\n"
        "from shutil import rmtree\n"
        "from codeforge.workspace_fs import WorkspaceRoot\n"
        "def f(ws, p: Path, root: WorkspaceRoot):\n"
        "    open(os.path.join(ws, 'a'))\n"
        "    p.read_text()\n"
        "    p.write_bytes(b'')\n"
        "    os.walk(ws)\n"
        "    p.rglob('*')\n"
        "    os.scandir(ws)\n"
        "    p.open()\n"
        "    'a'.replace('a', 'b')\n"
        "    root.read_text('a')\n"
        "    os.replace(ws, ws)\n"
        "    (p / 'x').replace(ws)\n"
        "    shutil.move(ws, ws)\n"
        "    glob.iglob('*')\n"
        "    Path(ws).touch()\n"
        "    items = [1]\n"
        "    items.remove(1)\n"
        "    os.remove(ws)\n"
        "    rmtree(ws)\n"
        "    q = p.parent\n"
        "    q.rename(ws)\n"
        "def g(root):\n"
        "    root.read_text('a')\n"
        "def h(ws):\n"
        "    with WorkspaceRoot(ws) as r:\n"
        "        r.read_text('a')\n"
        "        sub = r.subroot('x')\n"
        "        sub.write_text('a', '')\n"
        "        WorkspaceRoot.operator_dir(ws).read_bytes('a')\n"
    )
    found = file_access_calls(sample, tmp_path)
    assert [(function, call) for _, function, call, _ in found] == [
        ("f", "open"),
        ("f", "p.read_text"),
        ("f", "p.write_bytes"),
        ("f", "os.walk"),
        ("f", "p.rglob"),
        ("f", "os.scandir"),
        ("f", "p.open"),
        ("f", "os.replace"),
        ("f", "(p / 'x').replace"),
        ("f", "shutil.move"),
        ("f", "glob.iglob"),
        ("f", "Path(ws).touch"),
        ("f", "os.remove"),
        ("f", "shutil.rmtree"),
        ("f", "q.rename"),
        ("g", "root.read_text"),
    ]
