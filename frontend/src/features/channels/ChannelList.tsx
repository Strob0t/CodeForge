import { createEffect, createResource, createSignal, For, on, onCleanup, Show } from "solid-js";

import { api } from "~/api/client";
import { useAuth } from "~/components/AuthProvider";
import { useSidebar } from "~/components/SidebarProvider";
import { useWebSocket } from "~/components/WebSocketProvider";
import { NavSection } from "~/ui/layout";

import { applyUnreadEvent, parseChannelReadEvent } from "./channelEvents";
import { followedChannel, untrackedReads } from "./channelReading";

/** Delay before the counts are refetched after a read (bursts refetch once). */
const UNREAD_REFETCH_DELAY_MS = 300;

export default function ChannelList() {
  const [channels, { refetch }] = createResource(() => api.channels.list());
  const { collapsed } = useSidebar();
  const { user } = useAuth();

  // Unread counts start from the server's and follow the live messages; after
  // the user read a channel (in any session) they come from the server again.
  const [unread, setUnread] = createSignal<Readonly<Record<string, number>>>({});
  createEffect(() => {
    const list = channels();
    if (!list) return;
    setUnread(Object.fromEntries(list.map((ch) => [ch.id, ch.unread_count])));
  });

  let refetchTimer: ReturnType<typeof setTimeout> | undefined;
  function refetchCounts(): void {
    if (refetchTimer !== undefined) clearTimeout(refetchTimer);
    refetchTimer = setTimeout(() => {
      refetchTimer = undefined;
      void refetch();
    }, UNREAD_REFETCH_DELAY_MS);
  }
  onCleanup(() => {
    if (refetchTimer !== undefined) clearTimeout(refetchTimer);
  });
  createEffect(on(untrackedReads, refetchCounts, { defer: true }));

  const { onMessage } = useWebSocket();
  const unsubscribe = onMessage((msg) => {
    const read = parseChannelReadEvent(msg);
    if (read) {
      if (read.user_id === user()?.id) refetchCounts();
      return;
    }
    setUnread((counts) => applyUnreadEvent(counts, msg, user()?.id, followedChannel()));
  });
  onCleanup(unsubscribe);

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
                        <Show when={(unread()[ch.id] ?? 0) > 0}>
                          <span
                            class="ml-auto rounded-full bg-cf-accent px-1.5 text-xs font-semibold text-cf-accent-fg"
                            aria-label={`${unread()[ch.id]} unread`}
                          >
                            {unread()[ch.id]}
                          </span>
                        </Show>
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
