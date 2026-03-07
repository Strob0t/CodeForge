"""Tests for db_schema_audit.py — written BEFORE implementation (TDD RED phase)."""

from __future__ import annotations

import json
import sys
from pathlib import Path
from unittest.mock import MagicMock

# Ensure tools/ is importable
sys.path.insert(0, str(Path(__file__).parent))

from db_schema_audit import (
    Finding,
    audit_schema,
    calculate_score,
    format_output,
    parse_go_store_queries,
    parse_migrations,
)

# ---------------------------------------------------------------------------
# Fixtures — representative SQL snippets from real CodeForge migrations
# ---------------------------------------------------------------------------

MIGRATION_CREATE_TABLE = """\
-- +goose Up
CREATE TABLE IF NOT EXISTS projects (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    repo_url    TEXT NOT NULL DEFAULT '',
    provider    TEXT NOT NULL DEFAULT 'local',
    config      JSONB NOT NULL DEFAULT '{}',
    tenant_id   UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_projects_created_at ON projects (created_at DESC);
CREATE INDEX idx_projects_tenant ON projects (tenant_id);

CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_projects_updated_at
    BEFORE UPDATE ON projects
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS trg_projects_updated_at ON projects;
DROP FUNCTION IF EXISTS update_updated_at();
DROP TABLE IF EXISTS projects;
"""

MIGRATION_WITH_FK = """\
-- +goose Up
CREATE TABLE IF NOT EXISTS tasks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'completed', 'failed')),
    tenant_id   UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tasks_project_id ON tasks (project_id);
CREATE INDEX idx_tasks_tenant ON tasks (tenant_id);

-- +goose Down
DROP TABLE IF EXISTS tasks;
"""

MIGRATION_INDEXES = """\
-- +goose Up
CREATE INDEX idx_events_task_id ON agent_events (task_id, version);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_tenant ON users (email, tenant_id);
CREATE INDEX IF NOT EXISTS idx_tasks_project_active
    ON tasks(project_id, status) WHERE status IN ('queued', 'running');

-- +goose Down
DROP INDEX IF EXISTS idx_tasks_project_active;
DROP INDEX IF EXISTS idx_users_email_tenant;
DROP INDEX IF EXISTS idx_events_task_id;
"""

MIGRATION_CONCURRENTLY = """\
-- +goose Up
-- +goose StatementBegin
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_agent_events_tenant_task
    ON agent_events (tenant_id, task_id, version);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX CONCURRENTLY IF EXISTS idx_agent_events_tenant_task;
-- +goose StatementEnd
"""

MIGRATION_NO_TENANT = """\
-- +goose Up
CREATE TABLE IF NOT EXISTS quarantine_messages (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id    UUID NOT NULL,
    content     TEXT NOT NULL,
    risk_score  REAL NOT NULL DEFAULT 0.0,
    status      TEXT NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS quarantine_messages;
"""

MIGRATION_VARCHAR_MIX = """\
-- +goose Up
CREATE TABLE IF NOT EXISTS mixed_types (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    label       VARCHAR(100),
    tenant_id   UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS mixed_types;
"""

MIGRATION_TEXT_PK = """\
-- +goose Up
CREATE TABLE IF NOT EXISTS api_keys (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    key_hash    TEXT NOT NULL,
    user_id     TEXT NOT NULL,
    tenant_id   UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS api_keys;
"""

MIGRATION_JSONB_ARRAY = """\
-- +goose Up
CREATE TABLE IF NOT EXISTS agent_configs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    tools       JSONB NOT NULL DEFAULT '[]',
    settings    JSONB NOT NULL DEFAULT '{}',
    tenant_id   UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS agent_configs;
"""

MIGRATION_ALTER = """\
-- +goose Up
ALTER TABLE runs ADD COLUMN team_id UUID REFERENCES agent_teams(id) ON DELETE SET NULL;
ALTER TABLE runs ADD COLUMN output TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE runs DROP COLUMN IF EXISTS output;
ALTER TABLE runs DROP COLUMN IF EXISTS team_id;
"""

