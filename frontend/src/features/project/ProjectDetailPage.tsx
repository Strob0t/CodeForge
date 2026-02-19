import { useParams } from "@solidjs/router";
import { createResource, createSignal, For, onCleanup, Show } from "solid-js";

import { api } from "~/api/client";
import type { Branch, BudgetAlertEvent, GitStatus } from "~/api/types";
import { createCodeForgeWS } from "~/api/websocket";
import { useToast } from "~/components/Toast";

import { ProjectCostSection } from "../costs/CostDashboardPage";
import AgentPanel from "./AgentPanel";
import type { OutputLine } from "./LiveOutput";
import LiveOutput from "./LiveOutput";
import PlanPanel from "./PlanPanel";
import PolicyPanel from "./PolicyPanel";
import RepoMapPanel from "./RepoMapPanel";
import RetrievalPanel from "./RetrievalPanel";
import RoadmapPanel from "./RoadmapPanel";
import RunPanel from "./RunPanel";
import TaskPanel from "./TaskPanel";

export default function ProjectDetailPage() {
  const { show: toast } = useToast();
  const params = useParams<{ id: string }>();
  const { onMessage } = createCodeForgeWS();

  const [project, { refetch: refetchProject }] = createResource(
    () => params.id,
    (id) => api.projects.get(id),
  );
  const [tasks, { refetch: refetchTasks }] = createResource(
    () => params.id,
    (id) => api.tasks.list(id),
  );
  const [gitStatus, { refetch: refetchGitStatus }] = createResource<GitStatus | null>(
    () => (project()?.workspace_path ? params.id : null),
    (id) => (id ? api.projects.gitStatus(id) : null),
  );
  const [branches, { refetch: refetchBranches }] = createResource<Branch[] | null>(
    () => (project()?.workspace_path ? params.id : null),
    (id) => (id ? api.projects.branches(id) : null),
  );

  const [agents, { refetch: refetchAgents }] = createResource(
    () => params.id,
    (id) => api.agents.list(id),
  );

  const [cloning, setCloning] = createSignal(false);
  const [pulling, setPulling] = createSignal(false);
  const [error, setError] = createSignal("");
  const [outputLines, setOutputLines] = createSignal<OutputLine[]>([]);
  const [activeTaskId, setActiveTaskId] = createSignal<string | null>(null);
  const [budgetAlert, setBudgetAlert] = createSignal<BudgetAlertEvent | null>(null);

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
            toast("info", "Run completed successfully");
          } else if (status === "failed") {
            toast("error", "Run failed");
          } else if (status === "cancelled") {
            toast("info", "Run cancelled");
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
            toast("info", "Plan completed");
          } else if (status === "failed") {
            toast("error", "Plan failed");
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
          toast("warning", `Budget alert: ${pct.toFixed(0)}% of limit reached`);
        }
        break;
      }
      case "task.output": {
        const taskId = payload.task_id as string;
        setActiveTaskId(taskId);
        setOutputLines((prev) => [
          ...prev,
          {
            line: payload.line as string,
            stream: (payload.stream as "stdout" | "stderr") || "stdout",
            timestamp: Date.now(),
          },
        ]);
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
      toast("success", "Repository cloned");
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Clone failed";
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
      toast("success", "Pull completed");
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Pull failed";
      setError(msg);
      toast("error", msg);
    } finally {
      setPulling(false);
    }
  };

  const handleCheckout = async (branch: string) => {
    setError("");
    try {
      await api.projects.checkout(params.id, branch);
      refetchGitStatus();
      refetchBranches();
      toast("success", `Switched to branch ${branch}`);
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Checkout failed";
      setError(msg);
      toast("error", msg);
    }
  };

  return (
    <div>
      <Show when={project()} fallback={<p class="text-gray-500 dark:text-gray-400">Loading...</p>}>
        {(p) => (
          <>
            <div class="mb-6">
              <h2 class="text-2xl font-bold">{p().name}</h2>
              <p class="mt-1 text-gray-500 dark:text-gray-400">
                {p().description || "No description"}
              </p>
              <div class="mt-2 flex gap-4 text-sm text-gray-400 dark:text-gray-500">
                <span>Provider: {p().provider}</span>
                <Show when={p().repo_url}>
                  <span>Repo: {p().repo_url}</span>
                </Show>
              </div>
            </div>

            <Show when={error()}>
              <div
                class="mb-4 rounded bg-red-50 dark:bg-red-900/20 p-3 text-sm text-red-600 dark:text-red-400"
                role="alert"
              >
                {error()}
              </div>
            </Show>

            {/* Budget Alert Banner */}
            <Show when={budgetAlert()}>
              {(alert) => (
                <div
                  class="mb-4 flex items-center justify-between rounded bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-700 p-3 text-sm text-yellow-800 dark:text-yellow-300"
                  role="alert"
                  aria-live="assertive"
                >
                  <span>
                    Budget alert: run {alert().run_id.slice(0, 8)} has reached{" "}
                    {alert().percentage.toFixed(0)}% of budget (${alert().cost_usd.toFixed(4)} / $
                    {alert().max_cost.toFixed(2)})
                  </span>
                  <button
                    type="button"
                    class="ml-4 text-yellow-600 dark:text-yellow-400 hover:text-yellow-800 dark:hover:text-yellow-300"
                    onClick={() => setBudgetAlert(null)}
                    aria-label="Dismiss budget alert"
                  >
                    Dismiss
                  </button>
                </div>
              )}
            </Show>

            {/* Git Section */}
            <div class="mb-6 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
              <h3 class="mb-3 text-lg font-semibold">Git</h3>

              <Show
                when={p().workspace_path}
                fallback={
                  <div>
                    <p class="mb-2 text-sm text-gray-500 dark:text-gray-400">
                      Repository not cloned yet.
                    </p>
                    <Show when={p().repo_url}>
                      <button
                        type="button"
                        class="rounded bg-blue-600 px-4 py-2 text-sm text-white hover:bg-blue-700 disabled:opacity-50"
                        onClick={handleClone}
                        disabled={cloning()}
                        aria-label="Clone repository"
                      >
                        {cloning() ? "Cloning..." : "Clone Repository"}
                      </button>
                    </Show>
                  </div>
                }
              >
                {/* Git Status */}
                <Show when={gitStatus()}>
                  {(gs) => (
                    <div class="mb-4 grid grid-cols-2 gap-4 text-sm" aria-live="polite">
                      <div>
                        <span class="text-gray-500 dark:text-gray-400">Branch:</span>{" "}
                        <span class="font-mono font-medium">{gs().branch}</span>
                      </div>
                      <div>
                        <span class="text-gray-500 dark:text-gray-400">Status:</span>{" "}
                        <span
                          class={
                            gs().dirty
                              ? "text-yellow-600 dark:text-yellow-400"
                              : "text-green-600 dark:text-green-400"
                          }
                        >
                          {gs().dirty ? "dirty" : "clean"}
                        </span>
                      </div>
                      <div class="col-span-2">
                        <span class="text-gray-500 dark:text-gray-400">Last commit:</span>{" "}
                        <span class="font-mono text-xs">{gs().commit_hash.slice(0, 8)}</span>{" "}
                        {gs().commit_message}
                      </div>
                      <Show when={gs().ahead > 0 || gs().behind > 0}>
                        <div>
                          <span class="text-gray-500 dark:text-gray-400">Ahead:</span> {gs().ahead}{" "}
                          <span class="text-gray-500 dark:text-gray-400">Behind:</span>{" "}
                          {gs().behind}
                        </div>
                      </Show>
                    </div>
                  )}
                </Show>

                {/* Git Actions */}
                <div class="flex gap-2">
                  <button
                    type="button"
                    class="rounded bg-gray-100 dark:bg-gray-700 px-3 py-1.5 text-sm hover:bg-gray-200 dark:hover:bg-gray-600 disabled:opacity-50"
                    onClick={handlePull}
                    disabled={pulling()}
                    aria-label="Pull latest changes from remote"
                  >
                    {pulling() ? "Pulling..." : "Pull"}
                  </button>
                  <button
                    type="button"
                    class="rounded bg-gray-100 dark:bg-gray-700 px-3 py-1.5 text-sm hover:bg-gray-200 dark:hover:bg-gray-600"
                    onClick={() => refetchGitStatus()}
                    aria-label="Refresh git status"
                  >
                    Refresh
                  </button>
                </div>

                {/* Branches */}
                <Show when={(branches() ?? []).length > 0}>
                  <div class="mt-4">
                    <h4 class="mb-2 text-sm font-medium text-gray-500 dark:text-gray-400">
                      Branches
                    </h4>
                    <div class="flex flex-wrap gap-2">
                      <For each={branches() ?? []}>
                        {(b) => (
                          <button
                            type="button"
                            class={`rounded px-2 py-1 text-xs ${
                              b.current
                                ? "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400"
                                : "bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-gray-700 dark:text-gray-400 dark:hover:bg-gray-600"
                            }`}
                            onClick={() => !b.current && handleCheckout(b.name)}
                            disabled={b.current}
                            aria-label={
                              b.current ? `Current branch: ${b.name}` : `Switch to branch ${b.name}`
                            }
                            aria-current={b.current ? "true" : undefined}
                          >
                            {b.name}
                            {b.current ? " (current)" : ""}
                          </button>
                        )}
                      </For>
                    </div>
                  </div>
                </Show>
              </Show>
            </div>

            {/* Repo Map Section */}
            <Show when={p().workspace_path}>
              <div class="mb-6">
                <RepoMapPanel projectId={params.id} />
              </div>
              <div class="mb-6">
                <RetrievalPanel projectId={params.id} />
              </div>
            </Show>

            {/* Roadmap Section */}
            <div class="mb-6">
              <RoadmapPanel projectId={params.id} onError={setError} />
            </div>

            {/* Agents Section */}
            <div class="mb-6">
              <AgentPanel projectId={params.id} tasks={tasks() ?? []} onError={setError} />
            </div>

            {/* Policy Section */}
            <div class="mb-6">
              <PolicyPanel projectId={params.id} onError={setError} />
            </div>

            {/* Run Management Section */}
            <div class="mb-6">
              <RunPanel
                projectId={params.id}
                tasks={tasks() ?? []}
                agents={agents() ?? []}
                onError={setError}
              />
            </div>

            {/* Execution Plans Section */}
            <div class="mb-6">
              <PlanPanel
                projectId={params.id}
                tasks={tasks() ?? []}
                agents={agents() ?? []}
                onError={setError}
              />
            </div>

            {/* Live Output Section */}
            <Show when={outputLines().length > 0 || activeTaskId()}>
              <div class="mb-6">
                <LiveOutput taskId={activeTaskId()} lines={outputLines()} />
              </div>
            </Show>

            {/* Cost Section */}
            <div class="mb-6">
              <ProjectCostSection projectId={params.id} />
            </div>

            {/* Tasks Section */}
            <TaskPanel
              projectId={params.id}
              tasks={tasks() ?? []}
              onRefetch={() => refetchTasks()}
              onError={setError}
            />
          </>
        )}
      </Show>
    </div>
  );
}
