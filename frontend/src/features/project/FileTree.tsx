import { createMemo, createResource, For, type JSX, Show } from "solid-js";

import { api } from "~/api/client";
import type { FileEntry } from "~/api/types";
import { fileIconUrl } from "~/lib/file-icon";
import { Button } from "~/ui";

import { useFileTree } from "./FileTreeContext";

export interface FileTreeProps {
  projectId: string;
  onFileSelect: (path: string) => void;
  selectedPath?: string;
  onContextMenu?: (entry: FileEntry, x: number, y: number) => void;
}

// ---------------------------------------------------------------------------
// Search / Filter Helpers
// ---------------------------------------------------------------------------

/** Parse search query: `/pattern/` → regex, otherwise plain substring. */
function parseQuery(raw: string): { regex: RegExp | null; plain: string } {
  const trimmed = raw.trim();
  if (trimmed.startsWith("/") && trimmed.endsWith("/") && trimmed.length > 2) {
    try {
      return { regex: new RegExp(trimmed.slice(1, -1), "i"), plain: "" };
    } catch {
      // Invalid regex — fall back to literal
    }
  }
  return { regex: null, plain: trimmed.toLowerCase() };
}

/** Test if an entry matches the query. */
function matchesQuery(entry: FileEntry, query: { regex: RegExp | null; plain: string }): boolean {
  if (query.regex) return query.regex.test(entry.path);
  return entry.name.toLowerCase().includes(query.plain);
}

/** Build the set of visible paths (matching files + their ancestor dirs). */
function buildFilteredPaths(
  entries: FileEntry[],
  query: { regex: RegExp | null; plain: string },
): Set<string> {
  const visible = new Set<string>();
  for (const e of entries) {
    if (e.is_dir) continue; // Only match files, dirs are included as ancestors
    if (!matchesQuery(e, query)) continue;
    visible.add(e.path);
    // Add ancestor directories
    let parent = e.path;
    while (parent.includes("/")) {
      parent = parent.substring(0, parent.lastIndexOf("/"));
      if (visible.has(parent)) break; // Already added ancestors above
      visible.add(parent);
    }
  }
  // Also match directories directly by name
  for (const e of entries) {
    if (!e.is_dir) continue;
    if (!matchesQuery(e, query)) continue;
    visible.add(e.path);
    let parent = e.path;
    while (parent.includes("/")) {
      parent = parent.substring(0, parent.lastIndexOf("/"));
      if (visible.has(parent)) break;
      visible.add(parent);
    }
  }
  return visible;
}

/** Highlight matching text in a name. For regex, highlight full name. */
function HighlightMatch(props: {
  name: string;
  query: { regex: RegExp | null; plain: string };
}): JSX.Element {
  const parts = createMemo(() => {
    const name = props.name;
    const query = props.query;
    if (query.regex) {
      const isMatch = query.regex.test(name);
      if (!isMatch) return { before: name, matched: "", after: "" };
      return { before: "", matched: name, after: "" };
    }
    const idx = name.toLowerCase().indexOf(query.plain);
    if (idx === -1) return { before: name, matched: "", after: "" };
    return {
      before: name.slice(0, idx),
      matched: name.slice(idx, idx + query.plain.length),
      after: name.slice(idx + query.plain.length),
    };
  });

  return (
    <>
      {parts().before}
      <Show when={parts().matched}>
        <mark class="bg-cf-accent/25 text-inherit rounded-sm">{parts().matched}</mark>
      </Show>
      {parts().after}
    </>
  );
}

// ---------------------------------------------------------------------------
// Sort helper
// ---------------------------------------------------------------------------

function sortEntries(entries: FileEntry[]): FileEntry[] {
  return [...entries].sort((a, b) => {
    if (a.is_dir && !b.is_dir) return -1;
    if (!a.is_dir && b.is_dir) return 1;
    return a.name.localeCompare(b.name);
  });
}

// ---------------------------------------------------------------------------
// TreeNode (normal mode — lazy-loaded)
// ---------------------------------------------------------------------------

interface TreeNodeProps {
  entry: FileEntry;
  projectId: string;
  onFileSelect: (path: string) => void;
  selectedPath?: string;
  depth: number;
  onContextMenu?: (entry: FileEntry, x: number, y: number) => void;
}

