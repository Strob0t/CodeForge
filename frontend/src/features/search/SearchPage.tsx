import { A, useNavigate } from "@solidjs/router";
import { createResource, createSignal, For, onMount, Show } from "solid-js";

import { api } from "~/api/client";
import type { Project, RetrievalIndexStatus } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { extractErrorMessage } from "~/lib/errorUtils";
import { Badge, Card, LoadingState, PageLayout, Tabs } from "~/ui";

import ConversationResults, { type ConversationResult } from "./ConversationResults";

type SearchTab = "code" | "conversations";

/**
 * Searches the tenant's projects: code through each project's retrieval index
 * (POST /search), conversation messages by full text (POST /search/conversations).
 */
export default function SearchPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const navigate = useNavigate();
  onMount(() => {
    document.title = `${t("app.nav.search")} - CodeForge`;
  });

  // A hit names its conversation but not the project; the conversation does.
  async function openConversation(result: ConversationResult): Promise<void> {
    try {
      const conversation = await api.conversations.get(result.conversation_id);
      navigate(
        `/projects/${encodeURIComponent(conversation.project_id)}?conversation=${encodeURIComponent(conversation.id)}`,
      );
    } catch (err) {
      toast("error", extractErrorMessage(err, t("search.error")));
    }
  }

  const [query, setQuery] = createSignal("");
  const [debouncedQuery, setDebouncedQuery] = createSignal("");
  const [selectedProjectIds, setSelectedProjectIds] = createSignal<string[]>([]);
  const [activeTab, setActiveTab] = createSignal<SearchTab>("code");
  let debounceTimer: ReturnType<typeof setTimeout> | undefined;

  const [projects] = createResource(() => api.projects.list());

  // Code search resource — fires when tab is "code" and query is non-empty
  const [codeResults] = createResource(
    () => {
      const q = debouncedQuery();
      if (!q.trim() || activeTab() !== "code") return null;
      return { q, pids: selectedProjectIds().length > 0 ? selectedProjectIds() : undefined };
    },
    (params) => {
      if (!params) return null;
      return api.search.global(params.q, params.pids, 30);
    },
  );

  // Conversation search resource — fires when tab is "conversations" and query is non-empty
  const [convResults] = createResource(
    () => {
      const q = debouncedQuery();
      if (!q.trim() || activeTab() !== "conversations") return null;
      return { q, pids: selectedProjectIds().length > 0 ? selectedProjectIds() : undefined };
    },
    (params) => {
      if (!params) return null;
      return api.search.conversations(params.q, params.pids, 30);
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

  function indexNote(index: RetrievalIndexStatus): string {
    const project = projectName(index.project_id);
    if (index.status === "error") {
      return t("search.indexError", { project, error: index.error ?? "" });
    }
    if (index.status === "building") return t("search.indexBuilding", { project });
    return t("search.indexBm25Only", { project });
  }

  const tabItems = [
    { value: "code", label: t("search.tabCode") },
    { value: "conversations", label: t("search.tabConversations") },
  ];

  // Derived active resource for loading/error display
  const isLoading = () => (activeTab() === "code" ? codeResults.loading : convResults.loading);
  const hasError = () => (activeTab() === "code" ? codeResults.error : convResults.error);

  return (
    <PageLayout title={t("search.title")}>
      {/* Search input */}
      <div class="flex flex-col gap-3 sm:flex-row sm:items-start">
        <input
          type="text"
          aria-label={t("search.placeholder")}
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
              class={`rounded-full px-2.5 py-0.5 text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cf-focus-ring focus-visible:ring-offset-2 ${
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
                  class={`rounded-full px-2.5 py-0.5 text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cf-focus-ring focus-visible:ring-offset-2 ${
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

      {/* Tabs */}
      <div class="mt-4">
        <Tabs
          items={tabItems}
          value={activeTab()}
          onChange={(v) => setActiveTab(v as SearchTab)}
          variant="pills"
        />
      </div>

      {/* Loading */}
      <Show when={isLoading()}>
        <div class="mt-6">
          <LoadingState message={t("common.loading")} />
        </div>
      </Show>

      {/* Error */}
      <Show when={hasError()}>
        <p class="mt-6 text-sm text-cf-danger-fg">{t("search.error")}</p>
      </Show>

      {/* Code results */}
      <Show
        when={
          activeTab() === "code" &&
          !codeResults.loading &&
          !codeResults.error &&
          codeResults() !== undefined
        }
      >
        {/* Why the searched indexes find less (KI-150) */}
        <Show when={codeResults()?.indexes?.length}>
          <ul class="mt-4 space-y-1 text-xs text-cf-text-secondary">
            <For each={codeResults()?.indexes}>{(index) => <li>{indexNote(index)}</li>}</For>
          </ul>
        </Show>
        <Show
          when={codeResults()?.results?.length}
          fallback={
            <Show when={debouncedQuery().trim()}>
              <p class="mt-6 text-sm text-cf-text-muted">{t("search.noResults")}</p>
            </Show>
          }
        >
          <p class="mt-4 text-xs text-cf-text-secondary">
            {t("search.results", { count: codeResults()?.total ?? 0 })}
          </p>
          <div class="mt-2 space-y-2">
            <For each={codeResults()?.results}>
              {(hit) => (
                <A href={`/projects/${encodeURIComponent(hit.project_id)}`} class="block">
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
                </A>
              )}
            </For>
          </div>
        </Show>
      </Show>

      {/* Conversation results */}
      <Show
        when={
          activeTab() === "conversations" &&
          !convResults.loading &&
          !convResults.error &&
          convResults() !== undefined
        }
      >
        <Show
          when={convResults()?.results?.length}
          fallback={
            <Show when={debouncedQuery().trim()}>
              <p class="mt-6 text-sm text-cf-text-muted">{t("search.noResults")}</p>
            </Show>
          }
        >
          <p class="mt-4 text-xs text-cf-text-secondary">
            {t("search.results", { count: convResults()?.total ?? 0 })}
          </p>
          <ConversationResults
            results={convResults()?.results ?? []}
            onOpen={(result) => void openConversation(result)}
          />
        </Show>
      </Show>
    </PageLayout>
  );
}
