import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { CreateModeRequest, Mode } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";

export default function ModesPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [modes, { refetch }] = createResource(() => api.modes.list());
  const [showForm, setShowForm] = createSignal(false);
  const [error, setError] = createSignal("");

  // -- Form state --
  const [formId, setFormId] = createSignal("");
  const [formName, setFormName] = createSignal("");
  const [formDesc, setFormDesc] = createSignal("");
  const [formTools, setFormTools] = createSignal("");
  const [formDeniedTools, setFormDeniedTools] = createSignal("");
  const [formDeniedActions, setFormDeniedActions] = createSignal("");
  const [formRequiredArtifact, setFormRequiredArtifact] = createSignal("");
  const [formScenario, setFormScenario] = createSignal("default");
  const [formAutonomy, setFormAutonomy] = createSignal(3);
  const [formPrompt, setFormPrompt] = createSignal("");

  const resetForm = () => {
    setFormId("");
    setFormName("");
    setFormDesc("");
    setFormTools("");
    setFormDeniedTools("");
    setFormDeniedActions("");
    setFormRequiredArtifact("");
    setFormScenario("default");
    setFormAutonomy(3);
    setFormPrompt("");
  };

  const handleCreate = async (e: SubmitEvent) => {
    e.preventDefault();
    const id = formId().trim();
    const name = formName().trim();
    if (!id) {
      toast("error", t("modes.toast.idRequired"));
      return;
    }
    if (!name) {
      toast("error", t("modes.toast.nameRequired"));
      return;
    }
    setError("");
    try {
      const req: CreateModeRequest = {
        id,
        name,
        description: formDesc().trim() || undefined,
        tools: formTools()
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean),
        denied_tools: formDeniedTools()
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean),
        denied_actions: formDeniedActions()
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean),
        required_artifact: formRequiredArtifact().trim() || undefined,
        llm_scenario: formScenario().trim() || undefined,
        autonomy: formAutonomy(),
        prompt_prefix: formPrompt().trim() || undefined,
      };
      await api.modes.create(req);
      resetForm();
      setShowForm(false);
      refetch();
      toast("success", t("modes.toast.created"));
    } catch (err) {
      const msg = err instanceof Error ? err.message : t("modes.toast.createFailed");
      setError(msg);
      toast("error", msg);
    }
  };

  const sorted = () => {
    const list = modes() ?? [];
    return [...list].sort((a, b) => {
      if (a.builtin !== b.builtin) return a.builtin ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
  };

  return (
    <div>
      <div class="mb-6 flex items-center justify-between">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-gray-100">{t("modes.title")}</h2>
        <button
          type="button"
          class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
          onClick={() => setShowForm((v) => !v)}
        >
          {showForm() ? t("common.cancel") : t("modes.addMode")}
        </button>
      </div>

      <Show when={error()}>
        <div
          class="mb-4 rounded-md bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-400"
          role="alert"
        >
          {error()}
        </div>
      </Show>

      <Show when={showForm()}>
        <form
          onSubmit={handleCreate}
          class="mb-6 rounded-lg border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800"
          aria-label={t("modes.addMode")}
        >
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label
                for="mode-id"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("modes.form.id")} <span aria-hidden="true">*</span>
                <span class="sr-only">(required)</span>
              </label>
              <input
                id="mode-id"
                type="text"
                value={formId()}
                onInput={(e) => setFormId(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
                placeholder={t("modes.form.idPlaceholder")}
                aria-required="true"
              />
            </div>
            <div>
              <label
                for="mode-name"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("modes.form.name")} <span aria-hidden="true">*</span>
                <span class="sr-only">(required)</span>
              </label>
              <input
                id="mode-name"
                type="text"
                value={formName()}
                onInput={(e) => setFormName(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
                placeholder={t("modes.form.namePlaceholder")}
                aria-required="true"
              />
            </div>
            <div class="sm:col-span-2">
              <label
                for="mode-desc"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("modes.form.description")}
              </label>
              <input
                id="mode-desc"
                type="text"
                value={formDesc()}
                onInput={(e) => setFormDesc(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
                placeholder={t("modes.form.descriptionPlaceholder")}
              />
            </div>
            <div>
              <label
                for="mode-tools"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("modes.form.tools")}
              </label>
              <input
                id="mode-tools"
                type="text"
                value={formTools()}
                onInput={(e) => setFormTools(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
                placeholder={t("modes.form.toolsPlaceholder")}
              />
            </div>
            <div>
              <label
                for="mode-denied-tools"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("modes.form.deniedTools")}
              </label>
              <input
                id="mode-denied-tools"
                type="text"
                value={formDeniedTools()}
                onInput={(e) => setFormDeniedTools(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
                placeholder={t("modes.form.deniedToolsPlaceholder")}
              />
            </div>
            <div>
              <label
                for="mode-denied-actions"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("modes.form.deniedActions")}
              </label>
              <input
                id="mode-denied-actions"
                type="text"
                value={formDeniedActions()}
                onInput={(e) => setFormDeniedActions(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
                placeholder={t("modes.form.deniedActionsPlaceholder")}
              />
            </div>
            <div>
              <label
                for="mode-required-artifact"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("modes.form.requiredArtifact")}
              </label>
              <input
                id="mode-required-artifact"
                type="text"
                value={formRequiredArtifact()}
                onInput={(e) => setFormRequiredArtifact(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
                placeholder={t("modes.form.requiredArtifactPlaceholder")}
              />
            </div>
            <div>
              <label
                for="mode-scenario"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("modes.form.scenario")}
              </label>
              <input
                id="mode-scenario"
                type="text"
                value={formScenario()}
                onInput={(e) => setFormScenario(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
                placeholder={t("modes.form.scenarioPlaceholder")}
              />
            </div>
            <div>
              <label
                for="mode-autonomy"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("modes.form.autonomy")}
              </label>
              <input
                id="mode-autonomy"
                type="number"
                min="1"
                max="5"
                value={formAutonomy()}
                onInput={(e) => setFormAutonomy(Number(e.currentTarget.value))}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
              />
            </div>
            <div class="sm:col-span-2">
              <label
                for="mode-prompt"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("modes.form.prompt")}
              </label>
              <textarea
                id="mode-prompt"
                value={formPrompt()}
                onInput={(e) => setFormPrompt(e.currentTarget.value)}
                rows={3}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
                placeholder={t("modes.form.promptPlaceholder")}
              />
            </div>
          </div>
          <div class="mt-4 flex justify-end">
            <button
              type="submit"
              class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
            >
              {t("modes.form.create")}
            </button>
          </div>
        </form>
      </Show>

      <Show when={modes.loading}>
        <p class="text-sm text-gray-500 dark:text-gray-400">{t("modes.loading")}</p>
      </Show>

      <Show when={modes.error}>
        <p class="text-sm text-red-500 dark:text-red-400">{t("modes.loadError")}</p>
      </Show>

      <Show when={!modes.loading && !modes.error}>
        <Show
          when={sorted().length}
          fallback={<p class="text-sm text-gray-500 dark:text-gray-400">{t("modes.empty")}</p>}
        >
          <div class="grid grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-3">
            <For each={sorted()}>{(mode) => <ModeCard mode={mode} />}</For>
          </div>
        </Show>
      </Show>
    </div>
  );
}

function ModeCard(props: { mode: Mode }) {
  const { t } = useI18n();
  const [showPrompt, setShowPrompt] = createSignal(false);

  const autonomyColors = [
    "",
    "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
    "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
    "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
    "bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400",
    "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
  ];

  return (
    <div class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm transition-shadow hover:shadow-md dark:border-gray-700 dark:bg-gray-800 dark:shadow-gray-900/30 dark:hover:shadow-gray-900/30">
      <div class="flex items-start justify-between">
        <div>
          <h3 class="text-lg font-semibold text-gray-900 dark:text-gray-100">{props.mode.name}</h3>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{props.mode.description}</p>
        </div>
        <span
          class={`rounded-full px-2 py-0.5 text-xs font-medium ${
            props.mode.builtin
              ? "bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400"
              : "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400"
          }`}
        >
          {props.mode.builtin ? t("modes.builtin") : t("modes.custom")}
        </span>
      </div>

      <div class="mt-3 space-y-2">
        {/* Tools */}
        <div>
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
            {t("modes.tools")}
          </span>
          <div class="mt-1 flex flex-wrap gap-1">
            <For each={props.mode.tools}>
              {(tool) => (
                <span class="rounded bg-gray-100 px-1.5 py-0.5 font-mono text-xs text-gray-600 dark:bg-gray-700 dark:text-gray-400">
                  {tool}
                </span>
              )}
            </For>
          </div>
        </div>

        {/* Denied Tools */}
        <Show when={props.mode.denied_tools?.length}>
          <div>
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
              {t("modes.deniedTools")}
            </span>
            <div class="mt-1 flex flex-wrap gap-1">
              <For each={props.mode.denied_tools}>
                {(tool) => (
                  <span class="rounded bg-red-100 px-1.5 py-0.5 font-mono text-xs text-red-600 dark:bg-red-900/30 dark:text-red-400">
                    {tool}
                  </span>
                )}
              </For>
            </div>
          </div>
        </Show>

        {/* Denied Actions */}
        <Show when={props.mode.denied_actions?.length}>
          <div>
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
              {t("modes.deniedActions")}
            </span>
            <div class="mt-1 flex flex-wrap gap-1">
              <For each={props.mode.denied_actions}>
                {(action) => (
                  <span class="rounded bg-red-100 px-1.5 py-0.5 font-mono text-xs text-red-600 dark:bg-red-900/30 dark:text-red-400">
                    {action}
                  </span>
                )}
              </For>
            </div>
          </div>
        </Show>

        {/* Scenario + Autonomy + Required Artifact */}
        <div class="flex flex-wrap items-center gap-3 text-xs">
          <div>
            <span class="font-medium text-gray-500 dark:text-gray-400">
              {t("modes.scenario")}:{" "}
            </span>
            <span class="rounded bg-blue-50 px-1.5 py-0.5 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400">
              {props.mode.llm_scenario}
            </span>
          </div>
          <div>
            <span class="font-medium text-gray-500 dark:text-gray-400">
              {t("modes.autonomy")}:{" "}
            </span>
            <span class={`rounded px-1.5 py-0.5 ${autonomyColors[props.mode.autonomy] ?? ""}`}>
              {t("modes.autonomyLabel", { level: String(props.mode.autonomy) })}
            </span>
          </div>
          <Show when={props.mode.required_artifact}>
            <div>
              <span class="font-medium text-gray-500 dark:text-gray-400">
                {t("modes.requiredArtifact")}:{" "}
              </span>
              <span class="rounded bg-amber-50 px-1.5 py-0.5 text-amber-600 dark:bg-amber-900/30 dark:text-amber-400">
                {props.mode.required_artifact}
              </span>
            </div>
          </Show>
        </div>

        {/* Prompt toggle */}
        <Show when={props.mode.prompt_prefix}>
          <button
            type="button"
            class="text-xs text-blue-600 hover:underline dark:text-blue-400"
            onClick={() => setShowPrompt((v) => !v)}
            aria-label={
              showPrompt()
                ? t("modes.hidePromptAria", { name: props.mode.name })
                : t("modes.showPromptAria", { name: props.mode.name })
            }
          >
            {showPrompt() ? t("modes.hidePrompt") : t("modes.showPrompt")}
          </button>
          <Show when={showPrompt()}>
            <p class="mt-1 rounded bg-gray-50 p-2 text-xs text-gray-600 dark:bg-gray-900 dark:text-gray-400">
              {props.mode.prompt_prefix}
            </p>
          </Show>
        </Show>
      </div>
    </div>
  );
}