MIGRATION_NO_DOWN = """\
-- +goose Up
CREATE TABLE IF NOT EXISTS orphan_table (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    data TEXT NOT NULL,
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'
);
"""

MIGRATION_REDUNDANT_INDEX = """\
-- +goose Up
CREATE TABLE IF NOT EXISTS indexed_table (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects(id),
    status      TEXT NOT NULL DEFAULT 'active',
    tenant_id   UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_indexed_table_project ON indexed_table (project_id);
CREATE INDEX idx_indexed_table_project_status ON indexed_table (project_id, status);

-- +goose Down
DROP INDEX IF EXISTS idx_indexed_table_project_status;
DROP INDEX IF EXISTS idx_indexed_table_project;
DROP TABLE IF EXISTS indexed_table;
"""

MIGRATION_MISSING_FK_INDEX = """\
-- +goose Up
CREATE TABLE IF NOT EXISTS reviews (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    author_id   UUID NOT NULL REFERENCES users(id),
    content     TEXT NOT NULL,
    tenant_id   UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS reviews;
"""

GO_STORE_CONTENT = """\
package postgres

func (s *Store) GetConversation(ctx context.Context, id string) (*domain.Conversation, error) {
    row := s.db.QueryRow(ctx, `
        SELECT id, tenant_id, project_id, title, created_at, updated_at
        FROM conversations WHERE id = $1`, id)
    return scanConversation(row)
}

func (s *Store) ListConversations(ctx context.Context, projectID string) ([]*domain.Conversation, error) {
    tenantID := tenantFromCtx(ctx)
    rows, err := s.db.Query(ctx, `
        SELECT id, tenant_id, project_id, title, created_at, updated_at
        FROM conversations WHERE project_id = $1 AND tenant_id = $2
        ORDER BY created_at DESC`, projectID, tenantID)
    return scanConversations(rows)
}

func (s *Store) DeleteConversation(ctx context.Context, id string) error {
    _, err := s.db.Exec(ctx, `DELETE FROM conversations WHERE id = $1`, id)
    return err
}
"""


# ---------------------------------------------------------------------------
# Helper: write migrations to temp dir
# ---------------------------------------------------------------------------


def _write_migrations(tmpdir: Path, migrations: dict[str, str]) -> Path:
    """Write migration files to a temp directory, return the path."""
    mig_dir = tmpdir / "migrations"
    mig_dir.mkdir(parents=True, exist_ok=True)
    for filename, content in migrations.items():
        (mig_dir / filename).write_text(content)
    return mig_dir


# ===========================================================================
# 1. Parser Tests — CREATE TABLE
# ===========================================================================


