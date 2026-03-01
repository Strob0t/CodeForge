import { createMemo, For, Show } from "solid-js";

import type { PlanGraph, PlanGraphEdge, PlanGraphNode } from "~/api/types";
import { getVariant, nodeStatusVariant } from "~/config/statusVariants";
import { useI18n } from "~/i18n";
import { Badge, Card } from "~/ui";

interface AgentFlowGraphProps {
  graph: PlanGraph;
  taskNames: Record<string, string>;
  agentNames: Record<string, string>;
  onStepClick?: (stepId: string) => void;
}

// Layout constants
const NODE_WIDTH = 180;
const NODE_HEIGHT = 56;
const NODE_MARGIN_X = 40;
const NODE_MARGIN_Y = 32;
const PADDING = 24;

interface LayoutNode {
  node: PlanGraphNode;
  x: number;
  y: number;
  col: number;
  row: number;
}

/**
 * Compute a simple layered layout for the DAG.
 * Nodes with no dependencies go in column 0. Others go in the column after their latest dependency.
 */
function layoutDAG(nodes: PlanGraphNode[]): LayoutNode[] {
  const nodeMap = new Map<string, PlanGraphNode>();
  for (const n of nodes) nodeMap.set(n.id, n);

  // Compute column (layer) for each node
  const cols = new Map<string, number>();

  function getCol(id: string): number {
    if (cols.has(id)) return cols.get(id) as number;
    const node = nodeMap.get(id);
    if (!node || !node.depends_on || node.depends_on.length === 0) {
      cols.set(id, 0);
      return 0;
    }
    const maxDep = Math.max(...node.depends_on.map(getCol));
    const col = maxDep + 1;
    cols.set(id, col);
    return col;
  }

  for (const n of nodes) getCol(n.id);

  // Group by column and assign rows
  const byCol = new Map<number, PlanGraphNode[]>();
  for (const n of nodes) {
    const col = cols.get(n.id) ?? 0;
    if (!byCol.has(col)) byCol.set(col, []);
    (byCol.get(col) as PlanGraphNode[]).push(n);
  }

  const result: LayoutNode[] = [];
  for (const [col, colNodes] of byCol) {
    for (let row = 0; row < colNodes.length; row++) {
      result.push({
        node: colNodes[row],
        col,
        row,
        x: PADDING + col * (NODE_WIDTH + NODE_MARGIN_X),
        y: PADDING + row * (NODE_HEIGHT + NODE_MARGIN_Y),
      });
    }
  }

  return result;
}

function statusColor(status: string): string {
  switch (status) {
    case "running":
      return "#3b82f6"; // blue
    case "completed":
      return "#22c55e"; // green
    case "failed":
      return "#ef4444"; // red
    case "review":
      return "#f59e0b"; // amber
    default:
      return "#94a3b8"; // slate
  }
}

