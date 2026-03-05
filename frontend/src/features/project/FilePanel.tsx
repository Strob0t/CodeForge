import { createSignal, For, type JSX, Show } from "solid-js";

import { api } from "~/api/client";
import type { FileContent } from "~/api/types";
import { useToast } from "~/components/Toast";
import { fileIconUrl } from "~/lib/file-icon";
import { Button, Spinner } from "~/ui";

import CodeEditor from "./CodeEditor";
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
      <polyline points="4 6 8 10 12 6" />
      <polyline points="4 2 8 6 12 2" />
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
      <polyline points="4 10 8 6 12 10" />
      <polyline points="4 14 8 10 12 14" />
    </svg>
  );
}

// ---------------------------------------------------------------------------
// Sidebar header (needs FileTreeContext)
// ---------------------------------------------------------------------------

function SidebarHeader(props: { projectId: string }): JSX.Element {
  const [, actions] = useFileTree();

  return (
    <div class="flex items-center justify-between px-2 py-1.5 border-b border-cf-border">
      <span class="text-xs font-semibold text-cf-text-secondary uppercase tracking-wide">
        Files
      </span>
      <div class="flex items-center gap-0.5">
        <Button
          variant="icon"
          size="xs"
          onClick={() => actions.expandAll(props.projectId)}
          title="Expand All"
        >
          <ExpandAllIcon />
        </Button>
        <Button variant="icon" size="xs" onClick={() => actions.collapseAll()} title="Collapse All">
          <CollapseAllIcon />
        </Button>
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
  const [tabs, setTabs] = createSignal<OpenTab[]>([]);
  const [activeTab, setActiveTab] = createSignal<string | null>(null);
  const [saving, setSaving] = createSignal(false);

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
      const msg = e instanceof Error ? e.message : "Failed to open file";
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

  async function saveActiveFile() {
    const path = activeTab();
    if (!path) return;

    const tab = tabs().find((t) => t.path === path);
    if (!tab || !tab.modified) return;

    setSaving(true);
    try {
      await api.files.write(props.projectId, path, tab.content);
      setTabs((prev) =>
        prev.map((t) =>
          t.path === path ? { ...t, modified: false, originalContent: t.content } : t,
        ),
      );
      toast("success", `Saved ${fileName(path)}`);
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Failed to save file";
      toast("error", msg);
    } finally {
      setSaving(false);
    }
  }

  const currentTab = () => tabs().find((t) => t.path === activeTab());

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

  return (
    <div class="flex h-full min-h-0" style={{ cursor: dragging() ? "col-resize" : undefined }}>
      {/* File Tree Sidebar */}
      <FileTreeProvider>
        <div
          class="relative flex-shrink-0 border-r border-cf-border overflow-hidden flex flex-col bg-cf-bg-surface"
          style={{ width: `${sidebarWidth()}px` }}
        >
          <TreeLoadingOverlay />
          <SidebarHeader projectId={props.projectId} />
          <SearchInput />
          <div class="flex-1 overflow-y-auto">
            <FileTree
              projectId={props.projectId}
              onFileSelect={openFile}
              selectedPath={activeTab() ?? undefined}
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
                    title="Close"
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
              <div class="flex items-center justify-center h-full text-cf-text-muted text-sm">
                Select a file to edit
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
                    {saving() ? "Saving..." : "Save"}
                  </Button>
                </Show>
              </div>
            </div>
          )}
        </Show>
      </div>
    </div>
  );
}
