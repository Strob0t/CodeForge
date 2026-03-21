"""Tests for memory tenant isolation (FIX-035).

Verifies:
- MemoryStore.recall() SQL contains tenant_id filter
- MemoryRecallRequest has tenant_id field
- MemoryStoreRequest has tenant_id field
"""

from __future__ import annotations

import inspect

from codeforge.memory.models import MemoryRecallRequest, MemoryStoreRequest
from codeforge.memory.storage import MemoryStore


class TestMemoryTenantIsolation:
    """FIX-035: Verify memory operations enforce tenant isolation."""

    def test_recall_sql_contains_tenant_id(self) -> None:
        """The recall query MUST filter by tenant_id."""
        source = inspect.getsource(MemoryStore.recall)
        assert "tenant_id" in source, "recall() must filter by tenant_id in SQL"

    def test_recall_sql_uses_parameterized_tenant_id(self) -> None:
        """The recall query must use parameterized queries, not string interpolation."""
        source = inspect.getsource(MemoryStore.recall)
        # Must use %s placeholders (psycopg style), not f-strings or .format()
        assert "WHERE tenant_id = %s" in source, "recall() must use parameterized tenant_id filtering"

    def test_store_sql_contains_tenant_id(self) -> None:
        """The store query MUST include tenant_id."""
        source = inspect.getsource(MemoryStore.store)
        assert "tenant_id" in source, "store() must include tenant_id in INSERT"

    def test_recall_request_has_tenant_id_field(self) -> None:
        """MemoryRecallRequest must have tenant_id field."""
        req = MemoryRecallRequest(
            tenant_id="tenant-1",
            project_id="proj-1",
            query="test",
        )
        assert req.tenant_id == "tenant-1"

    def test_store_request_has_tenant_id_field(self) -> None:
        """MemoryStoreRequest must have tenant_id field."""
        req = MemoryStoreRequest(
            tenant_id="tenant-2",
            project_id="proj-2",
            content="test memory",
        )
        assert req.tenant_id == "tenant-2"

    def test_recall_request_default_tenant_id(self) -> None:
        """MemoryRecallRequest should have a default tenant_id (not empty)."""
        req = MemoryRecallRequest(project_id="proj-1", query="test")
        assert len(req.tenant_id) > 0, "Default tenant_id must not be empty"

    def test_store_request_default_tenant_id(self) -> None:
        """MemoryStoreRequest should have a default tenant_id (not empty)."""
        req = MemoryStoreRequest(project_id="proj-1", content="test")
        assert len(req.tenant_id) > 0, "Default tenant_id must not be empty"
