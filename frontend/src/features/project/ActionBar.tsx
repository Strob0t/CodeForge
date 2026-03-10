import { For, Show } from "solid-js";

import type { ActionRule } from "./actionRules";

interface ActionBarProps {
  rules: ActionRule[];
  agentSuggestions?: ActionRule[];
  onAction: (action: ActionRule) => void;
}

export default function ActionBar(props: ActionBarProps) {
  const allActions = () => {
    const seen = new Set<string>();
    const result: ActionRule[] = [];
    // Agent suggestions first (higher priority)
    for (const s of props.agentSuggestions ?? []) {
      if (!seen.has(s.label)) {
        seen.add(s.label);
        result.push(s);
      }
    }
    // Then rule-based, deduplicated
    for (const r of props.rules) {
      if (!seen.has(r.label)) {
        seen.add(r.label);
        result.push(r);
      }
    }
    return result;
  };

  return (
    <Show when={allActions().length > 0}>
      <div class="flex flex-wrap gap-1.5 px-2 py-1.5">
        <For each={allActions()}>
          {(action) => (
            <button
              class="rounded-cf-sm border border-cf-border bg-cf-bg-surface px-2.5 py-1 text-xs text-cf-text-primary hover:bg-cf-bg-inset hover:border-cf-accent transition-colors"
              onClick={() => props.onAction(action)}
            >
              {action.label}
            </button>
          )}
        </For>
      </div>
    </Show>
  );
}
