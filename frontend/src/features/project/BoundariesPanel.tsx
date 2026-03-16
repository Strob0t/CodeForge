import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { BoundaryConfig } from "~/api/types";
import { Button } from "~/ui";

const TYPE_COLORS: Record<string, string> = {
  api: "bg-cf-info-bg text-cf-info-fg",
  data: "bg-cf-success-bg text-cf-success-fg",
  "inter-service": "bg-cf-info-bg text-cf-info-fg",
  "cross-language": "bg-cf-warning-bg text-cf-warning-fg",
};

export default function BoundariesPanel(props: { projectId: string }) {
  const [analyzing, setAnalyzing] = createSignal(false);

  const fetchBoundaries = async (id: string): Promise<BoundaryConfig | null> => {
    try {
      return await api.projects.getBoundaries(id);
    } catch {
      return null;
    }
  };

  const [config, { refetch }] = createResource(() => props.projectId, fetchBoundaries);

  const triggerAnalysis = async () => {
    setAnalyzing(true);
    try {
      await api.projects.triggerBoundaryAnalysis(props.projectId);
      // Refetch after a short delay to allow analysis to complete
      setTimeout(() => refetch(), 3000);
    } finally {
      setAnalyzing(false);
    }
  };

  return (
    <div class="space-y-4">
      <div class="flex items-center justify-between">
        <h3 class="text-sm font-semibold text-cf-text-secondary">Boundary Files</h3>
        <Button
          variant="secondary"
          size="xs"
          onClick={triggerAnalysis}
          disabled={analyzing()}
          loading={analyzing()}
        >
          {analyzing() ? "Analyzing..." : "Re-analyze"}
        </Button>
      </div>

      <Show
        when={!config.loading && config()}
        fallback={
          <p class="text-sm text-cf-text-muted">
            {config.loading ? "Loading..." : "No boundaries detected yet."}
          </p>
        }
      >
        {(cfg) => (
          <div class="space-y-2">
            <For each={cfg().boundaries}>
              {(boundary) => (
                <div class="flex items-center justify-between rounded border border-cf-border px-3 py-2">
                  <div class="min-w-0 flex-1">
                    <p class="truncate font-mono text-xs text-cf-text-primary">{boundary.path}</p>
                    <Show when={boundary.counterpart}>
                      <p class="truncate font-mono text-xs text-cf-text-muted">
                        &#x21D4; {boundary.counterpart}
                      </p>
                    </Show>
                  </div>
                  <span
                    class={`ml-2 shrink-0 rounded px-2 py-0.5 text-xs font-medium ${TYPE_COLORS[boundary.type] ?? "bg-cf-bg-surface-alt text-cf-text-secondary"}`}
                  >
                    {boundary.type}
                  </span>
                </div>
              )}
            </For>
            <p class="text-xs text-cf-text-muted">
              {cfg().boundaries.length} boundaries &middot; Last analyzed:{" "}
              {new Date(cfg().last_analyzed).toLocaleDateString()}
            </p>
          </div>
        )}
      </Show>
    </div>
  );
}
