import { createResource, createSignal, For, onCleanup, Show } from "solid-js";
import { useParams } from "@solidjs/router";
import { api } from "~/api/client";
import { createCodeForgeWS } from "~/api/websocket";
import type { Branch, GitStatus } from "~/api/types";
import AgentPanel from "./AgentPanel";
import TaskPanel from "./TaskPanel";
import LiveOutput from "./LiveOutput";
import type { OutputLine } from "./LiveOutput";

export default function ProjectDetailPage() {
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

  const [cloning, setCloning] = createSignal(false);
  const [pulling, setPulling] = createSignal(false);
  const [error, setError] = createSignal("");
  const [outputLines, setOutputLines] = createSignal<OutputLine[]>([]);
  const [activeTaskId, setActiveTaskId] = createSignal<string | null>(null);

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
          // AgentPanel will refetch on its own via its resource
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
    } catch (e) {
      setError(e instanceof Error ? e.message : "Clone failed");
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
    } catch (e) {
      setError(e instanceof Error ? e.message : "Pull failed");
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
    } catch (e) {
      setError(e instanceof Error ? e.message : "Checkout failed");
    }
  };

  return (
    <div>
      <Show when={project()} fallback={<p class="text-gray-500">Loading...</p>}>
        {(p) => (
          <>
            <div class="mb-6">
              <h2 class="text-2xl font-bold">{p().name}</h2>
              <p class="mt-1 text-gray-500">{p().description || "No description"}</p>
              <div class="mt-2 flex gap-4 text-sm text-gray-400">
                <span>Provider: {p().provider}</span>
                <Show when={p().repo_url}>
                  <span>Repo: {p().repo_url}</span>
                </Show>
              </div>
            </div>

            <Show when={error()}>
              <div class="mb-4 rounded bg-red-50 p-3 text-sm text-red-600">{error()}</div>
            </Show>

            {/* Git Section */}
            <div class="mb-6 rounded-lg border border-gray-200 bg-white p-4">
              <h3 class="mb-3 text-lg font-semibold">Git</h3>

              <Show
                when={p().workspace_path}
                fallback={
                  <div>
                    <p class="mb-2 text-sm text-gray-500">Repository not cloned yet.</p>
                    <Show when={p().repo_url}>
                      <button
                        class="rounded bg-blue-600 px-4 py-2 text-sm text-white hover:bg-blue-700 disabled:opacity-50"
                        onClick={handleClone}
                        disabled={cloning()}
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
                    <div class="mb-4 grid grid-cols-2 gap-4 text-sm">
                      <div>
                        <span class="text-gray-500">Branch:</span>{" "}
                        <span class="font-mono font-medium">{gs().branch}</span>
                      </div>
                      <div>
                        <span class="text-gray-500">Status:</span>{" "}
                        <span class={gs().dirty ? "text-yellow-600" : "text-green-600"}>
                          {gs().dirty ? "dirty" : "clean"}
                        </span>
                      </div>
                      <div class="col-span-2">
                        <span class="text-gray-500">Last commit:</span>{" "}
                        <span class="font-mono text-xs">{gs().commit_hash.slice(0, 8)}</span>{" "}
                        {gs().commit_message}
                      </div>
                      <Show when={gs().ahead > 0 || gs().behind > 0}>
                        <div>
                          <span class="text-gray-500">Ahead:</span> {gs().ahead}{" "}
                          <span class="text-gray-500">Behind:</span> {gs().behind}
                        </div>
                      </Show>
                    </div>
                  )}
                </Show>

                {/* Git Actions */}
                <div class="flex gap-2">
                  <button
                    class="rounded bg-gray-100 px-3 py-1.5 text-sm hover:bg-gray-200 disabled:opacity-50"
                    onClick={handlePull}
                    disabled={pulling()}
                  >
                    {pulling() ? "Pulling..." : "Pull"}
                  </button>
                  <button
                    class="rounded bg-gray-100 px-3 py-1.5 text-sm hover:bg-gray-200"
                    onClick={() => refetchGitStatus()}
                  >
                    Refresh
                  </button>
                </div>

                {/* Branches */}
                <Show when={branches() && branches()!.length > 0}>
                  <div class="mt-4">
                    <h4 class="mb-2 text-sm font-medium text-gray-500">Branches</h4>
                    <div class="flex flex-wrap gap-2">
                      <For each={branches()!}>
                        {(b) => (
                          <button
                            class={`rounded px-2 py-1 text-xs ${
                              b.current
                                ? "bg-blue-100 text-blue-700"
                                : "bg-gray-100 text-gray-600 hover:bg-gray-200"
                            }`}
                            onClick={() => !b.current && handleCheckout(b.name)}
                            disabled={b.current}
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

            {/* Agents Section */}
            <div class="mb-6">
              <AgentPanel projectId={params.id} tasks={tasks() ?? []} onError={setError} />
            </div>

            {/* Live Output Section */}
            <Show when={outputLines().length > 0 || activeTaskId()}>
              <div class="mb-6">
                <LiveOutput taskId={activeTaskId()} lines={outputLines()} />
              </div>
            </Show>

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
