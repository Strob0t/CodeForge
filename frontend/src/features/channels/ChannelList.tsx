import { createResource, For, Show } from "solid-js";

import { api } from "~/api/client";
import { useSidebar } from "~/components/SidebarProvider";
import { NavSection } from "~/ui/layout";

export default function ChannelList() {
  const [channels] = createResource(() => api.channels.list());
  const { collapsed } = useSidebar();

  return (
    <NavSection label="Channels">
      <Show when={!collapsed()}>
        <div class="px-2 py-1">
          <Show
            when={channels()}
            fallback={<div class="px-2 py-1 text-xs text-cf-text-muted">Loading...</div>}
          >
            {(list) => (
              <ul class="list-none m-0 p-0">
                <For
                  each={list()}
                  fallback={<li class="px-2 py-1 text-xs text-cf-text-muted">No channels</li>}
                >
                  {(ch) => (
                    <li>
                      <a
                        href={`/channels/${ch.id}`}
                        class="flex items-center gap-2 rounded-cf-sm px-2 py-1.5 text-sm text-cf-text-secondary hover:bg-cf-bg-surface-alt hover:text-cf-text-primary transition-colors"
                      >
                        <span class="text-cf-text-muted">{ch.type === "project" ? "#" : ">"}</span>
                        <span class="truncate">{ch.name}</span>
                      </a>
                    </li>
                  )}
                </For>
              </ul>
            )}
          </Show>
        </div>
      </Show>
    </NavSection>
  );
}
