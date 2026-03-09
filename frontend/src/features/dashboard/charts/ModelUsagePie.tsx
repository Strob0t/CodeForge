import { VisDonut, VisSingleContainer } from "@unovis/solid";
import { type Component, For } from "solid-js";

import type { ModelUsage } from "~/api/types";

interface ModelUsagePieProps {
  data: ModelUsage[];
}

const ModelUsagePie: Component<ModelUsagePieProps> = (props) => {
  return (
    <div class="h-64">
      <VisSingleContainer data={props.data} height={250}>
        <VisDonut value={(d: ModelUsage) => d.cost_usd} arcWidth={0} />
      </VisSingleContainer>
      <div class="mt-2 flex flex-wrap justify-center gap-3 text-xs">
        <For each={props.data}>
          {(d) => (
            <span class="flex items-center gap-1 text-[var(--cf-text-secondary)]">
              {d.model}: ${d.cost_usd.toFixed(2)}
            </span>
          )}
        </For>
      </div>
    </div>
  );
};

export default ModelUsagePie;
