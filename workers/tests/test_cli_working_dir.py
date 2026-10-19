"""A CLI backend works in its task's workspace; an override must stay inside it (KI-96, O13)."""

from __future__ import annotations

import pytest

from codeforge.backends._cli_base import working_dir


@pytest.mark.parametrize(
    ("workspace", "override", "expected"),
    [
        ("/ws/p1", "", "/ws/p1"),
        ("/ws/p1", "sub", "/ws/p1/sub"),
        ("/ws/p1", "sub/../other", "/ws/p1/other"),
        ("/ws/p1", ".", "/ws/p1"),
        ("/ws/p1", "/ws/p1/abs", "/ws/p1/abs"),
        ("/ws/p1/", "sub/", "/ws/p1/sub"),
    ],
)
def test_working_dir_inside_the_workspace(workspace: str, override: str, expected: str) -> None:
    assert working_dir(workspace, override) == expected


@pytest.mark.parametrize(
    ("workspace", "override"),
    [
        ("/ws/p1", ".."),
        ("/ws/p1", "../p2"),
        ("/ws/p1", "/ws/p2"),
        ("/ws/p1", "/"),
        ("/ws/p1", "/ws/p1x"),
        ("", "sub"),
    ],
)
def test_working_dir_outside_the_workspace_is_refused(workspace: str, override: str) -> None:
    with pytest.raises(ValueError, match="workspace"):
        working_dir(workspace, override)
