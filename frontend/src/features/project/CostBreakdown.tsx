import { createSignal, Show } from "solid-js";

import { Button } from "~/ui";

interface CostBreakdownProps {
  costUsd: number;
  tokensIn: number;
  tokensOut: number;
  steps: number;
  model?: string;
}

export default function CostBreakdown(props: CostBreakdownProps) {
  const [expanded, setExpanded] = createSignal(false);

  return (
    <div class="text-xs">
      <Button
        variant="ghost"
        size="xs"
        class="text-cf-text-muted"
        onClick={() => setExpanded(!expanded())}
      >
        {expanded() ? "\u25BE" : "\u25B8"} Cost: ${props.costUsd.toFixed(4)}
      </Button>
      <Show when={expanded()}>
        <div class="ml-4 mt-1 space-y-0.5 text-cf-text-muted">
          <div>Input: {props.tokensIn.toLocaleString()} tokens</div>
          <div>Output: {props.tokensOut.toLocaleString()} tokens</div>
          <div>Steps: {props.steps}</div>
          <Show when={props.model}>
            <div>Model: {props.model}</div>
          </Show>
        </div>
      </Show>
    </div>
  );
}
