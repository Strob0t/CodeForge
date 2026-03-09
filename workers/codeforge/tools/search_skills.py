"""Built-in tool: search available skills by keyword."""

from __future__ import annotations

import logging

from codeforge.skills.models import Skill
from codeforge.skills.recommender import SkillRecommender
from codeforge.tools._base import ToolDefinition, ToolResult

logger = logging.getLogger(__name__)

DEFINITION = ToolDefinition(
    name="search_skills",
    description=(
        "Search for relevant skills by keyword or description. "
        "Use when you need a pattern, workflow, or reference that wasn't provided initially."
    ),
    parameters={
        "type": "object",
        "properties": {
            "query": {
                "type": "string",
                "description": "What kind of skill you're looking for.",
            },
            "type": {
                "type": "string",
                "enum": ["workflow", "pattern", "any"],
                "description": "Filter by skill type. Defaults to 'any'.",
            },
        },
        "required": ["query"],
    },
)

_MAX_RESULTS = 3


class SearchSkillsTool:
    """Executor for the search_skills tool."""

    def __init__(self, skills: list[Skill] | None = None) -> None:
        self._skills = skills or []

    def set_skills(self, skills: list[Skill]) -> None:
        """Update the available skills list (called before agent loop)."""
        self._skills = skills

    async def execute(self, arguments: dict[str, object], workspace_path: str) -> ToolResult:
        query = str(arguments.get("query", ""))
        type_filter = str(arguments.get("type", "any"))

        if not query or not self._skills:
            return ToolResult(output="No skills found matching your query.", success=True)

        # Filter by type if specified
        skills = self._skills
        if type_filter in ("workflow", "pattern"):
            skills = [s for s in skills if s.type == type_filter]

        if not skills:
            return ToolResult(output=f"No {type_filter} skills available.", success=True)

        recommender = SkillRecommender()
        recommender.index(skills)
        recs = recommender.recommend(query, top_k=_MAX_RESULTS)

        if not recs:
            return ToolResult(output="No skills found matching your query.", success=True)

        parts: list[str] = []
        for rec in recs:
            s = rec.skill
            parts.append(
                f"### {s.name} ({s.type})\n"
                f"**Description:** {s.description}\n"
                f"**Tags:** {', '.join(s.tags)}\n\n"
                f"{s.content}"
            )

        return ToolResult(output="\n\n---\n\n".join(parts), success=True)
