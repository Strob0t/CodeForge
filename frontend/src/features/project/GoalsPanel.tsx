import { createMemo, createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { CreateGoalRequest, GoalKind, ProjectGoal } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { Badge, Button } from "~/ui";

interface Props {
  projectId: string;
  onAIDiscoverStarted?: (conversationId: string) => void;
}

const KIND_ORDER: GoalKind[] = ["vision", "requirement", "constraint", "state", "context"];

const KIND_LABELS: Record<GoalKind, string> = {
  vision: "Vision",
  requirement: "Requirements",
  constraint: "Constraints & Decisions",
  state: "Current State",
  context: "Context",
};

const KIND_COLORS: Record<GoalKind, "success" | "info" | "warning" | "neutral" | "error"> = {
  vision: "success",
  requirement: "info",
  constraint: "warning",
  state: "neutral",
  context: "neutral",
};

export default function GoalsPanel(props: Props) {
  const { t } = useI18n();
  const { show: toast } = useToast();

  const [goals, { refetch }] = createResource(
    () => props.projectId,
    async (id) => {
      try {
        return await api.goals.list(id);
      } catch {
        return [];
      }
    },
  );

  const [detecting, setDetecting] = createSignal(false);
  const [aiDiscovering, setAiDiscovering] = createSignal(false);
  const [showForm, setShowForm] = createSignal(false);
  const [formKind, setFormKind] = createSignal<GoalKind>("vision");
  const [formTitle, setFormTitle] = createSignal("");
  const [formContent, setFormContent] = createSignal("");

  const groupedGoals = createMemo(() => {
    const all = goals() ?? [];
    const groups: Partial<Record<GoalKind, ProjectGoal[]>> = {};
    for (const g of all) {
      (groups[g.kind] ??= []).push(g);
    }
    return groups;
  });

  const handleDetect = async () => {
    setDetecting(true);
    try {
      const result = await api.goals.detect(props.projectId);
      toast("success", t("goals.toast.detected", { count: String(result.imported) }));
      refetch();
    } catch {
      toast("error", t("goals.toast.detectFailed"));
    } finally {
      setDetecting(false);
    }
  };

  const handleAIDiscover = async () => {
    setAiDiscovering(true);
    try {
      const result = await api.goals.aiDiscover(props.projectId);
      toast("success", t("goals.aiDiscoverStarted"));
      props.onAIDiscoverStarted?.(result.conversation_id);
    } catch {
      toast("error", t("goals.toast.aiDiscoverFailed"));
    } finally {
      setAiDiscovering(false);
    }
  };

  const handleCreate = async () => {
    if (!formTitle().trim()) {
      toast("error", t("goals.toast.titleRequired"));
      return;
    }
    try {
      const req: CreateGoalRequest = {
        kind: formKind(),
        title: formTitle().trim(),
        content: formContent().trim(),
      };
      await api.goals.create(props.projectId, req);
      toast("success", t("goals.toast.created"));
      setShowForm(false);
      setFormTitle("");
      setFormContent("");
      refetch();
    } catch {
      toast("error", t("goals.toast.createFailed"));
    }
  };

  const handleToggle = async (goal: ProjectGoal) => {
    try {
      await api.goals.update(goal.id, { enabled: !goal.enabled });
      refetch();
    } catch {
      toast("error", t("goals.toast.updateFailed"));
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await api.goals.delete(id);
      toast("info", t("goals.toast.deleted"));
      refetch();
    } catch {
      toast("error", t("goals.toast.deleteFailed"));
    }
  };

  return (
    <div class="flex flex-col h-full">
      <div class="flex items-center justify-between px-4 py-3 flex-shrink-0">
        <h3 class="text-sm font-semibold text-cf-text-primary">{t("goals.title")}</h3>
        <div class="flex items-center gap-2">
          <Button
            variant="secondary"
            size="sm"
            onClick={handleAIDiscover}
            disabled={aiDiscovering()}
            loading={aiDiscovering()}
          >
            {aiDiscovering() ? t("goals.aiDiscovering") : t("goals.aiDiscover")}
          </Button>
          <Button
            variant="secondary"
            size="sm"
            onClick={handleDetect}
            disabled={detecting()}
            loading={detecting()}
          >
            {detecting() ? t("goals.detecting") : t("goals.detect")}
          </Button>
          <Button variant="primary" size="sm" onClick={() => setShowForm(!showForm())}>
            {showForm() ? t("common.cancel") : t("goals.add")}
          </Button>
        </div>
      </div>

      {/* Create Form */}
      <Show when={showForm()}>
        <div class="mx-4 mb-3 p-3 border border-cf-border rounded-cf-sm bg-cf-bg-secondary space-y-2">
          <div class="flex gap-2">
            <select
              class="rounded-cf-sm border border-cf-border bg-cf-bg-primary text-cf-text-primary text-sm px-2 py-1"
              value={formKind()}
              onChange={(e) => setFormKind(e.target.value as GoalKind)}
            >
              <For each={KIND_ORDER}>{(k) => <option value={k}>{KIND_LABELS[k]}</option>}</For>
            </select>
            <input
              type="text"
              class="flex-1 rounded-cf-sm border border-cf-border bg-cf-bg-primary text-cf-text-primary text-sm px-2 py-1"
              placeholder={t("goals.form.titlePlaceholder")}
              value={formTitle()}
              onInput={(e) => setFormTitle(e.target.value)}
            />
          </div>
          <textarea
            class="w-full rounded-cf-sm border border-cf-border bg-cf-bg-primary text-cf-text-primary text-sm px-2 py-1 min-h-[60px]"
            placeholder={t("goals.form.contentPlaceholder")}
            value={formContent()}
            onInput={(e) => setFormContent(e.target.value)}
          />
          <div class="flex justify-end">
            <Button variant="primary" size="sm" onClick={handleCreate}>
              {t("common.create")}
            </Button>
          </div>
        </div>
      </Show>

      {/* Goals List */}
      <div class="flex-1 overflow-y-auto px-4 pb-4 space-y-4">
        <Show
          when={(goals() ?? []).length > 0}
          fallback={
            <p class="text-sm text-cf-text-tertiary py-4 text-center">{t("goals.empty")}</p>
          }
        >
          <For each={KIND_ORDER}>
            {(kind) => {
              const kindGoals = () => groupedGoals()[kind] ?? [];
              return (
                <Show when={kindGoals().length > 0}>
                  <div>
                    <h4 class="text-xs font-semibold uppercase tracking-wider text-cf-text-tertiary mb-2">
                      {KIND_LABELS[kind]}
                    </h4>
                    <div class="space-y-1.5">
                      <For each={kindGoals()}>
                        {(goal) => (
                          <div
                            class={`rounded-cf-sm border px-3 py-2 text-sm ${
                              goal.enabled
                                ? "border-cf-border bg-cf-bg-primary"
                                : "border-cf-border-subtle bg-cf-bg-secondary opacity-60"
                            }`}
                          >
                            <div class="flex items-start justify-between gap-2">
                              <div class="min-w-0 flex-1">
                                <div class="flex items-center gap-2 mb-1">
                                  <span class="font-medium text-cf-text-primary truncate">
                                    {goal.title}
                                  </span>
                                  <Show when={goal.source !== "manual"}>
                                    <Badge variant={KIND_COLORS[goal.kind]} pill>
                                      {goal.source}
                                      {goal.source_path ? `: ${goal.source_path}` : ""}
                                    </Badge>
                                  </Show>
                                </div>
                                <Show when={goal.content}>
                                  <p class="text-xs text-cf-text-secondary line-clamp-3 whitespace-pre-wrap">
                                    {goal.content.length > 300
                                      ? goal.content.slice(0, 300) + "..."
                                      : goal.content}
                                  </p>
                                </Show>
                              </div>
                              <div class="flex items-center gap-1 flex-shrink-0">
                                <Button
                                  variant="secondary"
                                  size="xs"
                                  class={goal.enabled ? "text-cf-success" : ""}
                                  onClick={() => handleToggle(goal)}
                                  title={goal.enabled ? t("goals.disable") : t("goals.enable")}
                                >
                                  {goal.enabled ? "ON" : "OFF"}
                                </Button>
                                <Button
                                  variant="icon"
                                  size="xs"
                                  onClick={() => handleDelete(goal.id)}
                                  title={t("common.delete")}
                                >
                                  x
                                </Button>
                              </div>
                            </div>
                          </div>
                        )}
                      </For>
                    </div>
                  </div>
                </Show>
              );
            }}
          </For>
        </Show>
      </div>
    </div>
  );
}
