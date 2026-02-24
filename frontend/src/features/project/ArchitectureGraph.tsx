import { createEffect, createResource, createSignal, For, on, onCleanup, Show } from "solid-js";

import { api } from "~/api/client";
import type { GraphNodeKind, GraphSearchHit, GraphStatus } from "~/api/types";
import { useI18n } from "~/i18n";
import { Alert, Badge, Button, Card, Input } from "~/ui";

interface ArchitectureGraphProps {
  projectId: string;
}

interface GraphNode {
  id: string;
  label: string;
  kind: GraphNodeKind;
  filepath: string;
  line: number;
  x: number;
  y: number;
  vx: number;
  vy: number;
}

interface GraphEdge {
  source: string;
  target: string;
}

const KIND_COLORS: Record<GraphNodeKind, string> = {
  function: "#3b82f6",
  class: "#8b5cf6",
  method: "#06b6d4",
  module: "#f59e0b",
};

const KIND_RADIUS: Record<GraphNodeKind, number> = {
  function: 6,
  class: 10,
  method: 5,
  module: 12,
};

/** Build a graph from search results by inferring edges from edge_path */
function buildGraph(hits: GraphSearchHit[]): { nodes: GraphNode[]; edges: GraphEdge[] } {
  const nodeMap = new Map<string, GraphNode>();
  const edgeSet = new Set<string>();
  const edges: GraphEdge[] = [];

  const width = 600;
  const height = 400;

  for (const hit of hits) {
    const nodeId = `${hit.filepath}:${hit.symbol_name}`;
    if (!nodeMap.has(nodeId)) {
      nodeMap.set(nodeId, {
        id: nodeId,
        label: hit.symbol_name,
        kind: hit.kind,
        filepath: hit.filepath,
        line: hit.start_line,
        x: Math.random() * (width - 40) + 20,
        y: Math.random() * (height - 40) + 20,
        vx: 0,
        vy: 0,
      });
    }

    // Create edges from edge_path
    if (hit.edge_path.length > 1) {
      for (let i = 0; i < hit.edge_path.length - 1; i++) {
        const src = hit.edge_path[i];
        const tgt = hit.edge_path[i + 1];
        // Ensure edge nodes exist (with approximate data)
        if (!nodeMap.has(src)) {
          nodeMap.set(src, {
            id: src,
            label: src.includes(":") ? (src.split(":").pop() ?? src) : src,
            kind: "function",
            filepath: src.includes(":") ? src.split(":")[0] : "",
            line: 0,
            x: Math.random() * (width - 40) + 20,
            y: Math.random() * (height - 40) + 20,
            vx: 0,
            vy: 0,
          });
        }
        if (!nodeMap.has(tgt)) {
          nodeMap.set(tgt, {
            id: tgt,
            label: tgt.includes(":") ? (tgt.split(":").pop() ?? tgt) : tgt,
            kind: "function",
            filepath: tgt.includes(":") ? tgt.split(":")[0] : "",
            line: 0,
            x: Math.random() * (width - 40) + 20,
            y: Math.random() * (height - 40) + 20,
            vx: 0,
            vy: 0,
          });
        }
        const edgeKey = `${src}->${tgt}`;
        if (!edgeSet.has(edgeKey)) {
          edgeSet.add(edgeKey);
          edges.push({ source: src, target: tgt });
        }
      }
    }
  }

  return { nodes: Array.from(nodeMap.values()), edges };
}

/** Simple force-directed layout simulation */
function simulateForces(nodes: GraphNode[], edges: GraphEdge[], width: number, height: number) {
  const repulsion = 1500;
  const attraction = 0.005;
  const damping = 0.85;
  const centerForce = 0.01;

  // Repulsion between all node pairs
  for (let i = 0; i < nodes.length; i++) {
    for (let j = i + 1; j < nodes.length; j++) {
      const dx = nodes[i].x - nodes[j].x;
      const dy = nodes[i].y - nodes[j].y;
      const dist = Math.max(Math.sqrt(dx * dx + dy * dy), 1);
      const force = repulsion / (dist * dist);
      const fx = (dx / dist) * force;
      const fy = (dy / dist) * force;
      nodes[i].vx += fx;
      nodes[i].vy += fy;
      nodes[j].vx -= fx;
      nodes[j].vy -= fy;
    }
  }

  // Attraction along edges
  const nodeIndex = new Map(nodes.map((n, i) => [n.id, i]));
  for (const edge of edges) {
    const si = nodeIndex.get(edge.source);
    const ti = nodeIndex.get(edge.target);
    if (si === undefined || ti === undefined) continue;
    const dx = nodes[ti].x - nodes[si].x;
    const dy = nodes[ti].y - nodes[si].y;
    const fx = dx * attraction;
    const fy = dy * attraction;
    nodes[si].vx += fx;
    nodes[si].vy += fy;
    nodes[ti].vx -= fx;
    nodes[ti].vy -= fy;
  }

  // Center gravity
  const cx = width / 2;
  const cy = height / 2;
  for (const node of nodes) {
    node.vx += (cx - node.x) * centerForce;
    node.vy += (cy - node.y) * centerForce;
  }

  // Apply velocity and damping
  for (const node of nodes) {
    node.vx *= damping;
    node.vy *= damping;
    node.x += node.vx;
    node.y += node.vy;
    // Clamp to bounds
    node.x = Math.max(20, Math.min(width - 20, node.x));
    node.y = Math.max(20, Math.min(height - 20, node.y));
  }
}

