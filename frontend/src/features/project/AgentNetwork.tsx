import { createResource, createSignal, For, onCleanup, Show } from "solid-js";

import { api } from "~/api/client";
import type { AgentTeam, TeamMember, TeamRole, TeamStatus } from "~/api/types";
import { createCodeForgeWS } from "~/api/websocket";
import { useI18n } from "~/i18n";

interface AgentNetworkProps {
  projectId: string;
}

interface NetworkNode {
  id: string;
  name: string;
  role: TeamRole;
  status: "idle" | "active" | "error";
  x: number;
  y: number;
}

interface NetworkEdge {
  source: string;
  target: string;
  active: boolean;
}

const ROLE_COLORS: Record<TeamRole, string> = {
  coder: "#3b82f6",
  reviewer: "#8b5cf6",
  tester: "#10b981",
  documenter: "#f59e0b",
  planner: "#ef4444",
};

const STATUS_RING: Record<string, string> = {
  idle: "#9ca3af",
  active: "#22c55e",
  error: "#ef4444",
};

const TEAM_STATUS_COLORS: Record<TeamStatus, string> = {
  initializing: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
  active: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
  completed: "bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300",
  failed: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
};

/** Arrange nodes in a circle */
function circleLayout(members: TeamMember[], agentNames: Map<string, string>): NetworkNode[] {
  const cx = 200;
  const cy = 150;
  const radius = Math.min(120, 30 + members.length * 20);

  return members.map((m, i) => {
    const angle = (2 * Math.PI * i) / members.length - Math.PI / 2;
    return {
      id: m.agent_id,
      name: agentNames.get(m.agent_id) ?? m.agent_id.slice(0, 8),
      role: m.role,
      status: "idle" as const,
      x: cx + radius * Math.cos(angle),
      y: cy + radius * Math.sin(angle),
    };
  });
}

/** Create edges between all team members (full mesh for now) */
function buildEdges(members: TeamMember[]): NetworkEdge[] {
  const edges: NetworkEdge[] = [];
  for (let i = 0; i < members.length; i++) {
    for (let j = i + 1; j < members.length; j++) {
      edges.push({
        source: members[i].agent_id,
        target: members[j].agent_id,
        active: false,
      });
    }
  }
  return edges;
}

