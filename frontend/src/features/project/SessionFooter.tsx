import { Show } from "solid-js";

import { getProvider } from "~/utils/providerMap";

import ContextGauge from "./ContextGauge";

interface SessionFooterProps {
  model?: string;
  steps: number;
  costUsd: number;
  tokensUsed: number;
  tokensTotal: number;
  visible: boolean;
}

export default function SessionFooter(props: SessionFooterProps) {
  return (
    <Show when={props.visible}>
      <div class="flex items-center gap-3 border-t border-cf-border px-3 py-1.5 text-[10px] text-cf-text-muted select-none">
        <Show when={props.model}>
          <span class="font-mono">{props.model}</span>
          <span class="opacity-40">&middot;</span>
          <span>{getProvider(props.model ?? "")}</span>
        </Show>
        <Show when={props.steps > 0}>
          <span class="opacity-40">&middot;</span>
          <span>{props.steps} steps</span>
        </Show>
        <Show when={props.costUsd > 0}>
          <span class="opacity-40">&middot;</span>
          <span class="text-cf-accent">${props.costUsd.toFixed(4)}</span>
        </Show>
        <div class="ml-auto">
          <ContextGauge used={props.tokensUsed} total={props.tokensTotal} />
        </div>
      </div>
    </Show>
  );
}
