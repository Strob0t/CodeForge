import { createResource, createSignal, For, onMount, Show } from "solid-js";

import { api } from "~/api/client";
import type {
  A2APushConfig,
  A2ARemoteAgent,
  A2ATask,
  A2ATaskDirection,
  A2ATaskState,
  CreateA2ARemoteAgentRequest,
  SendA2ATaskRequest,
} from "~/api/types";
import { useToast } from "~/components/Toast";
import { useAsyncAction } from "~/hooks";
import { useI18n } from "~/i18n";
import {
  Badge,
  Button,
  Card,
  ConfirmDialog,
  EmptyState,
  ErrorBanner,
  FormField,
  Input,
  LoadingState,
  PageLayout,
  Select,
  Table,
  Tabs,
  Textarea,
} from "~/ui";
import type { TableColumn } from "~/ui/composites/Table";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const TRUST_LEVELS = ["untrusted", "partial", "verified", "full"] as const;

const TASK_STATES: A2ATaskState[] = [
  "submitted",
  "working",
  "completed",
  "failed",
  "canceled",
  "rejected",
  "input-required",
  "auth-required",
];

const TERMINAL_STATES = new Set<string>(["completed", "failed", "canceled", "rejected"]);

const TABS = [
  { value: "agents", label: "" },
  { value: "tasks", label: "" },
  { value: "push-configs", label: "" },
] as const;

// ---------------------------------------------------------------------------
// A2A Page
// ---------------------------------------------------------------------------

export default function A2APage() {
  onMount(() => {
    document.title = "A2A Federation - CodeForge";
  });
  const { t } = useI18n();
  const [activeTab, setActiveTab] = createSignal("agents");

  const tabItems = () =>
    TABS.map((tab) => ({
      value: tab.value,
      label: t(`a2a.tab.${tab.value}`),
    }));

  return (
    <PageLayout title={t("a2a.title")} description={t("a2a.description")}>
      <Tabs items={tabItems()} value={activeTab()} onChange={setActiveTab} class="mb-4" />

      <Show when={activeTab() === "agents"}>
        <AgentsTab />
      </Show>
      <Show when={activeTab() === "tasks"}>
        <TasksTab />
      </Show>
      <Show when={activeTab() === "push-configs"}>
        <PushConfigsTab />
      </Show>
    </PageLayout>
  );
}

// ---------------------------------------------------------------------------
// Agents Tab
// ---------------------------------------------------------------------------

