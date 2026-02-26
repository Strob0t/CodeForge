"""Tests for GEMMAS-inspired collaboration metrics (IDS + UPR)."""

from __future__ import annotations

from codeforge.evaluation.collaboration import InformationDiversityScore, UnnecessaryPathRatio
from codeforge.evaluation.dag_builder import AgentMessage, CollaborationDAG


def _make_dag(messages: list[dict]) -> CollaborationDAG:
    """Helper to build a DAG from raw dicts."""
    parsed = [AgentMessage.model_validate(m) for m in messages]
    return CollaborationDAG(parsed)


class TestInformationDiversityScore:
    def test_single_agent_returns_1(self) -> None:
        dag = _make_dag(
            [
                {"agent_id": "a", "content": "hello world", "round": 1},
            ]
        )
        ids = InformationDiversityScore()
        assert ids.compute(dag) == 1.0

    def test_diverse_agents_high_score(self) -> None:
        dag = _make_dag(
            [
                {"agent_id": "coder", "content": "implement the sorting algorithm with quicksort", "round": 1},
                {
                    "agent_id": "reviewer",
                    "content": "check security vulnerabilities and SQL injection risks",
                    "round": 2,
                    "parent_agent_id": "coder",
                },
            ]
        )
        ids = InformationDiversityScore()
        score = ids.compute(dag)
        assert 0.0 <= score <= 1.0
        # Diverse content should yield a higher score
        assert score > 0.3

    def test_redundant_agents_lower_score(self) -> None:
        dag = _make_dag(
            [
                {"agent_id": "a", "content": "implement the sorting algorithm", "round": 1},
                {
                    "agent_id": "b",
                    "content": "implement the sorting algorithm",
                    "round": 2,
                    "parent_agent_id": "a",
                },
            ]
        )
        ids = InformationDiversityScore()
        score = ids.compute(dag)
        # Identical content should yield low diversity
        assert score < 0.3

    def test_no_connected_pairs(self) -> None:
        dag = _make_dag(
            [
                {"agent_id": "a", "content": "hello", "round": 1},
                {"agent_id": "b", "content": "world", "round": 1},
            ]
        )
        ids = InformationDiversityScore()
        # No edges = no connected pairs = trivially diverse
        assert ids.compute(dag) == 1.0

    def test_ids_with_embedding_function(self) -> None:
        """Test IDS with a mock embedding function producing orthogonal vectors."""
        dag = _make_dag(
            [
                {"agent_id": "coder", "content": "implement sorting", "round": 1},
                {
                    "agent_id": "reviewer",
                    "content": "check security",
                    "round": 2,
                    "parent_agent_id": "coder",
                },
            ]
        )

        # Orthogonal embeddings -> high diversity
        def mock_embed(texts: list[str]) -> list[list[float]]:
            return [[1.0, 0.0, 0.0], [0.0, 1.0, 0.0]]

        ids = InformationDiversityScore(embed_fn=mock_embed)
        score = ids.compute(dag)
        assert 0.0 <= score <= 1.0
        assert score > 0.5  # Orthogonal vectors -> high diversity

    def test_ids_embedding_fallback_on_error(self) -> None:
        """Test IDS falls back to TF-IDF when embedding function raises."""
        dag = _make_dag(
            [
                {"agent_id": "coder", "content": "implement the sorting algorithm with quicksort", "round": 1},
                {
                    "agent_id": "reviewer",
                    "content": "check security vulnerabilities and SQL injection risks",
                    "round": 2,
                    "parent_agent_id": "coder",
                },
            ]
        )

        def broken_embed(texts: list[str]) -> list[list[float]]:
            raise RuntimeError("embedding service unavailable")

        # With broken embed_fn, should fall back to TF-IDF
        ids_with_broken = InformationDiversityScore(embed_fn=broken_embed)
        score_broken = ids_with_broken.compute(dag)

        # Without embed_fn (pure TF-IDF)
        ids_tfidf = InformationDiversityScore()
        score_tfidf = ids_tfidf.compute(dag)

        # Both should produce the same result since broken falls back to TF-IDF
        assert abs(score_broken - score_tfidf) < 0.01


class TestUnnecessaryPathRatio:
    def test_no_scores_returns_zero(self) -> None:
        dag = _make_dag(
            [
                {"agent_id": "a", "content": "hello", "round": 1},
                {"agent_id": "b", "content": "world", "round": 2, "parent_agent_id": "a"},
            ]
        )
        upr = UnnecessaryPathRatio()
        assert upr.compute(dag) == 0.0

    def test_all_necessary_paths(self) -> None:
        dag = _make_dag(
            [
                {"agent_id": "a", "content": "plan", "round": 1},
                {"agent_id": "b", "content": "implement", "round": 2, "parent_agent_id": "a"},
            ]
        )
        paths = dag.enumerate_paths()
        scores = dict.fromkeys(paths, 0.8)
        upr = UnnecessaryPathRatio()
        assert upr.compute(dag, path_scores=scores) == 0.0

    def test_mixed_paths(self) -> None:
        dag = _make_dag(
            [
                {"agent_id": "a", "content": "plan", "round": 1},
                {"agent_id": "b", "content": "code", "round": 2, "parent_agent_id": "a"},
                {"agent_id": "c", "content": "review", "round": 2, "parent_agent_id": "a"},
            ]
        )
        paths = dag.enumerate_paths()
        # Make one path necessary and one unnecessary
        scores = {}
        for i, p in enumerate(paths):
            scores[p] = 0.8 if i == 0 else 0.2
        upr = UnnecessaryPathRatio()
        result = upr.compute(dag, path_scores=scores)
        assert 0.0 < result < 1.0


class TestDAGBuilder:
    def test_spatial_adjacency(self) -> None:
        dag = _make_dag(
            [
                {"agent_id": "a", "content": "x", "round": 1},
                {"agent_id": "b", "content": "y", "round": 2, "parent_agent_id": "a"},
            ]
        )
        adj = dag.spatial_adjacency()
        assert adj[0][1] == 1.0
        assert adj[1][0] == 1.0

    def test_temporal_adjacency(self) -> None:
        dag = _make_dag(
            [
                {"agent_id": "a", "content": "x", "round": 1},
                {"agent_id": "b", "content": "y", "round": 2},
            ]
        )
        temp = dag.temporal_adjacency()
        assert temp[0][1] > 0

    def test_enumerate_paths(self) -> None:
        dag = _make_dag(
            [
                {"agent_id": "root", "content": "start", "round": 1},
                {"agent_id": "mid", "content": "process", "round": 2, "parent_agent_id": "root"},
                {"agent_id": "leaf", "content": "done", "round": 3, "parent_agent_id": "mid"},
            ]
        )
        paths = dag.enumerate_paths()
        assert len(paths) == 1
        assert "root -> mid -> leaf" in paths[0]