function TreeNode(props: TreeNodeProps): JSX.Element {
  const [state, actions] = useFileTree();

  const isExpanded = (): boolean => !!state.expanded[props.entry.path];

  // Derive children from fullTree cache when available (instant, no API call)
  const cachedChildren = createMemo(() => {
    const tree = state.fullTree;
    if (!tree) return undefined;
    return tree.filter((e) => {
      const parent = e.path.includes("/") ? e.path.substring(0, e.path.lastIndexOf("/")) : "";
      return parent === props.entry.path;
    });
  });

  // Lazy-load only when fullTree is not cached
  const [lazyChildren] = createResource(
    () => {
      if (cachedChildren()) return undefined;
      const isOpen = isExpanded();
      const projId = props.projectId;
      return isOpen ? { path: props.entry.path, projId } : undefined;
    },
    (params) => api.files.list(params.projId, params.path),
  );

  // Use cached children when available, lazy-loaded otherwise
  const children = () => cachedChildren() ?? lazyChildren();

  function toggle() {
    if (props.entry.is_dir) {
      actions.toggleExpanded(props.entry.path);
    } else {
      props.onFileSelect(props.entry.path);
    }
  }

  const isSelected = () => !props.entry.is_dir && props.selectedPath === props.entry.path;

  return (
    <div>
      <Button
        variant="ghost"
        size="xs"
        fullWidth
        class={`flex items-center !justify-start gap-1 text-left pr-1 py-0.5 ${
          isSelected() ? "bg-cf-accent/15 text-cf-accent" : ""
        }`}
        style={{ "padding-left": `${props.depth * 12}px` }}
        onClick={toggle}
        onContextMenu={(e: MouseEvent) => {
          if (props.onContextMenu) {
            e.preventDefault();
            e.stopPropagation();
            props.onContextMenu(props.entry, e.clientX, e.clientY);
          }
        }}
        title={props.entry.path}
      >
        <Show when={props.entry.is_dir}>
          <span class="w-4 text-center text-xs text-cf-text-muted select-none flex-shrink-0">
            {isExpanded() ? "\u25BE" : "\u25B8"}
          </span>
        </Show>
        <Show when={!props.entry.is_dir}>
          <span class="w-4 flex-shrink-0" />
        </Show>
        <img
          src={fileIconUrl(props.entry.name, props.entry.is_dir, isExpanded())}
          alt=""
          class="w-4 h-4 flex-shrink-0"
        />
        <span class="truncate">{props.entry.name}</span>
      </Button>

      <Show when={isExpanded() && children()}>
        <div>
          <For each={sortEntries(children() ?? [])}>
            {(child) => (
              <TreeNode
                entry={child}
                projectId={props.projectId}
                onFileSelect={props.onFileSelect}
                selectedPath={props.selectedPath}
                depth={props.depth + 1}
                onContextMenu={props.onContextMenu}
              />
            )}
          </For>
        </div>
      </Show>
    </div>
  );
}

// ---------------------------------------------------------------------------
// FilteredTreeNode (search mode — flat list from full tree)
// ---------------------------------------------------------------------------

interface FilteredNodeProps {
  entry: FileEntry;
  onFileSelect: (path: string) => void;
  selectedPath?: string;
  query: { regex: RegExp | null; plain: string };
  onContextMenu?: (entry: FileEntry, x: number, y: number) => void;
}

function FilteredTreeNode(props: FilteredNodeProps): JSX.Element {
  const depth = () => props.entry.path.split("/").length - 1;
  const [state, actions] = useFileTree();
  const isExpanded = (): boolean => !!state.expanded[props.entry.path];

  function toggle() {
    if (props.entry.is_dir) {
      actions.toggleExpanded(props.entry.path);
    } else {
      props.onFileSelect(props.entry.path);
    }
  }

  const isSelected = () => !props.entry.is_dir && props.selectedPath === props.entry.path;

  return (
    <Button
      variant="ghost"
      size="xs"
      fullWidth
      class={`flex items-center !justify-start gap-1 text-left pr-1 py-0.5 ${
        isSelected() ? "bg-cf-accent/15 text-cf-accent" : ""
      }`}
      style={{ "padding-left": `${depth() * 12}px` }}
      onClick={toggle}
      onContextMenu={(e: MouseEvent) => {
        if (props.onContextMenu) {
          e.preventDefault();
          e.stopPropagation();
          props.onContextMenu(props.entry, e.clientX, e.clientY);
        }
      }}
      title={props.entry.path}
    >
      <Show when={props.entry.is_dir}>
        <span class="w-4 text-center text-xs text-cf-text-muted select-none flex-shrink-0">
          {isExpanded() ? "\u25BE" : "\u25B8"}
        </span>
      </Show>
      <Show when={!props.entry.is_dir}>
        <span class="w-4 flex-shrink-0" />
      </Show>
      <img
        src={fileIconUrl(props.entry.name, props.entry.is_dir, isExpanded())}
        alt=""
        class="w-4 h-4 flex-shrink-0"
      />
      <span class="truncate">
        <HighlightMatch name={props.entry.name} query={props.query} />
      </span>
    </Button>
  );
}

