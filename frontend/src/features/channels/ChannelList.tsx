import { createResource, For, Show } from "solid-js";

import { api } from "~/api/client";
import { useSidebar } from "~/components/SidebarProvider";

export default function ChannelList() {
  const [channels] = createResource(() => api.channels.list());
  const { collapsed } = useSidebar();

  return (
    <Show when={!collapsed()}>
      <div class="px-2 py-1">
        <div class="flex items-center justify-between px-2 py-1">
          <span class="text-xs font-semibold uppercase tracking-wider text-cf-text-muted">
            Channels
          </span>
        </div>
        <Show
          when={channels()}
          fallback={<div class="px-2 py-1 text-xs text-cf-text-muted">Loading...</div>}
        >
          {(list) => (
            <For
              each={list()}
              fallback={<div class="px-2 py-1 text-xs text-cf-text-muted">No channels</div>}
            >
              {(ch) => (
                <a
                  href={`/channels/${ch.id}`}
                  class="flex items-center gap-2 rounded-cf-sm px-2 py-1.5 text-sm text-cf-text-secondary hover:bg-cf-bg-surface-alt hover:text-cf-text-primary transition-colors"
                >
                  <span class="text-cf-text-muted">{ch.type === "project" ? "#" : ">"}</span>
                  <span class="truncate">{ch.name}</span>
                </a>
              )}
            </For>
          )}
        </Show>
      </div>
    </Show>
  );
}
