"""SkillRegistry: loads and manages skills from database and built-in YAML files."""

from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

import structlog
import yaml

from codeforge.skills.models import Skill

if TYPE_CHECKING:
    import psycopg

logger = structlog.get_logger()

_BUILTINS_DIR = Path(__file__).parent / "builtins"


def load_builtin_skills() -> list[Skill]:
    """Load built-in skills from the builtins/ directory."""
    skills: list[Skill] = []
    if not _BUILTINS_DIR.exists():
        return skills
    for yaml_path in sorted(_BUILTINS_DIR.glob("*.yaml")):
        with open(yaml_path) as f:
            data = yaml.safe_load(f)
        if not data or not isinstance(data, dict):
            continue
        name = data.get("name", yaml_path.stem)
        skills.append(
            Skill(
                id=f"builtin:{name}",
                name=name,
                type=data.get("type", "workflow"),
                description=data.get("description", ""),
                content=data.get("content", ""),
                tags=data.get("tags", []),
                source="builtin",
                format_origin="codeforge",
                status="active",
            )
        )
    logger.debug("builtin skills loaded", count=len(skills))
    return skills


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
