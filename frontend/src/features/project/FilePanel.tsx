import { createSignal, For, type JSX, Show } from "solid-js";

import { api } from "~/api/client";
import type { FileContent } from "~/api/types";
import { useToast } from "~/components/Toast";
import { Button } from "~/ui";

import CodeEditor from "./CodeEditor";
import FileTree from "./FileTree";

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
    // If already open, just activate
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

  return (
    <div class="flex h-full min-h-0">
      {/* File Tree Sidebar */}
      <div class="w-56 flex-shrink-0 border-r border-cf-border overflow-y-auto bg-cf-bg-surface">
        <div class="flex items-center justify-between px-2 py-1.5 border-b border-cf-border">
          <span class="text-xs font-semibold text-cf-text-secondary uppercase tracking-wide">
            Files
          </span>
        </div>
        <FileTree
          projectId={props.projectId}
          onFileSelect={openFile}
          selectedPath={activeTab() ?? undefined}
        />
      </div>

      {/* Editor Area */}
      <div class="flex-1 flex flex-col min-w-0">
        {/* Tab Bar */}
        <Show when={tabs().length > 0}>
          <div class="flex items-center border-b border-cf-border bg-cf-bg-surface overflow-x-auto flex-shrink-0">
            <For each={tabs()}>
              {(tab) => (
                <div
                  class={`group flex items-center gap-1 px-3 py-1.5 text-sm border-r border-cf-border cursor-pointer select-none whitespace-nowrap ${
                    tab.path === activeTab()
                      ? "bg-cf-bg-primary text-cf-text-primary"
                      : "bg-cf-bg-surface text-cf-text-tertiary hover:bg-cf-bg-surface-alt"
                  }`}
                  onClick={() => setActiveTab(tab.path)}
                >
                  <span class={tab.modified ? "italic" : ""}>
                    {tab.modified ? "\u2022 " : ""}
                    {fileName(tab.path)}
                  </span>
                  <button
                    type="button"
                    class="ml-1 text-cf-text-muted hover:text-cf-text-primary opacity-0 group-hover:opacity-100 transition-opacity"
                    onClick={(e) => {
                      e.stopPropagation();
                      closeTab(tab.path);
                    }}
                    title="Close"
                  >
                    {"\u00D7"}
                  </button>
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
