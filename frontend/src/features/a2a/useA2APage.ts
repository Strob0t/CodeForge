import type { Accessor, Resource } from "solid-js";
import { createResource, createSignal } from "solid-js";

import { api } from "~/api/client";
import type {
  A2APushConfig,
  A2ARemoteAgent,
  A2ATask,
  CreateA2ARemoteAgentRequest,
  SendA2ATaskRequest,
} from "~/api/types";
import type { ToastLevel } from "~/components/Toast";
import { useAsyncAction } from "~/hooks";
import type { TranslationKey } from "~/i18n";

type ToastFn = (level: ToastLevel, message: string, dismissMs?: number) => number;
type TranslateFn = (key: TranslationKey, params?: Record<string, string | number>) => string;

// ---------------------------------------------------------------------------
// Agents Tab hook
// ---------------------------------------------------------------------------

interface AgentsTabData {
  agents: Resource<A2ARemoteAgent[]>;
  showForm: Accessor<boolean>;
  deleteTarget: Accessor<A2ARemoteAgent | null>;
  name: Accessor<string>;
  agentUrl: Accessor<string>;
  trustLevel: Accessor<string>;
  registering: Accessor<boolean>;
  registerError: Accessor<string>;

  setShowForm: (v: boolean) => void;
  setDeleteTarget: (v: A2ARemoteAgent | null) => void;
  setName: (v: string) => void;
  setAgentUrl: (v: string) => void;
  setTrustLevel: (v: string) => void;
  clearRegisterError: () => void;

  handleRegister: () => Promise<void>;
  handleDiscover: (id: string) => Promise<void>;
  handleDelete: () => Promise<void>;
  resetForm: () => void;
}

export function useAgentsTab(toast: ToastFn, t: TranslateFn): AgentsTabData {
  const [agents, { refetch }] = createResource(() => api.a2a.listAgents());
  const [showForm, setShowForm] = createSignal(false);
  const [deleteTarget, setDeleteTarget] = createSignal<A2ARemoteAgent | null>(null);

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
    run: runRegister,
    loading: registering,
    error: registerError,
    clearError: clearRegisterError,
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

  const { run: runDiscover } = useAsyncAction(
    async (id: string) => {
      await api.a2a.discoverAgent(id);
      toast("success", t("a2a.agents.toast.discovered"));
      refetch();
    },
    { onError: () => toast("error", "Discovery failed.") },
  );

  const { run: runDelete } = useAsyncAction(
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

  return {
    agents,
    showForm,
    deleteTarget,
    name,
    agentUrl,
    trustLevel,
    registering,
    registerError,
    setShowForm,
    setDeleteTarget,
    setName,
    setAgentUrl,
    setTrustLevel,
    clearRegisterError,
    handleRegister: async () => {
      await runRegister();
    },
    handleDiscover: async (id: string) => {
      await runDiscover(id);
    },
    handleDelete: async () => {
      await runDelete();
    },
    resetForm,
  };
}

// ---------------------------------------------------------------------------
// Tasks Tab hook
// ---------------------------------------------------------------------------

interface TasksTabData {
  filterState: Accessor<string>;
  filterDirection: Accessor<string>;
  tasks: Resource<A2ATask[]>;
  showSendForm: Accessor<boolean>;
  agents: Resource<A2ARemoteAgent[]>;
  sendAgentId: Accessor<string>;
  sendSkillId: Accessor<string>;
  sendPrompt: Accessor<string>;
  sending: Accessor<boolean>;

  setFilterState: (v: string) => void;
  setFilterDirection: (v: string) => void;
  setShowSendForm: (v: boolean) => void;
  setSendAgentId: (v: string) => void;
  setSendSkillId: (v: string) => void;
  setSendPrompt: (v: string) => void;

  handleSend: () => Promise<void>;
  handleCancel: (id: string) => Promise<void>;
  resetSendForm: () => void;
}

export function useTasksTab(toast: ToastFn, t: TranslateFn): TasksTabData {
  const [filterState, setFilterState] = createSignal("");
  const [filterDirection, setFilterDirection] = createSignal("");

  const [tasks, { refetch }] = createResource(
    () => ({ state: filterState(), direction: filterDirection() }),
    (params) => api.a2a.listTasks(params.state || undefined, params.direction || undefined),
  );

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

  const { run: runSend, loading: sending } = useAsyncAction(
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

  const { run: runCancel } = useAsyncAction(
    async (id: string) => {
      await api.a2a.cancelTask(id);
      toast("success", t("a2a.tasks.toast.cancelled"));
      refetch();
    },
    { onError: () => toast("error", "Cancel failed.") },
  );

  return {
    filterState,
    filterDirection,
    tasks,
    showSendForm,
    agents,
    sendAgentId,
    sendSkillId,
    sendPrompt,
    sending,
    setFilterState,
    setFilterDirection,
    setShowSendForm,
    setSendAgentId,
    setSendSkillId,
    setSendPrompt,
    handleSend: async () => {
      await runSend();
    },
    handleCancel: async (id: string) => {
      await runCancel(id);
    },
    resetSendForm,
  };
}

// ---------------------------------------------------------------------------
// Push Configs Tab hook
// ---------------------------------------------------------------------------

interface PushConfigsTabData {
  allTasks: Resource<A2ATask[]>;
  selectedTaskId: Accessor<string>;
  deleteTarget: Accessor<A2APushConfig | null>;
  configs: Resource<A2APushConfig[]>;
  showForm: Accessor<boolean>;
  webhookUrl: Accessor<string>;
  webhookToken: Accessor<string>;
  creating: Accessor<boolean>;

  setSelectedTaskId: (v: string) => void;
  setDeleteTarget: (v: A2APushConfig | null) => void;
  setShowForm: (v: boolean) => void;
  setWebhookUrl: (v: string) => void;
  setWebhookToken: (v: string) => void;

  handleCreate: () => Promise<void>;
  handleDeleteConfig: () => Promise<void>;
  resetForm: () => void;
}

export function usePushConfigsTab(toast: ToastFn, t: TranslateFn): PushConfigsTabData {
  const [allTasks] = createResource(() => api.a2a.listTasks());
  const [selectedTaskId, setSelectedTaskId] = createSignal("");
  const [deleteTarget, setDeleteTarget] = createSignal<A2APushConfig | null>(null);

  const [configs, { refetch }] = createResource(
    () => selectedTaskId(),
    (taskId) => (taskId ? api.a2a.listPushConfigs(taskId) : Promise.resolve([])),
  );

  const [showForm, setShowForm] = createSignal(false);
  const [webhookUrl, setWebhookUrl] = createSignal("");
  const [webhookToken, setWebhookToken] = createSignal("");

  function resetForm(): void {
    setWebhookUrl("");
    setWebhookToken("");
    setShowForm(false);
  }

  const { run: runCreate, loading: creating } = useAsyncAction(
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

  const { run: runDeleteConfig } = useAsyncAction(
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

  return {
    allTasks,
    selectedTaskId,
    deleteTarget,
    configs,
    showForm,
    webhookUrl,
    webhookToken,
    creating,
    setSelectedTaskId,
    setDeleteTarget,
    setShowForm,
    setWebhookUrl,
    setWebhookToken,
    handleCreate: async () => {
      await runCreate();
    },
    handleDeleteConfig: async () => {
      await runDeleteConfig();
    },
    resetForm,
  };
}