class TestParseCreateTable:
    def test_parses_table_name(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"001_init.sql": MIGRATION_CREATE_TABLE})
        tables, _, _ = parse_migrations(mig_dir)
        assert "projects" in {t.name for t in tables}

    def test_parses_columns(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"001_init.sql": MIGRATION_CREATE_TABLE})
        tables, _, _ = parse_migrations(mig_dir)
        proj = next(t for t in tables if t.name == "projects")
        col_names = {c.name for c in proj.columns}
        assert {
            "id",
            "name",
            "description",
            "repo_url",
            "provider",
            "config",
            "tenant_id",
            "created_at",
            "updated_at",
        } == col_names

    def test_parses_column_types(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"001_init.sql": MIGRATION_CREATE_TABLE})
        tables, _, _ = parse_migrations(mig_dir)
        proj = next(t for t in tables if t.name == "projects")
        id_col = next(c for c in proj.columns if c.name == "id")
        assert id_col.data_type == "UUID"
        config_col = next(c for c in proj.columns if c.name == "config")
        assert config_col.data_type == "JSONB"

    def test_detects_pk(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"001_init.sql": MIGRATION_CREATE_TABLE})
        tables, _, _ = parse_migrations(mig_dir)
        proj = next(t for t in tables if t.name == "projects")
        assert proj.pk_type == "UUID"
        id_col = next(c for c in proj.columns if c.name == "id")
        assert id_col.is_pk is True

    def test_detects_fk(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"002_tasks.sql": MIGRATION_WITH_FK})
        tables, _, _ = parse_migrations(mig_dir)
        task = next(t for t in tables if t.name == "tasks")
        fk_col = next(c for c in task.columns if c.name == "project_id")
        assert fk_col.is_fk is True
        assert fk_col.references_table == "projects"

    def test_detects_tenant_id(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"001_init.sql": MIGRATION_CREATE_TABLE})
        tables, _, _ = parse_migrations(mig_dir)
        proj = next(t for t in tables if t.name == "projects")
        assert proj.has_tenant_id is True

    def test_detects_missing_tenant_id(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"005_no_tenant.sql": MIGRATION_NO_TENANT})
        tables, _, _ = parse_migrations(mig_dir)
        qt = next(t for t in tables if t.name == "quarantine_messages")
        assert qt.has_tenant_id is False

    def test_detects_updated_at(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"001_init.sql": MIGRATION_CREATE_TABLE})
        tables, _, _ = parse_migrations(mig_dir)
        proj = next(t for t in tables if t.name == "projects")
        assert proj.has_updated_at is True

    def test_detects_trigger(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"001_init.sql": MIGRATION_CREATE_TABLE})
        tables, _, _ = parse_migrations(mig_dir)
        proj = next(t for t in tables if t.name == "projects")
        assert proj.has_updated_at_trigger is True

    def test_detects_text_pk(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"022_api_keys.sql": MIGRATION_TEXT_PK})
        tables, _, _ = parse_migrations(mig_dir)
        ak = next(t for t in tables if t.name == "api_keys")
        assert ak.pk_type == "TEXT"

    def test_parses_check_constraint(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"002_tasks.sql": MIGRATION_WITH_FK})
        tables, _, _ = parse_migrations(mig_dir)
        task = next(t for t in tables if t.name == "tasks")
        status_col = next(c for c in task.columns if c.name == "status")
        assert status_col.data_type == "TEXT"

    def test_source_migration_tracked(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"001_init.sql": MIGRATION_CREATE_TABLE})
        tables, _, _ = parse_migrations(mig_dir)
        proj = next(t for t in tables if t.name == "projects")
        assert proj.source_migration == "001_init.sql"


# ===========================================================================
# 2. Parser Tests — CREATE INDEX
# ===========================================================================


class TestParseCreateIndex:
    def test_parses_regular_index(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"001_init.sql": MIGRATION_CREATE_TABLE})
        _, indexes, _ = parse_migrations(mig_dir)
        idx = next(i for i in indexes if i.name == "idx_projects_created_at")
        assert idx.table_name == "projects"
        assert idx.columns == ["created_at"]
        assert idx.is_unique is False
        assert idx.is_partial is False

    def test_parses_composite_index(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"003_indexes.sql": MIGRATION_INDEXES})
        _, indexes, _ = parse_migrations(mig_dir)
        idx = next(i for i in indexes if i.name == "idx_events_task_id")
        assert idx.columns == ["task_id", "version"]

    def test_parses_unique_index(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"003_indexes.sql": MIGRATION_INDEXES})
        _, indexes, _ = parse_migrations(mig_dir)
        idx = next(i for i in indexes if i.name == "idx_users_email_tenant")
        assert idx.is_unique is True
        assert idx.columns == ["email", "tenant_id"]

    def test_parses_partial_index(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"003_indexes.sql": MIGRATION_INDEXES})
        _, indexes, _ = parse_migrations(mig_dir)
        idx = next(i for i in indexes if i.name == "idx_tasks_project_active")
        assert idx.is_partial is True
        assert "status" in idx.condition.lower()

    def test_parses_concurrently_index(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"058_concurrent.sql": MIGRATION_CONCURRENTLY})
        _, indexes, _ = parse_migrations(mig_dir)
        idx = next(i for i in indexes if i.name == "idx_agent_events_tenant_task")
        assert idx.columns == ["tenant_id", "task_id", "version"]
        assert idx.table_name == "agent_events"


