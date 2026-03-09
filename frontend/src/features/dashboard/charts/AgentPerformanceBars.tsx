import { VisAxis, VisGroupedBar, VisXYContainer } from "@unovis/solid";
import { Orientation } from "@unovis/ts";
import { type Component } from "solid-js";

import type { AgentPerf } from "~/api/types";

interface AgentPerformanceBarsProps {
  data: AgentPerf[];
}

const AgentPerformanceBars: Component<AgentPerformanceBarsProps> = (props) => {
  return (
    <div class="h-64">
      <VisXYContainer data={props.data} height={250}>
        <VisGroupedBar
          x={(_d: AgentPerf, i: number) => i}
          y={[(d: AgentPerf) => d.success_rate]}
          orientation={Orientation.Horizontal}
          roundedCorners={4}
          color="var(--cf-accent)"
        />
        <VisAxis type="x" label="Success Rate (%)" />
        <VisAxis
          type="y"
          tickFormat={(tick: number | Date) => {
            const idx = typeof tick === "number" ? tick : 0;
            const d = props.data[idx];
            return d ? d.agent_name : "";
          }}
        />
      </VisXYContainer>
    </div>
  );
};

export default AgentPerformanceBars;
