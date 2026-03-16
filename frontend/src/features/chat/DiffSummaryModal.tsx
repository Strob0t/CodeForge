import type { Component } from "solid-js";
import { createSignal, For, Show } from "solid-js";

import { Modal } from "~/ui/composites/Modal";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface DiffHunk {
  old_start: number;
  old_lines: number;
  new_start: number;
  new_lines: number;
  old_content: string;
  new_content: string;
}

export interface DiffEntry {
  path: string;
  hunks: DiffHunk[];
}

interface DiffSummaryModalProps {
  diffs: DiffEntry[];
  visible: boolean;
  onClose: () => void;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function countChanges(hunks: DiffEntry["hunks"]): { added: number; removed: number } {
  let added = 0;
  let removed = 0;
  for (const hunk of hunks) {
    const oldLines = hunk.old_content.split("\n").filter(Boolean);
    const newLines = hunk.new_content.split("\n").filter(Boolean);
    removed += oldLines.length;
    added += newLines.length;
  }
  return { added, removed };
}

/** Build unified diff lines from a hunk for rendering. */
interface DiffLine {
  type: "removed" | "added" | "header";
  oldLineNo: number | null;
  newLineNo: number | null;
  content: string;
}

function buildUnifiedLines(hunk: DiffHunk): DiffLine[] {
  const lines: DiffLine[] = [];

  // Hunk header
  lines.push({
    type: "header",
    oldLineNo: null,
    newLineNo: null,
    content: `@@ -${hunk.old_start},${hunk.old_lines} +${hunk.new_start},${hunk.new_lines} @@`,
  });

  const oldLines = hunk.old_content ? hunk.old_content.split("\n") : [];
  const newLines = hunk.new_content ? hunk.new_content.split("\n") : [];

  // Removed lines
  for (let i = 0; i < oldLines.length; i++) {
    if (oldLines[i] === "" && i === oldLines.length - 1) continue;
    lines.push({
      type: "removed",
      oldLineNo: hunk.old_start + i,
      newLineNo: null,
      content: oldLines[i],
    });
  }

  // Added lines
  for (let i = 0; i < newLines.length; i++) {
    if (newLines[i] === "" && i === newLines.length - 1) continue;
    lines.push({
      type: "added",
      oldLineNo: null,
      newLineNo: hunk.new_start + i,
      content: newLines[i],
    });
  }

  return lines;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

const DiffSummaryModal: Component<DiffSummaryModalProps> = (props) => {
  const [expanded, setExpanded] = createSignal<Set<string>>(new Set());

  // --- Toggle file expansion ---

  function toggleFile(path: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(path)) {
        next.delete(path);
      } else {
        next.add(path);
      }
      return next;
    });
  }

  function isExpanded(path: string): boolean {
    return expanded().has(path);
  }

  // --- Line styling ---

  function lineClasses(type: DiffLine["type"]): string {
    switch (type) {
      case "removed":
        return "bg-red-100 dark:bg-red-900/20";
      case "added":
        return "bg-green-100 dark:bg-green-900/20";
      case "header":
        return "bg-cf-bg-inset text-cf-text-muted";
    }
  }

  function linePrefix(type: DiffLine["type"]): string {
    switch (type) {
      case "removed":
        return "-";
      case "added":
        return "+";
      case "header":
        return "";
    }
  }

  function lineTextColor(type: DiffLine["type"]): string {
    switch (type) {
      case "removed":
        return "text-red-700 dark:text-red-400";
      case "added":
        return "text-green-700 dark:text-green-400";
      case "header":
        return "text-cf-text-muted";
    }
  }

  // --- Render ---

  return (
    <Modal
      open={props.visible}
      onClose={props.onClose}
      title="Session Changes"
      class="w-full max-w-3xl max-h-[80vh] flex flex-col"
    >
      <div class="-mt-4 -mx-4">
        {/* Subtitle */}
        <div class="px-4 pb-2 pt-1">
          <span class="text-xs text-cf-text-muted">
            {props.diffs.length === 0
              ? "No changes"
              : `${props.diffs.length} file${props.diffs.length === 1 ? "" : "s"} changed`}
          </span>
        </div>

        {/* Scrollable content */}
        <div class="flex-1 overflow-y-auto max-h-[60vh]">
          {/* Empty state */}
          <Show when={props.diffs.length === 0}>
            <div class="flex flex-col items-center justify-center py-16 text-cf-text-muted">
              <svg
                class="mb-3 h-10 w-10 opacity-40"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="1.5"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
                <polyline points="14 2 14 8 20 8" />
                <line x1="9" y1="15" x2="15" y2="15" />
              </svg>
              <span class="text-sm">No file changes in this session</span>
            </div>
          </Show>

          {/* File list */}
          <Show when={props.diffs.length > 0}>
            <For each={props.diffs}>
              {(entry) => {
                const stats = () => countChanges(entry.hunks);

                return (
                  <div class="border-b border-cf-border last:border-b-0">
                    {/* File header — clickable to expand/collapse */}
                    <button
                      class="flex w-full items-center gap-2 px-4 py-2 text-left hover:bg-cf-bg-surface-alt transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cf-focus-ring focus-visible:ring-offset-2"
                      onClick={() => toggleFile(entry.path)}
                      aria-expanded={isExpanded(entry.path)}
                    >
                      <span class="text-cf-text-muted text-xs w-4 shrink-0 select-none">
                        {isExpanded(entry.path) ? "\u25BC" : "\u25B6"}
                      </span>
                      <span class="font-mono text-sm text-cf-text-primary truncate flex-1">
                        {entry.path}
                      </span>
                      <span class="inline-flex items-center gap-1.5 shrink-0">
                        <span class="text-xs font-medium text-green-700 dark:text-green-400 bg-green-100 dark:bg-green-900/30 rounded-full px-1.5 py-0.5">
                          +{stats().added}
                        </span>
                        <span class="text-xs font-medium text-red-700 dark:text-red-400 bg-red-100 dark:bg-red-900/30 rounded-full px-1.5 py-0.5">
                          -{stats().removed}
                        </span>
                      </span>
                    </button>

                    {/* Diff hunks (collapsed by default) */}
                    <Show when={isExpanded(entry.path)}>
                      <div class="border-t border-cf-border">
                        <For each={entry.hunks}>
                          {(hunk) => {
                            const lines = () => buildUnifiedLines(hunk);
                            return (
                              <div>
                                <For each={lines()}>
                                  {(line) => (
                                    <div class={`flex font-mono text-xs ${lineClasses(line.type)}`}>
                                      {/* Old line number */}
                                      <span class="w-10 shrink-0 select-none text-right pr-1 text-cf-text-muted/50">
                                        {line.oldLineNo ?? ""}
                                      </span>
                                      {/* New line number */}
                                      <span class="w-10 shrink-0 select-none text-right pr-1 text-cf-text-muted/50">
                                        {line.newLineNo ?? ""}
                                      </span>
                                      {/* Prefix and content */}
                                      <span
                                        class={`flex-1 whitespace-pre-wrap break-all px-2 ${lineTextColor(line.type)}`}
                                      >
                                        {line.type === "header"
                                          ? line.content
                                          : `${linePrefix(line.type)}${line.content}`}
                                      </span>
                                    </div>
                                  )}
                                </For>
                              </div>
                            );
                          }}
                        </For>
                      </div>
                    </Show>
                  </div>
                );
              }}
            </For>
          </Show>
        </div>
      </div>
    </Modal>
  );
};

export default DiffSummaryModal;
