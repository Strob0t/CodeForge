"""Tests for ToolRouter keyword-based tool pre-selection."""

from __future__ import annotations

import pytest

from codeforge.tools.tool_router import ToolRouter

# --- Fixtures ---


@pytest.fixture
def all_tools() -> list[str]:
    """Comprehensive tool list including built-in and MCP tools."""
    return [
        "read_file",
        "write_file",
        "edit_file",
        "bash",
        "search_files",
        "glob_files",
        "list_directory",
        "propose_goal",
        "transition_to_act",
        "search_conversations",
        "search_skills",
        "create_skill",
        "handoff",
        "mcp__docs__search_docs",
        "mcp__docs__scrape_docs",
        "mcp__docs__list_sources",
        "mcp__github__search_issues",
        "mcp__github__create_issue",
        "mcp__context__find_similar",
        "mcp__context__fetch_url",
    ]


@pytest.fixture
def router(all_tools: list[str]) -> ToolRouter:
    return ToolRouter(all_tool_names=all_tools)


# --- Base tool tests ---


class TestBaseTools:
    def test_base_tools_always_included(self, router: ToolRouter) -> None:
        selected = router.select("build a weather app")
        for base in ToolRouter.BASE_TOOLS:
            assert base in selected

    def test_base_tools_included_even_with_empty_message(self, router: ToolRouter) -> None:
        selected = router.select("")
        for base in ToolRouter.BASE_TOOLS:
            assert base in selected

    def test_base_tools_only_from_available(self) -> None:
        """BASE_TOOLS not in all_tool_names are excluded."""
        router = ToolRouter(all_tool_names=["read_file", "bash"])
        selected = router.select("hello")
        assert "read_file" in selected
        assert "bash" in selected
        assert "write_file" not in selected

    def test_empty_tool_list(self) -> None:
        router = ToolRouter(all_tool_names=[])
        selected = router.select("build something")
        assert selected == []


# --- MCP docs inclusion tests ---


class TestMCPDocsInclusion:
    def test_mcp_search_selected_for_docs_query(self, router: ToolRouter) -> None:
        selected = router.select("show me the api reference for createSignal in SolidJS")
        # "search" and "list" and "find" and "fetch" in tool name
        assert "mcp__docs__search_docs" in selected
        assert "mcp__docs__list_sources" in selected
        assert "mcp__context__find_similar" in selected
        assert "mcp__context__fetch_url" in selected
        # Write-only MCP tools excluded
        assert "mcp__docs__scrape_docs" not in selected
        assert "mcp__github__create_issue" not in selected

    def test_docs_keyword_triggers_mcp_tools(self, router: ToolRouter) -> None:
        for keyword in [
            "docs",
            "documentation",
            "how to",
            "api",
            "reference",
            "example",
            "usage",
            "tutorial",
            "guide",
            "library",
        ]:
            selected = router.select(f"please show me {keyword}")
            assert "mcp__docs__search_docs" in selected, f"keyword '{keyword}' should trigger MCP docs tools"

    def test_no_mcp_tools_without_docs_keywords(self, router: ToolRouter) -> None:
        selected = router.select("fix the bug in main.py")
        assert "mcp__docs__search_docs" not in selected
        assert "mcp__docs__list_sources" not in selected

    def test_mcp_search_tool_with_search_keyword_in_name(self) -> None:
        router = ToolRouter(all_tool_names=["read_file", "mcp__custom__search_users"])
        selected = router.select("check the api for user data")
        assert "mcp__custom__search_users" in selected


# --- Keyword-to-tool mapping tests ---


class TestKeywordMapping:
    def test_test_keyword_triggers_bash(self, router: ToolRouter) -> None:
        selected = router.select("run the tests")
        assert "bash" in selected

    def test_create_keyword_triggers_write(self, router: ToolRouter) -> None:
        selected = router.select("create a new file")
        assert "write_file" in selected

    def test_fix_keyword_triggers_edit_read_bash(self, router: ToolRouter) -> None:
        selected = router.select("fix the syntax error")
        assert "edit_file" in selected
        assert "read_file" in selected
        assert "bash" in selected

    def test_search_keyword_triggers_search_files(self, router: ToolRouter) -> None:
        selected = router.select("search for the function")
        assert "search_files" in selected

    def test_find_keyword_triggers_search_and_glob(self, router: ToolRouter) -> None:
        selected = router.select("find all python files")
        assert "search_files" in selected
        assert "glob_files" in selected

    def test_git_keyword_triggers_bash(self, router: ToolRouter) -> None:
        selected = router.select("commit the changes to git")
        assert "bash" in selected

    def test_install_keyword_triggers_bash(self, router: ToolRouter) -> None:
        selected = router.select("install the dependencies")
        assert "bash" in selected

    def test_run_keyword_triggers_bash(self, router: ToolRouter) -> None:
        selected = router.select("run the server")
        assert "bash" in selected

    def test_modify_keyword_triggers_edit_and_read(self, router: ToolRouter) -> None:
        selected = router.select("modify the config file")
        assert "edit_file" in selected
        assert "read_file" in selected


# --- max_tools limit ---


class TestMaxTools:
    def test_max_tools_respected(self, all_tools: list[str]) -> None:
        router = ToolRouter(all_tool_names=all_tools)
        selected = router.select("search the api documentation library example", max_tools=5)
        assert len(selected) <= 5

    def test_default_max_tools_is_12(self, router: ToolRouter) -> None:
        # Even with many keyword matches, should not exceed 12
        selected = router.select("search find create modify fix test run install commit git docs api")
        assert len(selected) <= 12


# --- Sorting and determinism ---


class TestDeterminism:
    def test_results_are_sorted(self, router: ToolRouter) -> None:
        selected = router.select("build something")
        assert selected == sorted(selected)

    def test_same_input_same_output(self, router: ToolRouter) -> None:
        """Verify deterministic output for identical inputs."""
        a = router.select("how does the api work")
        b = router.select("how does the api work")
        assert a == b


# --- Case insensitivity ---


class TestCaseInsensitivity:
    def test_uppercase_keywords_work(self, router: ToolRouter) -> None:
        selected = router.select("SEARCH the API DOCUMENTATION")
        assert "mcp__docs__search_docs" in selected
        assert "search_files" in selected

    def test_mixed_case_keywords(self, router: ToolRouter) -> None:
        selected = router.select("How To use the Library")
        assert "mcp__docs__search_docs" in selected
