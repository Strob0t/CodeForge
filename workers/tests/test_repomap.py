"""Tests for the repo map generator."""

from __future__ import annotations

import os
import tempfile
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.consumer import TaskConsumer
from codeforge.models import RepoMapRequest, RepoMapResult
from codeforge.repomap import RepoMapGenerator, SymbolTag


@pytest.fixture
def generator() -> RepoMapGenerator:
    """Create a RepoMapGenerator with default settings."""
    return RepoMapGenerator(token_budget=2048)


@pytest.fixture
def workspace(tmp_path: object) -> str:
    """Create a temporary workspace with sample source files."""
    ws = str(tmp_path)

    # Python file
    py_dir = os.path.join(ws, "src")
    os.makedirs(py_dir)
    with open(os.path.join(py_dir, "service.py"), "w") as f:
        f.write(
            "class UserService:\n"
            "    def get_user(self, user_id):\n"
            "        return None\n"
            "\n"
            "def create_handler():\n"
            "    svc = UserService()\n"
            "    return svc\n"
            "\n"
            "_internal = 42\n"
        )

    # Go file
    go_dir = os.path.join(ws, "pkg")
    os.makedirs(go_dir)
    with open(os.path.join(go_dir, "handler.go"), "w") as f:
        f.write(
            "package pkg\n"
            "\n"
            "func NewHandler() *Handler { return &Handler{} }\n"
            "\n"
            "type Handler struct {\n"
            "    Name string\n"
            "}\n"
            "\n"
            "func (h *Handler) Serve() {}\n"
            "\n"
            'const Version = "1.0"\n'
        )

    # TypeScript file
    ts_dir = os.path.join(ws, "web")
    os.makedirs(ts_dir)
    with open(os.path.join(ts_dir, "app.ts"), "w") as f:
        f.write(
            "export function startApp(): void {}\n"
            "\n"
            "class AppController {\n"
            "    handle(): void {}\n"
            "}\n"
            "\n"
            "interface Config {\n"
            "    name: string;\n"
            "}\n"
            "\n"
            "type Result = string | number;\n"
            "\n"
            "const version = '1.0';\n"
        )

    return ws


def test_detect_language(generator: RepoMapGenerator) -> None:
    """Extension mapping should return correct language names."""
    assert generator._detect_language("main.py") == "python"
    assert generator._detect_language("main.go") == "go"
    assert generator._detect_language("app.ts") == "typescript"
    assert generator._detect_language("app.tsx") == "tsx"
    assert generator._detect_language("index.js") == "javascript"
    assert generator._detect_language("index.jsx") == "javascript"
    assert generator._detect_language("Main.java") == "java"
    assert generator._detect_language("lib.rs") == "rust"
    assert generator._detect_language("app.rb") == "ruby"
    assert generator._detect_language("main.c") == "c"
    assert generator._detect_language("main.cpp") == "cpp"
    assert generator._detect_language("main.cc") == "cpp"
    assert generator._detect_language("main.cs") == "csharp"
    assert generator._detect_language("main.kt") == "kotlin"
    assert generator._detect_language("main.swift") == "swift"
    assert generator._detect_language("main.php") == "php"
    assert generator._detect_language("header.h") == "c"
    assert generator._detect_language("header.hpp") == "cpp"
    assert generator._detect_language("readme.md") is None
    assert generator._detect_language("data.json") is None


def test_extract_tags_python(generator: RepoMapGenerator) -> None:
    """Should extract class, function, and assignment definitions from Python."""
    with tempfile.NamedTemporaryFile(suffix=".py", mode="w", delete=False) as f:
        f.write(
            "class MyService:\n"
            "    def start(self):\n"
            "        pass\n"
            "\n"
            "def public_func():\n"
            "    pass\n"
            "\n"
            "_private_var = 42\n"
        )
        f.flush()
        tags = generator._extract_tags("service.py", f.name, "python")

    os.unlink(f.name)

    defs = [t for t in tags if t.kind == "def"]
    def_names = {t.name for t in defs}
    assert "MyService" in def_names
    assert "public_func" in def_names
    assert "_private_var" in def_names

    # Check scope detection
    private_tags = [t for t in defs if t.name == "_private_var"]
    assert len(private_tags) == 1
    assert private_tags[0].scope == "private"

    public_tags = [t for t in defs if t.name == "MyService"]
    assert len(public_tags) == 1
    assert public_tags[0].scope == "public"


