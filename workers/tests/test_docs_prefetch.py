"""Tests for proactive docs-mcp prefetch and framework detection."""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.consumer._conversation import _detect_frameworks, _find_search_docs_tool, _prefetch_docs
from codeforge.mcp_models import MCPTool, MCPToolCallResult

# --- _detect_frameworks tests ---


class TestDetectFrameworks:
    def test_empty_workspace(self, tmp_path: str) -> None:
        assert _detect_frameworks(str(tmp_path)) == []

    def test_invalid_workspace(self) -> None:
        assert _detect_frameworks("/nonexistent/path") == []

    def test_empty_string(self) -> None:
        assert _detect_frameworks("") == []

    def test_package_json_solidjs(self, tmp_path: str) -> None:
        pkg = {"dependencies": {"solid-js": "^1.8.0"}, "devDependencies": {"tailwindcss": "^3.0"}}
        (tmp_path / "package.json").write_text(json.dumps(pkg))
        frameworks = _detect_frameworks(str(tmp_path))
        assert "solidjs" in frameworks
        assert "tailwindcss" in frameworks

    def test_package_json_react(self, tmp_path: str) -> None:
        pkg = {"dependencies": {"react": "^18.0", "next": "^14.0"}}
        (tmp_path / "package.json").write_text(json.dumps(pkg))
        frameworks = _detect_frameworks(str(tmp_path))
        assert "react" in frameworks
        assert "nextjs" in frameworks

    def test_requirements_txt_fastapi(self, tmp_path: str) -> None:
        (tmp_path / "requirements.txt").write_text("fastapi>=0.100\nuvicorn\npydantic>=2.0\n")
        frameworks = _detect_frameworks(str(tmp_path))
        assert "fastapi" in frameworks
        assert "pydantic" in frameworks

    def test_pyproject_toml_django(self, tmp_path: str) -> None:
        (tmp_path / "pyproject.toml").write_text('[project]\ndependencies = ["django>=4.0"]\n')
        frameworks = _detect_frameworks(str(tmp_path))
        assert "django" in frameworks

    def test_go_mod_chi(self, tmp_path: str) -> None:
        (tmp_path / "go.mod").write_text("module example.com/app\nrequire github.com/go-chi/chi/v5 v5.0.0\n")
        frameworks = _detect_frameworks(str(tmp_path))
        assert "chi" in frameworks

    def test_max_five_frameworks(self, tmp_path: str) -> None:
        pkg = {
            "dependencies": {
                "solid-js": "1.0",
                "react": "18.0",
                "vue": "3.0",
                "next": "14.0",
                "svelte": "4.0",
                "express": "4.0",
                "tailwindcss": "3.0",
            }
        }
        (tmp_path / "package.json").write_text(json.dumps(pkg))
        frameworks = _detect_frameworks(str(tmp_path))
        assert len(frameworks) <= 5

    def test_no_duplicates_across_files(self, tmp_path: str) -> None:
        (tmp_path / "requirements.txt").write_text("fastapi\n")
        (tmp_path / "pyproject.toml").write_text('dependencies = ["fastapi"]\n')
        frameworks = _detect_frameworks(str(tmp_path))
        assert frameworks.count("fastapi") == 1

    def test_malformed_package_json(self, tmp_path: str) -> None:
        (tmp_path / "package.json").write_text("{invalid json")
        frameworks = _detect_frameworks(str(tmp_path))
        assert frameworks == []


# --- _find_search_docs_tool tests ---


class TestFindSearchDocsTool:
    def test_found(self) -> None:
        workbench = MagicMock()
        workbench._tools = [
            MCPTool(server_id="docs", name="search_docs", description="Search"),
            MCPTool(server_id="docs", name="scrape_docs", description="Scrape"),
        ]
        tool = _find_search_docs_tool(workbench)
        assert tool is not None
        assert tool.server_id == "docs"
        assert tool.name == "search_docs"

    def test_not_found(self) -> None:
        workbench = MagicMock()
        workbench._tools = [
            MCPTool(server_id="github", name="list_issues", description="List"),
        ]
        assert _find_search_docs_tool(workbench) is None

    def test_empty_tools(self) -> None:
        workbench = MagicMock()
        workbench._tools = []
        assert _find_search_docs_tool(workbench) is None