function AgentsTab() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [agents, { refetch }] = createResource(() => api.a2a.listAgents());
  const [showForm, setShowForm] = createSignal(false);
  const [deleteTarget, setDeleteTarget] = createSignal<A2ARemoteAgent | null>(null);

  // Form state
  const [name, setName] = createSignal("");
  const [agentUrl, setAgentUrl] = createSignal("");
  const [trustLevel, setTrustLevel] = createSignal("untrusted");

  function resetForm(): void {
    setName("");
    setAgentUrl("");
    setTrustLevel("untrusted");
    setShowForm(false);
  }

  const {
    run: handleRegister,
    loading: registering,
    error: registerError,
    clearError,
  } = useAsyncAction(
    async () => {
      const req: CreateA2ARemoteAgentRequest = {
        name: name().trim(),
        url: agentUrl().trim(),
        trust_level: trustLevel(),
      };
      if (!req.name || !req.url) return;
      await api.a2a.registerAgent(req);
      toast("success", t("a2a.agents.toast.registered"));
      resetForm();
      refetch();
    },
    { onError: () => toast("error", "Failed to register agent.") },
  );

  const { run: handleDiscover } = useAsyncAction(
    async (id: string) => {
      await api.a2a.discoverAgent(id);
      toast("success", t("a2a.agents.toast.discovered"));
      refetch();
    },
    { onError: () => toast("error", "Discovery failed.") },
  );

  const { run: handleDelete } = useAsyncAction(
    async () => {
      const target = deleteTarget();
      if (!target) return;
      await api.a2a.deleteAgent(target.id);
      toast("success", t("a2a.agents.toast.deleted"));
      setDeleteTarget(null);
      refetch();
    },
    { onError: () => toast("error", "Delete failed.") },
  );

  const columns: TableColumn<A2ARemoteAgent>[] = [
    {
      key: "name",
      header: t("a2a.agents.name"),
      render: (agent) => (
        <div>
          <span class="font-medium text-cf-text-primary">{agent.name}</span>
          <Show when={agent.description}>
            <p class="mt-0.5 text-xs text-cf-text-muted">{agent.description}</p>
          </Show>
        </div>
      ),
    },
    {
      key: "url",
      header: t("a2a.agents.url"),
      render: (agent) => <span class="font-mono text-xs">{agent.url}</span>,
    },
    {
      key: "trust_level",
      header: t("a2a.agents.trustLevel"),
      render: (agent) => <TrustBadge level={agent.trust_level} />,
    },
    {
      key: "enabled",
      header: t("a2a.agents.enabled"),
      render: (agent) => (
        <Badge variant={agent.enabled ? "success" : "default"}>
          {agent.enabled ? "Yes" : "No"}
        </Badge>
      ),
    },
    {
      key: "skills",
      header: t("a2a.agents.skills"),
      render: (agent) => (
        <div class="flex flex-wrap gap-1">
          <For each={agent.skills ?? []}>{(skill) => <Badge>{skill}</Badge>}</For>
          <Show when={!agent.skills?.length}>
            <span class="text-xs text-cf-text-muted">--</span>
          </Show>
        </div>
      ),
    },
    {
      key: "last_seen",
      header: t("a2a.agents.lastSeen"),
      render: (agent) => <span class="text-xs text-cf-text-muted">{agent.last_seen ?? "--"}</span>,
    },
    {
      key: "actions",
      header: "",
      render: (agent) => (
        <div class="flex items-center gap-2">
          <Button variant="secondary" size="sm" onClick={() => void handleDiscover(agent.id)}>
            {t("a2a.agents.discover")}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            class="text-cf-danger-fg hover:text-cf-danger-fg"
            onClick={() => setDeleteTarget(agent)}
          >
            {t("common.delete")}
          </Button>
        </div>
      ),
    },
  ];

  return (
    <>
      <div class="mb-4 flex justify-end">
        <Button
          variant={showForm() ? "secondary" : "primary"}
          onClick={() => {
            if (showForm()) resetForm();
            else setShowForm(true);
          }}
        >
          {showForm() ? t("common.cancel") : t("a2a.agents.register")}
        </Button>
      </div>

      <ErrorBanner error={registerError} onDismiss={clearError} />

      <Show when={showForm()}>
        <Card class="mb-6">
          <Card.Body>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void handleRegister();
              }}
            >
              <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
                <FormField label={t("a2a.agents.name")} id="a2a-agent-name" required>
                  <Input
                    id="a2a-agent-name"
                    type="text"
                    value={name()}
                    onInput={(e) => setName(e.currentTarget.value)}
                    placeholder="my-remote-agent"
                    aria-required="true"
                  />
                </FormField>
                <FormField label={t("a2a.agents.url")} id="a2a-agent-url" required>
                  <Input
                    id="a2a-agent-url"
                    type="text"
                    value={agentUrl()}
                    onInput={(e) => setAgentUrl(e.currentTarget.value)}
                    placeholder="https://agent.example.com"
                    mono
                    aria-required="true"
                  />
                </FormField>
                <FormField label={t("a2a.agents.trustLevel")} id="a2a-agent-trust">
                  <Select
                    id="a2a-agent-trust"
                    value={trustLevel()}
                    onChange={(e) => setTrustLevel(e.currentTarget.value)}
                  >
                    <For each={[...TRUST_LEVELS]}>
                      {(level) => <option value={level}>{level}</option>}
                    </For>
                  </Select>
                </FormField>
              </div>
              <div class="mt-4 flex justify-end gap-2">
                <Button variant="secondary" onClick={resetForm}>
                  {t("common.cancel")}
                </Button>
                <Button
                  type="submit"
                  disabled={registering() || !name().trim() || !agentUrl().trim()}
                  loading={registering()}
                >
                  {t("a2a.agents.register")}
                </Button>
              </div>
            </form>
          </Card.Body>
        </Card>
      </Show>

      <Show when={agents.loading}>
        <LoadingState message={t("common.loading")} />
      </Show>

      <Show when={!agents.loading && !agents.error}>
        <Show
          when={(agents() ?? []).length > 0}
          fallback={<EmptyState title={t("a2a.agents.empty")} />}
        >
          <Table<A2ARemoteAgent> columns={columns} data={agents() ?? []} rowKey={(a) => a.id} />
        </Show>
      </Show>

      <ConfirmDialog
        open={deleteTarget() !== null}
        title={t("common.delete")}
        message={`Delete agent "${deleteTarget()?.name ?? ""}"?`}
        variant="danger"
        confirmLabel={t("common.delete")}
        cancelLabel={t("common.cancel")}
        onConfirm={() => void handleDelete()}
        onCancel={() => setDeleteTarget(null)}
      />
    </>
  );
}

