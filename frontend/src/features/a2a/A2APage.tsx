import { createSignal, For, onMount, Show } from "solid-js";

import type {
  A2APushConfig,
  A2ARemoteAgent,
  A2ATask,
  A2ATaskDirection,
  A2ATaskState,
} from "~/api/types";
import { useToast } from "~/components/Toast";
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

import { useAgentsTab, usePushConfigsTab, useTasksTab } from "./useA2APage";

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
  const data = useAgentsTab(toast, t);

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
          <Button variant="secondary" size="sm" onClick={() => void data.handleDiscover(agent.id)}>
            {t("a2a.agents.discover")}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            class="text-cf-danger-fg hover:text-cf-danger-fg"
            onClick={() => data.setDeleteTarget(agent)}
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
          variant={data.showForm() ? "secondary" : "primary"}
          onClick={() => {
            if (data.showForm()) data.resetForm();
            else data.setShowForm(true);
          }}
        >
          {data.showForm() ? t("common.cancel") : t("a2a.agents.register")}
        </Button>
      </div>

      <ErrorBanner error={data.registerError} onDismiss={data.clearRegisterError} />

      <Show when={data.showForm()}>
        <Card class="mb-6">
          <Card.Body>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void data.handleRegister();
              }}
            >
              <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
                <FormField label={t("a2a.agents.name")} id="a2a-agent-name" required>
                  <Input
                    id="a2a-agent-name"
                    type="text"
                    value={data.name()}
                    onInput={(e) => data.setName(e.currentTarget.value)}
                    placeholder="my-remote-agent"
                    aria-required="true"
                  />
                </FormField>
                <FormField label={t("a2a.agents.url")} id="a2a-agent-url" required>
                  <Input
                    id="a2a-agent-url"
                    type="text"
                    value={data.agentUrl()}
                    onInput={(e) => data.setAgentUrl(e.currentTarget.value)}
                    placeholder="https://agent.example.com"
                    mono
                    aria-required="true"
                  />
                </FormField>
                <FormField label={t("a2a.agents.trustLevel")} id="a2a-agent-trust">
                  <Select
                    id="a2a-agent-trust"
                    value={data.trustLevel()}
                    onChange={(e) => data.setTrustLevel(e.currentTarget.value)}
                  >
                    <For each={[...TRUST_LEVELS]}>
                      {(level) => <option value={level}>{level}</option>}
                    </For>
                  </Select>
                </FormField>
              </div>
              <div class="mt-4 flex justify-end gap-2">
                <Button variant="secondary" onClick={data.resetForm}>
                  {t("common.cancel")}
                </Button>
                <Button
                  type="submit"
                  disabled={data.registering() || !data.name().trim() || !data.agentUrl().trim()}
                  loading={data.registering()}
                >
                  {t("a2a.agents.register")}
                </Button>
              </div>
            </form>
          </Card.Body>
        </Card>
      </Show>

      <Show when={data.agents.loading}>
        <LoadingState message={t("common.loading")} />
      </Show>

      <Show when={!data.agents.loading && !data.agents.error}>
        <Show
          when={(data.agents() ?? []).length > 0}
          fallback={<EmptyState title={t("a2a.agents.empty")} />}
        >
          <Table<A2ARemoteAgent>
            columns={columns}
            data={data.agents() ?? []}
            rowKey={(a) => a.id}
          />
        </Show>
      </Show>

      <ConfirmDialog
        open={data.deleteTarget() !== null}
        title={t("common.delete")}
        message={`Delete agent "${data.deleteTarget()?.name ?? ""}"?`}
        variant="danger"
        confirmLabel={t("common.delete")}
        cancelLabel={t("common.cancel")}
        onConfirm={() => void data.handleDelete()}
        onCancel={() => data.setDeleteTarget(null)}
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
  const data = useTasksTab(toast, t);

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
          <Button variant="secondary" size="sm" onClick={() => void data.handleCancel(task.id)}>
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
            value={data.filterState()}
            onChange={(e) => data.setFilterState(e.currentTarget.value)}
          >
            <option value="">{t("a2a.tasks.all")}</option>
            <For each={TASK_STATES}>{(s) => <option value={s}>{s}</option>}</For>
          </Select>
        </FormField>
        <FormField label={t("a2a.tasks.filterDirection")} id="a2a-filter-direction">
          <Select
            id="a2a-filter-direction"
            value={data.filterDirection()}
            onChange={(e) => data.setFilterDirection(e.currentTarget.value)}
          >
            <option value="">{t("a2a.tasks.all")}</option>
            <option value="inbound">inbound</option>
            <option value="outbound">outbound</option>
          </Select>
        </FormField>
        <Button
          variant={data.showSendForm() ? "secondary" : "primary"}
          onClick={() => {
            if (data.showSendForm()) data.resetSendForm();
            else data.setShowSendForm(true);
          }}
        >
          {data.showSendForm() ? t("common.cancel") : t("a2a.tasks.send")}
        </Button>
      </div>

      {/* Send task form */}
      <Show when={data.showSendForm()}>
        <Card class="mb-6">
          <Card.Body>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void data.handleSend();
              }}
            >
              <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <FormField label={t("a2a.tasks.remoteAgent")} id="a2a-send-agent" required>
                  <Select
                    id="a2a-send-agent"
                    value={data.sendAgentId()}
                    onChange={(e) => data.setSendAgentId(e.currentTarget.value)}
                  >
                    <option value="">{t("common.select")}</option>
                    <For each={data.agents() ?? []}>
                      {(agent) => <option value={agent.id}>{agent.name}</option>}
                    </For>
                  </Select>
                </FormField>
                <FormField label={t("a2a.tasks.skillId")} id="a2a-send-skill" required>
                  <Input
                    id="a2a-send-skill"
                    type="text"
                    value={data.sendSkillId()}
                    onInput={(e) => data.setSendSkillId(e.currentTarget.value)}
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
                    value={data.sendPrompt()}
                    onInput={(e) => data.setSendPrompt(e.currentTarget.value)}
                    rows={3}
                    placeholder="Describe the task..."
                  />
                </FormField>
              </div>
              <div class="mt-4 flex justify-end gap-2">
                <Button variant="secondary" onClick={data.resetSendForm}>
                  {t("common.cancel")}
                </Button>
                <Button
                  type="submit"
                  disabled={
                    data.sending() ||
                    !data.sendAgentId() ||
                    !data.sendSkillId().trim() ||
                    !data.sendPrompt().trim()
                  }
                  loading={data.sending()}
                >
                  {t("a2a.tasks.send")}
                </Button>
              </div>
            </form>
          </Card.Body>
        </Card>
      </Show>

      <Show when={data.tasks.loading}>
        <LoadingState message={t("common.loading")} />
      </Show>

      <Show when={!data.tasks.loading && !data.tasks.error}>
        <Show
          when={(data.tasks() ?? []).length > 0}
          fallback={<EmptyState title={t("a2a.tasks.empty")} />}
        >
          <Table<A2ATask> columns={columns} data={data.tasks() ?? []} rowKey={(t) => t.id} />
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
  const data = usePushConfigsTab(toast, t);

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
          onClick={() => data.setDeleteTarget(cfg)}
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
            value={data.selectedTaskId()}
            onChange={(e) => data.setSelectedTaskId(e.currentTarget.value)}
          >
            <option value="">{t("common.select")}</option>
            <For each={data.allTasks() ?? []}>
              {(task) => (
                <option value={task.id}>
                  {task.id.slice(0, 8)} ({task.state})
                </option>
              )}
            </For>
          </Select>
        </FormField>
        <Show when={data.selectedTaskId()}>
          <Button
            variant={data.showForm() ? "secondary" : "primary"}
            onClick={() => {
              if (data.showForm()) data.resetForm();
              else data.setShowForm(true);
            }}
          >
            {data.showForm() ? t("common.cancel") : t("a2a.pushConfigs.create")}
          </Button>
        </Show>
      </div>

      <Show when={data.showForm() && data.selectedTaskId()}>
        <Card class="mb-6">
          <Card.Body>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void data.handleCreate();
              }}
            >
              <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <FormField label={t("a2a.pushConfigs.url")} id="a2a-push-url" required>
                  <Input
                    id="a2a-push-url"
                    type="text"
                    value={data.webhookUrl()}
                    onInput={(e) => data.setWebhookUrl(e.currentTarget.value)}
                    placeholder="https://webhook.example.com/a2a"
                    mono
                    aria-required="true"
                  />
                </FormField>
                <FormField label={t("a2a.pushConfigs.token")} id="a2a-push-token">
                  <Input
                    id="a2a-push-token"
                    type="password"
                    value={data.webhookToken()}
                    onInput={(e) => data.setWebhookToken(e.currentTarget.value)}
                    placeholder="optional bearer token"
                    mono
                  />
                </FormField>
              </div>
              <div class="mt-4 flex justify-end gap-2">
                <Button variant="secondary" onClick={data.resetForm}>
                  {t("common.cancel")}
                </Button>
                <Button
                  type="submit"
                  disabled={data.creating() || !data.webhookUrl().trim()}
                  loading={data.creating()}
                >
                  {t("a2a.pushConfigs.create")}
                </Button>
              </div>
            </form>
          </Card.Body>
        </Card>
      </Show>

      <Show when={!data.selectedTaskId()}>
        <EmptyState title={t("a2a.pushConfigs.selectTask")} />
      </Show>

      <Show when={data.selectedTaskId()}>
        <Show when={data.configs.loading}>
          <LoadingState message={t("common.loading")} />
        </Show>

        <Show when={!data.configs.loading && !data.configs.error}>
          <Show
            when={(data.configs() ?? []).length > 0}
            fallback={<EmptyState title={t("a2a.pushConfigs.empty")} />}
          >
            <Table<A2APushConfig>
              columns={columns}
              data={data.configs() ?? []}
              rowKey={(c) => c.id}
            />
          </Show>
        </Show>
      </Show>

      <ConfirmDialog
        open={data.deleteTarget() !== null}
        title={t("common.delete")}
        message={`Delete push config "${data.deleteTarget()?.id.slice(0, 8) ?? ""}"?`}
        variant="danger"
        confirmLabel={t("common.delete")}
        cancelLabel={t("common.cancel")}
        onConfirm={() => void data.handleDeleteConfig()}
        onCancel={() => data.setDeleteTarget(null)}
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
