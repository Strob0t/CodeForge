import { For } from "solid-js";

export interface DiffHunk {
  old_start: number;
  old_lines: number;
  new_start: number;
  new_lines: number;
  old_content: string;
  new_content: string;
}

interface DiffViewProps {
  path: string;
  hunks: DiffHunk[];
}

export default function DiffView(props: DiffViewProps) {
  return (
    <div class="rounded-cf-sm border border-cf-border overflow-hidden text-xs font-mono">
      <div class="bg-cf-bg-inset px-3 py-1 text-cf-text-muted border-b border-cf-border">
        {props.path}
      </div>
      <For each={props.hunks}>
        {(hunk) => {
          const oldLines = hunk.old_content ? hunk.old_content.split("\n") : [];
          const newLines = hunk.new_content ? hunk.new_content.split("\n") : [];

          return (
            <div>
              <div class="bg-cf-bg-inset px-3 py-0.5 text-cf-text-muted text-[10px] border-b border-cf-border">
                @@ -{hunk.old_start},{hunk.old_lines} +{hunk.new_start},{hunk.new_lines} @@
              </div>
              <For each={oldLines}>
                {(line, i) => (
                  <div class="flex bg-red-500/10">
                    <span class="w-10 text-right pr-2 text-red-400/60 select-none flex-shrink-0">
                      {hunk.old_start + i()}
                    </span>
                    <span class="text-red-400 whitespace-pre-wrap break-all flex-1 px-1">
                      -{line}
                    </span>
                  </div>
                )}
              </For>
              <For each={newLines}>
                {(line, i) => (
                  <div class="flex bg-green-500/10">
                    <span class="w-10 text-right pr-2 text-green-400/60 select-none flex-shrink-0">
                      {hunk.new_start + i()}
                    </span>
                    <span class="text-green-400 whitespace-pre-wrap break-all flex-1 px-1">
                      +{line}
                    </span>
                  </div>
                )}
              </For>
            </div>
          );
        }}
      </For>
    </div>
  );
}
