"""SPEC "Output" / JSON: the report object, its problem fields and its counts."""

from __future__ import annotations

from typing import TYPE_CHECKING

from acceptance_helpers import column_of, make_tree, md, run_cli

if TYPE_CHECKING:
    from pathlib import Path

PROBLEM_FIELDS = {"path", "line", "column", "kind", "target", "message"}


def _without_messages(problems: object) -> list[dict[str, object]]:
    assert isinstance(problems, list), problems
    stripped = []
    for problem in problems:
        assert isinstance(problem, dict), problem
        assert set(problem) == PROBLEM_FIELDS, problem
        assert isinstance(problem["message"], str), problem
        assert problem["message"], problem
        stripped.append({key: value for key, value in problem.items() if key != "message"})
    return stripped


def test_json_report_has_the_documented_shape(tmp_path: Path) -> None:
    make_tree(tmp_path, {"docs/guide.md": "# Guide\n\nSee [setup](../setup.md).\n", "docs/api.md": "[g](guide.md)\n"})

    run = run_cli(["--format", "json"], tmp_path)

    assert run.exit_code == 1, run.describe()
    report = run.json()
    assert set(report) == {"files_checked", "links_checked", "problems"}, run.describe()
    assert report["files_checked"] == 2, run.describe()
    assert report["links_checked"] == 2, run.describe()
    assert _without_messages(report["problems"]) == [
        {"path": "docs/guide.md", "line": 3, "column": 5, "kind": "missing-file", "target": "../setup.md"}
    ]


def test_json_counts_only_local_and_anchor_links_without_check_external(tmp_path: Path) -> None:
    page = md(
        """
        # Top

        [a](exists.txt) ![b](img.png) [c](#top) [d](mailto:someone@example.com)
        [e](https://example.invalid/) <https://example.invalid/x> [f](ftp://example.invalid/)
        """
    )
    make_tree(
        tmp_path,
        {"page.md": page, "other.md": "[g](page.md#top)\n", "exists.txt": "text\n", "img.png": "png"},
    )

    run = run_cli(["--format", "json"], tmp_path)

    assert run.exit_code == 0, run.describe()
    assert run.json() == {"files_checked": 2, "links_checked": 4, "problems": []}, run.describe()


def test_json_problems_are_sorted_and_carry_the_target_as_written(tmp_path: Path) -> None:
    line = "[y](my%20gone.md?x=1#frag) [z](#nowhere)"
    make_tree(tmp_path, {"b.md": "[x][undefined id]\n", "a.md": f"Line one.\n{line}\n"})

    run = run_cli(["--format", "json"], tmp_path)

    assert run.exit_code == 1, run.describe()
    assert _without_messages(run.json()["problems"]) == [
        {"path": "a.md", "line": 2, "column": 1, "kind": "missing-file", "target": "my%20gone.md?x=1#frag"},
        {"path": "a.md", "line": 2, "column": column_of(line, "[z]"), "kind": "missing-anchor", "target": "#nowhere"},
        {"path": "b.md", "line": 1, "column": 1, "kind": "undefined-reference", "target": "undefined id"},
    ], run.describe()
