import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { AgentStatus, CreateAgentRequest, Task } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { Badge, Button, Card, FormField, Input, Select } from "~/ui";

interface AgentPanelProps {
  projectId: string;
  tasks: Task[];
  onError: (msg: string) => void;
}

function agentStatusVariant(status: AgentStatus): "success" | "info" | "danger" | "default" {
  switch (status) {
    case "idle":
      return "success";
    case "running":
      return "info";
    case "error":
      return "danger";
    case "stopped":
      return "default";
  }
}

export default function AgentPanel(props: AgentPanelProps) {
  const { t } = useI18n();
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
      toast("success", t("agent.toast.created"));
    } catch (err) {
      const msg = err instanceof Error ? err.message : t("agent.toast.createFailed");
      props.onError(msg);
      toast("error", msg);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await api.agents.delete(id);
      refetch();
      toast("success", t("agent.toast.deleted"));
    } catch (err) {
      const msg = err instanceof Error ? err.message : t("agent.toast.deleteFailed");
      props.onError(msg);
      toast("error", msg);
    }
  };

  const handleDispatch = async (agentId: string, taskId: string) => {
    setDispatching(agentId);
    try {
      await api.agents.dispatch(agentId, taskId);
      refetch();
      toast("success", t("agent.toast.dispatched"));
    } catch (err) {
      const msg = err instanceof Error ? err.message : t("agent.toast.dispatchFailed");
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
      toast("success", t("agent.toast.stopped"));
    } catch (err) {
      const msg = err instanceof Error ? err.message : t("agent.toast.stopFailed");
      props.onError(msg);
      toast("error", msg);
    }
  };

  const pendingTasks = () =>
    props.tasks.filter((task) => task.status === "pending" || task.status === "queued");

  return (
    <Card>
      <Card.Header>
        <div class="flex items-center justify-between">
          <h3 class="text-lg font-semibold">{t("agent.title")}</h3>
          <Button
            variant={showForm() ? "secondary" : "primary"}
            size="sm"
            onClick={() => setShowForm((v) => !v)}
          >
            {showForm() ? t("common.cancel") : t("agent.addAgent")}
          </Button>
        </div>
      </Card.Header>

      <Card.Body>
        <Show when={showForm()}>
          <form
            onSubmit={handleCreate}
            class="mb-4 rounded-cf-sm border border-cf-border-subtle bg-cf-bg-primary p-3"
          >
            <div class="grid grid-cols-2 gap-3">
              <FormField id="agent-name" label={t("agent.form.name")} required>
                <Input
                  id="agent-name"
                  type="text"
                  value={name()}
                  onInput={(e) => setName(e.currentTarget.value)}
                  placeholder={t("agent.form.namePlaceholder")}
                  aria-required="true"
                />
              </FormField>
              <div>
                <Show
                  when={backends()?.backends && (backends()?.backends ?? []).length > 0}
                  fallback={
                    <FormField id="agent-backend" label={t("agent.form.backend")} required>
                      <Input
                        id="agent-backend"
                        type="text"
                        value={backend()}
                        onInput={(e) => setBackend(e.currentTarget.value)}
                        placeholder="aider"
                        aria-required="true"
                      />
                    </FormField>
                  }
                >
                  <FormField id="agent-backend" label={t("agent.form.backend")} required>
                    <Select
                      id="agent-backend"
                      value={backend()}
                      onChange={(e) => setBackend(e.currentTarget.value)}
                      aria-required="true"
                    >
                      <option value="">{t("agent.form.backendPlaceholder")}</option>
                      <For each={backends()?.backends ?? []}>
                        {(b) => <option value={b}>{b}</option>}
                      </For>
                    </Select>
                  </FormField>
                </Show>
              </div>
            </div>
            <div class="mt-3 flex justify-end">
              <Button type="submit" variant="primary" size="sm">
                {t("agent.form.create")}
              </Button>
            </div>
          </form>
        </Show>

        <Show
          when={(agents() ?? []).length > 0}
          fallback={<p class="text-sm text-cf-text-tertiary">{t("agent.empty")}</p>}
        >
          <div class="space-y-3">
            <For each={agents() ?? []}>
              {(agent) => (
                <div class="rounded-cf-sm border border-cf-border-subtle p-3">
                  <div class="flex items-center justify-between">
                    <div class="flex items-center gap-2">
                      <span class="font-medium">{agent.name}</span>
                      <span class="text-xs text-cf-text-muted">({agent.backend})</span>
                      <Badge
                        variant={agentStatusVariant(agent.status)}
                        pill
                        aria-label={`Agent ${agent.name} status: ${agent.status}`}
                      >
                        {agent.status}
                      </Badge>
                    </div>
                    <div class="flex gap-2">
                      <Show when={agent.status === "idle" && pendingTasks().length > 0}>
                        <Select
                          aria-label={`Dispatch task to agent ${agent.name}`}
                          onChange={(e) => {
                            const taskId = e.currentTarget.value;
                            if (taskId) handleDispatch(agent.id, taskId);
                            e.currentTarget.value = "";
                          }}
                          disabled={dispatching() === agent.id}
                        >
                          <option value="">
                            {dispatching() === agent.id
                              ? t("agent.dispatching")
                              : t("agent.dispatchTask")}
                          </option>
                          <For each={pendingTasks()}>
                            {(task) => (
                              <option value={task.id}>
                                {task.title} ({task.id.slice(0, 8)})
                              </option>
                            )}
                          </For>
                        </Select>
                      </Show>
                      <Show when={agent.status === "running"}>
                        <Button
                          variant="danger"
                          size="sm"
                          onClick={() => {
                            const runningTask = props.tasks.find(
                              (task) => task.agent_id === agent.id && task.status === "running",
                            );
                            if (runningTask) handleStop(agent.id, runningTask.id);
                          }}
                          aria-label={`Stop agent ${agent.name}`}
                        >
                          {t("agent.stop")}
                        </Button>
                      </Show>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleDelete(agent.id)}
                        aria-label={`Delete agent ${agent.name}`}
                      >
                        {t("common.delete")}
                      </Button>
                    </div>
                  </div>
                </div>
              )}
            </For>
          </div>
        </Show>
      </Card.Body>
    </Card>
  );
}

export { agentStatusVariant };