def test_extract_tags_go(generator: RepoMapGenerator) -> None:
    """Should extract function, method, type, const, var definitions from Go."""
    with tempfile.NamedTemporaryFile(suffix=".go", mode="w", delete=False) as f:
        f.write(
            "package main\n"
            "\n"
            'func Hello() string { return "hello" }\n'
            "\n"
            "type Service struct { Name string }\n"
            "\n"
            "func (s *Service) Start() {}\n"
            "\n"
            'const Version = "1.0"\n'
            "\n"
            "var Count int\n"
        )
        f.flush()
        tags = generator._extract_tags("main.go", f.name, "go")

    os.unlink(f.name)

    defs = [t for t in tags if t.kind == "def"]
    def_names = {t.name for t in defs}
    assert "Hello" in def_names
    assert "Service" in def_names
    assert "Start" in def_names
    assert "Version" in def_names
    assert "Count" in def_names


def test_extract_tags_typescript(generator: RepoMapGenerator) -> None:
    """Should extract function, class, interface, type alias, and const definitions from TypeScript."""
    with tempfile.NamedTemporaryFile(suffix=".ts", mode="w", delete=False) as f:
        f.write(
            "export function hello(): string { return 'hi'; }\n"
            "\n"
            "class Controller {\n"
            "    handle(): void {}\n"
            "}\n"
            "\n"
            "interface Config {\n"
            "    name: string;\n"
            "}\n"
            "\n"
            "type Result = string | number;\n"
            "\n"
            "const version = '1.0';\n"
        )
        f.flush()
        tags = generator._extract_tags("app.ts", f.name, "typescript")

    os.unlink(f.name)

    defs = [t for t in tags if t.kind == "def"]
    def_names = {t.name for t in defs}
    assert "hello" in def_names
    assert "Controller" in def_names
    assert "Config" in def_names
    assert "Result" in def_names
    assert "version" in def_names


def test_build_graph(generator: RepoMapGenerator) -> None:
    """Should create edges from referencing files to defining files."""
    tags = [
        SymbolTag(rel_path="a.py", line=1, name="UserService", kind="def", scope="public"),
        SymbolTag(rel_path="a.py", line=5, name="create_user", kind="def", scope="public"),
        SymbolTag(rel_path="b.py", line=3, name="UserService", kind="ref", scope="public"),
        SymbolTag(rel_path="b.py", line=1, name="handle_request", kind="def", scope="public"),
    ]
    graph = generator._build_graph(tags, [])

    # b.py references UserService defined in a.py -> edge from b.py to a.py
    assert graph.has_edge("b.py", "a.py")
    assert not graph.has_edge("a.py", "b.py")


def test_build_graph_long_identifier_weight(generator: RepoMapGenerator) -> None:
    """Long identifiers (>=8 chars) should get 10x weight."""
    tags = [
        SymbolTag(rel_path="a.py", line=1, name="UserService", kind="def", scope="public"),
        SymbolTag(rel_path="b.py", line=3, name="UserService", kind="ref", scope="public"),
    ]
    graph = generator._build_graph(tags, [])

    # "UserService" has 11 chars >= 8, so weight should be 10.0
    assert graph.edge_weight("b.py", "a.py") == 10.0


def test_build_graph_active_file_weight(generator: RepoMapGenerator) -> None:
    """Active files should get 50x weight boost."""
    tags = [
        SymbolTag(rel_path="a.py", line=1, name="Svc", kind="def", scope="public"),
        SymbolTag(rel_path="b.py", line=3, name="Svc", kind="ref", scope="public"),
    ]
    graph = generator._build_graph(tags, ["b.py"])

    # "Svc" is short (3 chars), but b.py is active: 1.0 * 50.0 = 50.0
    assert graph.edge_weight("b.py", "a.py") == 50.0


