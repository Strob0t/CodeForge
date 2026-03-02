import { createSignal, For, onCleanup, Show } from "solid-js";

import { createCodeForgeWS } from "~/api/websocket";

interface ContextItem {
  key: string;
  author: string;
  version: number;
}

export default function SharedContextPanel() {
  const { onMessage } = createCodeForgeWS();
  const [items, setItems] = createSignal<ContextItem[]>([]);
  const [collapsed, setCollapsed] = createSignal(true);

  const cleanup = onMessage((msg) => {
    if (msg.type !== "shared.updated") return;
    const p = msg.payload;
    const key = p.key as string;
    const author = p.author as string;
    const version = p.version as number;

    setItems((prev) => {
      const idx = prev.findIndex((i) => i.key === key);
      if (idx >= 0) {
        const updated = [...prev];
        updated[idx] = { key, author, version };
        return updated;
      }
      return [...prev, { key, author, version }];
    });
  });
  onCleanup(cleanup);

  return (
    <div class="border-t border-cf-border bg-cf-bg-secondary">
      <button
        type="button"
        class="w-full flex items-center justify-between px-4 py-2 text-sm font-medium text-cf-text-secondary hover:bg-cf-bg-tertiary transition-colors"
        onClick={() => setCollapsed(!collapsed())}
      >
        <span>Shared Context ({items().length})</span>
        <svg
          class={`h-4 w-4 transition-transform ${collapsed() ? "" : "rotate-180"}`}
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
          stroke-width="2"
        >
          <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
        </svg>
      </button>
      <Show when={!collapsed()}>
        <div class="px-4 pb-3">
          <Show
            when={items().length > 0}
            fallback={<p class="text-xs text-cf-text-muted py-2">No shared context items yet.</p>}
          >
            <div class="space-y-1">
              <For each={items()}>
                {(item) => (
                  <div class="flex items-center justify-between text-xs py-1">
                    <span class="font-mono text-cf-text-primary">{item.key}</span>
                    <span class="text-cf-text-muted">
                      {item.author} (v{item.version})
                    </span>
                  </div>
                )}
              </For>
            </div>
          </Show>
        </div>
      </Show>
    </div>
  );
}
