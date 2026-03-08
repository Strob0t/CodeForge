import { type JSX, Show, splitProps } from "solid-js";
import { Portal } from "solid-js/web";

import { useFocusTrap } from "~/hooks/useFocusTrap";
import { cx } from "~/utils/cx";

import { Button } from "../primitives/Button";

export interface ModalProps {
  open: boolean;
  onClose: () => void;
  title?: string;
  class?: string;
  children: JSX.Element;
}

export function Modal(props: ModalProps): JSX.Element {
  const [local] = splitProps(props, ["open", "onClose", "title", "class", "children"]);
  let dialogRef: HTMLDivElement | undefined;

  const { onKeyDown: trapKeyDown } = useFocusTrap(
    () => dialogRef,
    () => local.open,
  );

  function handleKeyDown(e: KeyboardEvent) {
    if (e.key === "Escape") {
      e.stopPropagation();
      local.onClose();
      return;
    }
    trapKeyDown(e);
  }

  function handleBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) {
      local.onClose();
    }
  }

  return (
    <Show when={local.open}>
      <Portal>
        <div
          class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
          role="dialog"
          aria-modal="true"
          aria-label={local.title}
          onKeyDown={handleKeyDown}
          onClick={handleBackdropClick}
        >
          <div
            ref={dialogRef}
            class={cx(
              "relative mx-3 sm:mx-4 max-h-[85vh] w-full max-w-lg overflow-auto rounded-cf-lg border border-cf-border bg-cf-bg-surface shadow-cf-lg",
              local.class,
            )}
          >
            <Show when={local.title}>
              <div class="flex items-center justify-between border-b border-cf-border px-4 py-3">
                <h2 class="text-lg font-semibold text-cf-text-primary">{local.title}</h2>
                <Button variant="icon" size="xs" onClick={() => local.onClose()} aria-label="Close">
                  {"\u2715"}
                </Button>
              </div>
            </Show>
            <div class="p-4 pb-[max(1rem,env(safe-area-inset-bottom))]">{local.children}</div>
          </div>
        </div>
      </Portal>
    </Show>
  );
}