// ---------------------------------------------------------------------------
// FileTree (main export)
// ---------------------------------------------------------------------------

export default function FileTree(props: FileTreeProps): JSX.Element {
  const [state, actions] = useFileTree();

  // Normal mode: lazy-loaded root entries
  const [rootEntries] = createResource(
    () => props.projectId,
    (id) => api.files.list(id, "."),
  );

  // Search mode: trigger full tree fetch when debounced query becomes non-empty
  const searchActive = () => state.debouncedQuery.length > 0;

  // Fetch full tree reactively when search is activated
  createResource(
    () => (searchActive() ? props.projectId : undefined),
    (id) => actions.fetchFullTree(id),
  );

  const parsedQuery = createMemo(() => parseQuery(state.debouncedQuery));

  // Filtered + sorted entries for search mode
  const filteredEntries = createMemo(() => {
    const tree = state.fullTree;
    if (!tree || !searchActive()) return [];
    const query = parsedQuery();
    const visible = buildFilteredPaths(tree, query);
    // Build flat list grouped by parent — entries whose parent path is expanded (or is root)
    const result: FileEntry[] = [];
    for (const e of sortEntries(tree.filter((t) => visible.has(t.path)))) {
      const parentPath = e.path.includes("/") ? e.path.substring(0, e.path.lastIndexOf("/")) : "";
      // Show if root level or parent is expanded
      if (parentPath === "" || state.expanded[parentPath]) {
        result.push(e);
      }
    }
    return result;
  });

  // Auto-expand ancestors of matches when search changes
  const _autoExpand = createMemo(() => {
    const tree = state.fullTree;
    const query = parsedQuery();
    if (!tree || !searchActive()) return null;
    const visible = buildFilteredPaths(tree, query);
    // Expand all ancestor dirs
    for (const path of visible) {
      const entry = tree.find((e) => e.path === path);
      if (entry?.is_dir) {
        actions.setExpanded(path, true);
      }
    }
    return null;
  });
  void _autoExpand;

  return (
    <div
      class="overflow-y-auto text-sm min-h-full"
      onContextMenu={(e: MouseEvent) => {
        // Right-click on empty area triggers root-level context menu
        if (props.onContextMenu && e.target === e.currentTarget) {
          e.preventDefault();
          // Pass a synthetic root entry so FilePanel knows it is root-level
          const rootEntry: FileEntry = { name: "", path: "", is_dir: true, size: 0, mod_time: "" };
          props.onContextMenu(rootEntry, e.clientX, e.clientY);
        }
      }}
    >
      <Show
        when={searchActive()}
        fallback={
          /* Normal mode */
          <Show
            when={!rootEntries.loading}
            fallback={<p class="text-xs text-cf-text-muted p-2">Loading...</p>}
          >
            <Show when={rootEntries.error}>
              <p class="text-xs text-red-400 p-2">
                Failed to load files:{" "}
                {rootEntries.error instanceof Error ? rootEntries.error.message : "Unknown error"}
              </p>
            </Show>
            <Show
              when={!rootEntries.error && (rootEntries()?.length ?? 0) > 0}
              fallback={
                <Show when={!rootEntries.error}>
                  <p class="text-xs text-cf-text-muted p-2">No files found</p>
                </Show>
              }
            >
              <For each={sortEntries(rootEntries() ?? [])}>
                {(entry) => (
                  <TreeNode
                    entry={entry}
                    projectId={props.projectId}
                    onFileSelect={props.onFileSelect}
                    selectedPath={props.selectedPath}
                    depth={0}
                    onContextMenu={props.onContextMenu}
                  />
                )}
              </For>
            </Show>
          </Show>
        }
      >
        {/* Search mode */}
        <Show
          when={!state.treeLoading}
          fallback={<p class="text-xs text-cf-text-muted p-2">Searching...</p>}
        >
          <Show
            when={filteredEntries().length > 0}
            fallback={<p class="text-xs text-cf-text-muted p-2">No matches found</p>}
          >
            <For each={filteredEntries()}>
              {(entry) => (
                <FilteredTreeNode
                  entry={entry}
                  onFileSelect={props.onFileSelect}
                  selectedPath={props.selectedPath}
                  query={parsedQuery()}
                  onContextMenu={props.onContextMenu}
                />
              )}
            </For>
          </Show>
        </Show>
      </Show>
    </div>
  );
}
