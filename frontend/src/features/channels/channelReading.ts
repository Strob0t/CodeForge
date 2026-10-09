import { createSignal } from "solid-js";

/**
 * Reading state shared by the open channel view and the channel list.
 *
 * `followedChannel` is the channel whose newest messages the reader sees: it
 * is open and scrolled to the bottom. The list does not count its new
 * messages (the view marks them read); a channel that is open but scrolled up
 * counts like any other, as on the server.
 */
const [followedChannel, setFollowedChannel] = createSignal<string | undefined>();

/**
 * Bumped when this tab marked a channel read and no channel.read event will
 * follow (the server does not track a read position for a caller without an
 * account), so the list refetches its counts.
 */
const [untrackedReads, setUntrackedReads] = createSignal(0);

export function noteUntrackedRead(): void {
  setUntrackedReads((n) => n + 1);
}

export { followedChannel, setFollowedChannel, untrackedReads };