// ---------------------------------------------------------------------------
// Tasks Tab
// ---------------------------------------------------------------------------

function TasksTab() {
  const { t } = useI18n();
  const { show: toast } = useToast();

  // Filters
  const [filterState, setFilterState] = createSignal("");
  const [filterDirection, setFilterDirection] = createSignal("");

  const [tasks, { refetch }] = createResource(
    () => ({ state: filterState(), direction: filterDirection() }),
    (params) => api.a2a.listTasks(params.state || undefined, params.direction || undefined),
  );

  // Send-task form
  const [showSendForm, setShowSendForm] = createSignal(false);
  const [agents] = createResource(() => api.a2a.listAgents());
  const [sendAgentId, setSendAgentId] = createSignal("");
  const [sendSkillId, setSendSkillId] = createSignal("");
  const [sendPrompt, setSendPrompt] = createSignal("");

  function resetSendForm(): void {
    setSendAgentId("");
    setSendSkillId("");
    setSendPrompt("");
    setShowSendForm(false);
  }

  const { run: handleSend, loading: sending } = useAsyncAction(
    async () => {
      const agentId = sendAgentId();
      if (!agentId || !sendSkillId().trim() || !sendPrompt().trim()) return;
      const req: SendA2ATaskRequest = {
        skill_id: sendSkillId().trim(),
        prompt: sendPrompt().trim(),
      };
      await api.a2a.sendTask(agentId, req);
      toast("success", t("a2a.tasks.toast.sent"));
      resetSendForm();
      refetch();
    },
    { onError: () => toast("error", "Failed to send task.") },
  );

  const { run: handleCancel } = useAsyncAction(
    async (id: string) => {
      await api.a2a.cancelTask(id);
      toast("success", t("a2a.tasks.toast.cancelled"));
      refetch();
    },
    { onError: () => toast("error", "Cancel failed.") },
  );

  const columns: TableColumn<A2ATask>[] = [
    {
      key: "id",
      header: "ID",
      render: (task) => <span class="font-mono text-xs">{task.id.slice(0, 8)}</span>,
    },
    {
      key: "state",
      header: t("a2a.tasks.state"),
      render: (task) => <StateBadge state={task.state} />,
    },
    {
      key: "direction",
      header: t("a2a.tasks.direction"),
      render: (task) => <DirectionBadge direction={task.direction} />,
    },
    {
      key: "skill_id",
      header: t("a2a.tasks.skillId"),
      render: (task) => <span class="font-mono text-xs">{task.skill_id || "--"}</span>,
    },
    {
      key: "remote_agent_id",
      header: t("a2a.tasks.remoteAgent"),
      render: (task) => (
        <span class="font-mono text-xs">
          {task.remote_agent_id ? task.remote_agent_id.slice(0, 8) : "--"}
        </span>
      ),
    },
    {
      key: "source_addr",
      header: t("a2a.tasks.source"),
      render: (task) => <span class="text-xs">{task.source_addr || "--"}</span>,
    },
    {
      key: "created_at",
      header: "Created",
      render: (task) => <span class="text-xs text-cf-text-muted">{task.created_at}</span>,
    },
    {
      key: "actions",
      header: "",
      render: (task) => (
        <Show when={!TERMINAL_STATES.has(task.state)}>
          <Button variant="secondary" size="sm" onClick={() => void handleCancel(task.id)}>
            {t("a2a.tasks.cancel")}
          </Button>
        </Show>
      ),
    },
  ];

  return (
    <>
      {/* Filters + Send button */}
      <div class="mb-4 flex flex-wrap items-end gap-3">
        <FormField label={t("a2a.tasks.filterState")} id="a2a-filter-state">
          <Select
            id="a2a-filter-state"
            value={filterState()}
            onChange={(e) => setFilterState(e.currentTarget.value)}
          >
            <option value="">{t("a2a.tasks.all")}</option>
            <For each={TASK_STATES}>{(s) => <option value={s}>{s}</option>}</For>
          </Select>
        </FormField>
        <FormField label={t("a2a.tasks.filterDirection")} id="a2a-filter-direction">
          <Select
            id="a2a-filter-direction"
            value={filterDirection()}
            onChange={(e) => setFilterDirection(e.currentTarget.value)}
          >
            <option value="">{t("a2a.tasks.all")}</option>
            <option value="inbound">inbound</option>
            <option value="outbound">outbound</option>
          </Select>
        </FormField>
        <Button
          variant={showSendForm() ? "secondary" : "primary"}
          onClick={() => {
            if (showSendForm()) resetSendForm();
            else setShowSendForm(true);
          }}
        >
          {showSendForm() ? t("common.cancel") : t("a2a.tasks.send")}
        </Button>
      </div>

      {/* Send task form */}
      <Show when={showSendForm()}>
        <Card class="mb-6">
          <Card.Body>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void handleSend();
              }}
            >
              <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <FormField label={t("a2a.tasks.remoteAgent")} id="a2a-send-agent" required>
                  <Select
                    id="a2a-send-agent"
                    value={sendAgentId()}
                    onChange={(e) => setSendAgentId(e.currentTarget.value)}
                  >
                    <option value="">{t("common.select")}</option>
                    <For each={agents() ?? []}>
                      {(agent) => <option value={agent.id}>{agent.name}</option>}
                    </For>
                  </Select>
                </FormField>
                <FormField label={t("a2a.tasks.skillId")} id="a2a-send-skill" required>
                  <Input
                    id="a2a-send-skill"
                    type="text"
                    value={sendSkillId()}
                    onInput={(e) => setSendSkillId(e.currentTarget.value)}
                    placeholder="code_review"
                    mono
                    aria-required="true"
                  />
                </FormField>
                <FormField
                  label={t("a2a.tasks.prompt")}
                  id="a2a-send-prompt"
                  class="sm:col-span-2"
                  required
                >
                  <Textarea
                    id="a2a-send-prompt"
                    value={sendPrompt()}
                    onInput={(e) => setSendPrompt(e.currentTarget.value)}
                    rows={3}
                    placeholder="Describe the task..."
                  />
                </FormField>
              </div>
              <div class="mt-4 flex justify-end gap-2">
                <Button variant="secondary" onClick={resetSendForm}>
                  {t("common.cancel")}
                </Button>
                <Button
                  type="submit"
                  disabled={
                    sending() || !sendAgentId() || !sendSkillId().trim() || !sendPrompt().trim()
                  }
                  loading={sending()}
                >
                  {t("a2a.tasks.send")}
                </Button>
              </div>
            </form>
          </Card.Body>
        </Card>
      </Show>

      <Show when={tasks.loading}>
        <LoadingState message={t("common.loading")} />
      </Show>

      <Show when={!tasks.loading && !tasks.error}>
        <Show
          when={(tasks() ?? []).length > 0}
          fallback={<EmptyState title={t("a2a.tasks.empty")} />}
        >
          <Table<A2ATask> columns={columns} data={tasks() ?? []} rowKey={(t) => t.id} />
        </Show>
      </Show>
    </>
  );
}

