import { createSignal, For, type JSX, Show } from "solid-js";

import { api } from "~/api/client";
import type { FileContent, FileEntry } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useAsyncAction } from "~/hooks/useAsyncAction";
import { useBreakpoint } from "~/hooks/useBreakpoint";
import { useI18n } from "~/i18n";
import { fileIconUrl } from "~/lib/file-icon";
import { Button, FormField, Input, Modal, Spinner, Textarea } from "~/ui";
import { getErrorMessage } from "~/utils/getErrorMessage";

import CodeEditor from "./CodeEditor";
import { addContextFile } from "./contextFilesStore";
import FileContextMenu from "./FileContextMenu";
import FileTree from "./FileTree";
import { FileTreeProvider, useFileTree } from "./FileTreeContext";

const SIDEBAR_MIN = 120;
const SIDEBAR_MAX = 600;
const SIDEBAR_DEFAULT = 224; // w-56 = 14rem = 224px

interface OpenTab {
  path: string;
  content: string;
  language: string;
  modified: boolean;
  originalContent: string;
}

export interface FilePanelProps {
  projectId: string;
  /** Whether the project has a linked workspace directory. */
  hasWorkspace?: boolean;
  onNavigate?: (target: string) => void;
  /** Called when user right-clicks a file/folder and selects "Add to Context". */
  onAddToContext?: (path: string) => void;
}

// ---------------------------------------------------------------------------
// Inline SVG Icons for header buttons
// ---------------------------------------------------------------------------

function ExpandAllIcon(): JSX.Element {
  return (
    <svg
      width="14"
      height="14"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      stroke-width="1.5"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <polyline points="4 8 8 12 12 8" />
      <polyline points="4 4 8 8 12 4" />
    </svg>
  );
}

function CollapseAllIcon(): JSX.Element {
  return (
    <svg
      width="14"
      height="14"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      stroke-width="1.5"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <polyline points="4 8 8 4 12 8" />
      <polyline points="4 12 8 8 12 12" />
    </svg>
  );
}

// ---------------------------------------------------------------------------
// Sidebar header (needs FileTreeContext)
// ---------------------------------------------------------------------------

