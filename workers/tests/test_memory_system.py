"""Tests for the memory subsystem: CompositeScorer and ExperiencePool."""

from __future__ import annotations

import uuid
from datetime import UTC, datetime, timedelta
from unittest.mock import AsyncMock, MagicMock, patch

import numpy as np
import pytest

from codeforge.memory.experience import ExperiencePool, exp_cache
from codeforge.memory.models import ScoreWeights
from codeforge.memory.scorer import CompositeScorer, _cosine_similarity

# ---------------------------------------------------------------------------
# 1. CompositeScorer tests
# ---------------------------------------------------------------------------


class TestCosineSimHelper:
    """Low-level tests for the _cosine_similarity utility."""

    def test_identical_vectors(self) -> None:
        v = np.array([1.0, 2.0, 3.0])
        assert _cosine_similarity(v, v) == pytest.approx(1.0)

    def test_orthogonal_vectors(self) -> None:
        a = np.array([1.0, 0.0, 0.0])
        b = np.array([0.0, 1.0, 0.0])
        assert _cosine_similarity(a, b) == pytest.approx(0.0)

    def test_opposite_vectors(self) -> None:
        a = np.array([1.0, 0.0])
        b = np.array([-1.0, 0.0])
        assert _cosine_similarity(a, b) == pytest.approx(-1.0)

    def test_zero_vector_returns_zero(self) -> None:
        a = np.array([0.0, 0.0, 0.0])
        b = np.array([1.0, 2.0, 3.0])
        assert _cosine_similarity(a, b) == 0.0


class TestCompositeScorer:
    """Tests for the CompositeScorer's score() method."""

    def test_identical_vectors_high_semantic_score(self) -> None:
        scorer = CompositeScorer()
        v = np.array([1.0, 0.0, 0.0])
        now = datetime.now(UTC)
        score = scorer.score(v, v, created_at=now, importance=0.5)
        # semantic=1.0, recency~1.0 (just created), importance=0.5
        # expected ~ 0.5*1.0 + 0.3*1.0 + 0.2*0.5 = 0.9
        assert score == pytest.approx(0.9, abs=0.01)

    def test_orthogonal_vectors_zero_semantic(self) -> None:
        scorer = CompositeScorer()
        a = np.array([1.0, 0.0, 0.0])
        b = np.array([0.0, 1.0, 0.0])
        now = datetime.now(UTC)
        score = scorer.score(a, b, created_at=now, importance=0.5)
        # semantic=0.0, recency~1.0, importance=0.5
        # expected ~ 0.5*0.0 + 0.3*1.0 + 0.2*0.5 = 0.4
        assert score == pytest.approx(0.4, abs=0.01)

    def test_recent_memory_high_recency(self) -> None:
        scorer = CompositeScorer()
        v = np.array([1.0, 0.0])
        just_now = datetime.now(UTC) - timedelta(seconds=1)
        score = scorer.score(v, v, created_at=just_now, importance=0.0)
        # semantic=1.0, recency~1.0, importance=0.0
        # expected ~ 0.5*1.0 + 0.3*1.0 + 0.2*0.0 = 0.8
        assert score == pytest.approx(0.8, abs=0.01)

    def test_old_memory_low_recency(self) -> None:
        scorer = CompositeScorer()
        v = np.array([1.0, 0.0])
        # 30 days ago with default half_life=168h (7 days) -> ~4.3 half-lives -> recency ~0.05
        old = datetime.now(UTC) - timedelta(days=30)
        score = scorer.score(v, v, created_at=old, importance=0.0)
        # semantic=1.0 * 0.5 = 0.5, recency << 1.0
        assert score < 0.55  # recency contribution should be negligible

    def test_default_weights_sum_to_one(self) -> None:
        w = ScoreWeights()
        assert w.semantic + w.recency + w.importance == pytest.approx(1.0)

    def test_zero_weights_zero_score(self) -> None:
        scorer = CompositeScorer(weights=ScoreWeights(semantic=0.0, recency=0.0, importance=0.0))
        v = np.array([1.0, 1.0, 1.0])
        now = datetime.now(UTC)
        score = scorer.score(v, v, created_at=now, importance=1.0)
        assert score == pytest.approx(0.0)

    def test_only_importance_weight(self) -> None:
        scorer = CompositeScorer(weights=ScoreWeights(semantic=0.0, recency=0.0, importance=1.0))
        v = np.array([1.0, 0.0])
        now = datetime.now(UTC)
        score = scorer.score(v, v, created_at=now, importance=0.7)
        assert score == pytest.approx(0.7)

    def test_only_semantic_weight(self) -> None:
        scorer = CompositeScorer(weights=ScoreWeights(semantic=1.0, recency=0.0, importance=0.0))
        a = np.array([1.0, 0.0, 0.0])
        b = np.array([0.0, 1.0, 0.0])
        now = datetime.now(UTC)
        score = scorer.score(a, b, created_at=now, importance=1.0)
        assert score == pytest.approx(0.0)

    def test_custom_half_life_faster_decay(self) -> None:
        """A shorter half-life should produce lower recency for the same age."""
        fast_scorer = CompositeScorer(half_life_hours=1.0)
        slow_scorer = CompositeScorer(half_life_hours=168.0)
        v = np.array([1.0])
        created = datetime.now(UTC) - timedelta(hours=2)

        fast_score = fast_scorer.score(v, v, created_at=created, importance=0.0)
        slow_score = slow_scorer.score(v, v, created_at=created, importance=0.0)

        # fast decay scorer should have a lower total score (recency decays faster)
        assert fast_score < slow_score

    def test_half_life_decay_math(self) -> None:
        """After exactly one half-life, recency should be ~0.5."""
        half_life = 24.0  # hours
        scorer = CompositeScorer(
            weights=ScoreWeights(semantic=0.0, recency=1.0, importance=0.0),
            half_life_hours=half_life,
        )
        v = np.array([1.0])
        created = datetime.now(UTC) - timedelta(hours=half_life)
        score = scorer.score(v, v, created_at=created, importance=0.0)
        assert score == pytest.approx(0.5, abs=0.02)

    def test_importance_clamped_by_model(self) -> None:
        """ScoreWeights importance is always weighted; importance value range is 0..1 per the model."""
        scorer = CompositeScorer(weights=ScoreWeights(semantic=0.0, recency=0.0, importance=1.0))
        v = np.array([1.0])
        now = datetime.now(UTC)
        assert scorer.score(v, v, created_at=now, importance=0.0) == pytest.approx(0.0)
        assert scorer.score(v, v, created_at=now, importance=1.0) == pytest.approx(1.0)