// ---------------------------------------------------------------------------
// Push Configs Tab
// ---------------------------------------------------------------------------

function PushConfigsTab() {
  const { t } = useI18n();
  const { show: toast } = useToast();

  // Load tasks for the dropdown
  const [allTasks] = createResource(() => api.a2a.listTasks());
  const [selectedTaskId, setSelectedTaskId] = createSignal("");
  const [deleteTarget, setDeleteTarget] = createSignal<A2APushConfig | null>(null);

  // Push configs for the selected task
  const [configs, { refetch }] = createResource(
    () => selectedTaskId(),
    (taskId) => (taskId ? api.a2a.listPushConfigs(taskId) : Promise.resolve([])),
  );

  // Create form
  const [showForm, setShowForm] = createSignal(false);
  const [webhookUrl, setWebhookUrl] = createSignal("");
  const [webhookToken, setWebhookToken] = createSignal("");

  function resetForm(): void {
    setWebhookUrl("");
    setWebhookToken("");
    setShowForm(false);
  }

  const { run: handleCreate, loading: creating } = useAsyncAction(
    async () => {
      const taskId = selectedTaskId();
      if (!taskId || !webhookUrl().trim()) return;
      await api.a2a.createPushConfig(taskId, {
        url: webhookUrl().trim(),
        token: webhookToken().trim() || undefined,
      });
      toast("success", t("a2a.pushConfigs.toast.created"));
      resetForm();
      refetch();
    },
    { onError: () => toast("error", "Failed to create push config.") },
  );

  const { run: handleDeleteConfig } = useAsyncAction(
    async () => {
      const target = deleteTarget();
      if (!target) return;
      await api.a2a.deletePushConfig(target.id);
      toast("success", t("a2a.pushConfigs.toast.deleted"));
      setDeleteTarget(null);
      refetch();
    },
    { onError: () => toast("error", "Delete failed.") },
  );

  const columns: TableColumn<A2APushConfig>[] = [
    {
      key: "id",
      header: "ID",
      render: (cfg) => <span class="font-mono text-xs">{cfg.id.slice(0, 8)}</span>,
    },
    {
      key: "url",
      header: t("a2a.pushConfigs.url"),
      render: (cfg) => <span class="font-mono text-xs break-all">{cfg.url}</span>,
    },
    {
      key: "token",
      header: t("a2a.pushConfigs.token"),
      render: (cfg) => (
        <span class="font-mono text-xs text-cf-text-muted">
          {cfg.token ? "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022" : "--"}
        </span>
      ),
    },
    {
      key: "created_at",
      header: "Created",
      render: (cfg) => <span class="text-xs text-cf-text-muted">{cfg.created_at}</span>,
    },
    {
      key: "actions",
      header: "",
      render: (cfg) => (
        <Button
          variant="ghost"
          size="sm"
          class="text-cf-danger-fg hover:text-cf-danger-fg"
          onClick={() => setDeleteTarget(cfg)}
        >
          {t("common.delete")}
        </Button>
      ),
    },
  ];

  return (
    <>
      <div class="mb-4 flex flex-wrap items-end gap-3">
        <FormField label={t("a2a.pushConfigs.selectTask")} id="a2a-push-task">
          <Select
            id="a2a-push-task"
            value={selectedTaskId()}
            onChange={(e) => setSelectedTaskId(e.currentTarget.value)}
          >
            <option value="">{t("common.select")}</option>
            <For each={allTasks() ?? []}>
              {(task) => (
                <option value={task.id}>
                  {task.id.slice(0, 8)} ({task.state})
                </option>
              )}
            </For>
          </Select>
        </FormField>
        <Show when={selectedTaskId()}>
          <Button
            variant={showForm() ? "secondary" : "primary"}
            onClick={() => {
              if (showForm()) resetForm();
              else setShowForm(true);
            }}
          >
            {showForm() ? t("common.cancel") : t("a2a.pushConfigs.create")}
          </Button>
        </Show>
      </div>

      <Show when={showForm() && selectedTaskId()}>
        <Card class="mb-6">
          <Card.Body>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void handleCreate();
              }}
            >
              <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <FormField label={t("a2a.pushConfigs.url")} id="a2a-push-url" required>
                  <Input
                    id="a2a-push-url"
                    type="text"
                    value={webhookUrl()}
                    onInput={(e) => setWebhookUrl(e.currentTarget.value)}
                    placeholder="https://webhook.example.com/a2a"
                    mono
                    aria-required="true"
                  />
                </FormField>
                <FormField label={t("a2a.pushConfigs.token")} id="a2a-push-token">
                  <Input
                    id="a2a-push-token"
                    type="password"
                    value={webhookToken()}
                    onInput={(e) => setWebhookToken(e.currentTarget.value)}
                    placeholder="optional bearer token"
                    mono
                  />
                </FormField>
              </div>
              <div class="mt-4 flex justify-end gap-2">
                <Button variant="secondary" onClick={resetForm}>
                  {t("common.cancel")}
                </Button>
                <Button
                  type="submit"
                  disabled={creating() || !webhookUrl().trim()}
                  loading={creating()}
                >
                  {t("a2a.pushConfigs.create")}
                </Button>
              </div>
            </form>
          </Card.Body>
        </Card>
      </Show>

      <Show when={!selectedTaskId()}>
        <EmptyState title={t("a2a.pushConfigs.selectTask")} />
      </Show>

      <Show when={selectedTaskId()}>
        <Show when={configs.loading}>
          <LoadingState message={t("common.loading")} />
        </Show>

        <Show when={!configs.loading && !configs.error}>
          <Show
            when={(configs() ?? []).length > 0}
            fallback={<EmptyState title={t("a2a.pushConfigs.empty")} />}
          >
            <Table<A2APushConfig> columns={columns} data={configs() ?? []} rowKey={(c) => c.id} />
          </Show>
        </Show>
      </Show>

      <ConfirmDialog
        open={deleteTarget() !== null}
        title={t("common.delete")}
        message={`Delete push config "${deleteTarget()?.id.slice(0, 8) ?? ""}"?`}
        variant="danger"
        confirmLabel={t("common.delete")}
        cancelLabel={t("common.cancel")}
        onConfirm={() => void handleDeleteConfig()}
        onCancel={() => setDeleteTarget(null)}
      />
    </>
  );
}

// ---------------------------------------------------------------------------
// Badge helpers
// ---------------------------------------------------------------------------

function TrustBadge(props: { level: string }) {
  const variant = (): "success" | "info" | "default" | "danger" => {
    switch (props.level) {
      case "full":
        return "success";
      case "verified":
        return "info";
      case "partial":
        return "default";
      case "untrusted":
      default:
        return "danger";
    }
  };
  return (
    <Badge variant={variant()} pill>
      {props.level}
    </Badge>
  );
}

function StateBadge(props: { state: A2ATaskState }) {
  const variant = (): "success" | "info" | "default" | "danger" => {
    switch (props.state) {
      case "completed":
        return "success";
      case "working":
      case "submitted":
        return "info";
      case "failed":
      case "rejected":
      case "canceled":
        return "danger";
      default:
        return "default";
    }
  };
  return (
    <Badge variant={variant()} pill>
      {props.state}
    </Badge>
  );
}

function DirectionBadge(props: { direction: A2ATaskDirection }) {
  return (
    <Badge variant={props.direction === "inbound" ? "info" : "default"} pill>
      {props.direction}
    </Badge>
  );
}