export default function ArchitectureGraph(props: ArchitectureGraphProps) {
  const { t, fmt } = useI18n();

  const [graphStatus] = createResource(
    () => props.projectId || undefined,
    async (id: string): Promise<GraphStatus | null> => {
      try {
        return await api.graph.status(id);
      } catch {
        return null;
      }
    },
  );

  const [seedSymbols, setSeedSymbols] = createSignal("");
  const [maxHops, setMaxHops] = createSignal(2);
  const [searching, setSearching] = createSignal(false);
  const [hits, setHits] = createSignal<GraphSearchHit[]>([]);
  const [error, setError] = createSignal("");
  const [hoveredNode, setHoveredNode] = createSignal<string | null>(null);
  const [nodes, setNodes] = createSignal<GraphNode[]>([]);
  const [edges, setEdges] = createSignal<GraphEdge[]>([]);

  const SVG_WIDTH = 600;
  const SVG_HEIGHT = 400;

  const handleSearch = async (e: Event) => {
    e.preventDefault();
    const seeds = seedSymbols()
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    if (seeds.length === 0) return;

    setSearching(true);
    setError("");
    try {
      const result = await api.graph.search(props.projectId, {
        seed_symbols: seeds,
        max_hops: maxHops(),
        top_k: 30,
      });
      if (result.error) {
        setError(result.error);
      } else {
        setHits(result.results);
        const { nodes: n, edges: e } = buildGraph(result.results);
        setNodes(n);
        setEdges(e);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t("archGraph.error"));
    } finally {
      setSearching(false);
    }
  };

  // Animate force layout
  let animationFrame: number | undefined;

  createEffect(
    on(
      () => nodes().length,
      () => {
        if (nodes().length === 0) return;
        let iterations = 0;
        const maxIterations = 120;

        const tick = () => {
          if (iterations >= maxIterations) return;
          iterations++;
          const current = [...nodes().map((n) => ({ ...n }))];
          simulateForces(current, edges(), SVG_WIDTH, SVG_HEIGHT);
          setNodes(current);
          animationFrame = requestAnimationFrame(tick);
        };
        tick();
      },
    ),
  );

  onCleanup(() => {
    if (animationFrame !== undefined) cancelAnimationFrame(animationFrame);
  });

  const nodeById = () => new Map(nodes().map((n) => [n.id, n]));

  return (
    <Card>
      <Card.Header>
        <h3 class="text-lg font-semibold">{t("archGraph.title")}</h3>
        <p class="text-xs text-cf-text-tertiary">{t("archGraph.description")}</p>
      </Card.Header>

      <Card.Body>
        {/* Graph status */}
        <Show when={graphStatus()}>
          {(gs) => (
            <div class="mb-3 flex items-center gap-3 text-xs text-cf-text-tertiary">
              <span>
                {fmt.compact(gs().node_count)} {t("archGraph.nodes")}
              </span>
              <span>
                {fmt.compact(gs().edge_count)} {t("archGraph.edges")}
              </span>
              <Show when={gs().languages.length > 0}>
                <span>{gs().languages.join(", ")}</span>
              </Show>
            </div>
          )}
        </Show>

        {/* Search form */}
        <form class="mb-4 flex gap-2" onSubmit={handleSearch}>
          <Input
            type="text"
            class="flex-1"
            placeholder={t("archGraph.seedPlaceholder")}
            value={seedSymbols()}
            onInput={(e) => setSeedSymbols(e.currentTarget.value)}
            aria-label={t("archGraph.seedLabel")}
          />
          <div class="flex items-center gap-1">
            <label for="arch-hops" class="text-xs text-cf-text-tertiary">
              {t("archGraph.hops")}:
            </label>
            <Input
              id="arch-hops"
              type="number"
              min="1"
              max="5"
              class="w-14"
              value={maxHops()}
              onInput={(e) => setMaxHops(parseInt(e.currentTarget.value) || 2)}
            />
          </div>
          <Button
            type="submit"
            variant="primary"
            size="sm"
            disabled={searching() || !seedSymbols().trim() || graphStatus()?.status !== "ready"}
            loading={searching()}
          >
            {searching() ? t("archGraph.searching") : t("archGraph.explore")}
          </Button>
        </form>

        <Show when={error()}>
          <div class="mb-3">
            <Alert variant="error">{error()}</Alert>
          </div>
        </Show>

        {/* Graph SVG */}
        <Show
          when={nodes().length > 0}
          fallback={
            <div class="flex h-48 items-center justify-center rounded-cf-sm bg-cf-bg-inset text-sm text-cf-text-muted">
              {graphStatus()?.status === "ready"
                ? t("archGraph.enterSeeds")
                : t("archGraph.buildFirst")}
            </div>
          }
        >
          <div class="overflow-hidden rounded-cf-sm border border-cf-border">
            <svg viewBox={`0 0 ${SVG_WIDTH} ${SVG_HEIGHT}`} class="h-96 w-full bg-cf-bg-inset">
              {/* Edges */}
              <For each={edges()}>
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
                        stroke={
                          hoveredNode() === edge.source || hoveredNode() === edge.target
                            ? "#6366f1"
                            : "#d1d5db"
                        }
                        stroke-width={
                          hoveredNode() === edge.source || hoveredNode() === edge.target ? 2 : 1
                        }
                        stroke-opacity="0.6"
                      />
                    </Show>
                  );
                }}
              </For>

              {/* Nodes */}
              <For each={nodes()}>
                {(node) => (
                  <g
                    onMouseEnter={() => setHoveredNode(node.id)}
                    onMouseLeave={() => setHoveredNode(null)}
                    style={{ cursor: "pointer" }}
                  >
                    <circle
                      cx={node.x}
                      cy={node.y}
                      r={KIND_RADIUS[node.kind]}
                      fill={KIND_COLORS[node.kind]}
                      opacity={hoveredNode() === null || hoveredNode() === node.id ? 1 : 0.4}
                      stroke={hoveredNode() === node.id ? "#111827" : "none"}
                      stroke-width="2"
                    >
                      <title>
                        {node.label} ({node.kind}) - {node.filepath}:{node.line}
                      </title>
                    </circle>
                    <Show when={hoveredNode() === node.id || nodes().length <= 15}>
                      <text
                        x={node.x}
                        y={node.y - KIND_RADIUS[node.kind] - 4}
                        text-anchor="middle"
                        class="fill-cf-text-secondary text-[9px]"
                      >
                        {node.label}
                      </text>
                    </Show>
                  </g>
                )}
              </For>
            </svg>
          </div>

          {/* Legend */}
          <div class="mt-2 flex items-center gap-4 text-xs text-cf-text-tertiary">
            <span class="flex items-center gap-1">
              <span
                class="inline-block h-3 w-3 rounded-full"
                style={{ background: KIND_COLORS.module }}
              />
              {t("archGraph.kind.module")}
            </span>
            <span class="flex items-center gap-1">
              <span
                class="inline-block h-3 w-3 rounded-full"
                style={{ background: KIND_COLORS.class }}
              />
              {t("archGraph.kind.class")}
            </span>
            <span class="flex items-center gap-1">
              <span
                class="inline-block h-3 w-3 rounded-full"
                style={{ background: KIND_COLORS.function }}
              />
              {t("archGraph.kind.function")}
            </span>
            <span class="flex items-center gap-1">
              <span
                class="inline-block h-3 w-3 rounded-full"
                style={{ background: KIND_COLORS.method }}
              />
              {t("archGraph.kind.method")}
            </span>
            <span class="ml-auto">
              {nodes().length} {t("archGraph.nodes")}, {edges().length} {t("archGraph.edges")}
            </span>
          </div>

          {/* Hit list */}
          <Show when={hits().length > 0}>
            <details class="mt-3">
              <summary class="cursor-pointer text-xs text-cf-text-tertiary hover:text-cf-text-secondary">
                {t("archGraph.rawResults")} ({hits().length})
              </summary>
              <div class="mt-1 max-h-40 space-y-1 overflow-y-auto">
                <For each={hits()}>
                  {(hit) => (
                    <div class="flex items-center gap-2 text-xs text-cf-text-secondary">
                      <Badge
                        variant="default"
                        style={{
                          background: `${KIND_COLORS[hit.kind]}20`,
                          color: KIND_COLORS[hit.kind],
                        }}
                      >
                        {hit.kind}
                      </Badge>
                      <span class="font-mono">{hit.symbol_name}</span>
                      <span class="text-cf-text-muted">
                        {hit.filepath}:{hit.start_line}
                      </span>
                      <span class="ml-auto">{fmt.score(hit.score)}</span>
                    </div>
                  )}
                </For>
              </div>
            </details>
          </Show>
        </Show>
      </Card.Body>
    </Card>
  );
}
