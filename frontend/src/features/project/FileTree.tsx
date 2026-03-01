import { createResource, createSignal, For, type JSX, Show } from "solid-js";

import { api } from "~/api/client";
import type { FileEntry } from "~/api/types";

export interface FileTreeProps {
  projectId: string;
  onFileSelect: (path: string) => void;
  selectedPath?: string;
}

// Extension-to-icon mapping using Unicode symbols
function fileIcon(name: string, isDir: boolean): string {
  if (isDir) return "\u{1F4C1}";
  const ext = name.split(".").pop()?.toLowerCase() ?? "";
  const icons: Record<string, string> = {
    ts: "\u{1F7E6}",
    tsx: "\u{1F7E6}",
    js: "\u{1F7E8}",
    jsx: "\u{1F7E8}",
    go: "\u{1F535}",
    py: "\u{1F40D}",
    rs: "\u{1F9F0}",
    md: "\u{1F4D6}",
    json: "\u{1F4CB}",
    yaml: "\u{1F4CB}",
    yml: "\u{1F4CB}",
    css: "\u{1F3A8}",
    html: "\u{1F310}",
    svg: "\u{1F5BC}",
    png: "\u{1F5BC}",
    jpg: "\u{1F5BC}",
    gif: "\u{1F5BC}",
    sh: "\u{1F4DF}",
    sql: "\u{1F5C4}",
    toml: "\u{2699}",
    lock: "\u{1F512}",
  };
  return icons[ext] ?? "\u{1F4C4}";
}

interface TreeNodeProps {
  entry: FileEntry;
  projectId: string;
  onFileSelect: (path: string) => void;
  selectedPath?: string;
  depth: number;
}

function TreeNode(props: TreeNodeProps): JSX.Element {
  const [expanded, setExpanded] = createSignal(false);
  const [children] = createResource(
    () => (expanded() ? props.entry.path : undefined),
    (path) => api.files.list(props.projectId, path),
  );

  function toggle() {
    if (props.entry.is_dir) {
      setExpanded(!expanded());
    } else {
      props.onFileSelect(props.entry.path);
    }
  }

  const isSelected = () => !props.entry.is_dir && props.selectedPath === props.entry.path;

  return (
    <div>
      <button
        type="button"
        class={`flex items-center gap-1 w-full text-left px-1 py-0.5 text-sm rounded hover:bg-cf-bg-surface-alt transition-colors ${
          isSelected() ? "bg-cf-accent/15 text-cf-accent" : "text-cf-text-secondary"
        }`}
        style={{ "padding-left": `${props.depth * 16 + 4}px` }}
        onClick={toggle}
        title={props.entry.path}
      >
        <Show when={props.entry.is_dir}>
          <span class="w-4 text-center text-xs text-cf-text-muted select-none">
            {expanded() ? "\u25BE" : "\u25B8"}
          </span>
        </Show>
        <Show when={!props.entry.is_dir}>
          <span class="w-4" />
        </Show>
        <span class="text-xs select-none">{fileIcon(props.entry.name, props.entry.is_dir)}</span>
        <span class="truncate">{props.entry.name}</span>
      </button>

      <Show when={expanded() && children()}>
        <div>
          <For each={sortEntries(children() ?? [])}>
            {(child) => (
              <TreeNode
                entry={child}
                projectId={props.projectId}
                onFileSelect={props.onFileSelect}
                selectedPath={props.selectedPath}
                depth={props.depth + 1}
              />
            )}
          </For>
        </div>
      </Show>
    </div>
  );
}

function sortEntries(entries: FileEntry[]): FileEntry[] {
  return [...entries].sort((a, b) => {
    // Directories first
    if (a.is_dir && !b.is_dir) return -1;
    if (!a.is_dir && b.is_dir) return 1;
    return a.name.localeCompare(b.name);
  });
}

export default function FileTree(props: FileTreeProps): JSX.Element {
  const [rootEntries] = createResource(
    () => props.projectId,
    (id) => api.files.list(id, "."),
  );

  return (
    <div class="overflow-y-auto text-sm">
      <Show
        when={!rootEntries.loading}
        fallback={<p class="text-xs text-cf-text-muted p-2">Loading...</p>}
      >
        <Show
          when={(rootEntries()?.length ?? 0) > 0}
          fallback={<p class="text-xs text-cf-text-muted p-2">No files found</p>}
        >
          <For each={sortEntries(rootEntries() ?? [])}>
            {(entry) => (
              <TreeNode
                entry={entry}
                projectId={props.projectId}
                onFileSelect={props.onFileSelect}
                selectedPath={props.selectedPath}
                depth={0}
              />
            )}
          </For>
        </Show>
      </Show>
    </div>
  );
}
