import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type {
  Agent,
  AgentTeam,
  CreateTeamRequest,
  Project,
  SharedContext,
  TeamRole,
  TeamStatus,
} from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";

const TEAM_STATUS_COLORS: Record<TeamStatus, string> = {
  initializing: "bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300",
  active: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
  completed: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
  failed: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
};

const ROLE_COLORS: Record<TeamRole, string> = {
  coder: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
  reviewer: "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400",
  tester: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
  documenter: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
  planner: "bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400",
};

const PROTOCOLS = ["round-robin", "pipeline", "parallel", "consensus", "ping-pong"] as const;
const ROLES: TeamRole[] = ["coder", "reviewer", "tester", "documenter", "planner"];

interface MemberDraft {
  agent_id: string;
  role: TeamRole;
}

export default function TeamsPage() {
  const { t, fmt } = useI18n();
  const { show: toast } = useToast();

  const [selectedProjectId, setSelectedProjectId] = createSignal("");
  const [expandedTeamId, setExpandedTeamId] = createSignal<string | null>(null);

  // Form state
  const [formName, setFormName] = createSignal("");
  const [formProtocol, setFormProtocol] = createSignal<string>("round-robin");
  const [formMembers, setFormMembers] = createSignal<MemberDraft[]>([]);
  const [creating, setCreating] = createSignal(false);

  const [projects] = createResource(() => api.projects.list());
  const [teams, { refetch: refetchTeams }] = createResource(
    () => selectedProjectId(),
    (pid) => (pid ? api.teams.list(pid) : []),
  );
  const [agents] = createResource(
    () => selectedProjectId(),
    (pid) => (pid ? api.agents.list(pid) : []),
  );

  const [sharedCtx] = createResource(
    () => expandedTeamId(),
    (tid) => (tid ? api.teams.sharedContext(tid).catch(() => null) : null),
  );

  const agentName = (id: string): string => {
    const a = (agents() ?? []).find((ag: Agent) => ag.id === id);
    return a ? `${a.name} (${a.backend})` : id.slice(0, 8);
  };

  const addMember = () => {
    setFormMembers((prev) => [...prev, { agent_id: "", role: "coder" }]);
  };

  const removeMember = (idx: number) => {
    setFormMembers((prev) => prev.filter((_, i) => i !== idx));
  };

  const updateMember = (idx: number, field: keyof MemberDraft, value: string) => {
    setFormMembers((prev) => prev.map((m, i) => (i === idx ? { ...m, [field]: value } : m)));
  };

  const handleCreate = async () => {
    const pid = selectedProjectId();
    if (!pid) return;
    const name = formName().trim();
    if (!name) {
      toast("error", t("teams.toast.nameRequired"));
      return;
    }
    const members = formMembers().filter((m) => m.agent_id);
    if (members.length === 0) {
      toast("error", t("teams.toast.membersRequired"));
      return;
    }

    setCreating(true);
    try {
      const req: CreateTeamRequest = {
        name,
        protocol: formProtocol(),
        members,
      };
      await api.teams.create(pid, req);
      toast("success", t("teams.toast.created"));
      setFormName("");
      setFormProtocol("round-robin");
      setFormMembers([]);
      refetchTeams();
    } catch (e) {
      const msg = e instanceof Error ? e.message : t("teams.toast.createFailed");
      toast("error", msg);
    } finally {
      setCreating(false);
    }
  };

  const handleDelete = async (team: AgentTeam) => {
    try {
      await api.teams.delete(team.id);
      toast("success", t("teams.toast.deleted"));
      if (expandedTeamId() === team.id) setExpandedTeamId(null);
      refetchTeams();
    } catch (e) {
      const msg = e instanceof Error ? e.message : t("teams.toast.deleteFailed");
      toast("error", msg);
    }
  };

  return (
    <div>
      <h2 class="mb-6 text-2xl font-bold text-gray-900 dark:text-gray-100">{t("teams.title")}</h2>

      {/* Project selector */}
      <div class="mb-4">
        <select
          class="rounded border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700"
          value={selectedProjectId()}
          aria-label={t("teams.selectProject")}
          onChange={(e) => {
            setSelectedProjectId(e.currentTarget.value);
            setExpandedTeamId(null);
          }}
        >
          <option value="">{t("teams.selectProject")}</option>
          <For each={projects() ?? []}>
            {(p: Project) => <option value={p.id}>{p.name}</option>}
          </For>
        </select>
      </div>

      <Show when={selectedProjectId()}>
        {/* Create Team form */}
        <div class="mb-6 rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
          <h3 class="mb-3 text-lg font-semibold">{t("teams.createTeam")}</h3>
          <div class="mb-3 flex flex-wrap gap-2">
            <input
              type="text"
              class="rounded border border-gray-300 px-2 py-1.5 text-sm dark:border-gray-600 dark:bg-gray-700"
              placeholder={t("teams.form.namePlaceholder")}
              value={formName()}
              onInput={(e) => setFormName(e.currentTarget.value)}
              aria-label={t("teams.form.name")}
            />
            <select
              class="rounded border border-gray-300 px-2 py-1.5 text-sm dark:border-gray-600 dark:bg-gray-700"
              value={formProtocol()}
              aria-label={t("teams.form.protocol")}
              onChange={(e) => setFormProtocol(e.currentTarget.value)}
            >
              <For each={PROTOCOLS}>{(p) => <option value={p}>{p}</option>}</For>
            </select>
          </div>

          {/* Members */}
          <div class="mb-3">
            <div class="mb-1 flex items-center gap-2">
              <span class="text-sm font-medium text-gray-600 dark:text-gray-400">
                {t("teams.form.members")}
              </span>
              <button
                type="button"
                class="rounded bg-gray-200 px-2 py-0.5 text-xs font-medium text-gray-700 hover:bg-gray-300 dark:bg-gray-700 dark:text-gray-300 dark:hover:bg-gray-600"
                onClick={addMember}
              >
                {t("teams.form.addMember")}
              </button>
            </div>
            <div class="space-y-1">
              <For each={formMembers()}>
                {(member, idx) => (
                  <div class="flex items-center gap-2">
                    <select
                      class="flex-1 rounded border border-gray-300 px-2 py-1 text-sm dark:border-gray-600 dark:bg-gray-700"
                      value={member.agent_id}
                      aria-label={t("teams.form.selectAgent")}
                      onChange={(e) => updateMember(idx(), "agent_id", e.currentTarget.value)}
                    >
                      <option value="">{t("teams.form.selectAgent")}</option>
                      <For each={agents() ?? []}>
                        {(a: Agent) => (
                          <option value={a.id}>
                            {a.name} ({a.backend})
                          </option>
                        )}
                      </For>
                    </select>
                    <select
                      class="rounded border border-gray-300 px-2 py-1 text-sm dark:border-gray-600 dark:bg-gray-700"
                      value={member.role}
                      aria-label={t("teams.form.selectRole")}
                      onChange={(e) => updateMember(idx(), "role", e.currentTarget.value)}
                    >
                      <For each={ROLES}>
                        {(r) => <option value={r}>{t(`teams.role.${r}`)}</option>}
                      </For>
                    </select>
                    <button
                      type="button"
                      class="rounded px-2 py-1 text-xs text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20"
                      onClick={() => removeMember(idx())}
                      aria-label={t("teams.form.removeMemberAria")}
                    >
                      {t("common.delete")}
                    </button>
                  </div>
                )}
              </For>
            </div>
          </div>

          <button
            type="button"
            class="rounded bg-blue-600 px-4 py-1.5 text-sm text-white hover:bg-blue-700 disabled:opacity-50"
            onClick={handleCreate}
            disabled={creating()}
          >
            {creating() ? t("common.creating") : t("teams.form.create")}
          </button>
        </div>

        {/* Teams list */}
        <Show
          when={(teams() ?? []).length > 0}
          fallback={
            <div class="rounded-lg border border-gray-200 bg-white p-8 text-center dark:border-gray-700 dark:bg-gray-800">
              <p class="text-sm text-gray-500 dark:text-gray-400">{t("teams.empty")}</p>
            </div>
          }
        >
          <div class="space-y-3">
            <For each={teams() ?? []}>
              {(team: AgentTeam) => (
                <div class="rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
                  {/* Team header */}
                  <div class="flex items-center justify-between px-4 py-3">
                    <div class="flex items-center gap-3">
                      <button
                        type="button"
                        class="text-left font-medium text-gray-900 hover:text-blue-600 dark:text-gray-100 dark:hover:text-blue-400"
                        onClick={() =>
                          setExpandedTeamId((prev) => (prev === team.id ? null : team.id))
                        }
                        aria-expanded={expandedTeamId() === team.id}
                      >
                        {team.name}
                      </button>
                      <span
                        class={`rounded px-2 py-0.5 text-xs font-medium ${TEAM_STATUS_COLORS[team.status]}`}
                      >
                        {team.status}
                      </span>
                      <span class="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-500 dark:bg-gray-700 dark:text-gray-400">
                        {team.protocol}
                      </span>
                    </div>
                    <div class="flex items-center gap-2">
                      <span class="text-xs text-gray-400 dark:text-gray-500">
                        {t("teams.members", { count: String(team.members.length) })}
                      </span>
                      <span class="text-xs text-gray-400 dark:text-gray-500">
                        {fmt.date(team.created_at)}
                      </span>
                      <button
                        type="button"
                        class="rounded px-2 py-1 text-xs text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20"
                        onClick={() => handleDelete(team)}
                        aria-label={t("teams.deleteAria", { name: team.name })}
                      >
                        {t("common.delete")}
                      </button>
                    </div>
                  </div>

                  {/* Expanded detail */}
                  <Show when={expandedTeamId() === team.id}>
                    <div class="border-t border-gray-100 px-4 py-3 dark:border-gray-700">
                      {/* Members */}
                      <h4 class="mb-2 text-sm font-medium text-gray-600 dark:text-gray-400">
                        {t("teams.memberList")}
                      </h4>
                      <div class="mb-3 space-y-1">
                        <For each={team.members}>
                          {(m) => (
                            <div class="flex items-center gap-2 text-sm">
                              <span class={`rounded px-1.5 py-0.5 text-xs ${ROLE_COLORS[m.role]}`}>
                                {t(`teams.role.${m.role}`)}
                              </span>
                              <span class="text-gray-700 dark:text-gray-300">
                                {agentName(m.agent_id)}
                              </span>
                            </div>
                          )}
                        </For>
                      </div>

                      {/* Shared context */}
                      <Show when={sharedCtx()}>
                        {(ctx) => (
                          <div>
                            <h4 class="mb-2 text-sm font-medium text-gray-600 dark:text-gray-400">
                              {t("teams.sharedContext")}{" "}
                              <span class="text-xs font-normal text-gray-400">
                                v{(ctx() as SharedContext).version}
                              </span>
                            </h4>
                            <Show
                              when={(ctx() as SharedContext).items.length > 0}
                              fallback={
                                <p class="text-xs text-gray-400 dark:text-gray-500">
                                  {t("teams.noSharedContext")}
                                </p>
                              }
                            >
                              <div class="space-y-1">
                                <For each={(ctx() as SharedContext).items}>
                                  {(item) => (
                                    <div class="rounded bg-gray-50 px-3 py-1.5 text-xs dark:bg-gray-900">
                                      <span class="font-mono font-medium text-gray-600 dark:text-gray-400">
                                        {item.key}
                                      </span>
                                      <span class="ml-2 text-gray-500 dark:text-gray-500">
                                        by {item.author} ({item.tokens} tok)
                                      </span>
                                      <p class="mt-0.5 truncate text-gray-700 dark:text-gray-300">
                                        {item.value.slice(0, 200)}
                                      </p>
                                    </div>
                                  )}
                                </For>
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
      </Show>
    </div>
  );
}
