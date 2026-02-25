"""GEMMAS-inspired collaboration metrics for multi-agent evaluation.

Implements two core metrics from the GEMMAS paper (arxiv.org/abs/2507.13190):
- Information Diversity Score (IDS): measures diversity of agent contributions
- Unnecessary Path Ratio (UPR): measures efficiency of reasoning paths

No code was released with the paper, so this is a custom implementation
based on the metric definitions.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.metrics.pairwise import cosine_similarity

if TYPE_CHECKING:
    from codeforge.evaluation.dag_builder import CollaborationDAG


class InformationDiversityScore:
    """Measures information diversity across agent contributions.

    Higher scores (closer to 1.0) indicate more diverse contributions
    with less redundancy between agents. Uses TF-IDF cosine similarity
    as the base metric, with optional LiteLLM embedding enhancement.

    Algorithm:
    1. Build pairwise similarity matrix between all agent messages
    2. Weight by spatial adjacency (direct communication links)
    3. Compute IDS as weighted average of (1 - similarity)
    """

    def compute(self, dag: CollaborationDAG) -> float:
        """Compute the IDS score for a multi-agent collaboration DAG.

        Args:
            dag: Collaboration DAG containing agent messages and adjacency info.

        Returns:
            Score in [0, 1] where 1.0 means maximum diversity.
        """
        messages = dag.agent_messages
        if len(messages) < 2:
            return 1.0  # single agent = trivially diverse

        # Group messages by agent
        agents = sorted({m.agent_id for m in messages})
        if len(agents) < 2:
            return 1.0

        # Concatenate messages per agent
        agent_texts: dict[str, str] = {}
        for agent_id in agents:
            texts = [m.content for m in messages if m.agent_id == agent_id]
            agent_texts[agent_id] = " ".join(texts)

        # Compute TF-IDF similarity matrix
        vectorizer = TfidfVectorizer(max_features=5000, stop_words="english")
        corpus = [agent_texts[a] for a in agents]
        try:
            tfidf_matrix = vectorizer.fit_transform(corpus)
            sim_matrix = cosine_similarity(tfidf_matrix)
        except ValueError:
            # Empty vocabulary (no meaningful text)
            return 1.0

        # Weight by spatial adjacency
        adj = dag.spatial_adjacency(agents)
        n = len(agents)

        total_diversity = 0.0
        pair_count = 0
        for i in range(n):
            for j in range(i + 1, n):
                if adj[i][j] > 0:
                    diversity = 1.0 - float(sim_matrix[i, j])
                    total_diversity += diversity
                    pair_count += 1

        if pair_count == 0:
            return 1.0

        return total_diversity / pair_count


class UnnecessaryPathRatio:
    """Measures the ratio of unnecessary reasoning paths in a multi-agent DAG.

    Lower scores (closer to 0.0) indicate more efficient collaboration
    with fewer wasted reasoning paths.

    Algorithm:
    1. Enumerate all paths from root agents to the final output
    2. Classify each path as necessary (correctness >= threshold) or unnecessary
    3. Compute UPR = 1 - |necessary| / |all_paths|
    """

    def __init__(self, correctness_threshold: float = 0.5) -> None:
        self._threshold = correctness_threshold

    def compute(
        self,
        dag: CollaborationDAG,
        path_scores: dict[str, float] | None = None,
    ) -> float:
        """Compute the UPR score for a multi-agent collaboration DAG.

        Args:
            dag: Collaboration DAG with agent message flow.
            path_scores: Optional mapping of path_id -> correctness score.
                If not provided, all paths are considered necessary (UPR=0).

        Returns:
            Score in [0, 1] where 0.0 means all paths were necessary.
        """
        all_paths = dag.enumerate_paths()
        if not all_paths:
            return 0.0

        if path_scores is None:
            return 0.0  # no scoring data = assume all necessary

        necessary = sum(1 for pid in all_paths if path_scores.get(pid, 0.0) >= self._threshold)
        total = len(all_paths)

        if total == 0:
            return 0.0

        return 1.0 - (necessary / total)
