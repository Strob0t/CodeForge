import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type {
  CreateScopeRequest,
  KnowledgeBase,
  Project,
  RetrievalScope,
  ScopeType,
} from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";

const SCOPE_TYPES: ScopeType[] = ["shared", "global"];

export default function ScopesPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [scopes, { refetch }] = createResource(() => api.scopes.list());
  const [projects] = createResource(() => api.projects.list());
  const [showForm, setShowForm] = createSignal(false);
  const [expanded, setExpanded] = createSignal<string | null>(null);

  // -- Form state --
  const [formName, setFormName] = createSignal("");
  const [formDesc, setFormDesc] = createSignal("");
  const [formType, setFormType] = createSignal<ScopeType>("shared");
  const [formProjects, setFormProjects] = createSignal<string[]>([]);

  const resetForm = () => {
    setFormName("");
    setFormDesc("");
    setFormType("shared");
    setFormProjects([]);
  };

  const handleCreate = async (e: SubmitEvent) => {
    e.preventDefault();
    const name = formName().trim();
    if (!name) return;
    try {
      const req: CreateScopeRequest = {
        name,
        description: formDesc().trim(),
        type: formType(),
        project_ids: formProjects(),
      };
      await api.scopes.create(req);
      resetForm();
      setShowForm(false);
      refetch();
      toast("success", t("scope.toast.created"));
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to create scope");
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await api.scopes.delete(id);
      if (expanded() === id) setExpanded(null);
      refetch();
      toast("success", t("scope.toast.deleted"));
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete scope");
    }
  };

  const handleAddProject = async (scopeId: string, projectId: string) => {
    try {
      await api.scopes.addProject(scopeId, projectId);
      refetch();
      toast("success", t("scope.toast.projectAdded"));
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to add project");
    }
  };

  const handleRemoveProject = async (scopeId: string, projectId: string) => {
    try {
      await api.scopes.removeProject(scopeId, projectId);
      refetch();
      toast("success", t("scope.toast.projectRemoved"));
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to remove project");
    }
  };

  const toggleProject = (pid: string) => {
    const current = formProjects();
    if (current.includes(pid)) {
      setFormProjects(current.filter((p) => p !== pid));
    } else {
      setFormProjects([...current, pid]);
    }
  };

  const sorted = () => {
    const list = scopes() ?? [];
    return [...list].sort((a, b) => a.name.localeCompare(b.name));
  };

  return (
    <div>
      <div class="mb-6 flex items-center justify-between">
        <div>
          <h2 class="text-2xl font-bold text-gray-900 dark:text-gray-100">{t("scope.title")}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{t("scope.description")}</p>
        </div>
        <button
          type="button"
          class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
          onClick={() => setShowForm((v) => !v)}
        >
          {showForm() ? t("common.cancel") : t("scope.form.create")}
        </button>
      </div>

      <Show when={showForm()}>
        <form
          onSubmit={handleCreate}
          class="mb-6 rounded-lg border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800"
          aria-label={t("scope.form.create")}
        >
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label
                for="scope-name"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("scope.form.name")} *
              </label>
              <input
                id="scope-name"
                type="text"
                value={formName()}
                onInput={(e) => setFormName(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
                required
              />
            </div>
            <div>
              <label
                for="scope-type"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("scope.form.type")}
              </label>
              <select
                id="scope-type"
                value={formType()}
                onChange={(e) => setFormType(e.currentTarget.value as ScopeType)}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
              >
                <For each={SCOPE_TYPES}>
                  {(st) => (
                    <option value={st}>
                      {t(`scope.type.${st}` as keyof typeof import("~/i18n/en").default)}
                    </option>
                  )}
                </For>
              </select>
            </div>
            <div class="sm:col-span-2">
              <label
                for="scope-desc"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("scope.form.description")}
              </label>
              <input
                id="scope-desc"
                type="text"
                value={formDesc()}
                onInput={(e) => setFormDesc(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
              />
            </div>
            <div class="sm:col-span-2">
              <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
                {t("scope.form.projects")}
              </label>
              <div class="mt-2 flex flex-wrap gap-2">
                <For each={projects() ?? []}>
                  {(proj) => {
                    const selected = () => formProjects().includes(proj.id);
                    return (
                      <button
                        type="button"
                        class={`rounded-full border px-3 py-1 text-xs font-medium transition-colors ${
                          selected()
                            ? "border-blue-500 bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400"
                            : "border-gray-300 text-gray-600 hover:border-gray-400 dark:border-gray-600 dark:text-gray-400"
                        }`}
                        onClick={() => toggleProject(proj.id)}
                      >
                        {proj.name}
                      </button>
                    );
                  }}
                </For>
              </div>
            </div>
          </div>
          <div class="mt-4 flex justify-end">
            <button
              type="submit"
              class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
            >
              {t("scope.form.create")}
            </button>
          </div>
        </form>
      </Show>

      <Show when={scopes.loading}>
        <p class="text-sm text-gray-500 dark:text-gray-400">{t("scope.loading")}</p>
      </Show>

      <Show when={!scopes.loading}>
        <Show
          when={sorted().length}
          fallback={<p class="text-sm text-gray-500 dark:text-gray-400">{t("scope.empty")}</p>}
        >
          <div class="grid grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-3">
            <For each={sorted()}>
              {(scope) => (
                <ScopeCard
                  scope={scope}
                  projects={projects() ?? []}
                  expanded={expanded() === scope.id}
                  onToggle={() => setExpanded((v) => (v === scope.id ? null : scope.id))}
                  onDelete={handleDelete}
                  onAddProject={handleAddProject}
                  onRemoveProject={handleRemoveProject}
                  onRefetch={refetch}
                />
              )}
            </For>
          </div>
        </Show>
      </Show>
    </div>
  );
}

