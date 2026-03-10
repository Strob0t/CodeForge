import type { Component } from "solid-js";
import { Show } from "solid-js";

interface TokenBadgeProps {
  type: "@" | "#";
  label: string;
  onRemove?: () => void;
}

const TokenBadge: Component<TokenBadgeProps> = (props) => {
  const colorClasses = () =>
    props.type === "@"
      ? "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300"
      : "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300";

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
          class="ml-0.5 inline-flex items-center justify-center rounded-full hover:opacity-70 focus:outline-none"
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
