import { VisArea, VisAxis, VisXYContainer } from "@unovis/solid";
import { CurveType } from "@unovis/ts";
import { type Component } from "solid-js";

import type { DailyCost } from "~/api/types";

interface CostTrendChartProps {
  data: DailyCost[];
}

const CostTrendChart: Component<CostTrendChartProps> = (props) => {
  return (
    <div class="h-64">
      <VisXYContainer data={props.data} height={250}>
        <VisArea
          x={(_d: DailyCost, i: number) => i}
          y={(d: DailyCost) => d.cost_usd}
          curveType={CurveType.MonotoneX}
          color="var(--cf-accent)"
          opacity={0.7}
        />
        <VisAxis
          type="x"
          tickFormat={(tick: number | Date) => {
            const idx = typeof tick === "number" ? tick : 0;
            const d = props.data[idx];
            return d ? d.date.slice(5) : "";
          }}
          label="Date"
          numTicks={Math.min(props.data.length, 7)}
        />
        <VisAxis type="y" label="Cost ($)" />
      </VisXYContainer>
    </div>
  );
};

export default CostTrendChart;
