import { createResource, createSignal, For, onCleanup, Show } from "solid-js";

import { api } from "~/api/client";
import type { CreateProjectRequest, StackDetectionResult } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import type { TranslationKey } from "~/i18n/en";

import ProjectCard from "./ProjectCard";

const emptyForm: CreateProjectRequest = {
  name: "",
  description: "",
  repo_url: "",
  provider: "",
  config: {},
};

const categoryLabels: Record<string, string> = {
  mode: "dashboard.detect.category.mode",
  pipeline: "dashboard.detect.category.pipeline",
  linter: "dashboard.detect.category.linter",
  formatter: "dashboard.detect.category.formatter",
};

export default function DashboardPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [projects, { refetch }] = createResource(() => api.projects.list());
  const [providers] = createResource(() => api.providers.git().then((r) => r.providers));
  const [showForm, setShowForm] = createSignal(false);
  const [form, setForm] = createSignal<CreateProjectRequest>({ ...emptyForm });
  const [error, setError] = createSignal("");
  const [detecting, setDetecting] = createSignal(false);
  const [stackResult, setStackResult] = createSignal<StackDetectionResult | null>(null);
  const [editingId, setEditingId] = createSignal<string | null>(null);
  const [parsingUrl, setParsingUrl] = createSignal(false);
  const [formMode, setFormMode] = createSignal<"remote" | "local">("remote");
  const [localPath, setLocalPath] = createSignal("");

  let urlDebounceTimer: ReturnType<typeof setTimeout> | undefined;
  onCleanup(() => clearTimeout(urlDebounceTimer));

  function isEditing() {
    return editingId() !== null;
  }

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    setError("");

    const data = form();
    const isLocal = formMode() === "local" && !isEditing();

    if (isLocal) {
      const path = localPath().trim();
      if (!path) {
        setError(t("dashboard.toast.nameRequired"));
        return;
      }
      const derivedName = data.name.trim() || path.split("/").filter(Boolean).pop() || "";
      try {
        const created = await api.projects.create({
          name: derivedName,
          description: data.description,
          repo_url: "",
          provider: "",
          config: {},
        });
        toast("success", t("dashboard.toast.created"));
        await api.projects.adopt(created.id, { path });
        toast("info", t("dashboard.toast.setupStarted"));
        setForm({ ...emptyForm });
        setLocalPath("");
        setShowForm(false);
        setEditingId(null);
        setStackResult(null);
        await refetch();
      } catch (err) {
        const msg = err instanceof Error ? err.message : t("dashboard.toast.createFailed");
        setError(msg);
        toast("error", msg);
      }
      return;
    }

    if (!data.name.trim() && !data.repo_url.trim()) {
      setError(t("dashboard.toast.nameRequired"));
      return;
    }

    try {
      const eid = editingId();
      if (isEditing() && eid) {
        await api.projects.update(eid, {
          name: data.name || undefined,
          description: data.description || undefined,
          repo_url: data.repo_url || undefined,
          provider: data.provider || undefined,
        });
        toast("success", t("dashboard.toast.updated"));
      } else {
        const created = await api.projects.create(data);
        toast("success", t("dashboard.toast.created"));

        // Fire-and-forget: auto-setup (clone + detect stack + import specs)
        if (created.repo_url) {
          toast("info", t("dashboard.toast.setupStarted"));
          api.projects.setup(created.id).catch((setupErr) => {
            const setupMsg = setupErr instanceof Error ? setupErr.message : "setup failed";
            toast("error", setupMsg);
          });
        }
      }
      setForm({ ...emptyForm });
      setShowForm(false);
      setEditingId(null);
      setStackResult(null);
      await refetch();
    } catch (err) {
      const msg = err instanceof Error ? err.message : t("dashboard.toast.createFailed");
      setError(msg);
      toast("error", msg);
    }
  }

  function handleEdit(id: string) {
    const p = projects()?.find((proj) => proj.id === id);
    if (!p) return;
    setForm({
      name: p.name,
      description: p.description,
      repo_url: p.repo_url,
      provider: p.provider,
      config: p.config ?? {},
    });
    setEditingId(id);
    setShowForm(true);
  }

  function handleCancelForm() {
    setShowForm(false);
    setEditingId(null);
    setForm({ ...emptyForm });
    setLocalPath("");
    setFormMode("remote");
    setError("");
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

  async function handleDetectStack(projectId: string) {
    setDetecting(true);
    setStackResult(null);
    try {
      const result = await api.projects.detectStack(projectId);
      setStackResult(result);
    } catch (err) {
      const msg = err instanceof Error ? err.message : t("dashboard.detect.error");
      toast("error", msg);
    } finally {
      setDetecting(false);
    }
  }

  function updateField<K extends keyof CreateProjectRequest>(
    field: K,
    value: CreateProjectRequest[K],
  ) {
    setForm((prev) => ({ ...prev, [field]: value }));
  }

  function handleRepoUrlInput(url: string) {
    updateField("repo_url", url);
    clearTimeout(urlDebounceTimer);
    if (!url.trim()) return;
    urlDebounceTimer = setTimeout(async () => {
      try {
        setParsingUrl(true);
        const parsed = await api.projects.parseRepoURL(url);
        setForm((prev) => ({
          ...prev,
          name: prev.name || parsed.repo,
          provider: prev.provider || parsed.provider,
        }));
      } catch {
        // silently ignore parse errors during typing
      } finally {
        setParsingUrl(false);
      }
    }, 500);
  }

  return (
    <div>
      <div class="mb-6 flex items-center justify-between">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-gray-100">{t("dashboard.title")}</h2>
        <button
          type="button"
          class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
          onClick={() => {
            if (showForm()) {
              handleCancelForm();
            } else {
              setShowForm(true);
            }
          }}
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
          onSubmit={handleSubmit}
          class="mb-6 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-5"
        >
          {/* Mode tab toggle (hidden when editing) */}
          <Show when={!isEditing()}>
            <div class="mb-4 flex gap-0 rounded-md border border-gray-300 dark:border-gray-600 w-fit overflow-hidden">
              <button
                type="button"
                class={`px-4 py-1.5 text-sm font-medium transition-colors ${
                  formMode() === "remote"
                    ? "bg-blue-600 text-white"
                    : "bg-transparent text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
                }`}
                onClick={() => setFormMode("remote")}
              >
                {t("dashboard.form.modeRemote")}
              </button>
              <button
                type="button"
                class={`px-4 py-1.5 text-sm font-medium transition-colors ${
                  formMode() === "local"
                    ? "bg-blue-600 text-white"
                    : "bg-transparent text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
                }`}
                onClick={() => setFormMode("local")}
              >
                {t("dashboard.form.modeLocal")}
              </button>
            </div>
          </Show>

          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            {/* Local mode: path field */}
            <Show when={formMode() === "local" && !isEditing()}>
              <div class="sm:col-span-2">
                <label
                  for="local_path"
                  class="block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {t("dashboard.form.path")} <span aria-hidden="true">*</span>
                  <span class="sr-only">(required)</span>
                </label>
                <input
                  id="local_path"
                  type="text"
                  value={localPath()}
                  onInput={(e) => setLocalPath(e.currentTarget.value)}
                  class="mt-1 block w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-700 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                  placeholder={t("dashboard.form.pathPlaceholder")}
                  aria-required="true"
                />
              </div>
            </Show>

            <div>
              <label for="name" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
                {t("dashboard.form.name")}
                <Show when={formMode() === "remote" && !form().repo_url.trim()}>
                  {" "}
                  <span aria-hidden="true">*</span>
                  <span class="sr-only">(required)</span>
                </Show>
              </label>
              <input
                id="name"
                type="text"
                value={form().name}
                onInput={(e) => updateField("name", e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-700 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                placeholder={t("dashboard.form.namePlaceholder")}
                aria-required={
                  formMode() === "remote" && !form().repo_url.trim() ? "true" : "false"
                }
              />
            </div>

            {/* Remote mode: provider dropdown */}
            <Show when={formMode() === "remote" || isEditing()}>
              <div>
                <label
                  for="provider"
                  class="block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {t("dashboard.form.provider")}
                </label>
                <select
                  id="provider"
                  value={form().provider}
                  onChange={(e) => updateField("provider", e.currentTarget.value)}
                  class="mt-1 block w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-700 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                >
                  <option value="">{t("dashboard.form.providerPlaceholder")}</option>
                  <For each={providers() ?? []}>{(p) => <option value={p}>{p}</option>}</For>
                </select>
              </div>
            </Show>

            {/* Remote mode: repo URL field */}
            <Show when={formMode() === "remote" || isEditing()}>
              <div class="sm:col-span-2">
                <label
                  for="repo_url"
                  class="block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {t("dashboard.form.repoUrl")}
                  <Show when={parsingUrl()}>
                    <span class="ml-2 text-xs text-gray-400">detecting...</span>
                  </Show>
                </label>
                <input
                  id="repo_url"
                  type="text"
                  value={form().repo_url}
                  onInput={(e) => handleRepoUrlInput(e.currentTarget.value)}
                  class="mt-1 block w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-700 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                  placeholder={t("dashboard.form.repoUrlPlaceholder")}
                />
              </div>
            </Show>

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
              {isEditing() ? t("common.save") : t("dashboard.form.create")}
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
              {(p) => (
                <ProjectCard
                  project={p}
                  onDelete={handleDelete}
                  onEdit={handleEdit}
                  onDetectStack={handleDetectStack}
                  detecting={detecting()}
                />
              )}
            </For>
          </div>
        </Show>
      </Show>

      {/* Stack Detection Results Panel */}
      <Show when={stackResult()}>
        {(result) => (
          <div class="mt-6 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-5">
            <h3 class="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
              {t("dashboard.detect.languages")}
            </h3>

            <Show
              when={result().languages.length > 0}
              fallback={
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {t("dashboard.detect.noLanguages")}
                </p>
              }
            >
              <div class="flex flex-wrap gap-2 mb-4">
                <For each={result().languages}>
                  {(lang) => (
                    <div class="inline-flex items-center gap-2 rounded-full bg-blue-100 dark:bg-blue-900/30 px-3 py-1 text-sm text-blue-800 dark:text-blue-300">
                      <span class="font-medium">{lang.name}</span>
                      <span class="text-xs opacity-75">{Math.round(lang.confidence * 100)}%</span>
                      <Show when={lang.frameworks.length > 0}>
                        <span class="text-xs opacity-60">({lang.frameworks.join(", ")})</span>
                      </Show>
                    </div>
                  )}
                </For>
              </div>
            </Show>

            <Show when={result().recommendations.length > 0}>
              <h4 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-2">
                {t("dashboard.detect.recommendations")}
              </h4>
              <div class="flex flex-wrap gap-2">
                <For each={result().recommendations}>
                  {(rec) => (
                    <div class="inline-flex items-center gap-1 rounded-md border border-gray-200 dark:border-gray-600 px-2 py-1 text-xs">
                      <span class="font-medium text-gray-500 dark:text-gray-400">
                        {t((categoryLabels[rec.category] ?? rec.category) as TranslationKey)}
                      </span>
                      <span class="text-gray-900 dark:text-gray-100">{rec.name}</span>
                    </div>
                  )}
                </For>
              </div>
            </Show>
          </div>
        )}
      </Show>
    </div>
  );
}