# ===========================================================================
# 3. Parser Tests — Migrations metadata
# ===========================================================================


class TestParseMigrations:
    def test_detects_up_and_down(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"001_init.sql": MIGRATION_CREATE_TABLE})
        _, _, migrations = parse_migrations(mig_dir)
        m = next(m for m in migrations if m.number == 1)
        assert m.has_up is True
        assert m.has_down is True

    def test_detects_missing_down(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"099_no_down.sql": MIGRATION_NO_DOWN})
        _, _, migrations = parse_migrations(mig_dir)
        m = next(m for m in migrations if m.number == 99)
        assert m.has_up is True
        assert m.has_down is False

    def test_migration_name(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"001_init.sql": MIGRATION_CREATE_TABLE})
        _, _, migrations = parse_migrations(mig_dir)
        m = next(m for m in migrations if m.number == 1)
        assert m.name == "001_init.sql"

    def test_empty_directory(self, tmp_path: Path) -> None:
        mig_dir = tmp_path / "empty_migrations"
        mig_dir.mkdir()
        tables, indexes, migrations = parse_migrations(mig_dir)
        assert tables == []
        assert indexes == []
        assert migrations == []

    def test_multiple_migrations(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(
            tmp_path,
            {
                "001_init.sql": MIGRATION_CREATE_TABLE,
                "002_tasks.sql": MIGRATION_WITH_FK,
                "003_indexes.sql": MIGRATION_INDEXES,
            },
        )
        tables, _indexes, migrations = parse_migrations(mig_dir)
        assert len(migrations) == 3
        table_names = {t.name for t in tables}
        assert "projects" in table_names
        assert "tasks" in table_names

    def test_alter_table_adds_column(self, tmp_path: Path) -> None:
        """ALTER TABLE ADD COLUMN should update the existing table model."""
        mig_dir = _write_migrations(
            tmp_path,
            {
                "001_init.sql": MIGRATION_CREATE_TABLE,
                "010_alter.sql": MIGRATION_ALTER,
            },
        )
        tables, _, _ = parse_migrations(mig_dir)
        # ALTER on 'runs' — if runs doesn't exist as CREATE TABLE,
        # the parser should still track the ALTER columns
        # This tests graceful handling of ALTER on tables not yet seen
        assert len(tables) >= 1  # at least 'projects'


# ===========================================================================
# 4. Audit Category Tests
# ===========================================================================


class TestAuditMultiTenantIsolation:
    """Category 9: Multi-Tenant Isolation."""

    def test_missing_tenant_id_critical(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"005_no_tenant.sql": MIGRATION_NO_TENANT})
        tables, indexes, migrations = parse_migrations(mig_dir)
        findings = audit_schema(tables, indexes, migrations)
        tenant_findings = [
            f
            for f in findings
            if f.category == "multi_tenant_isolation"
            and f.table == "quarantine_messages"
            and "tenant_id" in f.message.lower()
        ]
        assert len(tenant_findings) >= 1
        assert tenant_findings[0].severity == "critical"

    def test_table_with_tenant_id_no_finding(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"001_init.sql": MIGRATION_CREATE_TABLE})
        tables, indexes, migrations = parse_migrations(mig_dir)
        findings = audit_schema(tables, indexes, migrations)
        tenant_findings = [
            f
            for f in findings
            if f.category == "multi_tenant_isolation"
            and f.table == "projects"
            and "tenant_id" in f.message.lower()
            and "lacks" in f.message.lower()
        ]
        assert len(tenant_findings) == 0