// ---------------------------------------------------------------------------
// ScopeCard
// ---------------------------------------------------------------------------

const typeColors: Record<string, string> = {
  shared: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
  global: "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400",
};

function ScopeCard(props: {
  scope: RetrievalScope;
  projects: Project[];
  expanded: boolean;
  onToggle: () => void;
  onDelete: (id: string) => void;
  onAddProject: (scopeId: string, projectId: string) => void;
  onRemoveProject: (scopeId: string, projectId: string) => void;
  onRefetch: () => void;
}) {
  const { t } = useI18n();

  const scopeProjects = () => props.projects.filter((p) => props.scope.project_ids?.includes(p.id));

  const availableProjects = () =>
    props.projects.filter((p) => !props.scope.project_ids?.includes(p.id));

  return (
    <div class="rounded-lg border border-gray-200 bg-white shadow-sm transition-shadow hover:shadow-md dark:border-gray-700 dark:bg-gray-800 dark:shadow-gray-900/30 dark:hover:shadow-gray-900/30">
      <div
        class="cursor-pointer p-5"
        onClick={() => props.onToggle()}
        role="button"
        tabIndex={0}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") props.onToggle();
        }}
      >
        <div class="flex items-start justify-between">
          <div class="min-w-0 flex-1">
            <h3 class="text-lg font-semibold text-gray-900 dark:text-gray-100">
              {props.scope.name}
            </h3>
            <Show when={props.scope.description}>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{props.scope.description}</p>
            </Show>
          </div>
        </div>

        <div class="mt-3 flex flex-wrap items-center gap-2">
          <span
            class={`rounded-full px-2 py-0.5 text-xs font-medium ${typeColors[props.scope.type] ?? typeColors.shared}`}
          >
            {t(`scope.type.${props.scope.type}` as keyof typeof import("~/i18n/en").default)}
          </span>
          <span class="text-xs text-gray-500 dark:text-gray-400">
            {props.scope.project_ids?.length ?? 0} {t("scope.projects.count")}
          </span>
        </div>
      </div>

      {/* Expanded detail panel */}
      <Show when={props.expanded}>
        <div class="border-t border-gray-200 p-5 dark:border-gray-700">
          <ScopeDetail
            scope={props.scope}
            scopeProjects={scopeProjects()}
            availableProjects={availableProjects()}
            onAddProject={props.onAddProject}
            onRemoveProject={props.onRemoveProject}
            onDelete={props.onDelete}
          />
        </div>
      </Show>
    </div>
  );
}

