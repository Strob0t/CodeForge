"""Role responsibility matrix for the 9 evaluation roles.

Maps each role to its expected input/output types, allowed/denied tools,
and whether its tests run in Go or Python.
"""

from __future__ import annotations

from dataclasses import dataclass, field


@dataclass(frozen=True)
class RoleSpec:
    """Specification for a single agent role."""

    role: str
    mode_id: str  # maps to Go builtin mode ID
    input_type: str
    output_artifact: str
    allowed_tools: list[str] = field(default_factory=list)
    denied_tools: list[str] = field(default_factory=list)
    test_location: str = "python"  # "python" or "go"


ROLE_MATRIX: dict[str, RoleSpec] = {
    "orchestrator": RoleSpec(
        role="orchestrator",
        mode_id="",  # Go-side only
        input_type="feature_request",
        output_artifact="strategy_selection",
        test_location="go",
    ),
    "architect": RoleSpec(
        role="architect",
        mode_id="architect",
        input_type="feature_request",
        output_artifact="PLAN.md",
        allowed_tools=["read", "list", "search", "graph"],
        denied_tools=["write", "shell", "browser"],
    ),
    "coder": RoleSpec(
        role="coder",
        mode_id="coder",
        input_type="plan_or_issue",
        output_artifact="DIFF",
        allowed_tools=["read", "write", "shell", "list", "search", "graph"],
        denied_tools=["browser"],
    ),
    "reviewer": RoleSpec(
        role="reviewer",
        mode_id="reviewer",
        input_type="diff_or_code",
        output_artifact="REVIEW.md",
        allowed_tools=["read", "list", "search", "graph"],
        denied_tools=["write", "shell", "browser"],
    ),
    "security": RoleSpec(
        role="security",
        mode_id="security",
        input_type="diff_or_code",
        output_artifact="AUDIT_REPORT",
        allowed_tools=["read", "list", "search", "graph"],
        denied_tools=["write", "shell", "browser"],
    ),
    "tester": RoleSpec(
        role="tester",
        mode_id="tester",
        input_type="code_or_spec",
        output_artifact="TEST_REPORT",
        allowed_tools=["read", "write", "shell", "list", "search"],
        denied_tools=["browser"],
    ),
    "debugger": RoleSpec(
        role="debugger",
        mode_id="debugger",
        input_type="bug_report",
        output_artifact="DIFF",
        allowed_tools=["read", "write", "shell", "list", "search", "graph"],
        denied_tools=["browser"],
    ),
    "proponent": RoleSpec(
        role="proponent",
        mode_id="",  # debate-internal role
        input_type="proposal",
        output_artifact="argument",
        test_location="python",
    ),
    "moderator": RoleSpec(
        role="moderator",
        mode_id="",  # debate-internal role
        input_type="arguments",
        output_artifact="verdict",
        test_location="python",
    ),
}
