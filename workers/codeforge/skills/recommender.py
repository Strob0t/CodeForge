"""SkillRecommender: BM25-based recommendation of relevant skills for a task."""

from __future__ import annotations

import bm25s
import structlog

from codeforge.skills.models import Skill, SkillRecommendation

logger = structlog.get_logger()


class SkillRecommender:
    """Recommends relevant skills based on BM25 similarity to task context."""

    def __init__(self) -> None:
        self._skills: list[Skill] = []
        self._retriever: bm25s.BM25 | None = None

    def index(self, skills: list[Skill]) -> None:
        """Build a BM25 index over skill descriptions and tags."""
        self._skills = skills
        if not skills:
            self._retriever = None
            return

        # Build corpus from description + tags
        corpus = []
        for s in skills:
            text = f"{s.name} {s.description} {' '.join(s.tags)}"
            corpus.append(text)

        corpus_tokens = bm25s.tokenize(corpus, stopwords="en")
        self._retriever = bm25s.BM25()
        self._retriever.index(corpus_tokens)
        logger.debug("skill index built", skill_count=len(skills))

    def recommend(self, task_context: str, top_k: int = 3) -> list[SkillRecommendation]:
        """Return the top-k most relevant skills for the given task context."""
        if not self._retriever or not self._skills:
            return []

        query_tokens = bm25s.tokenize([task_context], stopwords="en")
        results, scores = self._retriever.retrieve(query_tokens, k=min(top_k, len(self._skills)))

        recommendations: list[SkillRecommendation] = []
        for idx_arr, score_arr in zip(results[0], scores[0], strict=False):
            idx = int(idx_arr)
            score = float(score_arr)
            if score > 0 and 0 <= idx < len(self._skills):
                recommendations.append(SkillRecommendation(skill=self._skills[idx], score=score))

        recommendations.sort(key=lambda r: r.score, reverse=True)
        return recommendations[:top_k]
