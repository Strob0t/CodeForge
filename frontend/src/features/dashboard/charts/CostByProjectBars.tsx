import { VisAxis, VisStackedBar, VisXYContainer } from "@unovis/solid";
import { Orientation } from "@unovis/ts";
import { type Component } from "solid-js";

import type { ProjectCostBar } from "~/api/types";

interface CostByProjectBarsProps {
  data: ProjectCostBar[];
}

const CostByProjectBars: Component<CostByProjectBarsProps> = (props) => {
  return (
    <div class="h-64">
      <VisXYContainer data={props.data} height={250}>
        <VisStackedBar
          x={(_d: ProjectCostBar, i: number) => i}
          y={[(d: ProjectCostBar) => d.cost_usd]}
          orientation={Orientation.Horizontal}
          roundedCorners={4}
          color="var(--cf-accent)"
        />
        <VisAxis type="x" label="Cost ($)" />
        <VisAxis
          type="y"
          tickFormat={(tick: number | Date) => {
            const idx = typeof tick === "number" ? tick : 0;
            const d = props.data[idx];
            return d ? d.project_name || "Unknown" : "";
          }}
        />
      </VisXYContainer>
    </div>
  );
};

export default CostByProjectBars;
