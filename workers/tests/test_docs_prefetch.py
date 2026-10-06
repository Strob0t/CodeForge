"""Tests for proactive docs-mcp prefetch and framework detection."""

from __future__ import annotations

import json
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.consumer._conversation import _detect_frameworks, _prefetch_docs, _search_docs_tool
from codeforge.models import ToolCallDecision
from codeforge.tools import ToolRegistry
from codeforge.tools._base import ToolDefinition, ToolResult

if TYPE_CHECKING:
    from pathlib import Path

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


# --- _prefetch_docs: through the tool-call policy (KI-192) ---
#
# Before, the prefetch called search_docs on the workbench directly with the
# user's message: no policy decision, mode denied_tools ignored, supervised
# and ask-all presets bypassed.

SEARCH_DOCS = "mcp__docs__search_docs"
LONG_DOCS = "createSignal is a reactive primitive in SolidJS that returns a getter and setter pair. " * 5


class _DocsTool:
    def __init__(self, result: ToolResult | Exception) -> None:
        self.calls: list[dict[str, object]] = []
        self._result = result

    async def execute(self, arguments: dict[str, object], workspace_path: str) -> ToolResult:
        self.calls.append(arguments)
        if isinstance(self._result, Exception):
            raise self._result
        return self._result


def _registry(tool: _DocsTool, *, denied: tuple[str, ...] = ()) -> ToolRegistry:
    registry = ToolRegistry()
    registry.restrict_to_mode([], list(denied))
    registry.register(ToolDefinition(name=SEARCH_DOCS, description="Search documentation"), tool)
    return registry


def _runtime(decision: str = "allow") -> MagicMock:
    runtime = MagicMock()
    runtime.request_tool_call = AsyncMock(
        return_value=ToolCallDecision(call_id="call-1", decision=decision, reason="by policy")
    )
    runtime.report_tool_result = AsyncMock()
    return runtime


def _solid_workspace(tmp_path: Path) -> str:
    (tmp_path / "package.json").write_text(json.dumps({"dependencies": {"solid-js": "^1.8.0"}}))
    return str(tmp_path)


def _log() -> object:
    import structlog

    return structlog.get_logger()


class TestSearchDocsTool:
    def test_found(self) -> None:
        assert _search_docs_tool(["read_file", "mcp__gh__search_issues", SEARCH_DOCS]) == SEARCH_DOCS

    def test_first_of_several_servers(self) -> None:
        assert _search_docs_tool(["mcp__z__search_docs", "mcp__a__search_docs"]) == "mcp__a__search_docs"

    def test_not_found(self) -> None:
        assert _search_docs_tool(["mcp__github__list_issues", "search_docs"]) is None
        assert _search_docs_tool([]) is None


class TestPrefetchDocs:
    async def test_allowed_call_is_requested_executed_and_reported(self, tmp_path: Path) -> None:
        tool = _DocsTool(ToolResult(output=LONG_DOCS))
        runtime = _runtime()

        result = await _prefetch_docs(
            _registry(tool), runtime, _solid_workspace(tmp_path), "how to use signals", _log()
        )

        assert [(e.kind, e.path, e.priority) for e in result] == [("knowledge", "docs/solidjs", 80)]
        assert len(result[0].content) <= 2000
        assert tool.calls == [{"library": "solidjs", "query": "how to use signals", "limit": 3}]
        request = runtime.request_tool_call.await_args.kwargs
        assert request["tool"] == SEARCH_DOCS
        assert "how to use signals" in request["arguments_preview"]
        report = runtime.report_tool_result.await_args.kwargs
        assert (report["call_id"], report["tool"], report["success"]) == ("call-1", SEARCH_DOCS, True)

    @pytest.mark.parametrize("decision", ["deny", "ask"])
    async def test_a_call_the_policy_does_not_allow_is_not_made(self, tmp_path: Path, decision: str) -> None:
        tool = _DocsTool(ToolResult(output=LONG_DOCS))
        runtime = _runtime(decision)

        result = await _prefetch_docs(
            _registry(tool), runtime, _solid_workspace(tmp_path), "how to use signals", _log()
        )

        assert result == []
        assert tool.calls == []
        runtime.report_tool_result.assert_not_awaited()

    async def test_a_tool_the_mode_denies_is_not_requested(self, tmp_path: Path) -> None:
        tool = _DocsTool(ToolResult(output=LONG_DOCS))
        runtime = _runtime()

        result = await _prefetch_docs(
            _registry(tool, denied=(SEARCH_DOCS,)), runtime, _solid_workspace(tmp_path), "how to use signals", _log()
        )

        assert result == []
        runtime.request_tool_call.assert_not_awaited()
        assert tool.calls == []

    async def test_no_message(self, tmp_path: Path) -> None:
        runtime = _runtime()
        tool = _DocsTool(ToolResult(output=LONG_DOCS))
        assert await _prefetch_docs(_registry(tool), runtime, _solid_workspace(tmp_path), "", _log()) == []
        runtime.request_tool_call.assert_not_awaited()

    async def test_no_frameworks_detected(self, tmp_path: Path) -> None:
        runtime = _runtime()
        tool = _DocsTool(ToolResult(output=LONG_DOCS))
        assert await _prefetch_docs(_registry(tool), runtime, str(tmp_path), "how to use signals", _log()) == []
        runtime.request_tool_call.assert_not_awaited()

    async def test_short_output_skipped(self, tmp_path: Path) -> None:
        tool = _DocsTool(ToolResult(output="No results"))
        result = await _prefetch_docs(_registry(tool), _runtime(), _solid_workspace(tmp_path), "signals", _log())
        assert result == []

    async def test_mcp_error_is_reported_and_handled(self, tmp_path: Path) -> None:
        tool = _DocsTool(ConnectionError("MCP server down"))
        runtime = _runtime()

        result = await _prefetch_docs(_registry(tool), runtime, _solid_workspace(tmp_path), "signals", _log())

        assert result == []
        report = runtime.report_tool_result.await_args.kwargs
        assert report["success"] is False
        assert "MCP server down" in report["error"]
