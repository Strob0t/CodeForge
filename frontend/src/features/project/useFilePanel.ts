import { createSignal } from "solid-js";

import { api } from "~/api/client";
import type { FileContent, FileEntry } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useAsyncAction } from "~/hooks/useAsyncAction";
import { useI18n } from "~/i18n";
import { getErrorMessage } from "~/utils/getErrorMessage";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface OpenTab {
  path: string;
  content: string;
  language: string;
  modified: boolean;
  originalContent: string;
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const SIDEBAR_DEFAULT = 224;

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

/**
 * Custom hook that encapsulates file editor tab management, context menu
 * state, and sidebar layout for the FilePanel component.
 *
 * Extracted to reduce FilePanel from ~929 LOC (keeps rendering logic in the component).
 */
export function useFilePanel(projectId: () => string) {
  const { show: toast } = useToast();
  const { t } = useI18n();

  // ---- Tab / editor state ----

  const [tabs, setTabs] = createSignal<OpenTab[]>([]);
  const [activeTab, setActiveTab] = createSignal<string | null>(null);

  function fileName(path: string): string {
    const parts = path.split("/");
    return parts[parts.length - 1];
  }

  async function openFile(path: string) {
    const existing = tabs().find((tab) => tab.path === path);
    if (existing) {
      setActiveTab(path);
      return;
    }

    try {
      const file: FileContent = await api.files.read(projectId(), path);
      setTabs((prev) => [
        ...prev,
        {
          path: file.path,
          content: file.content,
          language: file.language,
          modified: false,
          originalContent: file.content,
        },
      ]);
      setActiveTab(file.path);
    } catch (e) {
      const msg = e instanceof Error ? e.message : t("files.openFailed");
      toast("error", msg);
    }
  }

  function closeTab(path: string) {
    setTabs((prev) => prev.filter((tab) => tab.path !== path));
    if (activeTab() === path) {
      const remaining = tabs().filter((tab) => tab.path !== path);
      setActiveTab(remaining.length > 0 ? remaining[remaining.length - 1].path : null);
    }
  }

  function updateContent(path: string, content: string) {
    setTabs((prev) =>
      prev.map((tab) =>
        tab.path === path ? { ...tab, content, modified: content !== tab.originalContent } : tab,
      ),
    );
  }

  const { run: saveActiveFile, loading: saving } = useAsyncAction(
    async () => {
      const path = activeTab();
      if (!path) return;

      const tab = tabs().find((t) => t.path === path);
      if (!tab || !tab.modified) return;

      await api.files.write(projectId(), path, tab.content);
      setTabs((prev) =>
        prev.map((t) =>
          t.path === path ? { ...t, modified: false, originalContent: t.content } : t,
        ),
      );
      toast("success", t("files.saved", { name: fileName(path) }));
    },
    {
      onError: (err) => toast("error", getErrorMessage(err, t("files.saveFailed"))),
    },
  );

  const currentTab = () => tabs().find((tab) => tab.path === activeTab());

  // ---- Context menu state ----

  const [ctxMenuVisible, setCtxMenuVisible] = createSignal(false);
  const [ctxMenuX, setCtxMenuX] = createSignal(0);
  const [ctxMenuY, setCtxMenuY] = createSignal(0);
  const [ctxMenuEntry, setCtxMenuEntry] = createSignal<FileEntry | null>(null);
  const ctxMenuIsRoot = () => {
    const entry = ctxMenuEntry();
    return entry !== null && entry.path === "" && entry.is_dir;
  };

  function handleContextMenu(entry: FileEntry, x: number, y: number) {
    setCtxMenuEntry(entry);
    setCtxMenuX(x);
    setCtxMenuY(y);
    setCtxMenuVisible(true);
  }

  function closeContextMenu() {
    setCtxMenuVisible(false);
  }

  // ---- Rename modal state ----

  const [showRenameModal, setShowRenameModal] = createSignal(false);
  const [renameOldPath, setRenameOldPath] = createSignal("");
  const [renameNewName, setRenameNewName] = createSignal("");

  const { run: handleRename, loading: renaming } = useAsyncAction(
    async () => {
      const oldPath = renameOldPath();
      const newName = renameNewName().trim();
      if (!oldPath || !newName) return;

      const parentDir = oldPath.includes("/") ? oldPath.substring(0, oldPath.lastIndexOf("/")) : "";
      const newPath = parentDir ? `${parentDir}/${newName}` : newName;

      await api.files.rename(projectId(), oldPath, newPath);
      toast("success", t("files.renamed", { name: newName }));

      // Update open tabs that match old path (or start with old path for folder renames)
      setTabs((prev) =>
        prev.map((tab) => {
          if (tab.path === oldPath) {
            return { ...tab, path: newPath };
          }
          if (tab.path.startsWith(oldPath + "/")) {
            return { ...tab, path: newPath + tab.path.substring(oldPath.length) };
          }
          return tab;
        }),
      );
      if (activeTab() === oldPath) {
        setActiveTab(newPath);
      } else {
        const current = activeTab();
        if (current?.startsWith(oldPath + "/")) {
          setActiveTab(newPath + current.substring(oldPath.length));
        }
      }

      setShowRenameModal(false);
      setRenameOldPath("");
      setRenameNewName("");
    },
    {
      onError: (err) => toast("error", getErrorMessage(err, t("files.renameFailed"))),
    },
  );

  // ---- Layout state ----

  const [sidebarWidth, setSidebarWidth] = createSignal(SIDEBAR_DEFAULT);
  const [dragging, setDragging] = createSignal(false);

  return {
    // Tab management
    tabs,
    activeTab,
    setActiveTab,
    currentTab,
    openFile,
    closeTab,
    updateContent,
    saveActiveFile,
    saving,
    fileName,

    // Context menu
    ctxMenuVisible,
    ctxMenuX,
    ctxMenuY,
    ctxMenuEntry,
    ctxMenuIsRoot,
    handleContextMenu,
    closeContextMenu,

    // Rename
    showRenameModal,
    setShowRenameModal,
    renameOldPath,
    setRenameOldPath,
    renameNewName,
    setRenameNewName,
    handleRename,
    renaming,

    // Layout
    sidebarWidth,
    setSidebarWidth,
    dragging,
    setDragging,
  };
}