def test_rank_files(generator: RepoMapGenerator) -> None:
    """PageRank should produce higher rank for heavily referenced files."""
    tags = [
        SymbolTag(rel_path="core.py", line=1, name="BaseModel", kind="def", scope="public"),
        SymbolTag(rel_path="a.py", line=1, name="ServiceA", kind="def", scope="public"),
        SymbolTag(rel_path="a.py", line=5, name="BaseModel", kind="ref", scope="public"),
        SymbolTag(rel_path="b.py", line=1, name="ServiceB", kind="def", scope="public"),
        SymbolTag(rel_path="b.py", line=5, name="BaseModel", kind="ref", scope="public"),
        SymbolTag(rel_path="c.py", line=1, name="ServiceC", kind="def", scope="public"),
        SymbolTag(rel_path="c.py", line=5, name="BaseModel", kind="ref", scope="public"),
    ]
    graph = generator._build_graph(tags, [])
    ranked = generator._rank_files(graph)

    # core.py should have highest rank since a.py, b.py, c.py all reference it
    assert ranked["core.py"] > ranked["a.py"]
    assert ranked["core.py"] > ranked["b.py"]
    assert ranked["core.py"] > ranked["c.py"]


def test_format_map_within_budget(generator: RepoMapGenerator) -> None:
    """Format should include all files when they fit within budget."""
    tags = [
        SymbolTag(rel_path="a.py", line=1, name="hello", kind="def", scope="public"),
        SymbolTag(rel_path="b.py", line=1, name="world", kind="def", scope="public"),
    ]
    result = generator._format_map(tags, token_budget=1000)

    assert "a.py" in result
    assert "hello" in result
    assert "b.py" in result
    assert "world" in result


def test_format_map_over_budget(generator: RepoMapGenerator) -> None:
    """Format should prune files when output exceeds budget."""
    # Create many tags so total exceeds a tiny budget
    tags = [
        SymbolTag(
            rel_path=f"module_{i}/service.py",
            line=1,
            name=f"LongFunctionName_{i}",
            kind="def",
            scope="public",
        )
        for i in range(50)
    ]

    # Very small budget: only a few files should fit
    result = generator._format_map(tags, token_budget=10)

    # Should have some content but not all 50 files
    assert len(result) > 0
    lines_with_module = [ln for ln in result.split("\n") if ln.startswith("module_")]
    assert len(lines_with_module) < 50


async def test_generate_full_pipeline(generator: RepoMapGenerator, workspace: str) -> None:
    """Full pipeline should produce a map with symbols from multiple languages."""
    result = await generator.generate(workspace)

    assert result.file_count >= 3
    assert result.symbol_count > 0
    assert result.token_count > 0
    assert len(result.map_text) > 0
    assert "python" in result.languages
    assert "go" in result.languages
    assert "typescript" in result.languages


async def test_generate_empty_workspace(generator: RepoMapGenerator) -> None:
    """Empty workspace should return an empty map without errors."""
    with tempfile.TemporaryDirectory() as ws:
        result = await generator.generate(ws)

    assert result.map_text == ""
    assert result.file_count == 0
    assert result.symbol_count == 0
    assert result.token_count == 0
    assert result.languages == []


def test_collect_files_skips_ignored(generator: RepoMapGenerator) -> None:
    """File collection should skip .git, node_modules, __pycache__, etc."""
    with tempfile.TemporaryDirectory() as ws:
        # Create files in ignored directories
        for skip_dir in [".git", "node_modules", "__pycache__", "vendor", "dist", "build"]:
            d = os.path.join(ws, skip_dir)
            os.makedirs(d)
            with open(os.path.join(d, "file.py"), "w") as f:
                f.write("x = 1\n")

        # Create a valid file
        with open(os.path.join(ws, "main.py"), "w") as f:
            f.write("def main(): pass\n")

        files = generator._collect_files(ws)

    # Only main.py should be collected
    assert len(files) == 1
    assert files[0].endswith("main.py")


async def test_handle_repomap_message() -> None:
    """Consumer should parse repomap request and publish result."""
    consumer = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")

    with tempfile.TemporaryDirectory() as ws:
        # Create a simple Python file in the workspace
        with open(os.path.join(ws, "hello.py"), "w") as f:
            f.write("def hello(): pass\n")

        request = RepoMapRequest(
            project_id="proj-1",
            workspace_path=ws,
            token_budget=2048,
        )

        msg = MagicMock()
        msg.data = request.model_dump_json().encode()
        msg.ack = AsyncMock()
        msg.nak = AsyncMock()

        consumer._js = AsyncMock()

        await consumer._handle_repomap(msg)

    # Should publish result
    consumer._js.publish.assert_called_once()
    call_args = consumer._js.publish.call_args
    assert call_args.args[0] == "repomap.generate.result"

    result = RepoMapResult.model_validate_json(call_args.args[1])
    assert result.project_id == "proj-1"
    assert result.file_count >= 1
    assert result.symbol_count >= 1

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()