# --- _prefetch_docs tests ---


class TestPrefetchDocs:
    @pytest.mark.asyncio
    async def test_no_workbench(self) -> None:
        import structlog

        log = structlog.get_logger()
        result = await _prefetch_docs(None, "/tmp", "hello", log)
        assert result == []

    @pytest.mark.asyncio
    async def test_no_message(self) -> None:
        import structlog

        log = structlog.get_logger()
        workbench = MagicMock()
        result = await _prefetch_docs(workbench, "/tmp", "", log)
        assert result == []

    @pytest.mark.asyncio
    async def test_no_search_docs_tool(self, tmp_path: str) -> None:
        import structlog

        log = structlog.get_logger()
        workbench = MagicMock()
        workbench._tools = []
        result = await _prefetch_docs(workbench, str(tmp_path), "how to use solidjs", log)
        assert result == []

    @pytest.mark.asyncio
    async def test_successful_prefetch(self, tmp_path: str) -> None:
        import structlog

        log = structlog.get_logger()

        # Set up workspace with solidjs
        pkg = {"dependencies": {"solid-js": "^1.8.0"}}
        (tmp_path / "package.json").write_text(json.dumps(pkg))

        # Set up workbench with search_docs tool
        workbench = MagicMock()
        workbench._tools = [
            MCPTool(server_id="docs", name="search_docs", description="Search documentation"),
        ]
        workbench.call_tool = AsyncMock(
            return_value=MCPToolCallResult(
                success=True,
                output="createSignal is a reactive primitive in SolidJS that returns a getter and setter pair. " * 5,
            )
        )

        result = await _prefetch_docs(workbench, str(tmp_path), "how to use signals", log)
        assert len(result) == 1
        assert result[0].kind == "knowledge"
        assert result[0].path == "docs/solidjs"
        assert result[0].priority == 80
        assert len(result[0].content) <= 2000

        # Verify call_tool was called with correct args
        workbench.call_tool.assert_called_once_with(
            "docs",
            "search_docs",
            {"library": "solidjs", "query": "how to use signals", "limit": 3},
        )

    @pytest.mark.asyncio
    async def test_short_output_skipped(self, tmp_path: str) -> None:
        import structlog

        log = structlog.get_logger()

        pkg = {"dependencies": {"solid-js": "^1.8.0"}}
        (tmp_path / "package.json").write_text(json.dumps(pkg))

        workbench = MagicMock()
        workbench._tools = [
            MCPTool(server_id="docs", name="search_docs", description="Search"),
        ]
        workbench.call_tool = AsyncMock(return_value=MCPToolCallResult(success=True, output="No results"))

        result = await _prefetch_docs(workbench, str(tmp_path), "how to use signals", log)
        assert result == []

    @pytest.mark.asyncio
    async def test_mcp_error_handled_gracefully(self, tmp_path: str) -> None:
        import structlog

        log = structlog.get_logger()

        pkg = {"dependencies": {"solid-js": "^1.8.0"}}
        (tmp_path / "package.json").write_text(json.dumps(pkg))

        workbench = MagicMock()
        workbench._tools = [
            MCPTool(server_id="docs", name="search_docs", description="Search"),
        ]
        workbench.call_tool = AsyncMock(side_effect=ConnectionError("MCP server down"))

        result = await _prefetch_docs(workbench, str(tmp_path), "how to use signals", log)
        assert result == []

    @pytest.mark.asyncio
    async def test_no_frameworks_detected(self, tmp_path: str) -> None:
        import structlog

        log = structlog.get_logger()

        # Empty workspace, no dependency files
        workbench = MagicMock()
        workbench._tools = [
            MCPTool(server_id="docs", name="search_docs", description="Search"),
        ]

        result = await _prefetch_docs(workbench, str(tmp_path), "how to use signals", log)
        assert result == []
