import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { CreateProjectRequest } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";

import ProjectCard from "./ProjectCard";

const emptyForm: CreateProjectRequest = {
  name: "",
  description: "",
  repo_url: "",
  provider: "",
  config: {},
};

export default function DashboardPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [projects, { refetch }] = createResource(() => api.projects.list());
  const [showForm, setShowForm] = createSignal(false);
  const [form, setForm] = createSignal<CreateProjectRequest>({ ...emptyForm });
  const [error, setError] = createSignal("");

  async function handleCreate(e: SubmitEvent) {
    e.preventDefault();
    setError("");

    const data = form();
    if (!data.name.trim()) {
      setError(t("dashboard.toast.nameRequired"));
      return;
    }

    try {
      await api.projects.create(data);
      setForm({ ...emptyForm });
      setShowForm(false);
      await refetch();
      toast("success", t("dashboard.toast.created"));
    } catch (err) {
      const msg = err instanceof Error ? err.message : t("dashboard.toast.createFailed");
      setError(msg);
      toast("error", msg);
    }
  }

  async function handleDelete(id: string) {
    try {
      await api.projects.delete(id);
      await refetch();
      toast("success", t("dashboard.toast.deleted"));
    } catch (err) {
      const msg = err instanceof Error ? err.message : t("dashboard.toast.deleteFailed");
      setError(msg);
      toast("error", msg);
    }
  }

  function updateField<K extends keyof CreateProjectRequest>(
    field: K,
    value: CreateProjectRequest[K],
  ) {
    setForm((prev) => ({ ...prev, [field]: value }));
  }

  return (
    <div>
      <div class="mb-6 flex items-center justify-between">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-gray-100">{t("dashboard.title")}</h2>
        <button
          type="button"
          class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
          onClick={() => setShowForm((v) => !v)}
        >
          {showForm() ? t("common.cancel") : t("dashboard.addProject")}
        </button>
      </div>

      <Show when={error()}>
        <div
          class="mb-4 rounded-md bg-red-50 dark:bg-red-900/20 p-3 text-sm text-red-700 dark:text-red-400"
          role="alert"
        >
          {error()}
        </div>
      </Show>

      <Show when={showForm()}>
        <form
          onSubmit={handleCreate}
          class="mb-6 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-5"
        >
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label for="name" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
                {t("dashboard.form.name")} <span aria-hidden="true">*</span>
                <span class="sr-only">(required)</span>
              </label>
              <input
                id="name"
                type="text"
                value={form().name}
                onInput={(e) => updateField("name", e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-700 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                placeholder={t("dashboard.form.namePlaceholder")}
                aria-required="true"
              />
            </div>

            <div>
              <label
                for="provider"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("dashboard.form.provider")}
              </label>
              <input
                id="provider"
                type="text"
                value={form().provider}
                onInput={(e) => updateField("provider", e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-700 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                placeholder={t("dashboard.form.providerPlaceholder")}
              />
            </div>

            <div class="sm:col-span-2">
              <label
                for="repo_url"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("dashboard.form.repoUrl")}
              </label>
              <input
                id="repo_url"
                type="text"
                value={form().repo_url}
                onInput={(e) => updateField("repo_url", e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-700 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                placeholder={t("dashboard.form.repoUrlPlaceholder")}
              />
            </div>

            <div class="sm:col-span-2">
              <label
                for="description"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("dashboard.form.description")}
              </label>
              <textarea
                id="description"
                value={form().description}
                onInput={(e) => updateField("description", e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-700 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                rows={2}
                placeholder={t("dashboard.form.descriptionPlaceholder")}
              />
            </div>
          </div>

          <div class="mt-4 flex justify-end">
            <button
              type="submit"
              class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
            >
              {t("dashboard.form.create")}
            </button>
          </div>
        </form>
      </Show>

      <Show when={projects.loading}>
        <p class="text-sm text-gray-500 dark:text-gray-400">{t("dashboard.loading")}</p>
      </Show>

      <Show when={projects.error}>
        <p class="text-sm text-red-500 dark:text-red-400">{t("dashboard.loadError")}</p>
      </Show>

      <Show when={!projects.loading && !projects.error}>
        <Show
          when={projects()?.length}
          fallback={<p class="text-sm text-gray-500 dark:text-gray-400">{t("dashboard.empty")}</p>}
        >
          <div class="grid grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-3">
            <For each={projects()}>
              {(p) => <ProjectCard project={p} onDelete={handleDelete} />}
            </For>
          </div>
        </Show>
      </Show>
    </div>
  );
}
