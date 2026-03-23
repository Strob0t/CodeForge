import type { CoreClient } from "../core";
import type { ModelPerformanceStats, RoutingOutcome } from "../types";

export function createRoutingResource(c: CoreClient) {
  return {
    stats: (taskType?: string, tier?: string) => {
      const params = new URLSearchParams();
      if (taskType) params.set("task_type", taskType);
      if (tier) params.set("tier", tier);
      const qs = params.toString();
      return c.get<ModelPerformanceStats[]>(`/routing/stats${qs ? `?${qs}` : ""}`);
    },

    refreshStats: () => c.post<{ status: string }>("/routing/stats/refresh"),

    outcomes: (limit?: number) =>
      c.get<RoutingOutcome[]>(`/routing/outcomes${limit ? `?limit=${limit}` : ""}`),

    recordOutcome: (data: Omit<RoutingOutcome, "id" | "created_at">) =>
      c.post<RoutingOutcome>("/routing/outcomes", data),

    seedFromBenchmarks: () =>
      c.post<{ status: string; outcomes_created: number }>("/routing/seed-from-benchmarks"),
  };
}
