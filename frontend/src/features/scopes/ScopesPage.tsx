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
import {
  Badge,
  Button,
  Card,
  EmptyState,
  FormField,
  Input,
  LoadingState,
  PageLayout,
  Select,
  Tabs,
} from "~/ui";

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
    <PageLayout
      title={t("scope.title")}
      description={t("scope.description")}
      action={
        <Button onClick={() => setShowForm((v) => !v)}>
          {showForm() ? t("common.cancel") : t("scope.form.create")}
        </Button>
      }
    >
      <Show when={showForm()}>
        <form onSubmit={handleCreate} class="mb-6" aria-label={t("scope.form.create")}>
          <Card>
            <Card.Body>
              <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <FormField label={t("scope.form.name")} id="scope-name" required>
                  <Input
                    id="scope-name"
                    type="text"
                    value={formName()}
                    onInput={(e) => setFormName(e.currentTarget.value)}
                    required
                  />
                </FormField>
                <FormField label={t("scope.form.type")} id="scope-type">
                  <Select
                    id="scope-type"
                    value={formType()}
                    onChange={(e) => setFormType(e.currentTarget.value as ScopeType)}
                  >
                    <For each={SCOPE_TYPES}>
                      {(st) => (
                        <option value={st}>
                          {t(`scope.type.${st}` as keyof typeof import("~/i18n/en").default)}
                        </option>
                      )}
                    </For>
                  </Select>
                </FormField>
                <FormField
                  label={t("scope.form.description")}
                  id="scope-desc"
                  class="sm:col-span-2"
                >
                  <Input
                    id="scope-desc"
                    type="text"
                    value={formDesc()}
                    onInput={(e) => setFormDesc(e.currentTarget.value)}
                  />
                </FormField>
                <div class="sm:col-span-2">
                  <label class="block text-sm font-medium text-cf-text-secondary">
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
                                ? "border-cf-accent bg-cf-accent/10 text-cf-accent"
                                : "border-cf-border text-cf-text-tertiary hover:border-cf-border-input"
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
                <Button type="submit">{t("scope.form.create")}</Button>
              </div>
            </Card.Body>
          </Card>
        </form>
      </Show>

      <Show when={scopes.loading}>
        <LoadingState message={t("scope.loading")} />
      </Show>

      <Show when={!scopes.loading}>
        <Show when={sorted().length} fallback={<EmptyState title={t("scope.empty")} />}>
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
    </PageLayout>
  );
}

// ---------------------------------------------------------------------------
// ScopeCard
// ---------------------------------------------------------------------------

const typeVariants: Record<string, "primary" | "info"> = {
  shared: "info",
  global: "primary",
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
    <Card class="transition-shadow hover:shadow-md">
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
            <h3 class="text-lg font-semibold text-cf-text-primary">{props.scope.name}</h3>
            <Show when={props.scope.description}>
              <p class="mt-1 text-sm text-cf-text-muted">{props.scope.description}</p>
            </Show>
          </div>
        </div>

        <div class="mt-3 flex flex-wrap items-center gap-2">
          <Badge variant={typeVariants[props.scope.type] ?? "info"} pill>
            {t(`scope.type.${props.scope.type}` as keyof typeof import("~/i18n/en").default)}
          </Badge>
          <span class="text-xs text-cf-text-muted">
            {props.scope.project_ids?.length ?? 0} {t("scope.projects.count")}
          </span>
        </div>
      </div>

      {/* Expanded detail panel */}
      <Show when={props.expanded}>
        <div class="border-t border-cf-border p-5">
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
    </Card>
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
  const [tab, setTab] = createSignal<string>("projects");
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

  const attachableKbs = (): KnowledgeBase[] => {
    const attached = new Set((kbs() ?? []).map((kb) => kb.id));
    return (allKbs() ?? []).filter((kb) => !attached.has(kb.id));
  };

  const tabItems = [
    { value: "projects", label: t("scope.form.projects") },
    { value: "kbs", label: t("scope.kbs.title") },
    { value: "search", label: t("scope.search.title") },
  ];

  return (
    <div>
      <div class="mb-4 flex items-center gap-2">
        <Tabs items={tabItems} value={tab()} onChange={setTab} variant="pills" />
        <div class="flex-1" />
        <Button variant="danger" size="sm" onClick={() => props.onDelete(props.scope.id)}>
          {t("common.delete")}
        </Button>
      </div>

      {/* Projects tab */}
      <Show when={tab() === "projects"}>
        <div class="space-y-2">
          <Show
            when={props.scopeProjects.length}
            fallback={<p class="text-sm text-cf-text-muted">{t("scope.projects.none")}</p>}
          >
            <For each={props.scopeProjects}>
              {(proj) => (
                <div class="flex items-center justify-between rounded-cf-md border border-cf-border px-3 py-2">
                  <span class="text-sm text-cf-text-primary">{proj.name}</span>
                  <Button
                    variant="danger"
                    size="sm"
                    onClick={() => props.onRemoveProject(props.scope.id, proj.id)}
                  >
                    {t("common.delete")}
                  </Button>
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
                    class="rounded-full border border-cf-border px-2 py-0.5 text-xs text-cf-text-tertiary hover:border-cf-accent hover:text-cf-accent"
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
            fallback={<p class="text-sm text-cf-text-muted">{t("scope.kbs.none")}</p>}
          >
            <For each={kbs() ?? []}>
              {(kb) => (
                <div class="flex items-center justify-between rounded-cf-md border border-cf-border px-3 py-2">
                  <span class="text-sm text-cf-text-primary">{kb.name}</span>
                  <Button variant="danger" size="sm" onClick={() => handleDetachKB(kb.id)}>
                    {t("scope.kbs.detach")}
                  </Button>
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
                    class="rounded-full border border-cf-border px-2 py-0.5 text-xs text-cf-text-tertiary hover:border-cf-accent hover:text-cf-accent"
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
            <Input
              type="text"
              value={searchQuery()}
              onInput={(e) => setSearchQuery(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleSearch();
              }}
              placeholder={t("scope.search.query")}
              class="flex-1"
            />
            <Button
              onClick={handleSearch}
              disabled={searching() || !searchQuery().trim()}
              loading={searching()}
            >
              {t("scope.search.hybrid")}
            </Button>
          </div>

          <Show when={searching()}>
            <LoadingState message={t("common.loading")} />
          </Show>

          <Show when={searchResults() !== null && !searching()}>
            {(() => {
              const results = () => searchResults() ?? [];
              return (
                <Show
                  when={results().length}
                  fallback={<p class="text-sm text-cf-text-muted">{t("scope.search.noResults")}</p>}
                >
                  <p class="text-xs text-cf-text-muted">
                    {results().length} {t("scope.search.results")}
                  </p>
                  <div class="max-h-80 space-y-2 overflow-y-auto">
                    <For each={results()}>
                      {(hit) => (
                        <Card>
                          <Card.Body class="p-3">
                            <div class="flex items-baseline justify-between">
                              <span class="text-xs font-medium text-cf-text-primary">
                                {hit.filepath}:{hit.start_line}
                              </span>
                              <span class="text-xs text-cf-text-muted">
                                score: {hit.score.toFixed(4)}
                              </span>
                            </div>
                            <Show when={hit.symbol_name}>
                              <Badge variant="info" class="mt-1">
                                {hit.symbol_name}
                              </Badge>
                            </Show>
                            <pre class="mt-1 max-h-24 overflow-auto rounded bg-cf-bg-surface-alt p-2 text-xs text-cf-text-secondary">
                              {hit.content.slice(0, 500)}
                            </pre>
                          </Card.Body>
                        </Card>
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
