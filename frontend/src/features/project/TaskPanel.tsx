import { createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { CreateTaskRequest, Task, TaskStatus } from "~/api/types";

interface TaskPanelProps {
  projectId: string;
  tasks: Task[];
  onRefetch: () => void;
  onError: (msg: string) => void;
}

function statusColor(status: TaskStatus): string {
  switch (status) {
    case "completed":
      return "bg-green-100 text-green-700";
    case "running":
      return "bg-blue-100 text-blue-700";
    case "failed":
      return "bg-red-100 text-red-700";
    case "queued":
      return "bg-yellow-100 text-yellow-700";
    case "cancelled":
      return "bg-gray-100 text-gray-500";
    default:
      return "bg-gray-100 text-gray-600";
  }
}

function formatCost(usd: number): string {
  if (usd === 0) return "";
  if (usd < 0.01) return `$${usd.toFixed(4)}`;
  return `$${usd.toFixed(2)}`;
}

export default function TaskPanel(props: TaskPanelProps) {
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
    } catch (err) {
      props.onError(err instanceof Error ? err.message : "Failed to create task");
    }
  };

  const totalCost = () => props.tasks.reduce((sum, t) => sum + t.cost_usd, 0);

  return (
    <div class="rounded-lg border border-gray-200 bg-white p-4">
      <div class="mb-3 flex items-center justify-between">
        <div class="flex items-center gap-3">
          <h3 class="text-lg font-semibold">Tasks</h3>
          <Show when={totalCost() > 0}>
            <span class="rounded bg-purple-100 px-2 py-0.5 text-xs text-purple-700">
              Total: {formatCost(totalCost())}
            </span>
          </Show>
        </div>
        <button
          class="rounded bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700"
          onClick={() => setShowForm((v) => !v)}
        >
          {showForm() ? "Cancel" : "New Task"}
        </button>
      </div>

      <Show when={showForm()}>
        <form onSubmit={handleCreate} class="mb-4 rounded border border-gray-100 bg-gray-50 p-3">
          <div class="space-y-3">
            <div>
              <label class="block text-xs font-medium text-gray-600">Title</label>
              <input
                type="text"
                value={title()}
                onInput={(e) => setTitle(e.currentTarget.value)}
                class="mt-1 block w-full rounded border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none"
                placeholder="Fix login bug"
              />
            </div>
            <div>
              <label class="block text-xs font-medium text-gray-600">Prompt</label>
              <textarea
                value={prompt()}
                onInput={(e) => setPrompt(e.currentTarget.value)}
                class="mt-1 block w-full rounded border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none"
                rows={3}
                placeholder="Describe the task for the agent..."
              />
            </div>
          </div>
          <div class="mt-3 flex justify-end">
            <button
              type="submit"
              class="rounded bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700"
            >
              Create Task
            </button>
          </div>
        </form>
      </Show>

      <Show
        when={props.tasks.length > 0}
        fallback={<p class="text-sm text-gray-500">No tasks yet.</p>}
      >
        <div class="space-y-2">
          <For each={props.tasks}>
            {(t) => (
              <div class="rounded border border-gray-100">
                <div
                  class="flex cursor-pointer items-center justify-between p-3"
                  onClick={() => setExpanded((prev) => (prev === t.id ? null : t.id))}
                >
                  <div class="flex items-center gap-2">
                    <span class="font-medium">{t.title}</span>
                    <span class="text-xs text-gray-400">{t.id.slice(0, 8)}</span>
                    <Show when={t.cost_usd > 0}>
                      <span class="rounded bg-purple-50 px-1.5 py-0.5 text-xs text-purple-600">
                        {formatCost(t.cost_usd)}
                      </span>
                    </Show>
                  </div>
                  <span class={`rounded-full px-2 py-0.5 text-xs ${statusColor(t.status)}`}>
                    {t.status}
                  </span>
                </div>

                <Show when={expanded() === t.id}>
                  <div class="border-t border-gray-100 bg-gray-50 p-3 text-sm">
                    <div class="mb-2 text-gray-500">
                      <span class="font-medium">Prompt:</span> {t.prompt}
                    </div>
                    <Show when={t.agent_id}>
                      <div class="text-xs text-gray-400">
                        Agent: {t.agent_id?.slice(0, 8) ?? ""}
                      </div>
                    </Show>
                    <Show when={t.result}>
                      {(result) => (
                        <div class="mt-2">
                          <Show when={result().output}>
                            <div class="mb-1">
                              <span class="text-xs font-medium text-gray-500">Output:</span>
                              <pre class="mt-1 max-h-40 overflow-auto rounded bg-gray-900 p-2 text-xs text-green-400">
                                {result().output}
                              </pre>
                            </div>
                          </Show>
                          <Show when={result().error}>
                            <div class="mt-1 text-xs text-red-600">Error: {result().error}</div>
                          </Show>
                          <Show
                            when={(result().tokens_in ?? 0) > 0 || (result().tokens_out ?? 0) > 0}
                          >
                            <div class="mt-1 text-xs text-gray-400">
                              Tokens: {result().tokens_in ?? 0} in / {result().tokens_out ?? 0} out
                            </div>
                          </Show>
                          <Show when={(result().files ?? []).length > 0}>
                            <div class="mt-1 text-xs text-gray-400">
                              Files: {(result().files ?? []).join(", ")}
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
