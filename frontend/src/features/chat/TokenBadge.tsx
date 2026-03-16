import type { Component } from "solid-js";
import { Show } from "solid-js";

interface TokenBadgeProps {
  type: "@" | "#";
  label: string;
  onRemove?: () => void;
}

const TokenBadge: Component<TokenBadgeProps> = (props) => {
  const colorClasses = () =>
    props.type === "@" ? "bg-cf-info-bg text-cf-info-fg" : "bg-cf-info-bg text-cf-info-fg";

  return (
    <span
      class={`inline-flex items-center rounded-full px-2 py-0.5 text-xs gap-1 ${colorClasses()}`}
    >
      <span>
        {props.type}
        {props.label}
      </span>
      <Show when={props.onRemove}>
        <button
          type="button"
          class="ml-0.5 inline-flex items-center justify-center rounded-full hover:opacity-70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cf-focus-ring focus-visible:ring-offset-2"
          aria-label={`Remove ${props.label}`}
          onClick={() => props.onRemove?.()}
        >
          &times;
        </button>
      </Show>
    </span>
  );
};

export default TokenBadge;
