import { createSignal, For, Show } from "solid-js";

import { useI18n } from "~/i18n";

interface DiffLine {
  type: "add" | "remove" | "context" | "header";
  content: string;
  oldNum?: number;
  newNum?: number;
}

interface DiffHunk {
  header: string;
  lines: DiffLine[];
}

interface DiffFile {
  oldPath: string;
  newPath: string;
  hunks: DiffHunk[];
}

/** Parse a unified diff string into structured files/hunks/lines */
function parseDiff(text: string): DiffFile[] {
  const files: DiffFile[] = [];
  const rawLines = text.split("\n");
  let current: DiffFile | null = null;
  let currentHunk: DiffHunk | null = null;
  let oldLine = 0;
  let newLine = 0;

  for (const line of rawLines) {
    if (line.startsWith("--- ")) {
      const path = line.slice(4).replace(/^a\//, "");
      current = { oldPath: path, newPath: "", hunks: [] };
      continue;
    }
    if (line.startsWith("+++ ")) {
      if (current) {
        current.newPath = line.slice(4).replace(/^b\//, "");
        files.push(current);
      }
      continue;
    }
    if (line.startsWith("@@ ")) {
      const match = line.match(/@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@(.*)/);
      oldLine = match ? parseInt(match[1], 10) : 0;
      newLine = match ? parseInt(match[2], 10) : 0;
      currentHunk = {
        header: line,
        lines: [{ type: "header", content: line }],
      };
      current?.hunks.push(currentHunk);
      continue;
    }
    if (!currentHunk) continue;

    if (line.startsWith("+")) {
      currentHunk.lines.push({ type: "add", content: line.slice(1), newNum: newLine++ });
    } else if (line.startsWith("-")) {
      currentHunk.lines.push({ type: "remove", content: line.slice(1), oldNum: oldLine++ });
    } else {
      currentHunk.lines.push({
        type: "context",
        content: line.startsWith(" ") ? line.slice(1) : line,
        oldNum: oldLine++,
        newNum: newLine++,
      });
    }
  }

  return files;
}

interface DiffPreviewProps {
  /** Unified diff text */
  diff: string;
  /** Max height in pixels (scrollable) */
  maxHeight?: number;
}

export function DiffPreview(props: DiffPreviewProps) {
  const { t } = useI18n();
  const [collapsed, setCollapsed] = createSignal<Record<number, boolean>>({});

  const files = () => parseDiff(props.diff);

  const toggleFile = (idx: number) => {
    setCollapsed((prev) => ({ ...prev, [idx]: !prev[idx] }));
  };

  const addCount = (file: DiffFile): number =>
    file.hunks.reduce((sum, h) => sum + h.lines.filter((l) => l.type === "add").length, 0);

  const removeCount = (file: DiffFile): number =>
    file.hunks.reduce((sum, h) => sum + h.lines.filter((l) => l.type === "remove").length, 0);

  return (
    <div
      class="overflow-auto rounded-cf-md border border-cf-border"
      style={{ "max-height": `${props.maxHeight ?? 400}px` }}
    >
      <Show
        when={files().length > 0}
        fallback={<p class="p-3 text-xs text-cf-text-tertiary">{t("diff.noDiff")}</p>}
      >
        <For each={files()}>
          {(file, idx) => (
            <div class="border-b border-cf-border last:border-b-0">
              {/* File header */}
              <button
                type="button"
                class="flex w-full items-center gap-2 bg-cf-bg-surface-alt px-3 py-2 text-left text-xs hover:bg-cf-bg-inset"
                onClick={() => toggleFile(idx())}
                aria-expanded={!collapsed()[idx()]}
              >
                <span class="font-mono font-medium text-cf-text-secondary">
                  {file.newPath || file.oldPath}
                </span>
                <span class="flex-1" />
                <span class="text-cf-success">+{addCount(file)}</span>
                <span class="text-cf-danger">-{removeCount(file)}</span>
                <span class="text-cf-text-muted">{collapsed()[idx()] ? "\u25B6" : "\u25BC"}</span>
              </button>

              {/* Hunks */}
              <Show when={!collapsed()[idx()]}>
                <div class="overflow-x-auto font-mono text-xs leading-5">
                  <For each={file.hunks}>
                    {(hunk) => (
                      <For each={hunk.lines}>
                        {(line) => {
                          const bg =
                            line.type === "add"
                              ? "bg-cf-success-bg"
                              : line.type === "remove"
                                ? "bg-cf-danger-bg"
                                : line.type === "header"
                                  ? "bg-cf-info-bg"
                                  : "";
                          const textColor =
                            line.type === "add"
                              ? "text-cf-success-fg"
                              : line.type === "remove"
                                ? "text-cf-danger-fg"
                                : line.type === "header"
                                  ? "text-cf-accent"
                                  : "text-cf-text-tertiary";
                          const prefix =
                            line.type === "add"
                              ? "+"
                              : line.type === "remove"
                                ? "-"
                                : line.type === "header"
                                  ? ""
                                  : " ";

                          return (
                            <div class={`flex ${bg}`}>
                              <Show when={line.type !== "header"}>
                                <span class="w-10 flex-shrink-0 select-none text-right text-cf-text-muted">
                                  {line.oldNum ?? ""}
                                </span>
                                <span class="w-10 flex-shrink-0 select-none text-right text-cf-text-muted">
                                  {line.newNum ?? ""}
                                </span>
                              </Show>
                              <span
                                class={`w-4 flex-shrink-0 select-none text-center ${textColor}`}
                              >
                                {prefix}
                              </span>
                              <span class={`flex-1 whitespace-pre ${textColor}`}>
                                {line.content}
                              </span>
                            </div>
                          );
                        }}
                      </For>
                    )}
                  </For>
                </div>
              </Show>
            </div>
          )}
        </For>
      </Show>
    </div>
  );
}