class TestAuditDataTypeConsistency:
    """Category 4: Data Type Consistency."""

    def test_varchar_text_mix(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"006_mixed.sql": MIGRATION_VARCHAR_MIX})
        tables, indexes, migrations = parse_migrations(mig_dir)
        findings = audit_schema(tables, indexes, migrations)
        dtype_findings = [f for f in findings if f.category == "data_type_consistency" and f.table == "mixed_types"]
        assert len(dtype_findings) >= 1
        assert any(f.severity == "medium" for f in dtype_findings)

    def test_pk_type_mixing(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(
            tmp_path,
            {
                "001_init.sql": MIGRATION_CREATE_TABLE,
                "022_api_keys.sql": MIGRATION_TEXT_PK,
            },
        )
        tables, indexes, migrations = parse_migrations(mig_dir)
        findings = audit_schema(tables, indexes, migrations)
        pk_findings = [f for f in findings if f.category == "data_type_consistency" and "pk" in f.message.lower()]
        assert len(pk_findings) >= 1


class TestAuditIndexStrategy:
    """Category 1: Index Strategy."""

    def test_redundant_index(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"007_redundant.sql": MIGRATION_REDUNDANT_INDEX})
        tables, indexes, migrations = parse_migrations(mig_dir)
        findings = audit_schema(tables, indexes, migrations)
        redundant = [f for f in findings if f.category == "index_strategy" and "redundant" in f.message.lower()]
        assert len(redundant) >= 1
        assert redundant[0].severity == "low"

    def test_missing_fk_index(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"008_no_fk_idx.sql": MIGRATION_MISSING_FK_INDEX})
        tables, indexes, migrations = parse_migrations(mig_dir)
        findings = audit_schema(tables, indexes, migrations)
        fk_findings = [
            f
            for f in findings
            if f.category == "index_strategy" and "missing" in f.message.lower() and "index" in f.message.lower()
        ]
        # Should flag both project_id and author_id as FK columns without indexes
        fk_tables = [f.table for f in fk_findings if f.table == "reviews"]
        assert len(fk_tables) >= 1
        assert any(f.severity == "high" for f in fk_findings if f.table == "reviews")


class TestAuditMigrationQuality:
    """Category 10: Migration Quality."""

    def test_missing_down_migration(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"099_no_down.sql": MIGRATION_NO_DOWN})
        tables, indexes, migrations = parse_migrations(mig_dir)
        findings = audit_schema(tables, indexes, migrations)
        down_findings = [f for f in findings if f.category == "migration_quality" and "down" in f.message.lower()]
        assert len(down_findings) >= 1
        assert down_findings[0].severity == "medium"


class TestAuditNamingConventions:
    """Category 5: Naming Conventions."""

    def test_valid_snake_case_no_finding(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"001_init.sql": MIGRATION_CREATE_TABLE})
        tables, indexes, migrations = parse_migrations(mig_dir)
        findings = audit_schema(tables, indexes, migrations)
        naming_findings = [f for f in findings if f.category == "naming_conventions" and f.table == "projects"]
        assert len(naming_findings) == 0


class TestAuditJsonbUsage:
    """Category 6: JSONB vs Relational."""

    def test_jsonb_column_flagged(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"009_jsonb.sql": MIGRATION_JSONB_ARRAY})
        tables, indexes, migrations = parse_migrations(mig_dir)
        findings = audit_schema(tables, indexes, migrations)
        jsonb_findings = [f for f in findings if f.category == "jsonb_usage" and f.table == "agent_configs"]
        assert len(jsonb_findings) >= 1
        assert any(f.severity == "low" for f in jsonb_findings)


