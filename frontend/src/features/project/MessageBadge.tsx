import { Show } from "solid-js";

import { getProvider } from "~/utils/providerMap";

interface MessageBadgeProps {
  model?: string;
  costUsd?: number;
  tokensIn?: number;
  tokensOut?: number;
}

export default function MessageBadge(props: MessageBadgeProps) {
  const provider = () => (props.model ? getProvider(props.model) : "");
  const costLabel = () => {
    if (props.costUsd === undefined || props.costUsd === 0) return "";
    return props.costUsd < 0.01
      ? `$${(props.costUsd * 100).toFixed(2)}\u00A2`
      : `$${props.costUsd.toFixed(4)}`;
  };

  return (
    <Show when={props.model}>
      <div class="flex items-center gap-1.5 text-[10px] text-cf-text-muted mt-1 select-none">
        <span class="font-mono">{props.model}</span>
        <Show when={provider()}>
          <span class="opacity-50">{"\u00B7"}</span>
          <span>{provider()}</span>
        </Show>
        <Show when={costLabel()}>
          <span class="opacity-50">{"\u00B7"}</span>
          <span class="text-cf-accent">{costLabel()}</span>
        </Show>
        <Show when={props.tokensIn || props.tokensOut}>
          <span class="opacity-50">{"\u00B7"}</span>
          <span>
            {(props.tokensIn ?? 0).toLocaleString()}
            {"\u2193"} {(props.tokensOut ?? 0).toLocaleString()}
            {"\u2191"}
          </span>
        </Show>
      </div>
    </Show>
  );
}
