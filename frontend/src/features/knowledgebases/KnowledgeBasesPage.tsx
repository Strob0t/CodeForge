import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { CreateKnowledgeBaseRequest, KnowledgeBase } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";

const CATEGORIES = ["framework", "paradigm", "language", "security", "custom"] as const;

export default function KnowledgeBasesPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [kbs, { refetch }] = createResource(() => api.knowledgeBases.list());
  const [showForm, setShowForm] = createSignal(false);

  // -- Form state --
  const [formName, setFormName] = createSignal("");
  const [formDesc, setFormDesc] = createSignal("");
  const [formCategory, setFormCategory] = createSignal<string>("custom");
  const [formTags, setFormTags] = createSignal("");
  const [formContentPath, setFormContentPath] = createSignal("");

  const resetForm = () => {
    setFormName("");
    setFormDesc("");
    setFormCategory("custom");
    setFormTags("");
    setFormContentPath("");
  };

  const handleCreate = async (e: SubmitEvent) => {
    e.preventDefault();
    const name = formName().trim();
    if (!name) return;
    try {
      const req: CreateKnowledgeBaseRequest = {
        name,
        description: formDesc().trim(),
        category: formCategory(),
        tags: formTags()
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean),
        content_path: formContentPath().trim(),
      };
      await api.knowledgeBases.create(req);
      resetForm();
      setShowForm(false);
      refetch();
      toast("success", t("kb.toast.created"));
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to create knowledge base");
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await api.knowledgeBases.delete(id);
      refetch();
      toast("success", t("kb.toast.deleted"));
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete knowledge base");
    }
  };

  const handleIndex = async (id: string) => {
    try {
      await api.knowledgeBases.index(id);
      refetch();
      toast("success", t("kb.toast.indexed"));
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to index knowledge base");
    }
  };

  const sorted = () => {
    const list = kbs() ?? [];
    return [...list].sort((a, b) => {
      if (a.builtin !== b.builtin) return a.builtin ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
  };

  return (
    <div>
      <div class="mb-6 flex items-center justify-between">
        <div>
          <h2 class="text-2xl font-bold text-gray-900 dark:text-gray-100">{t("kb.title")}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{t("kb.description")}</p>
        </div>
        <button
          type="button"
          class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
          onClick={() => setShowForm((v) => !v)}
        >
          {showForm() ? "Cancel" : t("kb.form.create")}
        </button>
      </div>

      <Show when={showForm()}>
        <form
          onSubmit={handleCreate}
          class="mb-6 rounded-lg border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800"
          aria-label={t("kb.form.create")}
        >
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label
                for="kb-name"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("kb.form.name")} *
              </label>
              <input
                id="kb-name"
                type="text"
                value={formName()}
                onInput={(e) => setFormName(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
                required
              />
            </div>
            <div>
              <label
                for="kb-category"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("kb.form.category")}
              </label>
              <select
                id="kb-category"
                value={formCategory()}
                onChange={(e) => setFormCategory(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
              >
                <For each={[...CATEGORIES]}>
                  {(cat) => (
                    <option value={cat}>
                      {t(`kb.category.${cat}` as keyof typeof import("~/i18n/en").default)}
                    </option>
                  )}
                </For>
              </select>
            </div>
            <div class="sm:col-span-2">
              <label
                for="kb-desc"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("kb.form.description")}
              </label>
              <input
                id="kb-desc"
                type="text"
                value={formDesc()}
                onInput={(e) => setFormDesc(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
              />
            </div>
            <div>
              <label
                for="kb-tags"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("kb.form.tags")}
              </label>
              <input
                id="kb-tags"
                type="text"
                value={formTags()}
                onInput={(e) => setFormTags(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
                placeholder="tag1, tag2, tag3"
              />
            </div>
            <div>
              <label
                for="kb-content-path"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("kb.form.contentPath")}
              </label>
              <input
                id="kb-content-path"
                type="text"
                value={formContentPath()}
                onInput={(e) => setFormContentPath(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
                placeholder="/path/to/content"
              />
            </div>
          </div>
          <div class="mt-4 flex justify-end">
            <button
              type="submit"
              class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
            >
              {t("kb.form.create")}
            </button>
          </div>
        </form>
      </Show>

      <Show when={kbs.loading}>
        <p class="text-sm text-gray-500 dark:text-gray-400">{t("kb.loading")}</p>
      </Show>

      <Show when={!kbs.loading}>
        <Show
          when={sorted().length}
          fallback={<p class="text-sm text-gray-500 dark:text-gray-400">{t("kb.empty")}</p>}
        >
          <div class="grid grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-3">
            <For each={sorted()}>
              {(kb) => <KBCard kb={kb} onDelete={handleDelete} onIndex={handleIndex} />}
            </For>
          </div>
        </Show>
      </Show>
    </div>
  );
}

const statusColors: Record<string, string> = {
  pending: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
  indexed: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
  error: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
};

const categoryColors: Record<string, string> = {
  framework: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
  paradigm: "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400",
  language: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
  security: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
  custom: "bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-400",
};

function KBCard(props: {
  kb: KnowledgeBase;
  onDelete: (id: string) => void;
  onIndex: (id: string) => void;
}) {
  const { t } = useI18n();

  const statusKey = () =>
    `kb.status.${props.kb.status}` as keyof typeof import("~/i18n/en").default;
  const categoryKey = () =>
    `kb.category.${props.kb.category}` as keyof typeof import("~/i18n/en").default;

  return (
    <div class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm transition-shadow hover:shadow-md dark:border-gray-700 dark:bg-gray-800 dark:shadow-gray-900/30 dark:hover:shadow-gray-900/30">
      <div class="flex items-start justify-between">
        <div class="min-w-0 flex-1">
          <h3 class="text-lg font-semibold text-gray-900 dark:text-gray-100">{props.kb.name}</h3>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{props.kb.description}</p>
        </div>
        <div class="ml-2 flex flex-shrink-0 gap-1">
          <Show when={props.kb.builtin}>
            <span class="rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600 dark:bg-gray-700 dark:text-gray-400">
              {t("kb.builtin")}
            </span>
          </Show>
        </div>
      </div>

      <div class="mt-3 flex flex-wrap items-center gap-2">
        <span
          class={`rounded-full px-2 py-0.5 text-xs font-medium ${categoryColors[props.kb.category] ?? categoryColors.custom}`}
        >
          {t(categoryKey())}
        </span>
        <span
          class={`rounded-full px-2 py-0.5 text-xs font-medium ${statusColors[props.kb.status] ?? statusColors.pending}`}
        >
          {t(statusKey())}
        </span>
        <Show when={props.kb.chunk_count > 0}>
          <span class="text-xs text-gray-500 dark:text-gray-400">
            {props.kb.chunk_count} {t("kb.chunks")}
          </span>
        </Show>
      </div>

      <Show when={props.kb.tags?.length}>
        <div class="mt-2 flex flex-wrap gap-1">
          <For each={props.kb.tags}>
            {(tag) => (
              <span class="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-600 dark:bg-gray-700 dark:text-gray-400">
                {tag}
              </span>
            )}
          </For>
        </div>
      </Show>

      <div class="mt-4 flex gap-2">
        <button
          type="button"
          class="rounded-md bg-blue-50 px-3 py-1.5 text-xs font-medium text-blue-600 hover:bg-blue-100 dark:bg-blue-900/20 dark:text-blue-400 dark:hover:bg-blue-900/40"
          onClick={() => props.onIndex(props.kb.id)}
        >
          {props.kb.status === "indexed" ? t("kb.index.reindex") : t("kb.index.button")}
        </button>
        <Show when={!props.kb.builtin}>
          <button
            type="button"
            class="rounded-md bg-red-50 px-3 py-1.5 text-xs font-medium text-red-600 hover:bg-red-100 dark:bg-red-900/20 dark:text-red-400 dark:hover:bg-red-900/40"
            onClick={() => props.onDelete(props.kb.id)}
          >
            Delete
          </button>
        </Show>
      </div>
    </div>
  );
}
