import { createContext, type JSX, type ParentProps, useContext } from "solid-js";
import { createStore, reconcile } from "solid-js/store";

import { api } from "~/api/client";
import type { FileEntry } from "~/api/types";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface FileTreeState {
  expanded: Record<string, boolean>;
  searchQuery: string;
  debouncedQuery: string;
  fullTree: FileEntry[] | null;
  treeLoading: boolean;
  expandAllLoading: boolean;
}

interface FileTreeActions {
  toggleExpanded: (path: string) => void;
  setExpanded: (path: string, open: boolean) => void;
  expandAll: (projectId: string) => Promise<void>;
  collapseAll: () => void;
  setSearchQuery: (query: string) => void;
  fetchFullTree: (projectId: string) => Promise<FileEntry[]>;
  invalidateCache: () => void;
}

type FileTreeContextValue = [FileTreeState, FileTreeActions];

// ---------------------------------------------------------------------------
// Context
// ---------------------------------------------------------------------------

const FileTreeCtx = createContext<FileTreeContextValue>();

export function useFileTree(): FileTreeContextValue {
  const ctx = useContext(FileTreeCtx);
  if (!ctx) throw new Error("useFileTree must be used within FileTreeProvider");
  return ctx;
}

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

export function FileTreeProvider(props: ParentProps): JSX.Element {
  const [state, setState] = createStore<FileTreeState>({
    expanded: {},
    searchQuery: "",
    debouncedQuery: "",
    fullTree: null,
    treeLoading: false,
    expandAllLoading: false,
  });

  let debounceTimer: ReturnType<typeof setTimeout> | undefined;

  // -- Actions ---------------------------------------------------------------

  const actions: FileTreeActions = {
    toggleExpanded(path: string) {
      setState("expanded", path, (v) => !v);
    },

    setExpanded(path: string, open: boolean) {
      setState("expanded", path, open);
    },

    async expandAll(projectId: string) {
      setState("expandAllLoading", true);
      try {
        const tree = state.fullTree ?? (await actions.fetchFullTree(projectId));
        setState(
          "expanded",
          reconcile(Object.fromEntries(tree.filter((e) => e.is_dir).map((e) => [e.path, true]))),
        );
      } finally {
        setState("expandAllLoading", false);
      }
    },

    collapseAll() {
      setState("expanded", reconcile({}));
    },

    setSearchQuery(query: string) {
      setState("searchQuery", query);
      clearTimeout(debounceTimer);
      debounceTimer = setTimeout(() => {
        setState("debouncedQuery", query);
      }, 150);
    },

    async fetchFullTree(projectId: string) {
      if (state.fullTree) return state.fullTree;
      setState("treeLoading", true);
      try {
        const tree = await api.files.tree(projectId);
        setState("fullTree", tree);
        return tree;
      } finally {
        setState("treeLoading", false);
      }
    },

    invalidateCache() {
      setState("fullTree", null);
    },
  };

  return <FileTreeCtx.Provider value={[state, actions]}>{props.children}</FileTreeCtx.Provider>;
}