export default function AgentNetwork(props: AgentNetworkProps) {
  const { t } = useI18n();

  const [teams] = createResource(
    () => props.projectId,
    async (id) => {
      try {
        return await api.teams.list(id);
      } catch {
        return [];
      }
    },
  );

  const [agents] = createResource(
    () => props.projectId,
    async (id) => {
      try {
        return await api.agents.list(id);
      } catch {
        return [];
      }
    },
  );

  const [selectedTeam, setSelectedTeam] = createSignal<AgentTeam | null>(null);
  const [networkNodes, setNetworkNodes] = createSignal<NetworkNode[]>([]);
  const [networkEdges, setNetworkEdges] = createSignal<NetworkEdge[]>([]);
  const [hoveredNode, setHoveredNode] = createSignal<string | null>(null);
  const [messageFlows, setMessageFlows] = createSignal<{ from: string; to: string; t: number }[]>(
    [],
  );

  const agentNames = () => new Map((agents() ?? []).map((a) => [a.id, a.name]));

  const selectTeam = (team: AgentTeam) => {
    setSelectedTeam(team);
    const nodes = circleLayout(team.members, agentNames());
    setNetworkNodes(nodes);
    setNetworkEdges(buildEdges(team.members));
    setMessageFlows([]);
  };

  // Listen for WS events to animate message flow
  const { onMessage } = createCodeForgeWS();
  const cleanup = onMessage((msg) => {
    if (msg.type === "team.message" || msg.type === "shared_context.update") {
      const from = msg.payload.from_agent as string | undefined;
      const to = msg.payload.to_agent as string | undefined;
      if (from && to) {
        setMessageFlows((prev) => [...prev.slice(-20), { from, to, t: Date.now() }]);
        // Highlight edge
        setNetworkEdges((prev) =>
          prev.map((e) =>
            (e.source === from && e.target === to) || (e.source === to && e.target === from)
              ? { ...e, active: true }
              : e,
          ),
        );
        // Reset after animation
        setTimeout(() => {
          setNetworkEdges((prev) =>
            prev.map((e) =>
              (e.source === from && e.target === to) || (e.source === to && e.target === from)
                ? { ...e, active: false }
                : e,
            ),
          );
        }, 1500);
      }
    }

    // Update agent status
    if (msg.type === "agent.status") {
      const agentId = msg.payload.agent_id as string;
      const status = msg.payload.status as string;
      setNetworkNodes((prev) =>
        prev.map((n) =>
          n.id === agentId
            ? {
                ...n,
                status: status === "running" ? "active" : status === "error" ? "error" : "idle",
              }
            : n,
        ),
      );
    }
  });
  onCleanup(cleanup);

  const SVG_WIDTH = 400;
  const SVG_HEIGHT = 300;

  const nodeById = () => new Map(networkNodes().map((n) => [n.id, n]));

  return (
    <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <h3 class="mb-1 text-lg font-semibold">{t("agentNetwork.title")}</h3>
      <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">{t("agentNetwork.description")}</p>

      {/* Team selector */}
      <Show
        when={(teams() ?? []).length > 0}
        fallback={
          <p class="text-sm text-gray-400 dark:text-gray-500">{t("agentNetwork.noTeams")}</p>
        }
      >
        <div class="mb-4 flex flex-wrap gap-2">
          <For each={teams()}>
            {(team) => (
              <button
                type="button"
                class={`rounded px-3 py-1.5 text-sm ${
                  selectedTeam()?.id === team.id
                    ? "bg-indigo-600 text-white"
                    : "bg-gray-100 text-gray-700 hover:bg-gray-200 dark:bg-gray-700 dark:text-gray-300 dark:hover:bg-gray-600"
                }`}
                onClick={() => selectTeam(team)}
              >
                {team.name}
                <span
                  class={`ml-1.5 inline-block rounded px-1.5 py-0.5 text-xs ${TEAM_STATUS_COLORS[team.status]}`}
                >
                  {team.status}
                </span>
              </button>
            )}
          </For>
        </div>
      </Show>

      {/* Network graph */}
      <Show when={selectedTeam()}>
        <div class="overflow-hidden rounded border border-gray-200 dark:border-gray-700">
          <svg
            viewBox={`0 0 ${SVG_WIDTH} ${SVG_HEIGHT}`}
            class="h-72 w-full bg-gray-50 dark:bg-gray-900"
          >
            <defs>
              <marker
                id="arrowhead"
                markerWidth="8"
                markerHeight="6"
                refX="8"
                refY="3"
                orient="auto"
              >
                <polygon points="0 0, 8 3, 0 6" fill="#6366f1" />
              </marker>
            </defs>

            {/* Edges */}
            <For each={networkEdges()}>
              {(edge) => {
                const src = () => nodeById().get(edge.source);
                const tgt = () => nodeById().get(edge.target);
                return (
                  <Show when={src() && tgt()}>
                    <line
                      x1={src()?.x ?? 0}
                      y1={src()?.y ?? 0}
                      x2={tgt()?.x ?? 0}
                      y2={tgt()?.y ?? 0}
                      stroke={edge.active ? "#6366f1" : "#e5e7eb"}
                      stroke-width={edge.active ? 3 : 1}
                      stroke-dasharray={edge.active ? "none" : "4,4"}
                      class={edge.active ? "animate-pulse" : ""}
                    />
                    <Show when={edge.active}>
                      <line
                        x1={src()?.x ?? 0}
                        y1={src()?.y ?? 0}
                        x2={tgt()?.x ?? 0}
                        y2={tgt()?.y ?? 0}
                        stroke="#6366f1"
                        stroke-width="3"
                        marker-end="url(#arrowhead)"
                      />
                    </Show>
                  </Show>
                );
              }}
            </For>

            {/* Nodes */}
            <For each={networkNodes()}>
              {(node) => (
                <g
                  onMouseEnter={() => setHoveredNode(node.id)}
                  onMouseLeave={() => setHoveredNode(null)}
                  style={{ cursor: "pointer" }}
                >
                  {/* Status ring */}
                  <circle
                    cx={node.x}
                    cy={node.y}
                    r="18"
                    fill="none"
                    stroke={STATUS_RING[node.status]}
                    stroke-width="2"
                    stroke-dasharray={node.status === "active" ? "none" : "3,3"}
                    class={node.status === "active" ? "animate-pulse" : ""}
                  />
                  {/* Node circle */}
                  <circle
                    cx={node.x}
                    cy={node.y}
                    r="14"
                    fill={ROLE_COLORS[node.role]}
                    opacity={hoveredNode() === null || hoveredNode() === node.id ? 1 : 0.5}
                  >
                    <title>
                      {node.name} ({node.role}) - {node.status}
                    </title>
                  </circle>
                  {/* Label */}
                  <text
                    x={node.x}
                    y={node.y + 30}
                    text-anchor="middle"
                    class="fill-gray-700 text-[10px] dark:fill-gray-300"
                  >
                    {node.name}
                  </text>
                  {/* Role badge */}
                  <text
                    x={node.x}
                    y={node.y + 4}
                    text-anchor="middle"
                    class="fill-white text-[8px] font-bold"
                  >
                    {node.role.charAt(0).toUpperCase()}
                  </text>
                </g>
              )}
            </For>
          </svg>
        </div>

        {/* Legend */}
        <div class="mt-2 flex flex-wrap items-center gap-3 text-xs text-gray-500 dark:text-gray-400">
          <For each={Object.entries(ROLE_COLORS) as [TeamRole, string][]}>
            {([role, color]) => (
              <span class="flex items-center gap-1">
                <span class="inline-block h-3 w-3 rounded-full" style={{ background: color }} />
                {t(`agentNetwork.role.${role}`)}
              </span>
            )}
          </For>
        </div>

        {/* Message flow log */}
        <Show when={messageFlows().length > 0}>
          <div class="mt-3 max-h-24 overflow-y-auto">
            <p class="mb-1 text-xs font-medium text-gray-500 dark:text-gray-400">
              {t("agentNetwork.messageLog")}
            </p>
            <For each={[...messageFlows()].reverse()}>
              {(flow) => (
                <div class="text-xs text-gray-400">
                  <span class="font-medium text-indigo-600 dark:text-indigo-400">
                    {agentNames().get(flow.from) ?? flow.from.slice(0, 8)}
                  </span>
                  {" \u2192 "}
                  <span class="font-medium text-indigo-600 dark:text-indigo-400">
                    {agentNames().get(flow.to) ?? flow.to.slice(0, 8)}
                  </span>
                </div>
              )}
            </For>
          </div>
        </Show>
      </Show>
    </div>
  );
}
