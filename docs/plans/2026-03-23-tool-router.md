# Tool Router + Proactive Context Injection — Implementation Plan

> Steps use checkbox (`- [ ]`) syntax. Execute task-by-task, commit after each.

**Goal:** Pre-select relevant tools and pre-fetch documentation context BEFORE the agent loop starts. No LLM call needed.

**Research basis:** Strands FAISS pattern, Portkey MCP Filter, Anthropic Deferred Loading, AllianceCoder, ToolBench

---

## Task 1: ToolRouter class with keyword-based tool selection

**Files:**
- Create: `workers/codeforge/tools/tool_router.py`
- Create: `workers/tests/test_tool_router.py`

- [ ] **Step 1: Write tests**

```python
def test_base_tools_always_included():
    router = ToolRouter(all_tool_names=["read_file", "write_file", "bash", "edit_file", "mcp__docs__search_docs", "mcp__docs__scrape_docs"])
    selected = router.select("build a weather app")
    assert "read_file" in selected
    assert "write_file" in selected
    assert "bash" in selected
    assert "edit_file" in selected

def test_mcp_search_selected_for_docs_query():
    router = ToolRouter(all_tool_names=["read_file", "write_file", "bash", "mcp__docs__search_docs", "mcp__docs__scrape_docs"])
    selected = router.select("how does createSignal work in SolidJS")
    assert "mcp__docs__search_docs" in selected
    assert "mcp__docs__scrape_docs" not in selected  # write tool filtered

def test_empty_query_returns_base_only():
    router = ToolRouter(all_tool_names=["read_file", "write_file", "bash", "mcp__docs__search_docs"])
    selected = router.select("")
    assert "read_file" in selected
    assert "mcp__docs__search_docs" not in selected
```

- [ ] **Step 2: Implement ToolRouter**

```python
"""Pre-selects relevant tools based on user message keywords.

No LLM call needed. Uses keyword matching against tool names and descriptions.
Base tools (read/write/edit/bash/search/glob/listdir) always included.
MCP read-only tools included when keywords match.
"""

class ToolRouter:
    BASE_TOOLS = frozenset({
        "read_file", "write_file", "edit_file", "bash",
        "search_files", "glob_files", "list_directory",
        "propose_goal", "transition_to_act",
    })

    # Keywords that trigger inclusion of documentation/search MCP tools
    DOCS_KEYWORDS = frozenset({
        "docs", "documentation", "how to", "api", "reference",
        "example", "usage", "tutorial", "guide", "library",
    })

    # Keywords that trigger inclusion of specific built-in tools
    TOOL_KEYWORDS = {
        "test": ["bash"],
        "install": ["bash"],
        "search": ["search_files"],
        "find": ["search_files", "glob_files"],
        "create": ["write_file"],
        "modify": ["edit_file", "read_file"],
        "fix": ["edit_file", "read_file", "bash"],
        "run": ["bash"],
        "commit": ["bash"],
        "git": ["bash"],
    }

    def __init__(self, all_tool_names: list[str]) -> None:
        self._all_tools = all_tool_names

    def select(self, user_message: str, max_tools: int = 12) -> list[str]:
        selected = set(self.BASE_TOOLS) & set(self._all_tools)

        if not user_message:
            return sorted(selected)

        msg_lower = user_message.lower()

        # Add MCP read-only tools if docs-related keywords found
        if any(kw in msg_lower for kw in self.DOCS_KEYWORDS):
            for tool in self._all_tools:
                if tool.startswith("mcp__") and any(
                    k in tool for k in ("search", "list", "find", "fetch")
                ):
                    selected.add(tool)

        # Add tools matching keyword triggers
        for keyword, tools in self.TOOL_KEYWORDS.items():
            if keyword in msg_lower:
                for t in tools:
                    if t in self._all_tools:
                        selected.add(t)

        return sorted(selected)[:max_tools]
```

- [ ] **Step 3: Run tests + ruff**
- [ ] **Step 4: Commit**

```
git commit -m "feat: ToolRouter — keyword-based tool pre-selection (no LLM)"
```

