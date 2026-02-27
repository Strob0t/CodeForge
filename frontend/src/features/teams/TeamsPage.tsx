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
import { Badge, Button, Card, EmptyState, Input, PageLayout, Select } from "~/ui";
import type { BadgeVariant } from "~/ui/primitives/Badge";

const TEAM_STATUS_VARIANTS: Record<TeamStatus, BadgeVariant> = {
  initializing: "default",
  active: "success",
  completed: "info",
  failed: "danger",
};

const ROLE_VARIANTS: Record<TeamRole, BadgeVariant> = {
  coder: "info",
  reviewer: "primary",
  tester: "success",
  documenter: "warning",
  planner: "danger",
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
  const [agents, { refetch: refetchAgents }] = createResource(
    () => selectedProjectId(),
    (pid) => (pid ? api.agents.list(pid) : []),
  );
  const [backends] = createResource(() => api.providers.agent());

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

  // Inline agent creation (when no agents exist yet)
  const [newAgentName, setNewAgentName] = createSignal("");
  const [newAgentBackend, setNewAgentBackend] = createSignal("");
  const [creatingAgent, setCreatingAgent] = createSignal(false);

  const handleCreateAgent = async () => {
    const pid = selectedProjectId();
    if (!pid || !newAgentName().trim() || !newAgentBackend()) return;
    setCreatingAgent(true);
    try {
      await api.agents.create(pid, { name: newAgentName().trim(), backend: newAgentBackend() });
      toast("success", t("agent.toast.created"));
      setNewAgentName("");
      setNewAgentBackend("");
      refetchAgents();
    } catch (e) {
      toast("error", e instanceof Error ? e.message : t("agent.toast.createFailed"));
    } finally {
      setCreatingAgent(false);
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
    <PageLayout title={t("teams.title")}>
      {/* Project selector */}
      <div class="mb-4">
        <Select
          value={selectedProjectId()}
          aria-label={t("teams.selectProject")}
          onChange={(e) => {
            setSelectedProjectId(e.currentTarget.value);
            setExpandedTeamId(null);
          }}
          class="max-w-xs"
        >
          <option value="">{t("teams.selectProject")}</option>
          <For each={projects() ?? []}>
            {(p: Project) => <option value={p.id}>{p.name}</option>}
          </For>
        </Select>
      </div>

      <Show when={selectedProjectId()}>
        {/* Create Team form */}
        <Card class="mb-6">
          <Card.Header>
            <h3 class="text-lg font-semibold text-cf-text-primary">{t("teams.createTeam")}</h3>
          </Card.Header>
          <Card.Body>
            <div class="mb-3 flex flex-wrap gap-2">
              <Input
                type="text"
                placeholder={t("teams.form.namePlaceholder")}
                value={formName()}
                onInput={(e) => setFormName(e.currentTarget.value)}
                aria-label={t("teams.form.name")}
                class="w-auto"
              />
              <Select
                value={formProtocol()}
                aria-label={t("teams.form.protocol")}
                onChange={(e) => setFormProtocol(e.currentTarget.value)}
                class="w-auto"
              >
                <For each={PROTOCOLS}>{(p) => <option value={p}>{p}</option>}</For>
              </Select>
            </div>

            {/* Members */}
            <div class="mb-3">
              <div class="mb-1 flex items-center gap-2">
                <span class="text-sm font-medium text-cf-text-tertiary">
                  {t("teams.form.members")}
                </span>
                <Button variant="secondary" size="sm" onClick={addMember}>
                  {t("teams.form.addMember")}
                </Button>
              </div>
              <Show when={(agents() ?? []).length === 0 && !agents.loading}>
                <div class="mb-2 rounded-cf-sm border border-cf-border bg-cf-bg-surface-alt p-3 text-sm">
                  <p class="mb-2 text-cf-text-muted">{t("teams.noAgentsHint")}</p>
                  <div class="flex flex-wrap items-end gap-2">
                    <Input
                      type="text"
                      value={newAgentName()}
                      onInput={(e) => setNewAgentName(e.currentTarget.value)}
                      placeholder={t("agent.form.namePlaceholder")}
                      aria-label={t("agent.form.name")}
                      class="w-auto"
                    />
                    <Select
                      value={newAgentBackend()}
                      onChange={(e) => setNewAgentBackend(e.currentTarget.value)}
                      aria-label={t("agent.form.backend")}
                      class="w-auto"
                    >
                      <option value="">{t("agent.form.backendPlaceholder")}</option>
                      <For each={backends()?.backends ?? []}>
                        {(b) => <option value={b}>{b}</option>}
                      </For>
                    </Select>
                    <Button
                      variant="primary"
                      size="sm"
                      onClick={handleCreateAgent}
                      loading={creatingAgent()}
                    >
                      {t("teams.createAgent")}
                    </Button>
                  </div>
                </div>
              </Show>
              <div class="space-y-1">
                <For each={formMembers()}>
                  {(member, idx) => (
                    <div class="flex items-center gap-2">
                      <Select
                        value={member.agent_id}
                        aria-label={t("teams.form.selectAgent")}
                        onChange={(e) => updateMember(idx(), "agent_id", e.currentTarget.value)}
                        class="flex-1"
                      >
                        <option value="">{t("teams.form.selectAgent")}</option>
                        <For each={agents() ?? []}>
                          {(a: Agent) => (
                            <option value={a.id}>
                              {a.name} ({a.backend})
                            </option>
                          )}
                        </For>
                      </Select>
                      <Select
                        value={member.role}
                        aria-label={t("teams.form.selectRole")}
                        onChange={(e) => updateMember(idx(), "role", e.currentTarget.value)}
                        class="w-auto"
                      >
                        <For each={ROLES}>
                          {(r) => <option value={r}>{t(`teams.role.${r}`)}</option>}
                        </For>
                      </Select>
                      <Button
                        variant="danger"
                        size="sm"
                        onClick={() => removeMember(idx())}
                        aria-label={t("teams.form.removeMemberAria")}
                      >
                        {t("common.delete")}
                      </Button>
                    </div>
                  )}
                </For>
              </div>
            </div>

            <Button onClick={handleCreate} loading={creating()}>
              {creating() ? t("common.creating") : t("teams.form.create")}
            </Button>
          </Card.Body>
        </Card>

        {/* Teams list */}
        <Show
          when={(teams() ?? []).length > 0}
          fallback={
            <Card>
              <Card.Body>
                <EmptyState title={t("teams.empty")} />
              </Card.Body>
            </Card>
          }
        >
          <div class="space-y-3">
            <For each={teams() ?? []}>
              {(team: AgentTeam) => (
                <Card>
                  {/* Team header */}
                  <div class="flex items-center justify-between px-4 py-3">
                    <div class="flex items-center gap-3">
                      <button
                        type="button"
                        class="text-left font-medium text-cf-text-primary hover:text-cf-accent"
                        onClick={() =>
                          setExpandedTeamId((prev) => (prev === team.id ? null : team.id))
                        }
                        aria-expanded={expandedTeamId() === team.id}
                      >
                        {team.name}
                      </button>
                      <Badge variant={TEAM_STATUS_VARIANTS[team.status]} pill>
                        {team.status}
                      </Badge>
                      <Badge variant="default">{team.protocol}</Badge>
                    </div>
                    <div class="flex items-center gap-2">
                      <span class="text-xs text-cf-text-muted">
                        {t("teams.members", { count: String(team.members.length) })}
                      </span>
                      <span class="text-xs text-cf-text-muted">{fmt.date(team.created_at)}</span>
                      <Button
                        variant="danger"
                        size="sm"
                        onClick={() => handleDelete(team)}
                        aria-label={t("teams.deleteAria", { name: team.name })}
                      >
                        {t("common.delete")}
                      </Button>
                    </div>
                  </div>

                  {/* Expanded detail */}
                  <Show when={expandedTeamId() === team.id}>
                    <div class="border-t border-cf-border px-4 py-3">
                      {/* Members */}
                      <h4 class="mb-2 text-sm font-medium text-cf-text-tertiary">
                        {t("teams.memberList")}
                      </h4>
                      <div class="mb-3 space-y-1">
                        <For each={team.members}>
                          {(m) => (
                            <div class="flex items-center gap-2 text-sm">
                              <Badge variant={ROLE_VARIANTS[m.role]} pill>
                                {t(`teams.role.${m.role}`)}
                              </Badge>
                              <span class="text-cf-text-secondary">{agentName(m.agent_id)}</span>
                            </div>
                          )}
                        </For>
                      </div>

                      {/* Shared context */}
                      <Show when={sharedCtx()}>
                        {(ctx) => {
                          const sc = () => ctx() as SharedContext | null;
                          return (
                            <Show when={sc()} keyed>
                              {(resolved) => (
                                <div>
                                  <h4 class="mb-2 text-sm font-medium text-cf-text-tertiary">
                                    {t("teams.sharedContext")}{" "}
                                    <span class="text-xs font-normal text-cf-text-muted">
                                      v{resolved.version}
                                    </span>
                                  </h4>
                                  <Show
                                    when={(resolved.items?.length ?? 0) > 0}
                                    fallback={
                                      <p class="text-xs text-cf-text-muted">
                                        {t("teams.noSharedContext")}
                                      </p>
                                    }
                                  >
                                    <div class="space-y-1">
                                      <For each={resolved.items}>
                                        {(item) => (
                                          <div class="rounded bg-cf-bg-surface-alt px-3 py-1.5 text-xs">
                                            <span class="font-mono font-medium text-cf-text-tertiary">
                                              {item.key}
                                            </span>
                                            <span class="ml-2 text-cf-text-muted">
                                              by {item.author} ({item.tokens} tok)
                                            </span>
                                            <p class="mt-0.5 truncate text-cf-text-secondary">
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
                          );
                        }}
                      </Show>
                    </div>
                  </Show>
                </Card>
              )}
            </For>
          </div>
        </Show>
      </Show>
    </PageLayout>
  );
}
