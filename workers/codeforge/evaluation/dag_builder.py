"""DAG builder for multi-agent collaboration analysis.

Constructs a CollaborationDAG from agent-to-agent messages, supporting
both IDS and UPR metric computation.
"""

from __future__ import annotations

from collections import defaultdict

import numpy as np
from pydantic import BaseModel


class AgentMessage(BaseModel):
    """Single message in a multi-agent collaboration."""

    agent_id: str
    content: str
    round: int
    parent_agent_id: str | None = None


class CollaborationDAG:
    """Directed acyclic graph representing multi-agent message flow.

    Nodes are agents, edges represent message flow between agents.
    Supports spatial adjacency (direct links) and temporal ordering
    (causal dependencies across rounds).
    """

    def __init__(self, messages: list[AgentMessage]) -> None:
        self._messages = messages
        self._edges: list[tuple[str, str]] = []
        self._build_edges()

    @property
    def agent_messages(self) -> list[AgentMessage]:
        return self._messages

    @property
    def agents(self) -> list[str]:
        return sorted({m.agent_id for m in self._messages})

    def _build_edges(self) -> None:
        """Build directed edges from parent_agent_id -> agent_id relationships."""
        for msg in self._messages:
            if msg.parent_agent_id is not None:
                edge = (msg.parent_agent_id, msg.agent_id)
                if edge not in self._edges:
                    self._edges.append(edge)

    def spatial_adjacency(self, agents: list[str] | None = None) -> np.ndarray:
        """Build spatial adjacency matrix for agent communication links.

        Args:
            agents: Ordered list of agent IDs. If None, uses discovered agents.

        Returns:
            NxN numpy array where entry [i,j] = 1 if agent i sent to agent j.
        """
        if agents is None:
            agents = self.agents
        n = len(agents)
        idx = {a: i for i, a in enumerate(agents)}
        matrix = np.zeros((n, n))

        for src, dst in self._edges:
            if src in idx and dst in idx:
                matrix[idx[src]][idx[dst]] = 1.0
                matrix[idx[dst]][idx[src]] = 1.0  # undirected for similarity

        return matrix

    def temporal_adjacency(self, agents: list[str] | None = None) -> np.ndarray:
        """Build temporal adjacency matrix based on causal round ordering.

        Agents that communicate across consecutive rounds have causal dependency.

        Args:
            agents: Ordered list of agent IDs. If None, uses discovered agents.

        Returns:
            NxN numpy array with temporal dependency weights.
        """
        if agents is None:
            agents = self.agents
        n = len(agents)
        idx = {a: i for i, a in enumerate(agents)}
        matrix = np.zeros((n, n))

        # Group messages by round
        rounds: dict[int, list[AgentMessage]] = defaultdict(list)
        for msg in self._messages:
            rounds[msg.round].append(msg)

        sorted_rounds = sorted(rounds.keys())
        for k in range(len(sorted_rounds) - 1):
            r_curr = sorted_rounds[k]
            r_next = sorted_rounds[k + 1]
            agents_curr = {m.agent_id for m in rounds[r_curr]}
            agents_next = {m.agent_id for m in rounds[r_next]}

            for a1 in agents_curr:
                for a2 in agents_next:
                    if a1 != a2 and a1 in idx and a2 in idx:
                        matrix[idx[a1]][idx[a2]] += 1.0

        return matrix

    def enumerate_paths(self) -> list[str]:
        """Enumerate all paths from root agents to leaf agents.

        Returns:
            List of path identifiers (e.g., "agent_a -> agent_b -> agent_c").
        """
        # Find roots (agents with no incoming edges)
        targets = {dst for _, dst in self._edges}
        sources = {src for src, _ in self._edges}
        all_agents = sources | targets
        roots = all_agents - targets
        leaves = all_agents - sources

        if not roots:
            # No clear DAG structure, return single path per agent
            return list(self.agents)

        # BFS from each root to find all paths to leaves
        adjacency: dict[str, list[str]] = defaultdict(list)
        for src, dst in self._edges:
            adjacency[src].append(dst)

        paths: list[str] = []
        stack: list[list[str]] = [[r] for r in sorted(roots)]

        while stack:
            path = stack.pop()
            current = path[-1]
            neighbors = adjacency.get(current, [])

            if not neighbors or current in leaves:
                paths.append(" -> ".join(path))
            else:
                stack.extend([*path, neighbor] for neighbor in neighbors if neighbor not in path)

        return paths


def build_collaboration_dag(messages: list[dict[str, object]]) -> CollaborationDAG:
    """Build a CollaborationDAG from raw agent message dictionaries.

    Args:
        messages: List of dicts with keys: agent_id, content, round, parent_agent_id.

    Returns:
        Structured CollaborationDAG ready for IDS/UPR computation.
    """
    parsed = [AgentMessage.model_validate(m) for m in messages]
    return CollaborationDAG(parsed)