# ---------------------------------------------------------------------------
# 2. ExperiencePool tests
# ---------------------------------------------------------------------------


def _make_pool(
    llm_mock: MagicMock | None = None,
    scorer: CompositeScorer | None = None,
    threshold: float = 0.85,
) -> ExperiencePool:
    """Build an ExperiencePool with mocked dependencies."""
    llm = llm_mock or MagicMock()
    return ExperiencePool(
        db_url="postgresql://test:test@localhost:5432/test",
        llm=llm,
        scorer=scorer,
        confidence_threshold=threshold,
    )


class TestExperiencePoolLookup:
    """Tests for ExperiencePool.lookup."""

    async def test_lookup_no_entries_returns_none(self) -> None:
        llm = MagicMock()
        llm.embedding = AsyncMock(return_value=[1.0, 0.0, 0.0])
        pool = _make_pool(llm_mock=llm)

        mock_cursor = MagicMock()
        mock_cursor.execute = AsyncMock()
        mock_cursor.fetchall = AsyncMock(return_value=[])
        mock_cursor.__aenter__ = AsyncMock(return_value=mock_cursor)
        mock_cursor.__aexit__ = AsyncMock(return_value=False)

        mock_conn = MagicMock()
        mock_conn.cursor = MagicMock(return_value=mock_cursor)
        mock_conn.__aenter__ = AsyncMock(return_value=mock_conn)
        mock_conn.__aexit__ = AsyncMock(return_value=False)

        with patch("psycopg.AsyncConnection.connect", new=AsyncMock(return_value=mock_conn)):
            result = await pool.lookup("build a REST API", "proj-1")

        assert result is None

    async def test_lookup_embedding_failure_returns_none(self) -> None:
        llm = MagicMock()
        llm.embedding = AsyncMock(side_effect=Exception("embedding error"))
        pool = _make_pool(llm_mock=llm)

        result = await pool.lookup("something", "proj-1")
        assert result is None

    async def test_lookup_below_threshold_returns_none(self) -> None:
        """If no entry exceeds the threshold, lookup returns None."""
        llm = MagicMock()
        # Return an embedding orthogonal to all stored ones
        llm.embedding = AsyncMock(return_value=[1.0, 0.0, 0.0])
        pool = _make_pool(llm_mock=llm, threshold=0.99)

        # Stored entry has an orthogonal embedding
        entry_emb = np.array([0.0, 1.0, 0.0], dtype=np.float32).tobytes()
        fake_row = (
            uuid.uuid4(),  # id
            "other task",  # task_description
            entry_emb,  # task_embedding
            "output",  # result_output
            0.01,  # result_cost
            "success",  # result_status
            0.9,  # confidence
            datetime.now(UTC),  # created_at
        )

        mock_cursor = MagicMock()
        mock_cursor.execute = AsyncMock()
        mock_cursor.fetchall = AsyncMock(return_value=[fake_row])
        mock_cursor.__aenter__ = AsyncMock(return_value=mock_cursor)
        mock_cursor.__aexit__ = AsyncMock(return_value=False)

        mock_conn = MagicMock()
        mock_conn.cursor = MagicMock(return_value=mock_cursor)
        mock_conn.__aenter__ = AsyncMock(return_value=mock_conn)
        mock_conn.__aexit__ = AsyncMock(return_value=False)

        with patch("psycopg.AsyncConnection.connect", new=AsyncMock(return_value=mock_conn)):
            result = await pool.lookup("build a REST API", "proj-1")

        assert result is None