class TestAuditConstraints:
    """Category 3: FK/Unique/Check Constraints."""

    def test_missing_check_on_status(self, tmp_path: Path) -> None:
        """A status column without CHECK should be flagged."""
        mig_dir = _write_migrations(tmp_path, {"005_no_tenant.sql": MIGRATION_NO_TENANT})
        tables, indexes, migrations = parse_migrations(mig_dir)
        findings = audit_schema(tables, indexes, migrations)
        check_findings = [
            f
            for f in findings
            if f.category == "constraints" and f.table == "quarantine_messages" and "check" in f.message.lower()
        ]
        # quarantine_messages.status has no CHECK constraint
        assert len(check_findings) >= 1


# ===========================================================================
# 5. Score Calculation
# ===========================================================================


class TestScoreCalculation:
    def test_no_findings_score_100(self) -> None:
        assert calculate_score([]) == 100

    def test_critical_deducts_5(self) -> None:
        findings = [Finding("multi_tenant_isolation", "critical", "t", "msg")]
        assert calculate_score(findings) == 95

    def test_high_deducts_3(self) -> None:
        findings = [Finding("index_strategy", "high", "t", "msg")]
        assert calculate_score(findings) == 97

    def test_medium_deducts_2(self) -> None:
        findings = [Finding("migration_quality", "medium", "t", "msg")]
        assert calculate_score(findings) == 98

    def test_low_deducts_1(self) -> None:
        findings = [Finding("naming_conventions", "low", "t", "msg")]
        assert calculate_score(findings) == 99

    def test_floor_at_zero(self) -> None:
        findings = [Finding("x", "critical", "t", "msg") for _ in range(30)]
        assert calculate_score(findings) == 0

    def test_mixed_severities(self) -> None:
        findings = [
            Finding("a", "critical", "t", "m"),  # -5
            Finding("b", "high", "t", "m"),  # -3
            Finding("c", "medium", "t", "m"),  # -2
            Finding("d", "low", "t", "m"),  # -1
        ]
        assert calculate_score(findings) == 89  # 100 - 5 - 3 - 2 - 1


# ===========================================================================
# 6. Go Store Cross-Reference
# ===========================================================================


class TestGoStoreCrossReference:
    def test_parses_queries(self, tmp_path: Path) -> None:
        store_dir = tmp_path / "stores"
        store_dir.mkdir()
        (store_dir / "store_conversation.go").write_text(GO_STORE_CONTENT)
        queries = parse_go_store_queries(store_dir)
        assert len(queries) >= 2
        # Should find queries on 'conversations' table
        conv_queries = [q for q in queries if q.get("table") == "conversations"]
        assert len(conv_queries) >= 2

    def test_detects_missing_tenant_in_query(self, tmp_path: Path) -> None:
        store_dir = tmp_path / "stores"
        store_dir.mkdir()
        (store_dir / "store_conversation.go").write_text(GO_STORE_CONTENT)
        mig_dir = _write_migrations(tmp_path, {"001_init.sql": MIGRATION_CREATE_TABLE})
        tables, indexes, migrations = parse_migrations(mig_dir)
        queries = parse_go_store_queries(store_dir)
        findings = audit_schema(tables, indexes, migrations, go_queries=queries)
        tenant_query_findings = [
            f
            for f in findings
            if f.category == "multi_tenant_isolation"
            and "query" in f.message.lower()
            and "tenant_id" in f.message.lower()
        ]
        # GetConversation and DeleteConversation miss tenant_id
        assert len(tenant_query_findings) >= 1


# ===========================================================================
# 7. Output Format
# ===========================================================================


