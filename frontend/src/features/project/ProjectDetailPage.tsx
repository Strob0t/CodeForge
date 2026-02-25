import { useParams } from "@solidjs/router";
import { createResource, createSignal, onCleanup, Show } from "solid-js";

import { api } from "~/api/client";
import type { BudgetAlertEvent } from "~/api/types";
import { createCodeForgeWS } from "~/api/websocket";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { Alert, Badge, Button } from "~/ui";

import ChatPanel from "./ChatPanel";
import CompactSettingsPopover from "./CompactSettingsPopover";
import RoadmapPanel from "./RoadmapPanel";

export default function ProjectDetailPage() {
  const { t, fmt } = useI18n();
  const { show: toast } = useToast();
  const params = useParams<{ id: string }>();
  const { onMessage } = createCodeForgeWS();

  const [project, { refetch: refetchProject }] = createResource(
    () => params.id,
    (id) => api.projects.get(id),
  );
  const [, { refetch: refetchTasks }] = createResource(
    () => params.id,
    (id) => api.tasks.list(id),
  );
  const [gitStatus, { refetch: refetchGitStatus }] = createResource(
    () => (project()?.workspace_path ? params.id : undefined),
    (id: string) => api.projects.gitStatus(id),
  );
  const [, { refetch: refetchBranches }] = createResource(
    () => (project()?.workspace_path ? params.id : undefined),
    (id: string) => api.projects.branches(id),
  );

  const [, { refetch: refetchAgents }] = createResource(
    () => params.id,
    (id) => api.agents.list(id),
  );

  const [cloning, setCloning] = createSignal(false);
  const [pulling, setPulling] = createSignal(false);
  const [error, setError] = createSignal("");
  const [budgetAlert, setBudgetAlert] = createSignal<BudgetAlertEvent | null>(null);
  const [settingsOpen, setSettingsOpen] = createSignal(false);

  // WebSocket event handling
  const cleanup = onMessage((msg) => {
    const payload = msg.payload;
    const projectId = params.id;

    switch (msg.type) {
      case "task.status": {
        const taskProjectId = payload.project_id as string;
        if (taskProjectId === projectId) {
          refetchTasks();
        }
        break;
      }
      case "agent.status": {
        const agentProjectId = payload.project_id as string;
        if (agentProjectId === projectId) {
          refetchAgents();
        }
        break;
      }
      case "run.status": {
        const runProjectId = payload.project_id as string;
        if (runProjectId === projectId) {
          const status = payload.status as string;
          if (status === "completed") {
            toast("info", t("detail.toast.runCompleted"));
          } else if (status === "failed") {
            toast("error", t("detail.toast.runFailed"));
          } else if (status === "cancelled") {
            toast("info", t("detail.toast.runCancelled"));
          }
        }
        break;
      }
      case "run.toolcall": {
        // Forward to RunPanel via WS
        break;
      }
      case "run.qualitygate": {
        const qgProjectId = payload.project_id as string;
        if (qgProjectId === projectId) {
          // Quality gate events are reflected via run.status updates
        }
        break;
      }
      case "run.delivery": {
        const delProjectId = payload.project_id as string;
        if (delProjectId === projectId) {
          // Delivery events are reflected via run.status updates
        }
        break;
      }
      case "plan.status": {
        const planProjectId = payload.project_id as string;
        if (planProjectId === projectId) {
          const status = payload.status as string;
          if (status === "completed") {
            toast("info", t("detail.toast.planCompleted"));
          } else if (status === "failed") {
            toast("error", t("detail.toast.planFailed"));
          }
        }
        break;
      }
      case "plan.step.status": {
        const stepProjectId = payload.project_id as string;
        if (stepProjectId === projectId) {
          // PlanPanel will refetch via its own resource
        }
        break;
      }
      case "repomap.status": {
        const rmProjectId = payload.project_id as string;
        if (rmProjectId === projectId) {
          // RepoMapPanel will refetch via its own resource
        }
        break;
      }
      case "retrieval.status": {
        const retProjectId = payload.project_id as string;
        if (retProjectId === projectId) {
          // RetrievalPanel handles its own state
        }
        break;
      }
      case "roadmap.status": {
        const rmProjectId = payload.project_id as string;
        if (rmProjectId === projectId) {
          // RoadmapPanel will refetch via its own resource
        }
        break;
      }
      case "run.budget_alert": {
        const alertProjectId = payload.project_id as string;
        if (alertProjectId === projectId) {
          setBudgetAlert(payload as unknown as BudgetAlertEvent);
          const pct = (payload as unknown as BudgetAlertEvent).percentage;
          toast("warning", t("detail.toast.budgetAlert", { pct: fmt.percent(pct) }));
        }
        break;
      }
      case "task.output": {
        // Task output handled by individual panels
        break;
      }
    }
  });
  onCleanup(cleanup);

  const handleClone = async () => {
    setCloning(true);
    setError("");
    try {
      await api.projects.clone(params.id);
      refetchProject();
      refetchGitStatus();
      refetchBranches();
      toast("success", t("detail.toast.cloned"));
    } catch (e) {
      const msg = e instanceof Error ? e.message : t("detail.toast.cloneFailed");
      setError(msg);
      toast("error", msg);
    } finally {
      setCloning(false);
    }
  };

  const handlePull = async () => {
    setPulling(true);
    setError("");
    try {
      await api.projects.pull(params.id);
      refetchGitStatus();
      toast("success", t("detail.toast.pulled"));
    } catch (e) {
      const msg = e instanceof Error ? e.message : t("detail.toast.pullFailed");
      setError(msg);
      toast("error", msg);
    } finally {
      setPulling(false);
    }
  };

  return (
    <div class="flex flex-col h-[calc(100vh-4rem)]">
      <Show
        when={project()}
        fallback={
          <Show
            when={project.error}
            fallback={<p class="text-cf-text-tertiary p-4">{t("detail.loading")}</p>}
          >
            <div class="flex flex-col items-center justify-center py-20 text-center">
              <h2 class="mb-2 text-xl font-bold text-cf-text-primary">
                {t("notFound.projectTitle")}
              </h2>
              <p class="mb-6 text-cf-text-tertiary">{t("notFound.projectMessage")}</p>
              <Button variant="primary" onClick={() => window.location.assign("/")}>
                {t("notFound.backToDashboard")}
              </Button>
            </div>
          </Show>
        }
      >
        {(p) => (
          <>
            {/* Header Bar */}
            <div class="flex items-center justify-between px-4 py-3 border-b border-cf-border flex-shrink-0">
              <div class="flex items-center gap-3">
                <h2 class="text-lg font-bold text-cf-text-primary">{p().name}</h2>

                {/* Git Status Badge */}
                <Show when={gitStatus()}>
                  {(gs) => (
                    <Badge variant={gs().dirty ? "warning" : "success"} pill>
                      <span class="font-mono">{gs().branch}</span>{" "}
                      <span>{gs().dirty ? t("detail.dirty") : t("detail.clean")}</span>
                    </Badge>
                  )}
                </Show>
              </div>

              <div class="flex items-center gap-2">
                {/* Clone Button */}
                <Show when={!p().workspace_path && p().repo_url}>
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={handleClone}
                    disabled={cloning()}
                    loading={cloning()}
                    aria-label={t("detail.cloneAria")}
                  >
                    {cloning() ? t("detail.cloning") : t("detail.cloneRepo")}
                  </Button>
                </Show>

                {/* Pull Button */}
                <Show when={p().workspace_path}>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={handlePull}
                    disabled={pulling()}
                    loading={pulling()}
                    aria-label={t("detail.pullAria")}
                  >
                    {pulling() ? t("detail.pulling") : t("detail.pull")}
                  </Button>
                </Show>

                {/* Settings Gear Icon */}
                <div class="relative">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setSettingsOpen(!settingsOpen())}
                    aria-label={t("detail.settings.gearTooltip")}
                    title={t("detail.settings.gearTooltip")}
                  >
                    <svg
                      class="h-5 w-5"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                      stroke-width="1.5"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.325.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 0 1 1.37.49l1.296 2.247a1.125 1.125 0 0 1-.26 1.431l-1.003.827c-.293.241-.438.613-.43.992a7.723 7.723 0 0 1 0 .255c-.008.378.137.75.43.991l1.004.827c.424.35.534.955.26 1.43l-1.298 2.247a1.125 1.125 0 0 1-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.47 6.47 0 0 1-.22.128c-.331.183-.581.495-.644.869l-.213 1.281c-.09.543-.56.94-1.11.94h-2.594c-.55 0-1.019-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 0 1-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 0 1-1.369-.49l-1.297-2.247a1.125 1.125 0 0 1 .26-1.431l1.004-.827c.292-.24.437-.613.43-.991a6.932 6.932 0 0 1 0-.255c.007-.38-.138-.751-.43-.992l-1.004-.827a1.125 1.125 0 0 1-.26-1.43l1.297-2.247a1.125 1.125 0 0 1 1.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.086.22-.128.332-.183.582-.495.644-.869l.214-1.28Z"
                      />
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        d="M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z"
                      />
                    </svg>
                  </Button>
                  <CompactSettingsPopover
                    projectId={params.id}
                    config={p().config ?? {}}
                    open={settingsOpen()}
                    onClose={() => setSettingsOpen(false)}
                    onSaved={() => {
                      refetchProject();
                      setSettingsOpen(false);
                    }}
                  />
                </div>
              </div>
            </div>

            {/* Error Banner */}
            <Show when={error()}>
              <div class="mx-4 mt-2 flex-shrink-0">
                <Alert variant="error" onDismiss={() => setError("")}>
                  {error()}
                </Alert>
              </div>
            </Show>

            {/* Budget Alert Banner */}
            <Show when={budgetAlert()}>
              {(alert) => (
                <div class="mx-4 mt-2 flex-shrink-0">
                  <Alert variant="warning" onDismiss={() => setBudgetAlert(null)}>
                    {t("detail.budgetAlert", {
                      runId: alert().run_id.slice(0, 8),
                      pct: fmt.percent(alert().percentage),
                      cost: fmt.currency(alert().cost_usd),
                      max: fmt.currency(alert().max_cost),
                    })}
                  </Alert>
                </div>
              )}
            </Show>

            {/* Side-by-side Layout: Roadmap (left) | Chat (right) */}
            <div class="flex flex-1 min-h-0">
              <div class="w-1/2 border-r border-cf-border overflow-y-auto p-4">
                <RoadmapPanel projectId={params.id} onError={setError} />
              </div>
              <div class="w-1/2 flex flex-col min-h-0">
                <ChatPanel projectId={params.id} />
              </div>
            </div>
          </>
        )}
      </Show>
    </div>
  );
}