class TestExperiencePoolStore:
    """Tests for ExperiencePool.store."""

    async def test_store_returns_entry_id(self) -> None:
        entry_id = uuid.uuid4()
        llm = MagicMock()
        llm.embedding = AsyncMock(return_value=[0.1, 0.2, 0.3])
        pool = _make_pool(llm_mock=llm)

        mock_cursor = MagicMock()
        mock_cursor.execute = AsyncMock()
        mock_cursor.fetchone = AsyncMock(return_value=(entry_id,))
        mock_cursor.__aenter__ = AsyncMock(return_value=mock_cursor)
        mock_cursor.__aexit__ = AsyncMock(return_value=False)

        mock_conn = MagicMock()
        mock_conn.cursor = MagicMock(return_value=mock_cursor)
        mock_conn.commit = AsyncMock()
        mock_conn.__aenter__ = AsyncMock(return_value=mock_conn)
        mock_conn.__aexit__ = AsyncMock(return_value=False)

        with patch("psycopg.AsyncConnection.connect", new=AsyncMock(return_value=mock_conn)):
            result = await pool.store(
                task_desc="build a cache",
                project_id="proj-1",
                result_output="implemented LRU",
                result_cost=0.05,
                result_status="success",
                run_id="run-1",
            )

        assert result == str(entry_id)
        # Verify INSERT was called
        mock_cursor.execute.assert_called_once()
        sql_arg = mock_cursor.execute.call_args[0][0]
        assert "INSERT INTO experience_entries" in sql_arg

    async def test_store_with_embedding_failure_stores_null(self) -> None:
        """When embedding fails, store should still succeed with null embedding."""
        entry_id = uuid.uuid4()
        llm = MagicMock()
        llm.embedding = AsyncMock(side_effect=Exception("embedding unavailable"))
        pool = _make_pool(llm_mock=llm)

        mock_cursor = MagicMock()
        mock_cursor.execute = AsyncMock()
        mock_cursor.fetchone = AsyncMock(return_value=(entry_id,))
        mock_cursor.__aenter__ = AsyncMock(return_value=mock_cursor)
        mock_cursor.__aexit__ = AsyncMock(return_value=False)

        mock_conn = MagicMock()
        mock_conn.cursor = MagicMock(return_value=mock_cursor)
        mock_conn.commit = AsyncMock()
        mock_conn.__aenter__ = AsyncMock(return_value=mock_conn)
        mock_conn.__aexit__ = AsyncMock(return_value=False)

        with patch("psycopg.AsyncConnection.connect", new=AsyncMock(return_value=mock_conn)):
            result = await pool.store(
                task_desc="test",
                project_id="proj-1",
                result_output="output",
                result_cost=0.0,
                result_status="success",
                run_id="run-x",
            )

        assert result == str(entry_id)
        # Check that None was passed for embedding_bytes
        insert_params = mock_cursor.execute.call_args[0][1]
        assert insert_params[3] is None  # embedding_bytes position


