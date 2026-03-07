#!/usr/bin/env python3
"""Automated Database Schema Audit for CodeForge PostgreSQL migrations.

Standalone CLI tool. No new dependencies — uses stdlib for static mode,
psycopg (already installed) for live mode.

Usage:
    python tools/db_schema_audit.py                                     # Static, JSON
    python tools/db_schema_audit.py --live                              # + live DB stats
    python tools/db_schema_audit.py --format markdown                   # Markdown output
    python tools/db_schema_audit.py --go-stores internal/adapter/postgres/
    python tools/db_schema_audit.py --threshold 60                      # Exit 1 if score < 60
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from dataclasses import asdict, dataclass, field
from pathlib import Path

# ---------------------------------------------------------------------------
# Data Model
# ---------------------------------------------------------------------------


@dataclass
class Column:
    name: str
    data_type: str
    nullable: bool = True
    default_value: str | None = None
    is_pk: bool = False
    is_fk: bool = False
    references_table: str | None = None
    has_check: bool = False


@dataclass
class Index:
    name: str
    table_name: str
    columns: list[str]
    is_unique: bool = False
    is_partial: bool = False
    condition: str = ""


@dataclass
class Table:
    name: str
    columns: list[Column] = field(default_factory=list)
    indexes: list[Index] = field(default_factory=list)
    has_tenant_id: bool = False
    has_updated_at: bool = False
    has_updated_at_trigger: bool = False
    pk_type: str = ""
    source_migration: str = ""


@dataclass
class Migration:
    number: int
    name: str
    has_up: bool = False
    has_down: bool = False


@dataclass
class Finding:
    category: str
    severity: str  # critical, high, medium, low
    table: str
    message: str
    sql_patch: str = ""


# ---------------------------------------------------------------------------
# Severity deductions
# ---------------------------------------------------------------------------

_SEVERITY_DEDUCTION = {"critical": 5, "high": 3, "medium": 2, "low": 1}


# ---------------------------------------------------------------------------
# Parser — regex-based for goose DDL migrations
# ---------------------------------------------------------------------------

_RE_CREATE_TABLE = re.compile(
    r"CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(\w+)\s*\((.*?)\);",
    re.IGNORECASE | re.DOTALL,
)

_RE_CREATE_INDEX = re.compile(
    r"CREATE\s+(?:(UNIQUE)\s+)?INDEX\s+(?:CONCURRENTLY\s+)?(?:IF\s+NOT\s+EXISTS\s+)?(\w+)"
    r"\s+ON\s+(\w+)\s*\(([^)]+)\)"
    r"(?:\s+WHERE\s+(.+?))?;",
    re.IGNORECASE | re.DOTALL,
)

_RE_ALTER_ADD_COLUMN = re.compile(
    r"ALTER\s+TABLE\s+(\w+)\s+ADD\s+COLUMN\s+(?:IF\s+NOT\s+EXISTS\s+)?(\w+)\s+(\w+)",
    re.IGNORECASE,
)

_RE_TRIGGER = re.compile(
    r"CREATE\s+(?:OR\s+REPLACE\s+)?TRIGGER\s+\w+\s+"
    r"BEFORE\s+UPDATE\s+ON\s+(\w+)\s+"
    r"FOR\s+EACH\s+ROW\s+EXECUTE\s+FUNCTION\s+update_updated_at\(\)",
    re.IGNORECASE | re.DOTALL,
)

_RE_REFERENCES = re.compile(
    r"REFERENCES\s+(\w+)\s*\(",
    re.IGNORECASE,
)

_RE_CHECK = re.compile(
    r"CHECK\s*\(",
    re.IGNORECASE,
)

_RE_GOOSE_UP = re.compile(r"^--\s*\+goose\s+Up", re.MULTILINE | re.IGNORECASE)
_RE_GOOSE_DOWN = re.compile(r"^--\s*\+goose\s+Down", re.MULTILINE | re.IGNORECASE)


def _parse_column_def(col_text: str) -> Column | None:
    """Parse a single column definition line from CREATE TABLE body."""
    col_text = col_text.strip().rstrip(",")
    if not col_text:
        return None
    # Skip table-level constraints
    upper = col_text.upper().lstrip()
    if upper.startswith(("CONSTRAINT", "PRIMARY KEY", "FOREIGN KEY", "UNIQUE", "CHECK")):
        return None

    parts = col_text.split()
    if len(parts) < 2:
        return None

    name = parts[0].strip('"')
    data_type = parts[1].upper()
    # Handle types like VARCHAR(255)
    if "(" in parts[1] and ")" not in parts[1] and len(parts) > 2:
        data_type = (parts[1] + parts[2]).upper()
    elif "(" in col_text.split(None, 2)[1] if len(col_text.split(None, 2)) > 1 else False:
        raw = col_text.split(None, 2)[1]
        paren_end = raw.find(")")
        if paren_end >= 0:
            data_type = raw[: paren_end + 1].upper()

    # Normalize VARCHAR(N) to VARCHAR
    if data_type.startswith("VARCHAR"):
        data_type = "VARCHAR"

    col_upper = col_text.upper()
    is_pk = "PRIMARY KEY" in col_upper
    nullable = "NOT NULL" not in col_upper or is_pk
    is_fk = bool(_RE_REFERENCES.search(col_text))
    references_table = None
    if is_fk:
        m = _RE_REFERENCES.search(col_text)
        if m:
            references_table = m.group(1)
    has_check = bool(_RE_CHECK.search(col_text))

    default_value = None
    default_match = re.search(
        r"DEFAULT\s+(.+?)(?:\s+(?:NOT|NULL|CHECK|REFERENCES|PRIMARY|UNIQUE|$))", col_text, re.IGNORECASE
    )
    if default_match:
        default_value = default_match.group(1).strip().rstrip(",")

    return Column(
        name=name,
        data_type=data_type,
        nullable=nullable,
        default_value=default_value,
        is_pk=is_pk,
        is_fk=is_fk,
        references_table=references_table,
        has_check=has_check,
    )


def _parse_table_body(body: str) -> list[Column]:
    """Parse column definitions from CREATE TABLE body."""
    columns: list[Column] = []
    # Split on commas at depth 0, handling nested parentheses
    lines: list[str] = []
    current = ""
    depth = 0
    for ch in body:
        if ch == "(":
            depth += 1
            current += ch
        elif ch == ")":
            depth -= 1
            current += ch
        elif ch == "," and depth == 0:
            lines.append(current.strip())
            current = ""
        else:
            current += ch
    if current.strip():
        lines.append(current.strip())

    for line in lines:
        col = _parse_column_def(line)
        if col:
            columns.append(col)
    return columns


def _parse_tables_from_content(content: str, filename: str, tables_by_name: dict[str, Table]) -> None:
    """Extract CREATE TABLE definitions from migration content."""
    for match in _RE_CREATE_TABLE.finditer(content):
        table_name = match.group(1)
        body = match.group(2)
        columns = _parse_table_body(body)

        pk_type = ""
        has_tenant = False
        has_updated = False
        for col in columns:
            if col.is_pk:
                pk_type = col.data_type
            if col.name == "tenant_id":
                has_tenant = True
            if col.name == "updated_at":
                has_updated = True

        table = Table(
            name=table_name,
            columns=columns,
            has_tenant_id=has_tenant,
            has_updated_at=has_updated,
            pk_type=pk_type,
            source_migration=filename,
        )
        tables_by_name[table_name] = table


def _parse_indexes_from_content(content: str, tables_by_name: dict[str, Table], all_indexes: list[Index]) -> None:
    """Extract CREATE INDEX definitions from migration content."""
    for match in _RE_CREATE_INDEX.finditer(content):
        is_unique = match.group(1) is not None
        idx_name = match.group(2)
        tbl_name = match.group(3)
        cols_raw = match.group(4)
        condition = (match.group(5) or "").strip()

        columns = [c.strip().split()[0].strip('"') for c in cols_raw.split(",")]

        idx = Index(
            name=idx_name,
            table_name=tbl_name,
            columns=columns,
            is_unique=is_unique,
            is_partial=bool(condition),
            condition=condition,
        )
        all_indexes.append(idx)

        if tbl_name in tables_by_name:
            tables_by_name[tbl_name].indexes.append(idx)


def _parse_alters_from_content(content: str, tables_by_name: dict[str, Table]) -> None:
    """Extract ALTER TABLE ADD COLUMN and trigger definitions."""
    for match in _RE_ALTER_ADD_COLUMN.finditer(content):
        tbl_name = match.group(1)
        col_name = match.group(2)
        col_type = match.group(3).upper()

        if tbl_name in tables_by_name:
            table = tables_by_name[tbl_name]
            if not any(c.name == col_name for c in table.columns):
                line_match = re.search(
                    rf"ALTER\s+TABLE\s+{re.escape(tbl_name)}\s+ADD\s+COLUMN\s+(?:IF\s+NOT\s+EXISTS\s+)?{re.escape(col_name)}\s+.+",
                    content,
                    re.IGNORECASE,
                )
                full_line = line_match.group(0) if line_match else ""
                is_fk = bool(_RE_REFERENCES.search(full_line))
                ref_match = _RE_REFERENCES.search(full_line)
                ref_table = ref_match.group(1) if ref_match else None

                col = Column(
                    name=col_name,
                    data_type=col_type,
                    is_fk=is_fk,
                    references_table=ref_table,
                )
                table.columns.append(col)
                if col_name == "tenant_id":
                    table.has_tenant_id = True
                if col_name == "updated_at":
                    table.has_updated_at = True

    for match in _RE_TRIGGER.finditer(content):
        tbl_name = match.group(1)
        if tbl_name in tables_by_name:
            tables_by_name[tbl_name].has_updated_at_trigger = True


def parse_migrations(mig_dir: Path) -> tuple[list[Table], list[Index], list[Migration]]:
    """Parse all .sql migration files in the given directory.

    Returns (tables, indexes, migrations).
    """
    if not mig_dir.exists():
        return [], [], []

    sql_files = sorted(mig_dir.glob("*.sql"))
    if not sql_files:
        return [], [], []

    tables_by_name: dict[str, Table] = {}
    all_indexes: list[Index] = []
    all_migrations: list[Migration] = []

    for sql_file in sql_files:
        content = sql_file.read_text()
        filename = sql_file.name

        num_match = re.match(r"(\d+)", filename)
        mig_number = int(num_match.group(1)) if num_match else 0

        has_up = bool(_RE_GOOSE_UP.search(content))
        has_down = bool(_RE_GOOSE_DOWN.search(content))

        all_migrations.append(
            Migration(
                number=mig_number,
                name=filename,
                has_up=has_up,
                has_down=has_down,
            )
        )

        _parse_tables_from_content(content, filename, tables_by_name)
        _parse_indexes_from_content(content, tables_by_name, all_indexes)
        _parse_alters_from_content(content, tables_by_name)

    return list(tables_by_name.values()), all_indexes, all_migrations


# ---------------------------------------------------------------------------
# Go Store Query Parser
# ---------------------------------------------------------------------------

_RE_SQL_QUERY = re.compile(
    r"(?:QueryRow|Query|Exec)\s*\(\s*ctx\s*,\s*`([^`]+)`",
    re.DOTALL,
)

_RE_SQL_TABLE = re.compile(
    r"(?:FROM|INTO|UPDATE|DELETE\s+FROM)\s+(\w+)",
    re.IGNORECASE,
)

_RE_SQL_WHERE = re.compile(
    r"WHERE\s+(.+?)(?:ORDER|LIMIT|GROUP|HAVING|RETURNING|ON\s+CONFLICT|$)",
    re.IGNORECASE | re.DOTALL,
)


@dataclass
class GoQuery:
    table: str
    where_columns: list[str]
    has_tenant_id: bool
    source_file: str
    raw_sql: str


def parse_go_store_queries(store_dir: Path) -> list[dict[str, object]]:
    """Parse SQL queries from Go store files."""
    if not store_dir.exists():
        return []

    queries: list[dict[str, object]] = []

    for go_file in sorted(store_dir.glob("store*.go")):
        if go_file.name.endswith("_test.go"):
            continue
        content = go_file.read_text()

        for match in _RE_SQL_QUERY.finditer(content):
            sql = match.group(1).strip()
            # Extract table name
            table_match = _RE_SQL_TABLE.search(sql)
            if not table_match:
                continue
            table_name = table_match.group(1)

            # Extract WHERE columns
            where_match = _RE_SQL_WHERE.search(sql)
            where_cols: list[str] = []
            has_tenant = False
            if where_match:
                where_clause = where_match.group(1)
                # Extract column names from WHERE (col = $N patterns)
                for col_match in re.finditer(r"(\w+)\s*=\s*\$\d+", where_clause):
                    col_name = col_match.group(1)
                    where_cols.append(col_name)
                    if col_name == "tenant_id":
                        has_tenant = True

            queries.append(
                {
                    "table": table_name,
                    "where_columns": where_cols,
                    "has_tenant_id": has_tenant,
                    "source_file": go_file.name,
                    "raw_sql": sql[:200],
                }
            )

    return queries


# ---------------------------------------------------------------------------
# Audit Engine — 10 categories
# ---------------------------------------------------------------------------

_STATUS_LIKE_NAMES = {"status", "state", "type", "kind", "role", "level", "tier", "phase"}


def audit_schema(
    tables: list[Table],
    indexes: list[Index],
    migrations: list[Migration],
    go_queries: list[dict[str, object]] | None = None,
) -> list[Finding]:
    """Run all audit checks and return findings."""
    findings: list[Finding] = []

    findings.extend(_check_multi_tenant_isolation(tables, go_queries))
    findings.extend(_check_index_strategy(tables, indexes))
    findings.extend(_check_data_type_consistency(tables))
    findings.extend(_check_naming_conventions(tables, indexes))
    findings.extend(_check_jsonb_usage(tables))
    findings.extend(_check_constraints(tables))
    findings.extend(_check_migration_quality(migrations))
    findings.extend(_check_normalization(tables))
    findings.extend(_check_partitioning(tables))

    return findings


def _check_multi_tenant_isolation(
    tables: list[Table],
    go_queries: list[dict[str, object]] | None = None,
) -> list[Finding]:
    """Category 9: Tables missing tenant_id, queries missing tenant_id."""
    tenant_exempt = {"tenants"}

    findings = [
        Finding(
            category="multi_tenant_isolation",
            severity="critical",
            table=table.name,
            message="Table lacks tenant_id column",
            sql_patch=(
                f"ALTER TABLE {table.name} ADD COLUMN tenant_id UUID NOT NULL "
                f"DEFAULT '00000000-0000-0000-0000-000000000000';\n"
                f"CREATE INDEX idx_{table.name}_tenant ON {table.name}(tenant_id);"
            ),
        )
        for table in tables
        if not table.has_tenant_id and table.name not in tenant_exempt
    ]

    if go_queries:
        findings.extend(
            Finding(
                category="multi_tenant_isolation",
                severity="high",
                table=str(q.get("table", "")),
                message=f"Go query on {q['table']} in {q['source_file']} missing tenant_id in WHERE clause",
            )
            for q in go_queries
            if not q.get("has_tenant_id") and q.get("where_columns")
        )

    return findings


def _check_index_strategy(tables: list[Table], indexes: list[Index]) -> list[Finding]:
    """Category 1: Missing FK indexes, redundant indexes, missing tenant prefix."""
    findings: list[Finding] = []

    # Build index lookup: table -> set of first-column of each index
    idx_by_table: dict[str, list[Index]] = {}
    for idx in indexes:
        idx_by_table.setdefault(idx.table_name, []).append(idx)

    for table in tables:
        table_indexes = idx_by_table.get(table.name, [])
        indexed_first_cols = {idx.columns[0] for idx in table_indexes if idx.columns}

        # Check FK columns without indexes
        findings.extend(
            Finding(
                category="index_strategy",
                severity="high",
                table=table.name,
                message=f"Missing index on FK column '{col.name}' (references {col.references_table})",
                sql_patch=f"CREATE INDEX idx_{table.name}_{col.name} ON {table.name}({col.name});",
            )
            for col in table.columns
            if col.is_fk and col.name not in indexed_first_cols
        )

        # Check redundant single-column indexes subsumed by composites
        for idx in table_indexes:
            if len(idx.columns) == 1 and not idx.is_unique:
                col = idx.columns[0]
                # Check if any composite index has this as its first column
                for other in table_indexes:
                    if other.name != idx.name and len(other.columns) > 1 and other.columns[0] == col:
                        findings.append(
                            Finding(
                                category="index_strategy",
                                severity="low",
                                table=table.name,
                                message=(
                                    f"Redundant index '{idx.name}' on ({col}) — "
                                    f"subsumed by composite index '{other.name}' "
                                    f"on ({', '.join(other.columns)})"
                                ),
                            )
                        )
                        break

    return findings


def _check_data_type_consistency(tables: list[Table]) -> list[Finding]:
    """Category 4: VARCHAR vs TEXT mixing, PK type inconsistency."""
    findings: list[Finding] = []

    # Check VARCHAR/TEXT mixing within a table
    findings.extend(
        Finding(
            category="data_type_consistency",
            severity="medium",
            table=table.name,
            message=(
                f"Mixed VARCHAR and TEXT types — columns "
                f"{[c.name for c in table.columns if c.data_type.startswith('VARCHAR')]} "
                f"use VARCHAR while others use TEXT. Prefer TEXT consistently."
            ),
        )
        for table in tables
        if any(c.data_type.startswith("VARCHAR") for c in table.columns)
        and any(c.data_type == "TEXT" and not c.is_pk for c in table.columns)
    )

    # Check PK type consistency across all tables
    pk_types: dict[str, list[str]] = {}
    for table in tables:
        if table.pk_type:
            pk_types.setdefault(table.pk_type, []).append(table.name)

    if len(pk_types) > 1:
        type_summary = ", ".join(f"{t}: {tbls}" for t, tbls in pk_types.items())
        findings.append(
            Finding(
                category="data_type_consistency",
                severity="medium",
                table="(global)",
                message=f"Inconsistent PK types across tables — {type_summary}",
            )
        )

    return findings


def _check_naming_conventions(tables: list[Table], indexes: list[Index]) -> list[Finding]:
    """Category 5: snake_case tables/columns, idx_ prefix, plural names."""
    findings: list[Finding] = []

    snake_case_re = re.compile(r"^[a-z][a-z0-9]*(_[a-z0-9]+)*$")

    findings.extend(
        Finding(
            category="naming_conventions",
            severity="low",
            table=table.name,
            message=f"Table name '{table.name}' does not follow snake_case convention",
        )
        for table in tables
        if not snake_case_re.match(table.name)
    )
    for table in tables:
        findings.extend(
            Finding(
                category="naming_conventions",
                severity="low",
                table=table.name,
                message=f"Column '{col.name}' does not follow snake_case convention",
            )
            for col in table.columns
            if not snake_case_re.match(col.name)
        )

    findings.extend(
        Finding(
            category="naming_conventions",
            severity="low",
            table=idx.table_name,
            message=f"Index '{idx.name}' does not follow idx_ prefix convention",
        )
        for idx in indexes
        if not idx.name.startswith("idx_")
    )

    return findings


def _check_jsonb_usage(tables: list[Table]) -> list[Finding]:
    """Category 6: Flag JSONB columns for AI review."""
    return [
        Finding(
            category="jsonb_usage",
            severity="low",
            table=table.name,
            message=f"JSONB column '{col.name}' — review whether relational modeling would be more appropriate",
        )
        for table in tables
        for col in table.columns
        if col.data_type == "JSONB"
    ]


def _check_constraints(tables: list[Table]) -> list[Finding]:
    """Category 3: Missing CHECK on status/type columns."""
    return [
        Finding(
            category="constraints",
            severity="medium",
            table=table.name,
            message=f"Column '{col.name}' looks like an enum but has no CHECK constraint",
        )
        for table in tables
        for col in table.columns
        if col.name in _STATUS_LIKE_NAMES and col.data_type == "TEXT" and not col.has_check
    ]


def _check_migration_quality(migrations: list[Migration]) -> list[Finding]:
    """Category 10: Up/Down consistency."""
    return [
        Finding(
            category="migration_quality",
            severity="medium",
            table="(migration)",
            message=f"Migration {mig.name} has Up but no Down section",
        )
        for mig in migrations
        if mig.has_up and not mig.has_down
    ]


def _check_normalization(tables: list[Table]) -> list[Finding]:
    """Category 2: JSONB columns with array defaults that might be separate tables."""
    return [
        Finding(
            category="normalization",
            severity="low",
            table=table.name,
            message=(
                f"JSONB column '{col.name}' defaults to '[]' — "
                f"consider a separate relational table instead of array-of-objects"
            ),
        )
        for table in tables
        for col in table.columns
        if col.data_type == "JSONB" and col.default_value and "[]" in str(col.default_value)
    ]


def _check_partitioning(tables: list[Table]) -> list[Finding]:
    """Category 7: Large event/audit tables without partitioning."""
    event_table_patterns = {"events", "audit", "log", "history", "trajectory"}

    return [
        Finding(
            category="partitioning",
            severity="low",
            table=table.name,
            message=(
                f"Event/audit table '{table.name}' may benefit from time-based partitioning for query performance"
            ),
        )
        for table in tables
        if any(p in table.name for p in event_table_patterns)
    ]


# ---------------------------------------------------------------------------
# Score Calculation
# ---------------------------------------------------------------------------


def calculate_score(findings: list[Finding]) -> int:
    """Calculate schema health score (0-100)."""
    score = 100
    for f in findings:
        score -= _SEVERITY_DEDUCTION.get(f.severity, 0)
    return max(0, score)


# ---------------------------------------------------------------------------
# Output Formatting
# ---------------------------------------------------------------------------


def format_output(
    findings: list[Finding],
    score: int,
    tables: list[Table],
    indexes: list[Index],
    migrations: list[Migration],
    fmt: str = "json",
) -> str:
    """Format audit results as JSON or Markdown."""
    summary = {"critical": 0, "high": 0, "medium": 0, "low": 0}
    for f in findings:
        if f.severity in summary:
            summary[f.severity] += 1

    if fmt == "json":
        result = {
            "score": score,
            "summary": summary,
            "findings": [{k: v for k, v in asdict(f).items() if v} for f in findings],
            "tables_analyzed": len(tables),
            "indexes_analyzed": len(indexes),
            "migrations_analyzed": len(migrations),
        }
        return json.dumps(result, indent=2)

    # Markdown format
    lines: list[str] = []
    lines.append("# Database Schema Audit")
    lines.append("")
    lines.append(f"**Score: {score}/100**")
    lines.append("")
    lines.append("## Summary")
    lines.append("")
    lines.append("| Severity | Count |")
    lines.append("|----------|-------|")
    for sev, count in summary.items():
        lines.append(f"| {sev.capitalize()} | {count} |")
    lines.append("")
    lines.append(f"- Tables analyzed: {len(tables)}")
    lines.append(f"- Indexes analyzed: {len(indexes)}")
    lines.append(f"- Migrations analyzed: {len(migrations)}")
    lines.append("")

    # Group findings by category
    by_category: dict[str, list[Finding]] = {}
    for f in findings:
        by_category.setdefault(f.category, []).append(f)

    for category, cat_findings in sorted(by_category.items()):
        lines.append(f"## {category.replace('_', ' ').title()}")
        lines.append("")
        for f in cat_findings:
            icon = {"critical": "[CRIT]", "high": "[HIGH]", "medium": "[MED]", "low": "[LOW]"}.get(f.severity, "")
            lines.append(f"- {icon} **{f.table}**: {f.message}")
            if f.sql_patch:
                lines.append("  ```sql")
                lines.append(f"  {f.sql_patch}")
                lines.append("  ```")
        lines.append("")

    return "\n".join(lines)


# ---------------------------------------------------------------------------
# Live Mode — psycopg queries against running DB
# ---------------------------------------------------------------------------


def get_live_findings(conn: object) -> list[Finding]:
    """Query live PostgreSQL for runtime statistics."""
    findings: list[Finding] = []

    with conn.cursor() as cur:  # type: ignore[union-attr]
        # Unused indexes
        cur.execute("""
            SELECT indexrelname, relname, idx_scan, pg_relation_size(indexrelid)
            FROM pg_stat_user_indexes
            WHERE idx_scan = 0
            AND indexrelname NOT LIKE 'pg_%'
            ORDER BY pg_relation_size(indexrelid) DESC
        """)
        for row in cur.fetchall():
            idx_name, table_name, _scan_count, size_bytes = row
            findings.append(
                Finding(
                    category="index_strategy",
                    severity="medium",
                    table=table_name,
                    message=(f"Unused index '{idx_name}' (0 scans, {size_bytes / 1024:.0f} KB) — consider dropping"),
                    sql_patch=f"DROP INDEX IF EXISTS {idx_name};",
                )
            )

    return findings


# ---------------------------------------------------------------------------
# CLI Entry Point
# ---------------------------------------------------------------------------


def main() -> int:
    parser = argparse.ArgumentParser(description="Database Schema Audit for CodeForge")
    parser.add_argument(
        "--migrations",
        default="internal/adapter/postgres/migrations",
        help="Path to migrations directory",
    )
    parser.add_argument("--go-stores", default=None, help="Path to Go store files")
    parser.add_argument("--format", choices=["json", "markdown"], default="json")
    parser.add_argument("--threshold", type=int, default=0, help="Min score (exit 1 if below)")
    parser.add_argument("--live", action="store_true", help="Include live DB stats")
    args = parser.parse_args()

    mig_path = Path(args.migrations)
    tables, indexes, migrations = parse_migrations(mig_path)

    go_queries = None
    if args.go_stores:
        go_queries = parse_go_store_queries(Path(args.go_stores))

    findings = audit_schema(tables, indexes, migrations, go_queries=go_queries)

    if args.live:
        try:
            import os

            import psycopg  # type: ignore[import-untyped]

            db_url = os.environ.get("DATABASE_URL", "")
            if db_url:
                with psycopg.connect(db_url) as conn:
                    findings.extend(get_live_findings(conn))
            else:
                print("Warning: --live requires DATABASE_URL env var", file=sys.stderr)
        except ImportError:
            print("Warning: --live requires psycopg package", file=sys.stderr)

    score = calculate_score(findings)
    output = format_output(findings, score, tables, indexes, migrations, fmt=args.format)
    print(output)

    if args.threshold and score < args.threshold:
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