---

## Task 2: Proactive docs-mcp context injection

**Files:**
- Modify: `workers/codeforge/consumer/_conversation.py`

- [ ] **Step 1: Add _prefetch_docs() method**

After MCP tools are discovered but before the agent loop starts, call `search_docs` for the detected stack frameworks and inject results as context.

```python
async def _prefetch_docs(
    self,
    workbench: McpWorkbench,
    stack_summary: str,
    user_message: str,
    log: BoundLogger,
) -> list[dict]:
    """Pre-fetch documentation from docs-mcp-server for detected frameworks.

    Returns context entries to inject into the system prompt.
    Called BEFORE the agent loop — the agent sees docs directly,
    doesn't need to call search_docs itself.
    """
    if not workbench or not stack_summary:
        return []

    entries = []
    # Extract framework names from stack summary like "typescript (solidjs), python (fastapi)"
    import re
    frameworks = re.findall(r'\((\w+)\)', stack_summary)

    for framework in frameworks[:3]:  # max 3 frameworks
        try:
            result = await workbench.call_tool(
                "search_docs",
                {"library": framework, "query": user_message, "limit": 3},
            )
            if result and result.output and len(result.output) > 50:
                entries.append({
                    "kind": "knowledge",
                    "path": f"docs/{framework}",
                    "content": result.output[:2000],  # cap at 2K chars
                    "tokens": len(result.output) // 4,
                    "priority": 80,
                })
                log.info("prefetched docs", framework=framework, chars=len(result.output))
        except Exception as exc:
            log.debug("docs prefetch failed", framework=framework, error=str(exc))

    return entries
```

- [ ] **Step 2: Wire into conversation handler**

Find where MCP workbench is created and agent loop starts. Insert prefetch between them.

The stack_summary comes from `run_msg.stack_summary` or needs to be passed through NATS. Check if it's available in the Python worker.

If not available: extract from workspace detection (read package.json, requirements.txt).

- [ ] **Step 3: Inject prefetched docs into context entries**

Append the prefetched entries to `run_msg.context` before passing to agent loop.

- [ ] **Step 4: Run ruff**
- [ ] **Step 5: Commit**

```
git commit -m "feat: proactive docs-mcp context injection before agent loop"
```

---

## Task 3: Wire ToolRouter into conversation handler

**Files:**
- Modify: `workers/codeforge/consumer/_conversation.py`

- [ ] **Step 1: Import and instantiate ToolRouter**

After registry is built and MCP tools merged, before agent loop:

```python
from codeforge.tools.tool_router import ToolRouter

# After registry + MCP tools merged:
all_tool_names = [t["function"]["name"] for t in registry.get_openai_tools()]
router = ToolRouter(all_tool_names)
user_msg = run_msg.messages[-1].content if run_msg.messages else ""
selected = router.select(user_msg)
log.info("tool router selected", count=len(selected), tools=selected)
```

- [ ] **Step 2: Pass selected tools to LoopConfig**

Add `selected_tools: list[str] | None` to LoopConfig. The agent loop uses this to filter the registry before LLM calls (instead of or in addition to capability-level filtering).

- [ ] **Step 3: Apply in agent_loop.py _filter_tools_for_capability**

If `cfg.selected_tools` is set, use it as the primary filter instead of TOOLS_BY_CAPABILITY:

```python
if cfg.selected_tools:
    allowed = frozenset(cfg.selected_tools)
else:
    allowed = TOOLS_BY_CAPABILITY.get(capability, frozenset())
```

- [ ] **Step 4: Run tests + ruff**
- [ ] **Step 5: Commit**

```
git commit -m "feat: wire ToolRouter into conversation handler pre-loop"
```

---

## Task Summary

| Task | What | Latency | LLM Cost |
|---|---|---|---|
| 1 | ToolRouter keyword selection | <1ms | $0 |
| 2 | Proactive docs-mcp prefetch | <500ms (MCP call) | $0 |
| 3 | Wire into conversation handler | 0ms | $0 |
| **Total** | | <500ms | **$0** |
