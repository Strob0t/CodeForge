import { createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { CreateTaskRequest, Task, TaskStatus } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";

interface TaskPanelProps {
  projectId: string;
  tasks: Task[];
  onRefetch: () => void;
  onError: (msg: string) => void;
}

function statusColor(status: TaskStatus): string {
  switch (status) {
    case "completed":
      return "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400";
    case "running":
      return "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400";
    case "failed":
      return "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400";
    case "queued":
      return "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400";
    case "cancelled":
      return "bg-gray-100 text-gray-500 dark:bg-gray-700 dark:text-gray-400";
    default:
      return "bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400";
  }
}

export default function TaskPanel(props: TaskPanelProps) {
  const { t, fmt } = useI18n();
  const { show: toast } = useToast();
  const [showForm, setShowForm] = createSignal(false);
  const [title, setTitle] = createSignal("");
  const [prompt, setPrompt] = createSignal("");
  const [expanded, setExpanded] = createSignal<string | null>(null);

  const handleCreate = async (e: SubmitEvent) => {
    e.preventDefault();
    if (!title().trim() || !prompt().trim()) return;

    const data: CreateTaskRequest = { title: title(), prompt: prompt() };
    try {
      await api.tasks.create(props.projectId, data);
      setTitle("");
      setPrompt("");
      setShowForm(false);
      props.onRefetch();
      toast("success", t("task.toast.created"));
    } catch (err) {
      const msg = err instanceof Error ? err.message : t("task.toast.createFailed");
      props.onError(msg);
      toast("error", msg);
    }
  };

  const totalCost = () => props.tasks.reduce((sum, task) => sum + task.cost_usd, 0);

  return (
    <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <div class="mb-3 flex items-center justify-between">
        <div class="flex items-center gap-3">
          <h3 class="text-lg font-semibold">{t("task.title")}</h3>
          <Show when={totalCost() > 0}>
            <span class="rounded bg-purple-100 px-2 py-0.5 text-xs text-purple-700 dark:bg-purple-900/30 dark:text-purple-400">
              {t("task.total")} {fmt.currency(totalCost())}
            </span>
          </Show>
        </div>
        <button
          type="button"
          class="rounded bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700"
          onClick={() => setShowForm((v) => !v)}
        >
          {showForm() ? t("common.cancel") : t("task.newTask")}
        </button>
      </div>

      <Show when={showForm()}>
        <form
          onSubmit={handleCreate}
          class="mb-4 rounded border border-gray-100 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-900"
        >
          <div class="space-y-3">
            <div>
              <label
                for="task-title"
                class="block text-xs font-medium text-gray-600 dark:text-gray-400"
              >
                {t("task.form.title")} <span aria-hidden="true">*</span>
                <span class="sr-only">(required)</span>
              </label>
              <input
                id="task-title"
                type="text"
                value={title()}
                onInput={(e) => setTitle(e.currentTarget.value)}
                class="mt-1 block w-full rounded border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-700"
                placeholder={t("task.form.titlePlaceholder")}
                aria-required="true"
              />
            </div>
            <div>
              <label
                for="task-prompt"
                class="block text-xs font-medium text-gray-600 dark:text-gray-400"
              >
                {t("task.form.prompt")} <span aria-hidden="true">*</span>
                <span class="sr-only">(required)</span>
              </label>
              <textarea
                id="task-prompt"
                value={prompt()}
                onInput={(e) => setPrompt(e.currentTarget.value)}
                class="mt-1 block w-full rounded border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-700"
                rows={3}
                placeholder={t("task.form.promptPlaceholder")}
                aria-required="true"
              />
            </div>
          </div>
          <div class="mt-3 flex justify-end">
            <button
              type="submit"
              class="rounded bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700"
            >
              {t("task.form.create")}
            </button>
          </div>
        </form>
      </Show>

      <Show
        when={props.tasks.length > 0}
        fallback={<p class="text-sm text-gray-500 dark:text-gray-400">{t("task.empty")}</p>}
      >
        <div class="space-y-2">
          <For each={props.tasks}>
            {(task) => (
              <div class="rounded border border-gray-100 dark:border-gray-700">
                <div
                  class="flex cursor-pointer items-center justify-between p-3"
                  role="button"
                  tabIndex={0}
                  aria-expanded={expanded() === task.id}
                  aria-label={`Task: ${task.title}, status: ${task.status}`}
                  onClick={() => setExpanded((prev) => (prev === task.id ? null : task.id))}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      setExpanded((prev) => (prev === task.id ? null : task.id));
                    }
                  }}
                >
                  <div class="flex items-center gap-2">
                    <span class="font-medium">{task.title}</span>
                    <span class="text-xs text-gray-400 dark:text-gray-500">
                      {task.id.slice(0, 8)}
                    </span>
                    <Show when={task.cost_usd > 0}>
                      <span class="rounded bg-purple-50 px-1.5 py-0.5 text-xs text-purple-600 dark:bg-purple-900/30 dark:text-purple-400">
                        {fmt.currency(task.cost_usd)}
                      </span>
                    </Show>
                  </div>
                  <span class={`rounded-full px-2 py-0.5 text-xs ${statusColor(task.status)}`}>
                    {task.status}
                  </span>
                </div>

                <Show when={expanded() === task.id}>
                  <div class="border-t border-gray-100 bg-gray-50 p-3 text-sm dark:border-gray-700 dark:bg-gray-900">
                    <div class="mb-2 text-gray-500 dark:text-gray-400">
                      <span class="font-medium">{t("task.prompt")}</span> {task.prompt}
                    </div>
                    <Show when={task.agent_id}>
                      <div class="text-xs text-gray-400 dark:text-gray-500">
                        {t("task.agent")} {task.agent_id?.slice(0, 8) ?? ""}
                      </div>
                    </Show>
                    <Show when={task.result}>
                      {(result) => (
                        <div class="mt-2">
                          <Show when={result().output}>
                            <div class="mb-1">
                              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
                                {t("task.output")}
                              </span>
                              <pre class="mt-1 max-h-40 overflow-auto rounded bg-gray-900 p-2 text-xs text-green-400">
                                {result().output}
                              </pre>
                            </div>
                          </Show>
                          <Show when={result().error}>
                            <div class="mt-1 text-xs text-red-600 dark:text-red-400">
                              {t("task.errorLabel")} {result().error}
                            </div>
                          </Show>
                          <Show
                            when={(result().tokens_in ?? 0) > 0 || (result().tokens_out ?? 0) > 0}
                          >
                            <div class="mt-1 text-xs text-gray-400 dark:text-gray-500">
                              {t("task.tokens")} {result().tokens_in ?? 0} in /{" "}
                              {result().tokens_out ?? 0} out
                            </div>
                          </Show>
                          <Show when={(result().files ?? []).length > 0}>
                            <div class="mt-1 text-xs text-gray-400 dark:text-gray-500">
                              {t("task.files")} {(result().files ?? []).join(", ")}
                            </div>
                          </Show>
                        </div>
                      )}
                    </Show>
                  </div>
                </Show>
              </div>
            )}
          </For>
        </div>
      </Show>
    </div>
  );
}
