import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { AgentStatus, CreateAgentRequest, Task } from "~/api/types";
import { useToast } from "~/components/Toast";

interface AgentPanelProps {
  projectId: string;
  tasks: Task[];
  onError: (msg: string) => void;
}

function agentStatusColor(status: AgentStatus): string {
  switch (status) {
    case "idle":
      return "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400";
    case "running":
      return "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400";
    case "error":
      return "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400";
    case "stopped":
      return "bg-gray-100 text-gray-500 dark:bg-gray-700 dark:text-gray-400";
  }
}

export default function AgentPanel(props: AgentPanelProps) {
  const { show: toast } = useToast();
  const [agents, { refetch }] = createResource(
    () => props.projectId,
    (id) => api.agents.list(id),
  );
  const [backends] = createResource(() => api.providers.agent());
  const [showForm, setShowForm] = createSignal(false);
  const [name, setName] = createSignal("");
  const [backend, setBackend] = createSignal("");
  const [dispatching, setDispatching] = createSignal<string | null>(null);

  const handleCreate = async (e: SubmitEvent) => {
    e.preventDefault();
    if (!name().trim() || !backend().trim()) return;

    const data: CreateAgentRequest = { name: name(), backend: backend() };
    try {
      await api.agents.create(props.projectId, data);
      setName("");
      setBackend("");
      setShowForm(false);
      refetch();
      toast("success", "Agent created");
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to create agent";
      props.onError(msg);
      toast("error", msg);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await api.agents.delete(id);
      refetch();
      toast("success", "Agent deleted");
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to delete agent";
      props.onError(msg);
      toast("error", msg);
    }
  };

  const handleDispatch = async (agentId: string, taskId: string) => {
    setDispatching(agentId);
    try {
      await api.agents.dispatch(agentId, taskId);
      refetch();
      toast("success", "Task dispatched to agent");
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Dispatch failed";
      props.onError(msg);
      toast("error", msg);
    } finally {
      setDispatching(null);
    }
  };

  const handleStop = async (agentId: string, taskId: string) => {
    try {
      await api.agents.stop(agentId, taskId);
      refetch();
      toast("success", "Agent stopped");
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Stop failed";
      props.onError(msg);
      toast("error", msg);
    }
  };

  const pendingTasks = () =>
    props.tasks.filter((t) => t.status === "pending" || t.status === "queued");

  return (
    <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <div class="mb-3 flex items-center justify-between">
        <h3 class="text-lg font-semibold">Agents</h3>
        <button
          type="button"
          class="rounded bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700"
          onClick={() => setShowForm((v) => !v)}
        >
          {showForm() ? "Cancel" : "Add Agent"}
        </button>
      </div>

      <Show when={showForm()}>
        <form
          onSubmit={handleCreate}
          class="mb-4 rounded border border-gray-100 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-900"
        >
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label
                for="agent-name"
                class="block text-xs font-medium text-gray-600 dark:text-gray-400"
              >
                Name <span aria-hidden="true">*</span>
                <span class="sr-only">(required)</span>
              </label>
              <input
                id="agent-name"
                type="text"
                value={name()}
                onInput={(e) => setName(e.currentTarget.value)}
                class="mt-1 block w-full rounded border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-700"
                placeholder="my-agent"
                aria-required="true"
              />
            </div>
            <div>
              <label
                for="agent-backend"
                class="block text-xs font-medium text-gray-600 dark:text-gray-400"
              >
                Backend <span aria-hidden="true">*</span>
                <span class="sr-only">(required)</span>
              </label>
              <Show
                when={backends()?.backends && (backends()?.backends ?? []).length > 0}
                fallback={
                  <input
                    id="agent-backend"
                    type="text"
                    value={backend()}
                    onInput={(e) => setBackend(e.currentTarget.value)}
                    class="mt-1 block w-full rounded border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-700"
                    placeholder="aider"
                    aria-required="true"
                  />
                }
              >
                <select
                  id="agent-backend"
                  value={backend()}
                  onChange={(e) => setBackend(e.currentTarget.value)}
                  class="mt-1 block w-full rounded border border-gray-300 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-700"
                  aria-required="true"
                >
                  <option value="">Select...</option>
                  <For each={backends()?.backends ?? []}>
                    {(b) => <option value={b}>{b}</option>}
                  </For>
                </select>
              </Show>
            </div>
          </div>
          <div class="mt-3 flex justify-end">
            <button
              type="submit"
              class="rounded bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700"
            >
              Create
            </button>
          </div>
        </form>
      </Show>

      <Show
        when={(agents() ?? []).length > 0}
        fallback={<p class="text-sm text-gray-500 dark:text-gray-400">No agents yet.</p>}
      >
        <div class="space-y-3">
          <For each={agents() ?? []}>
            {(agent) => (
              <div class="rounded border border-gray-100 p-3 dark:border-gray-700">
                <div class="flex items-center justify-between">
                  <div class="flex items-center gap-2">
                    <span class="font-medium">{agent.name}</span>
                    <span class="text-xs text-gray-400 dark:text-gray-500">({agent.backend})</span>
                    <span
                      class={`rounded-full px-2 py-0.5 text-xs ${agentStatusColor(agent.status)}`}
                      role="status"
                      aria-label={`Agent ${agent.name} status: ${agent.status}`}
                    >
                      {agent.status}
                    </span>
                  </div>
                  <div class="flex gap-2">
                    <Show when={agent.status === "idle" && pendingTasks().length > 0}>
                      <select
                        class="rounded border border-gray-200 px-2 py-1 text-xs dark:border-gray-600 dark:bg-gray-700"
                        aria-label={`Dispatch task to agent ${agent.name}`}
                        onChange={(e) => {
                          const taskId = e.currentTarget.value;
                          if (taskId) handleDispatch(agent.id, taskId);
                          e.currentTarget.value = "";
                        }}
                        disabled={dispatching() === agent.id}
                      >
                        <option value="">
                          {dispatching() === agent.id ? "Dispatching..." : "Dispatch task..."}
                        </option>
                        <For each={pendingTasks()}>
                          {(t) => (
                            <option value={t.id}>
                              {t.title} ({t.id.slice(0, 8)})
                            </option>
                          )}
                        </For>
                      </select>
                    </Show>
                    <Show when={agent.status === "running"}>
                      <button
                        type="button"
                        class="rounded bg-red-100 px-2 py-1 text-xs text-red-700 hover:bg-red-200 dark:bg-red-900/30 dark:text-red-400 dark:hover:bg-red-800"
                        onClick={() => {
                          const runningTask = props.tasks.find(
                            (t) => t.agent_id === agent.id && t.status === "running",
                          );
                          if (runningTask) handleStop(agent.id, runningTask.id);
                        }}
                        aria-label={`Stop agent ${agent.name}`}
                      >
                        Stop
                      </button>
                    </Show>
                    <button
                      type="button"
                      class="rounded px-2 py-1 text-xs text-red-500 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
                      onClick={() => handleDelete(agent.id)}
                      aria-label={`Delete agent ${agent.name}`}
                    >
                      Delete
                    </button>
                  </div>
                </div>
              </div>
            )}
          </For>
        </div>
      </Show>
    </div>
  );
}

export { agentStatusColor };
