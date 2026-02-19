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
      class="overflow-auto rounded border border-gray-200 dark:border-gray-700"
      style={{ "max-height": `${props.maxHeight ?? 400}px` }}
    >
      <Show
        when={files().length > 0}
        fallback={<p class="p-3 text-xs text-gray-500 dark:text-gray-400">{t("diff.noDiff")}</p>}
      >
        <For each={files()}>
          {(file, idx) => (
            <div class="border-b border-gray-100 last:border-b-0 dark:border-gray-700">
              {/* File header */}
              <button
                type="button"
                class="flex w-full items-center gap-2 bg-gray-50 px-3 py-2 text-left text-xs hover:bg-gray-100 dark:bg-gray-800 dark:hover:bg-gray-750"
                onClick={() => toggleFile(idx())}
                aria-expanded={!collapsed()[idx()]}
              >
                <span class="font-mono font-medium text-gray-700 dark:text-gray-300">
                  {file.newPath || file.oldPath}
                </span>
                <span class="flex-1" />
                <span class="text-green-600 dark:text-green-400">+{addCount(file)}</span>
                <span class="text-red-500 dark:text-red-400">-{removeCount(file)}</span>
                <span class="text-gray-400">{collapsed()[idx()] ? "\u25B6" : "\u25BC"}</span>
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
                              ? "bg-green-50 dark:bg-green-900/20"
                              : line.type === "remove"
                                ? "bg-red-50 dark:bg-red-900/20"
                                : line.type === "header"
                                  ? "bg-blue-50 dark:bg-blue-900/10"
                                  : "";
                          const textColor =
                            line.type === "add"
                              ? "text-green-800 dark:text-green-300"
                              : line.type === "remove"
                                ? "text-red-800 dark:text-red-300"
                                : line.type === "header"
                                  ? "text-blue-600 dark:text-blue-400"
                                  : "text-gray-600 dark:text-gray-400";
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
                                <span class="w-10 flex-shrink-0 select-none text-right text-gray-400 dark:text-gray-600">
                                  {line.oldNum ?? ""}
                                </span>
                                <span class="w-10 flex-shrink-0 select-none text-right text-gray-400 dark:text-gray-600">
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
