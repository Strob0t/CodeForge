import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { Project } from "~/api/types";
import { useI18n } from "~/i18n";
import { Badge, Card, LoadingState, PageLayout } from "~/ui";

export default function SearchPage() {
  const { t } = useI18n();

  const [query, setQuery] = createSignal("");
  const [debouncedQuery, setDebouncedQuery] = createSignal("");
  const [selectedProjectIds, setSelectedProjectIds] = createSignal<string[]>([]);
  let debounceTimer: ReturnType<typeof setTimeout> | undefined;

  const [projects] = createResource(() => api.projects.list());

  const [results] = createResource(
    () => {
      const q = debouncedQuery();
      if (!q.trim()) return null;
      return { q, pids: selectedProjectIds().length > 0 ? selectedProjectIds() : undefined };
    },
    (params) => {
      if (!params) return null;
      return api.search.global(params.q, params.pids, 30);
    },
  );

  function onInput(value: string) {
    setQuery(value);
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => setDebouncedQuery(value), 300);
  }

  function toggleProject(id: string) {
    setSelectedProjectIds((prev) =>
      prev.includes(id) ? prev.filter((p) => p !== id) : [...prev, id],
    );
  }

  function projectName(id: string): string {
    return (projects() ?? []).find((p: Project) => p.id === id)?.name ?? id.slice(0, 8);
  }

  return (
    <PageLayout title={t("search.title")}>
      {/* Search input */}
      <div class="flex flex-col gap-3 sm:flex-row sm:items-start">
        <input
          type="text"
          value={query()}
          onInput={(e) => onInput(e.currentTarget.value)}
          placeholder={t("search.placeholder")}
          class="flex-1 rounded-cf-md border border-cf-border bg-cf-bg-surface px-3 py-2 text-sm text-cf-text-primary placeholder:text-cf-text-muted focus:border-cf-accent focus:outline-none focus:ring-1 focus:ring-cf-accent"
        />
      </div>

      {/* Project filter */}
      <Show when={(projects() ?? []).length > 0}>
        <div class="mt-3">
          <p class="mb-1.5 text-xs font-medium text-cf-text-secondary">
            {t("search.filterProjects")}
          </p>
          <div class="flex flex-wrap gap-1.5">
            <button
              type="button"
              class={`rounded-full px-2.5 py-0.5 text-xs transition-colors ${
                selectedProjectIds().length === 0
                  ? "bg-cf-accent text-white"
                  : "bg-cf-bg-surface-alt text-cf-text-secondary hover:text-cf-text-primary"
              }`}
              onClick={() => setSelectedProjectIds([])}
            >
              {t("search.allProjects")}
            </button>
            <For each={projects()}>
              {(p) => (
                <button
                  type="button"
                  class={`rounded-full px-2.5 py-0.5 text-xs transition-colors ${
                    selectedProjectIds().includes(p.id)
                      ? "bg-cf-accent text-white"
                      : "bg-cf-bg-surface-alt text-cf-text-secondary hover:text-cf-text-primary"
                  }`}
                  onClick={() => toggleProject(p.id)}
                >
                  {p.name}
                </button>
              )}
            </For>
          </div>
        </div>
      </Show>

      {/* Loading */}
      <Show when={results.loading}>
        <div class="mt-6">
          <LoadingState message={t("common.loading")} />
        </div>
      </Show>

      {/* Error */}
      <Show when={results.error}>
        <p class="mt-6 text-sm text-cf-danger-fg">{t("search.error")}</p>
      </Show>

      {/* Results */}
      <Show when={!results.loading && !results.error && results() !== undefined}>
        <Show
          when={results()?.results?.length}
          fallback={
            <Show when={debouncedQuery().trim()}>
              <p class="mt-6 text-sm text-cf-text-muted">{t("search.noResults")}</p>
            </Show>
          }
        >
          <p class="mt-4 text-xs text-cf-text-secondary">
            {t("search.results", { count: results()?.total ?? 0 })}
          </p>
          <div class="mt-2 space-y-2">
            <For each={results()?.results}>
              {(hit) => (
                <a href={`/projects/${hit.project_id}`} class="block">
                  <Card class="transition-shadow hover:shadow-md">
                    <Card.Body>
                      <div class="flex flex-wrap items-center gap-2">
                        <Badge>{projectName(hit.project_id)}</Badge>
                        <span class="text-sm font-medium text-cf-text-primary">{hit.file}</span>
                        <span class="text-xs text-cf-text-muted">
                          {t("search.line", { line: hit.start_line })}
                        </span>
                        <Show when={hit.language}>
                          <span class="text-xs text-cf-text-muted">{hit.language}</span>
                        </Show>
                      </div>
                      <Show when={hit.snippet}>
                        <pre class="mt-2 overflow-x-auto rounded bg-cf-bg-inset px-3 py-2 text-xs text-cf-text-secondary">
                          {hit.snippet}
                        </pre>
                      </Show>
                    </Card.Body>
                  </Card>
                </a>
              )}
            </For>
          </div>
        </Show>
      </Show>
    </PageLayout>
  );
}
