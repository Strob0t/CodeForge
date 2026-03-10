import { createEffect, For, type JSX, onCleanup, Show } from "solid-js";

import type { FileEntry } from "~/api/types";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface ContextMenuAction {
  label: string;
  icon: string;
  action: string;
}

export interface FileContextMenuProps {
  visible: boolean;
  x: number;
  y: number;
  entry: FileEntry | null;
  /** When true, show root-level actions (New File / New Folder only). */
  isRootArea: boolean;
  onAction: (action: string) => void;
  onClose: () => void;
}

// ---------------------------------------------------------------------------
// Menu item definitions
// ---------------------------------------------------------------------------

const FOLDER_ACTIONS: ContextMenuAction[] = [
  { label: "New File", icon: "+", action: "new-file" },
  { label: "New Folder", icon: "\u25A1", action: "new-folder" },
  { label: "Upload File", icon: "\u2191", action: "upload" },
  { label: "Rename", icon: "\u270E", action: "rename" },
  { label: "Delete Folder", icon: "\u2716", action: "delete" },
];

const FILE_ACTIONS: ContextMenuAction[] = [
  { label: "Rename", icon: "\u270E", action: "rename" },
  { label: "Delete File", icon: "\u2716", action: "delete" },
];

const ROOT_ACTIONS: ContextMenuAction[] = [
  { label: "New File", icon: "+", action: "new-file" },
  { label: "New Folder", icon: "\u25A1", action: "new-folder" },
];

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function FileContextMenu(props: FileContextMenuProps): JSX.Element {
  let menuRef: HTMLDivElement | undefined;

  // Close on click-outside
  createEffect(() => {
    if (!props.visible) return;

    function handleClickOutside(e: MouseEvent) {
      if (menuRef && !menuRef.contains(e.target as Node)) {
        props.onClose();
      }
    }

    function handleEscape(e: KeyboardEvent) {
      if (e.key === "Escape") {
        props.onClose();
      }
    }

    // Use setTimeout so the current contextmenu event does not immediately close
    const timerId = setTimeout(() => {
      document.addEventListener("mousedown", handleClickOutside);
      document.addEventListener("keydown", handleEscape);
    }, 0);

    onCleanup(() => {
      clearTimeout(timerId);
      document.removeEventListener("mousedown", handleClickOutside);
      document.removeEventListener("keydown", handleEscape);
    });
  });

  const actions = (): ContextMenuAction[] => {
    if (props.isRootArea) return ROOT_ACTIONS;
    if (!props.entry) return [];
    return props.entry.is_dir ? FOLDER_ACTIONS : FILE_ACTIONS;
  };

  // Ensure the menu does not overflow the viewport
  const adjustedPosition = (): { top: string; left: string } => {
    const menuWidth = 180;
    const menuHeight = actions().length * 32 + 8; // approximate
    const vw = window.innerWidth;
    const vh = window.innerHeight;

    const left = props.x + menuWidth > vw ? vw - menuWidth - 4 : props.x;
    const top = props.y + menuHeight > vh ? vh - menuHeight - 4 : props.y;

    return {
      top: `${Math.max(0, top)}px`,
      left: `${Math.max(0, left)}px`,
    };
  };

  return (
    <Show when={props.visible}>
      <div
        ref={menuRef}
        class="fixed z-50 min-w-[160px] bg-cf-bg-surface border border-cf-border shadow-cf-lg rounded-cf-md py-1 select-none"
        style={adjustedPosition()}
      >
        <For each={actions()}>
          {(item) => (
            <button
              type="button"
              class="flex items-center gap-2 w-full text-left text-sm text-cf-text-primary px-3 py-1.5 hover:bg-cf-bg-surface-alt transition-colors"
              onClick={() => {
                props.onAction(item.action);
              }}
            >
              <span class="w-4 text-center text-xs text-cf-text-muted flex-shrink-0">
                {item.icon}
              </span>
              <span>{item.label}</span>
            </button>
          )}
        </For>
      </div>
    </Show>
  );
}
