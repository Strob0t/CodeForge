import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type {
  FeatureStatus,
  ImportResult,
  Milestone,
  ProviderInfo,
  Roadmap,
  RoadmapFeature,
  RoadmapStatus,
} from "~/api/types";
import { useToast } from "~/components/Toast";

interface RoadmapPanelProps {
  projectId: string;
  onError: (msg: string) => void;
}

const STATUS_COLORS: Record<RoadmapStatus, string> = {
  draft: "bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300",
  active: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
  complete: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
  archived: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
};

const FEATURE_COLORS: Record<FeatureStatus, string> = {
  backlog: "bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400",
  planned: "bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400",
  in_progress: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
  done: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
  cancelled: "bg-red-100 text-red-600 dark:bg-red-900/30 dark:text-red-400",
};

export default function RoadmapPanel(props: RoadmapPanelProps) {
  const { show: toast } = useToast();
  const [roadmap, { refetch }] = createResource(
    () => props.projectId,
    (id) => api.roadmap.get(id).catch(() => null),
  );

  const [creating, setCreating] = createSignal(false);
  const [title, setTitle] = createSignal("");
  const [description, setDescription] = createSignal("");
  const [detecting, setDetecting] = createSignal(false);
  const [aiPreview, setAiPreview] = createSignal<string | null>(null);

  // Milestone form
  const [showMilestoneForm, setShowMilestoneForm] = createSignal(false);
  const [milestoneTitle, setMilestoneTitle] = createSignal("");

  // Feature form
  const [featureMilestoneId, setFeatureMilestoneId] = createSignal<string | null>(null);
  const [featureTitle, setFeatureTitle] = createSignal("");

  // Import state
  const [importing, setImporting] = createSignal(false);
  const [importResult, setImportResult] = createSignal<ImportResult | null>(null);
  const [showPMImport, setShowPMImport] = createSignal(false);
  const [pmProviders] = createResource(() => api.providers.pm().catch(() => [] as ProviderInfo[]));
  const [selectedPM, setSelectedPM] = createSignal("");
  const [pmProjectRef, setPmProjectRef] = createSignal("");

  const handleCreate = async () => {
    if (!title()) return;
    setCreating(true);
    try {
      await api.roadmap.create(props.projectId, {
        title: title(),
        description: description() || undefined,
      });
      setTitle("");
      setDescription("");
      refetch();
      toast("success", "Roadmap created");
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Failed to create roadmap";
      props.onError(msg);
      toast("error", msg);
    } finally {
      setCreating(false);
    }
  };

  const handleDetect = async () => {
    setDetecting(true);
    try {
      const result = await api.roadmap.detect(props.projectId);
      if (result.found) {
        props.onError("");
        toast("success", `Detected ${result.format} at ${result.path}`);
      } else {
        props.onError("No spec files detected in workspace");
        toast("warning", "No spec files detected");
      }
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Detection failed";
      props.onError(msg);
      toast("error", msg);
    } finally {
      setDetecting(false);
    }
  };

  const handleAIView = async () => {
    try {
      const view = await api.roadmap.ai(props.projectId, "markdown");
      setAiPreview(view.content);
    } catch (e) {
      props.onError(e instanceof Error ? e.message : "Failed to load AI view");
    }
  };

  const handleAddMilestone = async () => {
    if (!milestoneTitle()) return;
    try {
      await api.roadmap.createMilestone(props.projectId, { title: milestoneTitle() });
      setMilestoneTitle("");
      setShowMilestoneForm(false);
      refetch();
      toast("success", "Milestone created");
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Failed to create milestone";
      props.onError(msg);
      toast("error", msg);
    }
  };

  const handleAddFeature = async (milestoneId: string) => {
    if (!featureTitle()) return;
    try {
      await api.roadmap.createFeature(milestoneId, { title: featureTitle() });
      setFeatureTitle("");
      setFeatureMilestoneId(null);
      refetch();
      toast("success", "Feature created");
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Failed to create feature";
      props.onError(msg);
      toast("error", msg);
    }
  };

  const handleDeleteRoadmap = async () => {
    if (!confirm("Delete this roadmap and all milestones/features?")) return;
    try {
      await api.roadmap.delete(props.projectId);
      refetch();
      toast("success", "Roadmap deleted");
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Failed to delete roadmap";
      props.onError(msg);
      toast("error", msg);
    }
  };

  const handleImportSpecs = async () => {
    setImporting(true);
    setImportResult(null);
    try {
      const result = await api.roadmap.importSpecs(props.projectId);
      setImportResult(result);
      refetch();
      toast("success", "Specs imported successfully");
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Spec import failed";
      props.onError(msg);
      toast("error", msg);
    } finally {
      setImporting(false);
    }
  };

  const handleImportPM = async () => {
    if (!selectedPM() || !pmProjectRef()) return;
    setImporting(true);
    setImportResult(null);
    try {
      const result = await api.roadmap.importPMItems(props.projectId, {
        provider: selectedPM(),
        project_ref: pmProjectRef(),
      });
      setImportResult(result);
      setShowPMImport(false);
      setPmProjectRef("");
      refetch();
      toast("success", "PM items imported successfully");
    } catch (e) {
      const msg = e instanceof Error ? e.message : "PM import failed";
      props.onError(msg);
      toast("error", msg);
    } finally {
      setImporting(false);
    }
  };

  return (
    <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <h3 class="mb-3 text-lg font-semibold">Roadmap</h3>

      <Show
        when={roadmap() !== null && roadmap() !== undefined}
        fallback={
          <div>
            <p class="mb-3 text-sm text-gray-500 dark:text-gray-400">No roadmap yet.</p>
            <div class="flex flex-col gap-2">
              <input
                type="text"
                class="rounded border border-gray-300 px-3 py-1.5 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
                placeholder="Roadmap title"
                value={title()}
                onInput={(e) => setTitle(e.currentTarget.value)}
              />
              <input
                type="text"
                class="rounded border border-gray-300 px-3 py-1.5 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
                placeholder="Description (optional)"
                value={description()}
                onInput={(e) => setDescription(e.currentTarget.value)}
              />
              <div class="flex gap-2">
                <button
                  class="rounded bg-blue-600 px-4 py-1.5 text-sm text-white hover:bg-blue-700 disabled:opacity-50"
                  onClick={handleCreate}
                  disabled={creating() || !title()}
                >
                  {creating() ? "Creating..." : "Create Roadmap"}
                </button>
                <button
                  class="rounded bg-gray-100 px-4 py-1.5 text-sm hover:bg-gray-200 disabled:opacity-50 dark:bg-gray-700 dark:hover:bg-gray-600"
                  onClick={handleDetect}
                  disabled={detecting()}
                >
                  {detecting() ? "Detecting..." : "Auto-Detect"}
                </button>
              </div>
            </div>
          </div>
        }
      >
        {(rm: () => Roadmap) => (
          <>
            <div class="mb-4 flex items-center justify-between">
              <div class="flex items-center gap-2">
                <span class="text-base font-medium">{rm().title}</span>
                <span
                  class={`rounded px-2 py-0.5 text-xs font-medium ${STATUS_COLORS[rm().status]}`}
                >
                  {rm().status}
                </span>
              </div>
              <div class="flex gap-2">
                <button
                  class="rounded bg-green-100 px-3 py-1 text-xs text-green-700 hover:bg-green-200 disabled:opacity-50 dark:bg-green-900/30 dark:text-green-400 dark:hover:bg-green-900/50"
                  onClick={handleImportSpecs}
                  disabled={importing()}
                >
                  {importing() ? "Importing..." : "Import Specs"}
                </button>
                <button
                  class="rounded bg-indigo-100 px-3 py-1 text-xs text-indigo-700 hover:bg-indigo-200 dark:bg-indigo-900/30 dark:text-indigo-400 dark:hover:bg-indigo-900/50"
                  onClick={() => setShowPMImport(!showPMImport())}
                >
                  Import from PM
                </button>
                <button
                  class="rounded bg-gray-100 px-3 py-1 text-xs hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600"
                  onClick={handleAIView}
                >
                  AI View
                </button>
                <button
                  class="rounded bg-red-50 px-3 py-1 text-xs text-red-600 hover:bg-red-100 dark:bg-red-900/30 dark:text-red-400 dark:hover:bg-red-900/50"
                  onClick={handleDeleteRoadmap}
                >
                  Delete
                </button>
              </div>
            </div>

            <Show when={rm().description}>
              <p class="mb-3 text-sm text-gray-500 dark:text-gray-400">{rm().description}</p>
            </Show>

            {/* AI Preview */}
            <Show when={aiPreview()}>
              <div class="mb-4 rounded border border-purple-200 bg-purple-50 p-3 dark:border-purple-800 dark:bg-purple-900/30">
                <div class="mb-2 flex items-center justify-between">
                  <span class="text-xs font-medium text-purple-700 dark:text-purple-400">
                    AI View (Markdown)
                  </span>
                  <button
                    class="text-xs text-purple-500 hover:text-purple-700 dark:text-purple-400 dark:hover:text-purple-300"
                    onClick={() => setAiPreview(null)}
                  >
                    Close
                  </button>
                </div>
                <pre class="max-h-48 overflow-auto whitespace-pre-wrap text-xs text-gray-700 dark:text-gray-300">
                  {aiPreview()}
                </pre>
              </div>
            </Show>

            {/* PM Import Form */}
            <Show when={showPMImport()}>
              <div class="mb-4 rounded border border-indigo-200 bg-indigo-50 p-3 dark:border-indigo-800 dark:bg-indigo-900/30">
                <div class="mb-2 text-xs font-medium text-indigo-700 dark:text-indigo-400">
                  Import from PM Tool
                </div>
                <div class="flex flex-col gap-2">
                  <select
                    class="rounded border border-gray-300 px-2 py-1 text-xs dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
                    value={selectedPM()}
                    onChange={(e) => setSelectedPM(e.currentTarget.value)}
                  >
                    <option value="">Select provider...</option>
                    <For each={pmProviders() ?? []}>
                      {(p: ProviderInfo) => <option value={p.name}>{p.name}</option>}
                    </For>
                  </select>
                  <input
                    type="text"
                    class="rounded border border-gray-300 px-2 py-1 text-xs dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
                    placeholder="Project ref (e.g. owner/repo)"
                    value={pmProjectRef()}
                    onInput={(e) => setPmProjectRef(e.currentTarget.value)}
                  />
                  <div class="flex gap-2">
                    <button
                      class="rounded bg-indigo-600 px-3 py-1 text-xs text-white hover:bg-indigo-700 disabled:opacity-50"
                      onClick={handleImportPM}
                      disabled={importing() || !selectedPM() || !pmProjectRef()}
                    >
                      {importing() ? "Importing..." : "Import"}
                    </button>
                    <button
                      class="rounded bg-gray-100 px-3 py-1 text-xs hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600"
                      onClick={() => setShowPMImport(false)}
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              </div>
            </Show>

            {/* Import Result */}
            <Show when={importResult()}>
              {(result: () => ImportResult) => (
                <div class="mb-4 rounded border border-green-200 bg-green-50 p-3 dark:border-green-800 dark:bg-green-900/30">
                  <div class="mb-1 flex items-center justify-between">
                    <span class="text-xs font-medium text-green-700 dark:text-green-400">
                      Import Complete ({result().source})
                    </span>
                    <button
                      class="text-xs text-green-500 hover:text-green-700 dark:text-green-400 dark:hover:text-green-300"
                      onClick={() => setImportResult(null)}
                    >
                      Dismiss
                    </button>
                  </div>
                  <p class="text-xs text-green-600 dark:text-green-400">
                    {result().milestones_created} milestones, {result().features_created} features
                    created
                  </p>
                  <Show when={(result().errors ?? []).length > 0}>
                    <div class="mt-1 text-xs text-red-600 dark:text-red-400">
                      <For each={result().errors ?? []}>{(err) => <p>{err}</p>}</For>
                    </div>
                  </Show>
                </div>
              )}
            </Show>

            {/* Milestones */}
            <For each={rm().milestones ?? []}>
              {(m: Milestone) => (
                <div class="mb-3 rounded border border-gray-100 bg-gray-50 p-3 dark:border-gray-600 dark:bg-gray-700">
                  <div class="mb-2 flex items-center gap-2">
                    <span class="text-sm font-medium">{m.title}</span>
                    <span class={`rounded px-1.5 py-0.5 text-xs ${STATUS_COLORS[m.status]}`}>
                      {m.status}
                    </span>
                  </div>

                  <Show when={m.description}>
                    <p class="mb-2 text-xs text-gray-500 dark:text-gray-400">{m.description}</p>
                  </Show>

                  {/* Features */}
                  <div class="space-y-1">
                    <For each={m.features ?? []}>
                      {(f: RoadmapFeature) => (
                        <div class="flex items-center justify-between rounded bg-white px-2 py-1.5 text-sm dark:bg-gray-800">
                          <div class="flex items-center gap-2">
                            <span
                              class={`rounded px-1.5 py-0.5 text-xs ${FEATURE_COLORS[f.status]}`}
                            >
                              {f.status}
                            </span>
                            <span>{f.title}</span>
                          </div>
                          <Show when={(f.labels ?? []).length > 0}>
                            <div class="flex gap-1">
                              <For each={f.labels}>
                                {(label) => (
                                  <span class="rounded bg-gray-200 px-1.5 py-0.5 text-xs text-gray-600 dark:bg-gray-600 dark:text-gray-300">
                                    {label}
                                  </span>
                                )}
                              </For>
                            </div>
                          </Show>
                        </div>
                      )}
                    </For>
                  </div>

                  {/* Add Feature */}
                  <Show
                    when={featureMilestoneId() === m.id}
                    fallback={
                      <button
                        class="mt-2 text-xs text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
                        onClick={() => setFeatureMilestoneId(m.id)}
                      >
                        + Add Feature
                      </button>
                    }
                  >
                    <div class="mt-2 flex gap-2">
                      <input
                        type="text"
                        class="flex-1 rounded border border-gray-300 px-2 py-1 text-xs dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
                        placeholder="Feature title"
                        value={featureTitle()}
                        onInput={(e) => setFeatureTitle(e.currentTarget.value)}
                      />
                      <button
                        class="rounded bg-blue-600 px-2 py-1 text-xs text-white hover:bg-blue-700"
                        onClick={() => handleAddFeature(m.id)}
                      >
                        Add
                      </button>
                      <button
                        class="rounded bg-gray-100 px-2 py-1 text-xs hover:bg-gray-200 dark:bg-gray-600 dark:hover:bg-gray-500"
                        onClick={() => {
                          setFeatureMilestoneId(null);
                          setFeatureTitle("");
                        }}
                      >
                        Cancel
                      </button>
                    </div>
                  </Show>
                </div>
              )}
            </For>

            {/* Add Milestone */}
            <Show
              when={showMilestoneForm()}
              fallback={
                <button
                  class="mt-2 text-sm text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
                  onClick={() => setShowMilestoneForm(true)}
                >
                  + Add Milestone
                </button>
              }
            >
              <div class="mt-2 flex gap-2">
                <input
                  type="text"
                  class="flex-1 rounded border border-gray-300 px-3 py-1.5 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
                  placeholder="Milestone title"
                  value={milestoneTitle()}
                  onInput={(e) => setMilestoneTitle(e.currentTarget.value)}
                />
                <button
                  class="rounded bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700"
                  onClick={handleAddMilestone}
                >
                  Add
                </button>
                <button
                  class="rounded bg-gray-100 px-3 py-1.5 text-sm hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600"
                  onClick={() => {
                    setShowMilestoneForm(false);
                    setMilestoneTitle("");
                  }}
                >
                  Cancel
                </button>
              </div>
            </Show>
          </>
        )}
      </Show>
    </div>
  );
}
