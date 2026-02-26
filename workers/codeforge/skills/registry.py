"""SkillRegistry: loads and manages skills from database and built-in YAML files."""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from codeforge.skills.models import Skill

if TYPE_CHECKING:
    import psycopg

logger = structlog.get_logger()


class SkillRegistry:
    """Manages the collection of available skills."""

    def __init__(self, db: psycopg.AsyncConnection[object] | None = None) -> None:
        self._db = db
        self._skills: list[Skill] = []

    async def load_skills(self, project_id: str) -> list[Skill]:
        """Load skills from the database for a project (including global)."""
        if self._db is None:
            return self._skills

        async with self._db.cursor() as cur:
            await cur.execute(
                """SELECT id, tenant_id, project_id, name, description, language, code, tags, enabled
                   FROM skills
                   WHERE (project_id = %s OR project_id = '') AND enabled = TRUE
                   ORDER BY created_at ASC""",
                (project_id,),
            )
            rows = await cur.fetchall()

        self._skills = [
            Skill(
                id=str(row[0]),
                tenant_id=str(row[1]),
                project_id=row[2] or "",
                name=row[3],
                description=row[4] or "",
                language=row[5] or "",
                code=row[6],
                tags=row[7] or [],
                enabled=row[8],
            )
            for row in rows
        ]

        logger.info("skills loaded", count=len(self._skills), project_id=project_id)
        return self._skills

    @property
    def skills(self) -> list[Skill]:
        """Return the currently loaded skills."""
        return self._skills
