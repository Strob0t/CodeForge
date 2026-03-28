import { createSignal, type JSX, onCleanup, onMount } from "solid-js";

import { useI18n } from "~/i18n";
import type { KeyCombo } from "~/shortcuts";

// ---------------------------------------------------------------------------
// ShortcutRecorder — captures a single key combo from the user
// ---------------------------------------------------------------------------

interface Props {
  onRecord: (combo: KeyCombo) => void;
  onCancel: () => void;
}

export function ShortcutRecorder(props: Props): JSX.Element {
  const { t } = useI18n();
  const [waiting, setWaiting] = createSignal(true);
  let ref: HTMLDivElement | undefined;

  function handleKeydown(e: KeyboardEvent): void {
    e.preventDefault();
    e.stopPropagation();

    // Escape cancels
    if (e.key === "Escape") {
      props.onCancel();
      return;
    }

    // Ignore modifier-only presses
    if (["Control", "Meta", "Shift", "Alt"].includes(e.key)) return;

    setWaiting(false);
    props.onRecord({
      mod: e.metaKey || e.ctrlKey,
      shift: e.shiftKey,
      alt: e.altKey,
      key: e.key,
    });
  }

  onMount(() => {
    ref?.focus();
    // Use capture phase to intercept before other handlers
    document.addEventListener("keydown", handleKeydown, true);
  });

  onCleanup(() => {
    document.removeEventListener("keydown", handleKeydown, true);
  });

  return (
    <div
      ref={ref}
      role="textbox"
      tabIndex={0}
      class="inline-flex items-center rounded-cf-sm border-2 border-cf-accent bg-cf-accent/10 px-2 py-1 text-xs font-medium text-cf-accent outline-none animate-pulse"
      aria-live="polite"
      aria-label={t("settings.shortcuts.recording")}
    >
      {waiting() ? t("settings.shortcuts.recording") : "..."}
    </div>
  );
}
