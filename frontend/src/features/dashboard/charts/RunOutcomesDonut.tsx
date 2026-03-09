import { VisDonut, VisSingleContainer } from "@unovis/solid";
import { type Component, For } from "solid-js";

import type { RunOutcome } from "~/api/types";

const statusColor: Record<string, string> = {
  completed: "var(--cf-success)",
  failed: "var(--cf-danger)",
  timeout: "var(--cf-warning)",
  cancelled: "var(--cf-text-muted)",
  running: "var(--cf-accent)",
};

interface RunOutcomesDonutProps {
  data: RunOutcome[];
}

const RunOutcomesDonut: Component<RunOutcomesDonutProps> = (props) => {
  const total = () => props.data.reduce((sum, d) => sum + d.count, 0);

  return (
    <div class="h-64">
      <VisSingleContainer data={props.data} height={250}>
        <VisDonut
          value={(d: RunOutcome) => d.count}
          arcWidth={40}
          centralLabel={String(total())}
          centralSubLabel="runs"
          color={(d: RunOutcome) => statusColor[d.status] ?? "var(--cf-text-muted)"}
        />
      </VisSingleContainer>
      <div class="mt-2 flex flex-wrap justify-center gap-3 text-xs">
        <For each={props.data}>
          {(d) => (
            <span class="flex items-center gap-1">
              <span
                class="inline-block h-2.5 w-2.5 rounded-full"
                style={{ background: statusColor[d.status] ?? "var(--cf-text-muted)" }}
              />
              {d.status}: {d.count}
            </span>
          )}
        </For>
      </div>
    </div>
  );
};

export default RunOutcomesDonut;