class TestOutputFormat:
    def test_json_output_schema(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"005_no_tenant.sql": MIGRATION_NO_TENANT})
        tables, indexes, migrations = parse_migrations(mig_dir)
        findings = audit_schema(tables, indexes, migrations)
        score = calculate_score(findings)
        output = format_output(findings, score, tables, indexes, migrations, fmt="json")
        data = json.loads(output)
        assert "score" in data
        assert "summary" in data
        assert "findings" in data
        assert "tables_analyzed" in data
        assert "indexes_analyzed" in data
        assert "migrations_analyzed" in data
        assert isinstance(data["score"], int)
        assert isinstance(data["findings"], list)
        if data["findings"]:
            f = data["findings"][0]
            assert "category" in f
            assert "severity" in f
            assert "table" in f
            assert "message" in f

    def test_json_summary_counts(self, tmp_path: Path) -> None:
        findings = [
            Finding("a", "critical", "t", "m"),
            Finding("b", "high", "t", "m"),
            Finding("c", "medium", "t", "m"),
            Finding("d", "low", "t", "m"),
        ]
        output = format_output(findings, 89, [], [], [], fmt="json")
        data = json.loads(output)
        assert data["summary"]["critical"] == 1
        assert data["summary"]["high"] == 1
        assert data["summary"]["medium"] == 1
        assert data["summary"]["low"] == 1

    def test_markdown_output(self, tmp_path: Path) -> None:
        findings = [Finding("a", "critical", "table1", "Something is wrong")]
        output = format_output(findings, 95, [], [], [], fmt="markdown")
        assert "# Database Schema Audit" in output
        assert "95" in output
        assert "Something is wrong" in output

    def test_sql_patch_in_output(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(tmp_path, {"005_no_tenant.sql": MIGRATION_NO_TENANT})
        tables, indexes, migrations = parse_migrations(mig_dir)
        findings = audit_schema(tables, indexes, migrations)
        score = calculate_score(findings)
        output = format_output(findings, score, tables, indexes, migrations, fmt="json")
        data = json.loads(output)
        # Missing tenant_id findings should include sql_patch
        tenant_findings = [
            f
            for f in data["findings"]
            if f["category"] == "multi_tenant_isolation"
            and "tenant_id" in f["message"].lower()
            and "lacks" in f["message"].lower()
        ]
        assert any("sql_patch" in f for f in tenant_findings)


# ===========================================================================
# 8. Edge Cases
# ===========================================================================


class TestEdgeCases:
    def test_empty_migrations_score_100(self, tmp_path: Path) -> None:
        mig_dir = tmp_path / "empty_mig"
        mig_dir.mkdir()
        tables, indexes, migrations = parse_migrations(mig_dir)
        findings = audit_schema(tables, indexes, migrations)
        assert calculate_score(findings) == 100

    def test_nonexistent_directory(self, tmp_path: Path) -> None:
        mig_dir = tmp_path / "does_not_exist"
        tables, indexes, migrations = parse_migrations(mig_dir)
        assert tables == []
        assert indexes == []
        assert migrations == []

    def test_non_sql_files_ignored(self, tmp_path: Path) -> None:
        mig_dir = _write_migrations(
            tmp_path,
            {
                "001_init.sql": MIGRATION_CREATE_TABLE,
                "README.md": "# Migrations\nThis is not SQL.",
            },
        )
        _tables, _indexes, migrations = parse_migrations(mig_dir)
        assert len(migrations) == 1


# ===========================================================================
# 9. Live Mode (mocked psycopg)
# ===========================================================================


class TestLiveMode:
    def test_live_queries_unused_indexes(self) -> None:
        """Live mode should query pg_stat_user_indexes for unused indexes."""
        # We import here to avoid issues if psycopg not available
        from db_schema_audit import get_live_findings

        mock_conn = MagicMock()
        mock_cursor = MagicMock()
        mock_conn.cursor.return_value.__enter__ = lambda s: mock_cursor
        mock_conn.cursor.return_value.__exit__ = MagicMock(return_value=False)
        mock_cursor.fetchall.return_value = [
            ("idx_unused_thing", "some_table", 0, 8192),
        ]

        findings = get_live_findings(mock_conn)
        assert len(findings) >= 1
        assert any("unused" in f.message.lower() for f in findings)
