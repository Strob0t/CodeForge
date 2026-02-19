import { createSignal, type JSX, onCleanup, onMount, Show } from "solid-js";

import { useI18n } from "~/i18n";

/**
 * Displays a banner when the browser is offline or the WebSocket is disconnected.
 * Uses `navigator.onLine` + event listeners for detection.
 */
export function OfflineBanner(props: { wsConnected: () => boolean }): JSX.Element {
  const { t } = useI18n();
  const [online, setOnline] = createSignal(navigator.onLine);

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

  const showBanner = () => !online() || !props.wsConnected();

  const label = () => {
    if (!online()) return t("offline.network");
    if (!props.wsConnected()) return t("offline.websocket");
    return "";
  };

  return (
    <Show when={showBanner()}>
      <div
        role="alert"
        class="flex items-center gap-2 bg-yellow-500 px-4 py-2 text-sm font-medium text-white"
      >
        <span class="inline-block h-2 w-2 animate-pulse rounded-full bg-white" aria-hidden="true" />
        {label()}
      </div>
    </Show>
  );
}