class TestExperiencePoolInvalidate:
    """Tests for ExperiencePool.invalidate."""

    async def test_invalidate_executes_delete(self) -> None:
        pool = _make_pool()

        mock_cursor = MagicMock()
        mock_cursor.execute = AsyncMock()
        mock_cursor.__aenter__ = AsyncMock(return_value=mock_cursor)
        mock_cursor.__aexit__ = AsyncMock(return_value=False)

        mock_conn = MagicMock()
        mock_conn.cursor = MagicMock(return_value=mock_cursor)
        mock_conn.commit = AsyncMock()
        mock_conn.__aenter__ = AsyncMock(return_value=mock_conn)
        mock_conn.__aexit__ = AsyncMock(return_value=False)

        with patch("psycopg.AsyncConnection.connect", new=AsyncMock(return_value=mock_conn)):
            await pool.invalidate("entry-123")

        mock_cursor.execute.assert_called_once()
        sql_arg = mock_cursor.execute.call_args[0][0]
        assert "DELETE FROM experience_entries" in sql_arg


# ---------------------------------------------------------------------------
# 3. exp_cache decorator tests
# ---------------------------------------------------------------------------


class TestExpCacheDecorator:
    """Tests for the @exp_cache decorator."""

    async def test_cache_miss_executes_function(self) -> None:
        llm = MagicMock()
        llm.embedding = AsyncMock(return_value=[1.0, 0.0])
        pool = _make_pool(llm_mock=llm)

        # Mock lookup to return None (cache miss)
        pool.lookup = AsyncMock(return_value=None)
        pool.store = AsyncMock(return_value="new-entry-id")

        call_count = 0

        @exp_cache(pool, project_id_arg="project_id", task_desc_arg="task_desc")
        async def run_task(project_id: str = "", task_desc: str = "") -> str:
            nonlocal call_count
            call_count += 1
            return "computed result"

        result = await run_task(project_id="proj-1", task_desc="build API")

        assert result == "computed result"
        assert call_count == 1
        pool.lookup.assert_called_once_with("build API", "proj-1")
        pool.store.assert_called_once()

    async def test_cache_hit_skips_function(self) -> None:
        llm = MagicMock()
        pool = _make_pool(llm_mock=llm)

        # Mock lookup to return a cached entry
        pool.lookup = AsyncMock(
            return_value={
                "id": "cached-id",
                "task_description": "build API",
                "result_output": "cached output",
                "similarity": 0.95,
            }
        )

        call_count = 0

        @exp_cache(pool, project_id_arg="project_id", task_desc_arg="task_desc")
        async def run_task(project_id: str = "", task_desc: str = "") -> str:
            nonlocal call_count
            call_count += 1
            return "this should NOT be called"

        result = await run_task(project_id="proj-1", task_desc="build API")

        assert result == "cached output"
        assert call_count == 0  # function was NOT called
        pool.lookup.assert_called_once()

    async def test_cache_skipped_when_no_project_id(self) -> None:
        """When project_id is empty, cache should be bypassed entirely."""
        pool = _make_pool()
        pool.lookup = AsyncMock()
        pool.store = AsyncMock()

        @exp_cache(pool, project_id_arg="project_id", task_desc_arg="task_desc")
        async def run_task(project_id: str = "", task_desc: str = "") -> str:
            return "direct result"

        result = await run_task(project_id="", task_desc="build API")

        assert result == "direct result"
        pool.lookup.assert_not_called()

    async def test_cache_skipped_when_no_task_desc(self) -> None:
        """When task_desc is empty, cache should be bypassed entirely."""
        pool = _make_pool()
        pool.lookup = AsyncMock()
        pool.store = AsyncMock()

        @exp_cache(pool, project_id_arg="project_id", task_desc_arg="task_desc")
        async def run_task(project_id: str = "", task_desc: str = "") -> str:
            return "direct result"

        result = await run_task(project_id="proj-1", task_desc="")

        assert result == "direct result"
        pool.lookup.assert_not_called()

    async def test_cache_does_not_store_falsy_result(self) -> None:
        """When the function returns a falsy value, nothing should be stored."""
        pool = _make_pool()
        pool.lookup = AsyncMock(return_value=None)
        pool.store = AsyncMock()

        @exp_cache(pool, project_id_arg="project_id", task_desc_arg="task_desc")
        async def run_task(project_id: str = "", task_desc: str = "") -> str:
            return ""

        result = await run_task(project_id="proj-1", task_desc="build API")

        assert result == ""
        pool.store.assert_not_called()
