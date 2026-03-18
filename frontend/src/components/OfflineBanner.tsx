import { createEffect, createSignal, type JSX, onCleanup, onMount, Show } from "solid-js";

import { useI18n } from "~/i18n";

/**
 * Displays a banner when the browser is offline or the WebSocket is disconnected.
 * Uses `navigator.onLine` + event listeners for detection.
 *
 * WebSocket disconnects are debounced by 2 seconds to avoid flashing the banner
 * during brief reconnections. The banner is also suppressed for the first 3 seconds
 * after mount so initial connection setup does not trigger a flash.
 */
export function OfflineBanner(props: { wsConnected: () => boolean }): JSX.Element {
  const { t } = useI18n();
  const [online, setOnline] = createSignal(navigator.onLine);
  const [showWsBanner, setShowWsBanner] = createSignal(false);
  let debounceTimer: ReturnType<typeof setTimeout> | undefined;
  const mountedAt = Date.now();

  onMount(() => {
    const goOnline = () => setOnline(true);
    const goOffline = () => setOnline(false);

    window.addEventListener("online", goOnline);
    window.addEventListener("offline", goOffline);

    onCleanup(() => {
      window.removeEventListener("online", goOnline);
      window.removeEventListener("offline", goOffline);
    });
  });

  createEffect(() => {
    const disconnected = !props.wsConnected();
    if (disconnected) {
      if (Date.now() - mountedAt < 3000) return; // suppress during initial load
      debounceTimer = setTimeout(() => setShowWsBanner(true), 2000);
    } else {
      clearTimeout(debounceTimer);
      setShowWsBanner(false);
    }
  });

  onCleanup(() => clearTimeout(debounceTimer));

  const showBanner = () => !online() || showWsBanner();

  const label = () => {
    if (!online()) return t("offline.network");
    if (showWsBanner()) return t("offline.websocket");
    return "";
  };

  return (
    <Show when={showBanner()}>
      <div
        role="alert"
        class="flex items-center gap-2 bg-cf-warning-bg border-b border-cf-warning-border px-4 py-2 text-sm font-medium text-cf-warning-fg"
      >
        <span class="inline-block h-2 w-2 animate-pulse rounded-full bg-white" aria-hidden="true" />
        {label()}
      </div>
    </Show>
  );
}