// ---------------------------------------------------------------------------
// ScopeDetail (expanded panel)
// ---------------------------------------------------------------------------

function ScopeDetail(props: {
  scope: RetrievalScope;
  scopeProjects: Project[];
  availableProjects: Project[];
  onAddProject: (scopeId: string, projectId: string) => void;
  onRemoveProject: (scopeId: string, projectId: string) => void;
  onDelete: (id: string) => void;
}) {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [tab, setTab] = createSignal<"projects" | "kbs" | "search">("projects");
  const [kbVersion, setKbVersion] = createSignal(0);
  const [kbs] = createResource(
    () => ({ id: props.scope.id, v: kbVersion() }),
    ({ id }) => api.knowledgeBases.listByScope(id),
  );
  const [allKbs] = createResource(() => api.knowledgeBases.list());

  // -- Search state --
  const [searchQuery, setSearchQuery] = createSignal("");
  const [searchResults, setSearchResults] = createSignal<
    import("~/api/types").RetrievalSearchHit[] | null
  >(null);
  const [searching, setSearching] = createSignal(false);

  const handleSearch = async () => {
    const q = searchQuery().trim();
    if (!q) return;
    setSearching(true);
    try {
      const result = await api.scopes.search(props.scope.id, { query: q, top_k: 20 });
      setSearchResults(result.results);
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Search failed");
    } finally {
      setSearching(false);
    }
  };

  const handleAttachKB = async (kbId: string) => {
    try {
      await api.knowledgeBases.attachToScope(props.scope.id, kbId);
      setKbVersion((v) => v + 1);
      toast("success", t("scope.toast.updated"));
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to attach KB");
    }
  };

  const handleDetachKB = async (kbId: string) => {
    try {
      await api.knowledgeBases.detachFromScope(props.scope.id, kbId);
      setKbVersion((v) => v + 1);
      toast("success", t("scope.toast.updated"));
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to detach KB");
    }
  };

  const tabClass = (active: boolean) =>
    `px-3 py-1.5 text-xs font-medium rounded-md transition-colors ${
      active
        ? "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400"
        : "text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300"
    }`;

  const attachableKbs = (): KnowledgeBase[] => {
    const attached = new Set((kbs() ?? []).map((kb) => kb.id));
    return (allKbs() ?? []).filter((kb) => !attached.has(kb.id));
  };

  return (
    <div>
      <div class="mb-4 flex items-center gap-2">
        <button
          type="button"
          class={tabClass(tab() === "projects")}
          onClick={() => setTab("projects")}
        >
          {t("scope.form.projects")}
        </button>
        <button type="button" class={tabClass(tab() === "kbs")} onClick={() => setTab("kbs")}>
          {t("scope.kbs.title")}
        </button>
        <button type="button" class={tabClass(tab() === "search")} onClick={() => setTab("search")}>
          {t("scope.search.title")}
        </button>
        <div class="flex-1" />
        <button
          type="button"
          class="rounded-md bg-red-50 px-3 py-1.5 text-xs font-medium text-red-600 hover:bg-red-100 dark:bg-red-900/20 dark:text-red-400 dark:hover:bg-red-900/40"
          onClick={() => props.onDelete(props.scope.id)}
        >
          {t("common.delete")}
        </button>
      </div>

      {/* Projects tab */}
      <Show when={tab() === "projects"}>
        <div class="space-y-2">
          <Show
            when={props.scopeProjects.length}
            fallback={
              <p class="text-sm text-gray-400 dark:text-gray-500">{t("scope.projects.none")}</p>
            }
          >
            <For each={props.scopeProjects}>
              {(proj) => (
                <div class="flex items-center justify-between rounded-md border border-gray-100 px-3 py-2 dark:border-gray-700">
                  <span class="text-sm text-gray-800 dark:text-gray-200">{proj.name}</span>
                  <button
                    type="button"
                    class="text-xs text-red-500 hover:text-red-700"
                    onClick={() => props.onRemoveProject(props.scope.id, proj.id)}
                  >
                    {t("common.delete")}
                  </button>
                </div>
              )}
            </For>
          </Show>

          <Show when={props.availableProjects.length}>
            <div class="mt-3 flex flex-wrap gap-1">
              <For each={props.availableProjects}>
                {(proj) => (
                  <button
                    type="button"
                    class="rounded-full border border-gray-300 px-2 py-0.5 text-xs text-gray-600 hover:border-blue-400 hover:text-blue-600 dark:border-gray-600 dark:text-gray-400"
                    onClick={() => props.onAddProject(props.scope.id, proj.id)}
                  >
                    + {proj.name}
                  </button>
                )}
              </For>
            </div>
          </Show>
        </div>
      </Show>

      {/* Knowledge Bases tab */}
      <Show when={tab() === "kbs"}>
        <div class="space-y-2">
          <Show
            when={(kbs() ?? []).length}
            fallback={<p class="text-sm text-gray-400 dark:text-gray-500">{t("scope.kbs.none")}</p>}
          >
            <For each={kbs() ?? []}>
              {(kb) => (
                <div class="flex items-center justify-between rounded-md border border-gray-100 px-3 py-2 dark:border-gray-700">
                  <span class="text-sm text-gray-800 dark:text-gray-200">{kb.name}</span>
                  <button
                    type="button"
                    class="text-xs text-red-500 hover:text-red-700"
                    onClick={() => handleDetachKB(kb.id)}
                  >
                    {t("scope.kbs.detach")}
                  </button>
                </div>
              )}
            </For>
          </Show>

          <Show when={attachableKbs().length}>
            <div class="mt-3 flex flex-wrap gap-1">
              <For each={attachableKbs()}>
                {(kb) => (
                  <button
                    type="button"
                    class="rounded-full border border-gray-300 px-2 py-0.5 text-xs text-gray-600 hover:border-blue-400 hover:text-blue-600 dark:border-gray-600 dark:text-gray-400"
                    onClick={() => handleAttachKB(kb.id)}
                  >
                    + {kb.name}
                  </button>
                )}
              </For>
            </div>
          </Show>
        </div>
      </Show>

      {/* Search tab */}
      <Show when={tab() === "search"}>
        <div class="space-y-3">
          <div class="flex gap-2">
            <input
              type="text"
              value={searchQuery()}
              onInput={(e) => setSearchQuery(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleSearch();
              }}
              placeholder={t("scope.search.query")}
              class="flex-1 rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
            />
            <button
              type="button"
              class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
              onClick={handleSearch}
              disabled={searching() || !searchQuery().trim()}
            >
              {t("scope.search.hybrid")}
            </button>
          </div>

          <Show when={searching()}>
            <p class="text-sm text-gray-500">{t("common.loading")}</p>
          </Show>

          <Show when={searchResults() !== null && !searching()}>
            {(() => {
              const results = () => searchResults() ?? [];
              return (
                <Show
                  when={results().length}
                  fallback={<p class="text-sm text-gray-400">{t("scope.search.noResults")}</p>}
                >
                  <p class="text-xs text-gray-500">
                    {results().length} {t("scope.search.results")}
                  </p>
                  <div class="max-h-80 space-y-2 overflow-y-auto">
                    <For each={results()}>
                      {(hit) => (
                        <div class="rounded-md border border-gray-100 p-3 dark:border-gray-700">
                          <div class="flex items-baseline justify-between">
                            <span class="text-xs font-medium text-gray-800 dark:text-gray-200">
                              {hit.filepath}:{hit.start_line}
                            </span>
                            <span class="text-xs text-gray-400">score: {hit.score.toFixed(4)}</span>
                          </div>
                          <Show when={hit.symbol_name}>
                            <span class="text-xs text-blue-600 dark:text-blue-400">
                              {hit.symbol_name}
                            </span>
                          </Show>
                          <pre class="mt-1 max-h-24 overflow-auto rounded bg-gray-50 p-2 text-xs text-gray-700 dark:bg-gray-900 dark:text-gray-300">
                            {hit.content.slice(0, 500)}
                          </pre>
                        </div>
                      )}
                    </For>
                  </div>
                </Show>
              );
            })()}
          </Show>
        </div>
      </Show>
    </div>
  );
}
