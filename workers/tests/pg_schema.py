"""A throwaway PostgreSQL schema with CodeForge migrations, for tests of the worker's own SQL.

The worker writes some tables directly with psycopg (experience pool, agent
skill drafts). These tests run that SQL against the real column types of the
migrations: set ``CODEFORGE_TEST_DATABASE_URL`` (for example
``postgresql://codeforge:codeforge_dev@localhost:5432/codeforge``) to run them;
without it they are skipped. Each test gets its own schema, dropped afterwards,
so the database's own tables are never touched.
"""

from __future__ import annotations

import os
import uuid
from pathlib import Path
from typing import TYPE_CHECKING

import psycopg
import pytest
from psycopg.conninfo import make_conninfo

if TYPE_CHECKING:
    from collections.abc import AsyncIterator

MIGRATIONS_DIR = Path(__file__).resolve().parents[2] / "internal/adapter/postgres/migrations"
TEST_DATABASE_ENV = "CODEFORGE_TEST_DATABASE_URL"


def goose_up(migration: str) -> str:
    """Return the Up section of a goose migration file."""
    text = (MIGRATIONS_DIR / migration).read_text()
    return text.split("-- +goose Up", 1)[1].split("-- +goose Down", 1)[0]


async def create_schema(*migrations: str) -> AsyncIterator[str]:
    """Yield a conninfo whose search_path is a new schema with *migrations* applied.

    The schema gets a minimal ``projects`` table first, for foreign keys.
    """
    base = os.environ.get(TEST_DATABASE_ENV, "")
    if not base:
        pytest.skip(f"{TEST_DATABASE_ENV} is not set")
    schema = f"cf_test_{uuid.uuid4().hex[:12]}"
    dsn = make_conninfo(base, options=f"-c search_path={schema}")
    async with await psycopg.AsyncConnection.connect(base, autocommit=True) as admin:
        await admin.execute(f'CREATE SCHEMA "{schema}"')
    try:
        async with await psycopg.AsyncConnection.connect(dsn, autocommit=True) as conn:
            await conn.execute("CREATE TABLE projects (id UUID PRIMARY KEY DEFAULT gen_random_uuid())")
            for migration in migrations:
                await conn.execute(goose_up(migration))  # type: ignore[arg-type]
        yield dsn
    finally:
        async with await psycopg.AsyncConnection.connect(base, autocommit=True) as admin:
            await admin.execute(f'DROP SCHEMA "{schema}" CASCADE')
