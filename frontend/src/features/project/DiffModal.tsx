import { For, Show } from "solid-js";

import { Modal } from "~/ui/composites/Modal";

import type { DiffHunk } from "./DiffView";

interface DiffModalProps {
  path: string;
  hunks: DiffHunk[];
  open: boolean;
  onClose: () => void;
  onAccept?: () => void;
  onReject?: () => void;
}

export default function DiffModal(props: DiffModalProps) {
  return (
    <Modal
      open={props.open}
      onClose={props.onClose}
      title={props.path}
      class="w-[90vw] max-w-5xl max-h-[80vh] flex flex-col"
    >
      {/* Side-by-side content */}
      <div class="flex-1 overflow-auto -mx-4 -mt-4">
        <For each={props.hunks}>
          {(hunk) => {
            const oldLines = hunk.old_content ? hunk.old_content.split("\n") : [];
            const newLines = hunk.new_content ? hunk.new_content.split("\n") : [];
            const maxLines = Math.max(oldLines.length, newLines.length);

            return (
              <div>
                <div class="bg-cf-bg-inset px-3 py-1 text-[10px] text-cf-text-muted border-b border-cf-border font-mono">
                  @@ -{hunk.old_start},{hunk.old_lines} +{hunk.new_start},{hunk.new_lines} @@
                </div>
                <div class="grid grid-cols-2 divide-x divide-cf-border text-xs font-mono">
                  {/* Old (left) */}
                  <div>
                    {Array.from({ length: maxLines }, (_, i) => {
                      const line = oldLines[i];
                      return (
                        <div class={`flex ${line !== undefined ? "bg-red-500/10" : ""}`}>
                          <span class="w-10 text-right pr-2 text-cf-text-muted/50 select-none flex-shrink-0">
                            {line !== undefined ? hunk.old_start + i : ""}
                          </span>
                          <span class="text-red-400 whitespace-pre-wrap break-all flex-1 px-1">
                            {line !== undefined ? `-${line}` : ""}
                          </span>
                        </div>
                      );
                    })}
                  </div>
                  {/* New (right) */}
                  <div>
                    {Array.from({ length: maxLines }, (_, i) => {
                      const line = newLines[i];
                      return (
                        <div class={`flex ${line !== undefined ? "bg-green-500/10" : ""}`}>
                          <span class="w-10 text-right pr-2 text-cf-text-muted/50 select-none flex-shrink-0">
                            {line !== undefined ? hunk.new_start + i : ""}
                          </span>
                          <span class="text-green-400 whitespace-pre-wrap break-all flex-1 px-1">
                            {line !== undefined ? `+${line}` : ""}
                          </span>
                        </div>
                      );
                    })}
                  </div>
                </div>
              </div>
            );
          }}
        </For>
      </div>

      {/* Footer with buttons */}
      <div class="flex justify-end gap-2 border-t border-cf-border -mx-4 px-4 py-3 -mb-4">
        <Show when={props.onReject}>
          <button
            class="px-3 py-1.5 rounded-cf-sm bg-red-600 text-white text-sm font-medium hover:bg-red-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cf-focus-ring focus-visible:ring-offset-2"
            onClick={() => props.onReject?.()}
          >
            Reject
          </button>
        </Show>
        <Show when={props.onAccept}>
          <button
            class="px-3 py-1.5 rounded-cf-sm bg-green-600 text-white text-sm font-medium hover:bg-green-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cf-focus-ring focus-visible:ring-offset-2"
            onClick={() => props.onAccept?.()}
          >
            Accept
          </button>
        </Show>
        <button
          class="px-3 py-1.5 rounded-cf-sm bg-cf-bg-surface border border-cf-border text-cf-text-primary text-sm font-medium hover:bg-cf-bg-inset focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cf-focus-ring focus-visible:ring-offset-2"
          onClick={() => props.onClose()}
        >
          Close
        </button>
      </div>
    </Modal>
  );
}
