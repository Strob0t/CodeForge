import { For, Show } from "solid-js";

export interface DiffHunk {
  old_start: number;
  old_lines: number;
  new_start: number;
  new_lines: number;
  old_content: string;
  new_content: string;
}

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
    <Show when={props.open}>
      <div
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
        onClick={(e) => {
          if (e.target === e.currentTarget) props.onClose();
        }}
      >
        <div class="bg-cf-bg-surface rounded-cf-md shadow-lg w-[90vw] max-w-5xl max-h-[80vh] flex flex-col">
          {/* Header */}
          <div class="flex items-center justify-between border-b border-cf-border px-4 py-3">
            <span class="font-mono text-sm text-cf-text-primary">{props.path}</span>
            <button
              class="text-cf-text-muted hover:text-cf-text-primary text-lg leading-none"
              onClick={() => props.onClose()}
            >
              {"\u2715"}
            </button>
          </div>

          {/* Side-by-side content */}
          <div class="flex-1 overflow-auto">
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
          <div class="flex justify-end gap-2 border-t border-cf-border px-4 py-3">
            <Show when={props.onReject}>
              <button
                class="px-3 py-1.5 rounded-cf-sm bg-red-600 text-white text-sm font-medium hover:bg-red-700"
                onClick={() => props.onReject?.()}
              >
                Reject
              </button>
            </Show>
            <Show when={props.onAccept}>
              <button
                class="px-3 py-1.5 rounded-cf-sm bg-green-600 text-white text-sm font-medium hover:bg-green-700"
                onClick={() => props.onAccept?.()}
              >
                Accept
              </button>
            </Show>
            <button
              class="px-3 py-1.5 rounded-cf-sm bg-cf-bg-surface border border-cf-border text-cf-text-primary text-sm font-medium hover:bg-cf-bg-inset"
              onClick={() => props.onClose()}
            >
              Close
            </button>
          </div>
        </div>
      </div>
    </Show>
  );
}