export default function AgentFlowGraph(props: AgentFlowGraphProps) {
  const { t } = useI18n();

  const layout = createMemo(() => layoutDAG(props.graph.nodes));

  const posMap = createMemo(() => {
    const map = new Map<string, LayoutNode>();
    for (const ln of layout()) map.set(ln.node.id, ln);
    return map;
  });

  const svgSize = createMemo(() => {
    const nodes = layout();
    if (nodes.length === 0) return { width: 300, height: 100 };
    let maxX = 0;
    let maxY = 0;
    for (const n of nodes) {
      maxX = Math.max(maxX, n.x + NODE_WIDTH);
      maxY = Math.max(maxY, n.y + NODE_HEIGHT);
    }
    return { width: maxX + PADDING, height: maxY + PADDING };
  });

  const edgePaths = createMemo(() => {
    const map = posMap();
    return props.graph.edges
      .map((edge: PlanGraphEdge) => {
        const from = map.get(edge.from);
        const to = map.get(edge.to);
        if (!from || !to) return null;
        const x1 = from.x + NODE_WIDTH;
        const y1 = from.y + NODE_HEIGHT / 2;
        const x2 = to.x;
        const y2 = to.y + NODE_HEIGHT / 2;
        const cx = (x1 + x2) / 2;
        return {
          path: `M ${x1} ${y1} C ${cx} ${y1}, ${cx} ${y2}, ${x2} ${y2}`,
          edge,
        };
      })
      .filter(Boolean) as { path: string; edge: PlanGraphEdge }[];
  });

  return (
    <Card>
      <Card.Header>
        <div class="flex items-center justify-between">
          <h3 class="text-lg font-semibold">{t("plan.flow.title")}</h3>
          <div class="flex items-center gap-2">
            <Badge variant={getVariant(nodeStatusVariant, props.graph.status)}>
              {props.graph.status}
            </Badge>
            <Badge variant="default">{props.graph.protocol}</Badge>
          </div>
        </div>
      </Card.Header>
      <Card.Body>
        <Show
          when={props.graph.nodes.length > 0}
          fallback={<p class="text-sm text-cf-text-muted">{t("plan.flow.noSteps")}</p>}
        >
          <div class="overflow-x-auto">
            <svg
              width={svgSize().width}
              height={svgSize().height}
              class="min-w-full"
              role="img"
              aria-label={`Execution flow graph for plan: ${props.graph.name}`}
            >
              {/* Marker for arrowheads */}
              <defs>
                <marker
                  id="arrowhead"
                  markerWidth="8"
                  markerHeight="6"
                  refX="8"
                  refY="3"
                  orient="auto"
                >
                  <polygon points="0 0, 8 3, 0 6" fill="#94a3b8" />
                </marker>
              </defs>

              {/* Edges */}
              <For each={edgePaths()}>
                {(ep) => (
                  <path
                    d={ep.path}
                    fill="none"
                    stroke="#94a3b8"
                    stroke-width="1.5"
                    marker-end="url(#arrowhead)"
                    class="transition-all"
                  />
                )}
              </For>

              {/* Nodes */}
              <For each={layout()}>
                {(ln) => {
                  const color = () => statusColor(ln.node.status);
                  return (
                    <g
                      class="cursor-pointer"
                      onClick={() => props.onStepClick?.(ln.node.id)}
                      role="button"
                      tabIndex={0}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault();
                          props.onStepClick?.(ln.node.id);
                        }
                      }}
                    >
                      {/* Node background */}
                      <rect
                        x={ln.x}
                        y={ln.y}
                        width={NODE_WIDTH}
                        height={NODE_HEIGHT}
                        rx="6"
                        fill="var(--cf-bg-surface, #1e293b)"
                        stroke={color()}
                        stroke-width="2"
                        class="transition-colors"
                      />
                      {/* Status indicator */}
                      <circle cx={ln.x + 14} cy={ln.y + NODE_HEIGHT / 2} r="5" fill={color()} />
                      {/* Task name */}
                      <text
                        x={ln.x + 28}
                        y={ln.y + 22}
                        font-size="12"
                        font-weight="600"
                        fill="var(--cf-text-primary, #e2e8f0)"
                        class="pointer-events-none"
                      >
                        {(props.taskNames[ln.node.task_id] ?? ln.node.task_id.slice(0, 10)).slice(
                          0,
                          18,
                        )}
                      </text>
                      {/* Agent name */}
                      <text
                        x={ln.x + 28}
                        y={ln.y + 38}
                        font-size="10"
                        fill="var(--cf-text-secondary, #94a3b8)"
                        class="pointer-events-none"
                      >
                        {(
                          props.agentNames[ln.node.agent_id] ?? ln.node.agent_id.slice(0, 10)
                        ).slice(0, 20)}
                      </text>
                      {/* Status text */}
                      <text
                        x={ln.x + NODE_WIDTH - 8}
                        y={ln.y + 14}
                        font-size="9"
                        text-anchor="end"
                        fill={color()}
                        class="pointer-events-none"
                      >
                        {ln.node.status}
                      </text>
                    </g>
                  );
                }}
              </For>
            </svg>
          </div>
        </Show>
      </Card.Body>
    </Card>
  );
}
