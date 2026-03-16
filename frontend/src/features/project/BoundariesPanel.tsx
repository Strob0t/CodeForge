import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { BoundaryConfig } from "~/api/types";
import { Button } from "~/ui";

const TYPE_COLORS: Record<string, string> = {
  api: "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300",
  data: "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300",
  "inter-service": "bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300",
  "cross-language": "bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300",
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
        <h3 class="text-sm font-semibold text-zinc-700 dark:text-zinc-300">Boundary Files</h3>
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
          <p class="text-sm text-zinc-500">
            {config.loading ? "Loading..." : "No boundaries detected yet."}
          </p>
        }
      >
        {(cfg) => (
          <div class="space-y-2">
            <For each={cfg().boundaries}>
              {(boundary) => (
                <div class="flex items-center justify-between rounded border border-zinc-200 px-3 py-2 dark:border-zinc-700">
                  <div class="min-w-0 flex-1">
                    <p class="truncate font-mono text-xs text-zinc-800 dark:text-zinc-200">
                      {boundary.path}
                    </p>
                    <Show when={boundary.counterpart}>
                      <p class="truncate font-mono text-xs text-zinc-500">
                        &#x21D4; {boundary.counterpart}
                      </p>
                    </Show>
                  </div>
                  <span
                    class={`ml-2 shrink-0 rounded px-2 py-0.5 text-xs font-medium ${TYPE_COLORS[boundary.type] ?? "bg-zinc-100 text-zinc-600"}`}
                  >
                    {boundary.type}
                  </span>
                </div>
              )}
            </For>
            <p class="text-xs text-zinc-400">
              {cfg().boundaries.length} boundaries &middot; Last analyzed:{" "}
              {new Date(cfg().last_analyzed).toLocaleDateString()}
            </p>
          </div>
        )}
      </Show>
    </div>
  );
}