function SidebarHeader(props: {
  projectId: string;
  onCreateClick?: () => void;
  onUploadClick?: () => void;
}): JSX.Element {
  const [, actions] = useFileTree();
  const { t } = useI18n();

  return (
    <div class="flex items-center justify-between px-2 py-1.5 border-b border-cf-border">
      <span class="text-xs font-semibold text-cf-text-secondary uppercase tracking-wide">
        {t("detail.tab.files")}
      </span>
      <div class="flex items-center gap-0.5">
        <button
          type="button"
          class="inline-flex items-center justify-center h-7 w-7 rounded-cf-sm text-cf-text-muted hover:text-cf-text-primary hover:bg-cf-bg-surface-alt transition-colors"
          onClick={() => actions.expandAll(props.projectId)}
          title={t("files.expandAll")}
        >
          <ExpandAllIcon />
        </button>
        <button
          type="button"
          class="inline-flex items-center justify-center h-7 w-7 rounded-cf-sm text-cf-text-muted hover:text-cf-text-primary hover:bg-cf-bg-surface-alt transition-colors"
          onClick={() => actions.collapseAll()}
          title={t("files.collapseAll")}
        >
          <CollapseAllIcon />
        </button>
        <Show when={props.onUploadClick}>
          <button
            type="button"
            class="inline-flex items-center justify-center h-7 w-7 rounded-cf-sm text-cf-text-muted hover:text-cf-text-primary hover:bg-cf-bg-surface-alt transition-colors"
            onClick={() => props.onUploadClick?.()}
            title={t("files.uploadFile")}
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 16 16"
              fill="currentColor"
              class="w-3.5 h-3.5"
            >
              <path d="M7.25 10.25a.75.75 0 0 0 1.5 0V4.56l1.72 1.72a.75.75 0 1 0 1.06-1.06l-3-3a.75.75 0 0 0-1.06 0l-3 3a.75.75 0 0 0 1.06 1.06l1.72-1.72v5.69Z" />
              <path d="M3.5 9.75a.75.75 0 0 0-1.5 0v1.5A2.75 2.75 0 0 0 4.75 14h6.5A2.75 2.75 0 0 0 14 11.25v-1.5a.75.75 0 0 0-1.5 0v1.5c0 .69-.56 1.25-1.25 1.25h-6.5c-.69 0-1.25-.56-1.25-1.25v-1.5Z" />
            </svg>
          </button>
        </Show>
        <Show when={props.onCreateClick}>
          <button
            type="button"
            class="inline-flex items-center justify-center h-7 w-7 rounded-cf-sm text-cf-text-muted hover:text-cf-text-primary hover:bg-cf-bg-surface-alt transition-colors"
            onClick={() => props.onCreateClick?.()}
            title={t("files.createFile")}
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 16 16"
              fill="currentColor"
              class="w-3.5 h-3.5"
            >
              <path d="M8 2a.75.75 0 0 1 .75.75v4.5h4.5a.75.75 0 0 1 0 1.5h-4.5v4.5a.75.75 0 0 1-1.5 0v-4.5h-4.5a.75.75 0 0 1 0-1.5h4.5v-4.5A.75.75 0 0 1 8 2Z" />
            </svg>
          </button>
        </Show>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Search input (needs FileTreeContext)
// ---------------------------------------------------------------------------

function SearchInput(): JSX.Element {
  const [state, actions] = useFileTree();

  return (
    <div class="px-1.5 py-1 border-b border-cf-border">
      <input
        type="text"
        placeholder="Filter files... (/regex/)"
        value={state.searchQuery}
        onInput={(e) => actions.setSearchQuery(e.currentTarget.value)}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            actions.setSearchQuery("");
            (e.target as HTMLInputElement).blur();
          }
        }}
        class="w-full bg-cf-bg-primary border border-cf-border rounded-cf-sm px-2 py-1 text-xs text-cf-text-primary placeholder:text-cf-text-muted focus:outline-none focus:border-cf-accent"
      />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Tree loading overlay (shown during Expand All)
// ---------------------------------------------------------------------------

function TreeLoadingOverlay(): JSX.Element {
  const [state] = useFileTree();

  return (
    <Show when={state.expandAllLoading}>
      <div class="absolute inset-0 z-10 flex items-center justify-center bg-cf-bg-surface/70 backdrop-blur-[1px]">
        <Spinner size="sm" />
      </div>
    </Show>
  );
}

// ---------------------------------------------------------------------------
// FilePanel (main export)
// ---------------------------------------------------------------------------

export default function FilePanel(props: FilePanelProps): JSX.Element {
  const { show: toast } = useToast();

  const { t } = useI18n();
  const [showCreateModal, setShowCreateModal] = createSignal(false);
  const [newFilePath, setNewFilePath] = createSignal("");
  const [newFileContent, setNewFileContent] = createSignal("");

  const { run: handleCreateFile, loading: creating } = useAsyncAction(
    async () => {
      const filePath = newFilePath().trim();
      if (!filePath) return;
      await api.files.write(props.projectId, filePath, newFileContent());
      toast("success", t("files.createSuccess"));
      setShowCreateModal(false);
      setNewFilePath("");
      setNewFileContent("");
      openFile(filePath);
    },
    {
      onError: (err) =>
        toast("error", t("files.createFailed") + ": " + getErrorMessage(err, "Create failed")),
    },
  );

  // --- File upload ---
  let uploadInputRef: HTMLInputElement | undefined;

  function handleUploadChange(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    // Reset so the same file can be re-selected.
    input.value = "";

    const reader = new FileReader();
    reader.onload = async () => {
      const content = reader.result as string;
      try {
        await api.files.write(props.projectId, file.name, content);
        toast("success", t("files.uploadSuccess"));
        openFile(file.name);
      } catch (err) {
        toast("error", t("files.uploadFailed") + ": " + getErrorMessage(err, "Upload failed"));
      }
    };
    reader.onerror = () => {
      toast("error", t("files.textFilesOnly"));
    };
    reader.readAsText(file);
  }

  const [tabs, setTabs] = createSignal<OpenTab[]>([]);
  const [activeTab, setActiveTab] = createSignal<string | null>(null);

  function fileName(path: string): string {
    const parts = path.split("/");
    return parts[parts.length - 1];
  }

  async function openFile(path: string) {
    const existing = tabs().find((t) => t.path === path);
    if (existing) {
      setActiveTab(path);
      return;
    }

    try {
      const file: FileContent = await api.files.read(props.projectId, path);
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
    setTabs((prev) => prev.filter((t) => t.path !== path));
    if (activeTab() === path) {
      const remaining = tabs().filter((t) => t.path !== path);
      setActiveTab(remaining.length > 0 ? remaining[remaining.length - 1].path : null);
    }
  }

  function updateContent(path: string, content: string) {
    setTabs((prev) =>
      prev.map((t) =>
        t.path === path ? { ...t, content, modified: content !== t.originalContent } : t,
      ),
    );
  }

  const { run: saveActiveFile, loading: saving } = useAsyncAction(
    async () => {
      const path = activeTab();
      if (!path) return;

      const tab = tabs().find((t) => t.path === path);
      if (!tab || !tab.modified) return;

      await api.files.write(props.projectId, path, tab.content);
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

  const currentTab = () => tabs().find((t) => t.path === activeTab());

  // -- Mobile breakpoint -----------------------------------------------------
  const { isMobile } = useBreakpoint();
  const [fileDrawerOpen, setFileDrawerOpen] = createSignal(false);

  // -- Resizable sidebar ----------------------------------------------------
  const [sidebarWidth, setSidebarWidth] = createSignal(SIDEBAR_DEFAULT);
  const [dragging, setDragging] = createSignal(false);

  function onDragStart(e: MouseEvent) {
    e.preventDefault();
    setDragging(true);
    const startX = e.clientX;
    const startW = sidebarWidth();

    function onMove(ev: MouseEvent) {
      const newW = Math.min(SIDEBAR_MAX, Math.max(SIDEBAR_MIN, startW + (ev.clientX - startX)));
      setSidebarWidth(newW);
    }
    function onUp() {
      setDragging(false);
      document.removeEventListener("mousemove", onMove);
      document.removeEventListener("mouseup", onUp);
    }
    document.addEventListener("mousemove", onMove);
    document.addEventListener("mouseup", onUp);
  }

  // Double-click resets to default
  function onDragDblClick() {
    setSidebarWidth(SIDEBAR_DEFAULT);
  }

  // -- Context menu state ---------------------------------------------------
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

  // -- Upload to specific folder (context menu) -----------------------------
  let ctxUploadInputRef: HTMLInputElement | undefined;
  const [uploadTargetFolder, setUploadTargetFolder] = createSignal("");

  function handleCtxUploadChange(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    input.value = "";

    const targetFolder = uploadTargetFolder();
    const targetPath = targetFolder ? `${targetFolder}/${file.name}` : file.name;

    const reader = new FileReader();
    reader.onload = async () => {
      const content = reader.result as string;
      try {
        await api.files.write(props.projectId, targetPath, content);
        toast("success", t("files.uploadSuccess"));
        openFile(targetPath);
      } catch (err) {
        toast("error", t("files.uploadFailed") + ": " + getErrorMessage(err, "Upload failed"));
      }
    };
    reader.onerror = () => {
      toast("error", t("files.textFilesOnly"));
    };
    reader.readAsText(file);
  }

  // -- Rename modal ---------------------------------------------------------
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

      await api.files.rename(props.projectId, oldPath, newPath);
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
      onError: (err) =>
        toast(
          "error",
          t("files.renameFailed") + ": " + getErrorMessage(err, t("files.renameFailed")),
        ),
    },
  );

  // -- New Folder modal -----------------------------------------------------
  const [showFolderModal, setShowFolderModal] = createSignal(false);
  const [newFolderName, setNewFolderName] = createSignal("");
  const [newFolderPrefix, setNewFolderPrefix] = createSignal("");

  const { run: handleCreateFolder, loading: creatingFolder } = useAsyncAction(
    async () => {
      const name = newFolderName().trim();
      if (!name) return;
      const folderPath = newFolderPrefix() ? `${newFolderPrefix()}/${name}` : name;
      await api.files.write(props.projectId, `${folderPath}/.gitkeep`, "");
      toast("success", t("files.createFolderSuccess", { name }));
      setShowFolderModal(false);
      setNewFolderName("");
      setNewFolderPrefix("");
    },
    {
      onError: (err) =>
        toast(
          "error",
          t("files.createFolderFailed") + ": " + getErrorMessage(err, "Create folder failed"),
        ),
    },
  );

  // -- Delete confirmation --------------------------------------------------
  const [showDeleteModal, setShowDeleteModal] = createSignal(false);
  const [deletePath, setDeletePath] = createSignal("");
  const [deleteIsDir, setDeleteIsDir] = createSignal(false);

  const { run: handleDelete, loading: deleting } = useAsyncAction(
    async () => {
      const path = deletePath();
      if (!path) return;

      await api.files.delete(props.projectId, path);
      toast("success", t("files.deleted", { path }));

      // Close any open tabs for deleted file or files inside deleted folder
      setTabs((prev) =>
        prev.filter((tab) => tab.path !== path && !tab.path.startsWith(path + "/")),
      );
      if (activeTab() === path || activeTab()?.startsWith(path + "/")) {
        const remaining = tabs().filter(
          (tab) => tab.path !== path && !tab.path.startsWith(path + "/"),
        );
        setActiveTab(remaining.length > 0 ? remaining[remaining.length - 1].path : null);
      }

      setShowDeleteModal(false);
      setDeletePath("");
      setDeleteIsDir(false);
    },
    {
      onError: (err) =>
        toast(
          "error",
          t("files.deleteFailed") + ": " + getErrorMessage(err, t("files.deleteFailed")),
        ),
    },
  );

  // -- Context menu action dispatcher ---------------------------------------
  function handleContextMenuAction(action: string) {
    const entry = ctxMenuEntry();
    closeContextMenu();

    // Determine the folder prefix for new file / new folder / upload
    const folderPrefix = entry
      ? entry.is_dir
        ? entry.path
        : entry.path.includes("/")
          ? entry.path.substring(0, entry.path.lastIndexOf("/"))
          : ""
      : "";

    switch (action) {
      case "new-file": {
        setNewFilePath(folderPrefix ? folderPrefix + "/" : "");
        setNewFileContent("");
        setShowCreateModal(true);
        break;
      }
      case "new-folder": {
        setNewFolderPrefix(folderPrefix);
        setNewFolderName("");
        setShowFolderModal(true);
        break;
      }
      case "upload": {
        setUploadTargetFolder(folderPrefix);
        ctxUploadInputRef?.click();
        break;
      }
      case "rename": {
        if (!entry || entry.path === "") return;
        setRenameOldPath(entry.path);
        setRenameNewName(entry.name);
        setShowRenameModal(true);
        break;
      }
      case "delete": {
        if (!entry || entry.path === "") return;
        setDeletePath(entry.path);
        setDeleteIsDir(entry.is_dir);
        setShowDeleteModal(true);
        break;
      }
      case "add-to-context": {
        if (!entry || entry.path === "") return;
        addContextFile(entry.path);
        toast("success", t("files.addedToContext", { path: entry.path }));
        if (props.onAddToContext) {
          props.onAddToContext(entry.path);
        }
        break;
      }
    }
  }

  return (
    <div class="flex h-full min-h-0" style={{ cursor: dragging() ? "col-resize" : undefined }}>
      {/* File Tree Sidebar (desktop/tablet) */}
      <Show when={!isMobile()}>
        <FileTreeProvider>
          <div
            class="relative flex-shrink-0 border-r border-cf-border overflow-hidden flex flex-col bg-cf-bg-surface"
            style={{ width: `${sidebarWidth()}px` }}
          >
            <TreeLoadingOverlay />
            <SidebarHeader
              projectId={props.projectId}
              onCreateClick={() => setShowCreateModal(true)}
              onUploadClick={() => uploadInputRef?.click()}
            />
            <input ref={uploadInputRef} type="file" class="hidden" onChange={handleUploadChange} />
            <input
              ref={ctxUploadInputRef}
              type="file"
              class="hidden"
              onChange={handleCtxUploadChange}
            />
            <SearchInput />
            <div class="flex-1 overflow-y-auto">
              <FileTree
                projectId={props.projectId}
                onFileSelect={openFile}
                selectedPath={activeTab() ?? undefined}
                onContextMenu={handleContextMenu}
              />
            </div>
          </div>
          {/* Resize handle */}
          <div
            class="w-1 flex-shrink-0 cursor-col-resize hover:bg-cf-accent/40 active:bg-cf-accent/60 transition-colors"
            classList={{ "bg-cf-accent/60": dragging() }}
            onMouseDown={onDragStart}
            onDblClick={onDragDblClick}
            title="Drag to resize, double-click to reset"
          />
        </FileTreeProvider>
      </Show>

      {/* Mobile file tree */}
      <Show when={isMobile()}>
        {/* Mobile file tree toggle button */}
        <Show when={!fileDrawerOpen()}>
          <button
            type="button"
            class="flex items-center gap-2 px-3 py-2 text-sm text-cf-text-secondary hover:bg-cf-bg-surface-alt border-b border-cf-border w-full min-h-[44px]"
            onClick={() => setFileDrawerOpen(true)}
          >
            <svg
              class="h-4 w-4"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
              stroke-width="2"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M2.25 12.75V12A2.25 2.25 0 0 1 4.5 9.75h15A2.25 2.25 0 0 1 21.75 12v.75m-8.69-6.44-2.12-2.12a1.5 1.5 0 0 0-1.061-.44H4.5A2.25 2.25 0 0 0 2.25 6v12a2.25 2.25 0 0 0 2.25 2.25h15A2.25 2.25 0 0 0 21.75 18V9a2.25 2.25 0 0 0-2.25-2.25h-5.379a1.5 1.5 0 0 1-1.06-.44Z"
              />
            </svg>
            {t("detail.tab.files")}
          </button>
        </Show>

        {/* Mobile file tree overlay */}
        <Show when={fileDrawerOpen()}>
          <div class="fixed inset-0 z-40 bg-black/50" onClick={() => setFileDrawerOpen(false)} />
          <div class="fixed inset-y-0 left-0 z-50 w-72 flex flex-col border-r border-cf-border bg-cf-bg-surface shadow-cf-lg">
            <FileTreeProvider>
              <div class="flex items-center justify-between p-2 border-b border-cf-border">
                <span class="text-sm font-medium px-2">{t("detail.tab.files")}</span>
                <button
                  type="button"
                  class="p-2 min-h-[44px] min-w-[44px] flex items-center justify-center rounded-cf-md text-cf-text-muted hover:bg-cf-bg-surface-alt"
                  onClick={() => setFileDrawerOpen(false)}
                >
                  {"\u2715"}
                </button>
              </div>
              <SidebarHeader projectId={props.projectId} />
              <SearchInput />
              <div class="flex-1 overflow-y-auto">
                <FileTree
                  projectId={props.projectId}
                  onFileSelect={(path) => {
                    openFile(path);
                    setFileDrawerOpen(false);
                  }}
                  selectedPath={activeTab() ?? undefined}
                  onContextMenu={handleContextMenu}
                />
              </div>
            </FileTreeProvider>
          </div>
        </Show>
      </Show>

      {/* Editor Area */}
      <div class="flex-1 flex flex-col min-w-0">
        {/* Tab Bar */}
        <Show when={tabs().length > 0}>
          <div class="flex items-center border-b border-cf-border bg-cf-bg-surface overflow-x-auto flex-shrink-0">
            <For each={tabs()}>
              {(tab) => (
                <div
                  class={`group flex items-center gap-1.5 px-3 py-1.5 text-sm border-r border-cf-border cursor-pointer select-none whitespace-nowrap ${
                    tab.path === activeTab()
                      ? "bg-cf-bg-primary text-cf-text-primary"
                      : "bg-cf-bg-surface text-cf-text-tertiary hover:bg-cf-bg-surface-alt"
                  }`}
                  onClick={() => setActiveTab(tab.path)}
                >
                  <img
                    src={fileIconUrl(fileName(tab.path), false)}
                    alt=""
                    class="w-4 h-4 flex-shrink-0"
                  />
                  <span class={tab.modified ? "italic" : ""}>
                    {tab.modified ? "\u2022 " : ""}
                    {fileName(tab.path)}
                  </span>
                  <Button
                    variant="icon"
                    size="xs"
                    class="ml-1 opacity-0 group-hover:opacity-100 transition-opacity"
                    onClick={(e) => {
                      e.stopPropagation();
                      closeTab(tab.path);
                    }}
                    title={t("common.close")}
                  >
                    {"\u00D7"}
                  </Button>
                </div>
              )}
            </For>
          </div>
        </Show>

        {/* Editor Content */}
        <div class="flex-1 min-h-0">
          <Show
            when={currentTab()}
            fallback={
              <div class="flex flex-col items-center justify-center gap-3 py-16 text-center">
                <p class="text-sm text-cf-text-muted">
                  {props.hasWorkspace ? t("empty.files.select") : t("empty.files")}
                </p>
                <Show when={!props.hasWorkspace}>
                  <button
                    class="text-sm text-cf-accent hover:underline"
                    onClick={() => props.onNavigate?.("setup")}
                  >
                    {t("empty.files.action")}
                  </button>
                </Show>
              </div>
            }
          >
            {(tab) => (
              <CodeEditor
                value={tab().content}
                language={tab().language}
                path={tab().path}
                onChange={(val) => updateContent(tab().path, val)}
                onSave={saveActiveFile}
              />
            )}
          </Show>
        </div>

        {/* Status Bar */}
        <Show when={currentTab()}>
          {(tab) => (
            <div class="flex items-center justify-between px-3 py-1 border-t border-cf-border bg-cf-bg-surface text-xs text-cf-text-muted flex-shrink-0">
              <span class="truncate">{tab().path}</span>
              <div class="flex items-center gap-2">
                <span>{tab().language}</span>
                <Show when={tab().modified}>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={saveActiveFile}
                    disabled={saving()}
                    class="text-xs py-0 px-1.5"
                  >
                    {saving() ? t("files.saving") : t("files.save")}
                  </Button>
                </Show>
              </div>
            </div>
          )}
        </Show>
      </div>

      {/* Create File Modal */}
      <Modal
        open={showCreateModal()}
        onClose={() => setShowCreateModal(false)}
        title={t("files.createFile")}
      >
        <div class="flex flex-col gap-3 p-4">
          <FormField label={t("files.fileName")}>
            <Input
              placeholder={t("files.fileNamePlaceholder")}
              value={newFilePath()}
              onInput={(e) => setNewFilePath(e.currentTarget.value)}
              autofocus
            />
          </FormField>
          <FormField label={t("files.fileContent")}>
            <Textarea
              placeholder={t("files.fileContentPlaceholder")}
              value={newFileContent()}
              onInput={(e) => setNewFileContent(e.currentTarget.value)}
              rows={8}
            />
          </FormField>
          <div class="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setShowCreateModal(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              onClick={handleCreateFile}
              disabled={!newFilePath().trim() || creating()}
              loading={creating()}
            >
              {t("files.createFile")}
            </Button>
          </div>
        </div>
      </Modal>

      {/* New Folder Modal */}
      <Modal
        open={showFolderModal()}
        onClose={() => setShowFolderModal(false)}
        title={t("files.createFolder")}
      >
        <div class="flex flex-col gap-3 p-4">
          <FormField label={t("files.folderName")}>
            <Input
              placeholder={t("files.folderNamePlaceholder")}
              value={newFolderName()}
              onInput={(e) => setNewFolderName(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && newFolderName().trim()) {
                  handleCreateFolder();
                }
              }}
              autofocus
            />
          </FormField>
          <div class="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setShowFolderModal(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              onClick={handleCreateFolder}
              disabled={!newFolderName().trim() || creatingFolder()}
              loading={creatingFolder()}
            >
              {t("files.createFolder")}
            </Button>
          </div>
        </div>
      </Modal>

      {/* File Context Menu */}
      <FileContextMenu
        visible={ctxMenuVisible()}
        x={ctxMenuX()}
        y={ctxMenuY()}
        entry={ctxMenuEntry()}
        isRootArea={ctxMenuIsRoot()}
        onAction={handleContextMenuAction}
        onClose={closeContextMenu}
      />

      {/* Rename Modal */}
      <Modal
        open={showRenameModal()}
        onClose={() => setShowRenameModal(false)}
        title={t("files.rename")}
      >
        <div class="flex flex-col gap-3 p-4">
          <FormField label={t("files.newName")}>
            <Input
              placeholder={t("files.newNamePlaceholder")}
              value={renameNewName()}
              onInput={(e) => setRenameNewName(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && renameNewName().trim()) {
                  handleRename();
                }
              }}
              autofocus
            />
          </FormField>
          <div class="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setShowRenameModal(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              onClick={handleRename}
              disabled={!renameNewName().trim() || renaming()}
              loading={renaming()}
            >
              {t("files.rename")}
            </Button>
          </div>
        </div>
      </Modal>

      {/* Delete Confirmation Modal */}
      <Modal
        open={showDeleteModal()}
        onClose={() => setShowDeleteModal(false)}
        title={deleteIsDir() ? t("files.deleteFolder") : t("files.deleteFile")}
      >
        <div class="flex flex-col gap-3 p-4">
          <p class="text-sm text-cf-text-secondary">
            {t("files.deleteConfirm")}{" "}
            <span class="font-medium text-cf-text-primary">{deletePath()}</span>?
            {deleteIsDir() ? " " + t("files.deleteFolderWarning") : ""}
          </p>
          <div class="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setShowDeleteModal(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              onClick={handleDelete}
              disabled={deleting()}
              loading={deleting()}
              class="!bg-red-600 hover:!bg-red-700 !text-white"
            >
              {t("common.delete")}
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
