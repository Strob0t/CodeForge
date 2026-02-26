import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type {
  FeatureStatus,
  ImportResult,
  Milestone,
  ProviderInfo,
  RoadmapFeature,
  RoadmapStatus,
} from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { Badge, Button, ConfirmDialog, Input, Select } from "~/ui";

import DragList, { type DragHandleProps } from "./DragList";

interface RoadmapPanelProps {
  projectId: string;
  onError: (msg: string) => void;
}

function roadmapStatusVariant(status: RoadmapStatus): "default" | "info" | "success" | "warning" {
  switch (status) {
    case "draft":
      return "default";
    case "active":
      return "info";
    case "complete":
      return "success";
    case "archived":
      return "warning";
  }
}

function featureStatusVariant(
  status: FeatureStatus,
): "default" | "info" | "warning" | "success" | "danger" {
  switch (status) {
    case "backlog":
      return "default";
    case "planned":
      return "info";
    case "in_progress":
      return "warning";
    case "done":
      return "success";
    case "cancelled":
      return "danger";
  }
}

export default function RoadmapPanel(props: RoadmapPanelProps) {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [roadmap, { refetch }] = createResource(
    () => props.projectId,
    (id) => api.roadmap.get(id).catch(() => null),
  );

  const [creating, setCreating] = createSignal(false);
  const [title, setTitle] = createSignal("");
  const [description, setDescription] = createSignal("");
  const [detecting, setDetecting] = createSignal(false);
  const [detectionResult, setDetectionResult] = createSignal<
    import("~/api/types").DetectionResult | null
  >(null);
  const [aiPreview, setAiPreview] = createSignal<string | null>(null);

  // Milestone form
  const [showMilestoneForm, setShowMilestoneForm] = createSignal(false);
  const [milestoneTitle, setMilestoneTitle] = createSignal("");

  // Feature form
  const [featureMilestoneId, setFeatureMilestoneId] = createSignal<string | null>(null);
  const [featureTitle, setFeatureTitle] = createSignal("");

  // Sync-to-file state
  const [syncing, setSyncing] = createSignal(false);

  // Delete confirmation dialog
  const [showDeleteConfirm, setShowDeleteConfirm] = createSignal(false);

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
      toast("success", t("roadmap.toast.created"));
    } catch (e) {
      const msg = e instanceof Error ? e.message : t("roadmap.toast.createFailed");
      props.onError(msg);
      toast("error", msg);
    } finally {
      setCreating(false);
    }
  };

  const handleDetect = async () => {
    setDetecting(true);
    setDetectionResult(null);
    try {
      const result = await api.roadmap.detect(props.projectId);
      if (result.found) {
        setDetectionResult(result);
        props.onError("");
        toast("success", t("roadmap.toast.detected", { format: result.format, path: result.path }));
      } else {
        props.onError(t("roadmap.toast.notDetected"));
        toast("warning", t("roadmap.toast.notDetected"));
      }
    } catch (e) {
      const msg = e instanceof Error ? e.message : t("roadmap.toast.detectFailed");
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
      props.onError(e instanceof Error ? e.message : t("roadmap.toast.aiFailed"));
    }
  };

  const handleAddMilestone = async () => {
    if (!milestoneTitle()) return;
    try {
      await api.roadmap.createMilestone(props.projectId, { title: milestoneTitle() });
      setMilestoneTitle("");
      setShowMilestoneForm(false);
      refetch();
      toast("success", t("roadmap.toast.milestoneCreated"));
    } catch (e) {
      const msg = e instanceof Error ? e.message : t("roadmap.toast.milestoneFailed");
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
      toast("success", t("roadmap.toast.featureCreated"));
    } catch (e) {
      const msg = e instanceof Error ? e.message : t("roadmap.toast.featureFailed");
      props.onError(msg);
      toast("error", msg);
    }
  };

  const handleDeleteRoadmap = async () => {
    try {
      await api.roadmap.delete(props.projectId);
      refetch();
      toast("success", t("roadmap.toast.deleted"));
    } catch (e) {
      const msg = e instanceof Error ? e.message : t("roadmap.toast.deleteFailed");
      props.onError(msg);
      toast("error", msg);
    } finally {
      setShowDeleteConfirm(false);
    }
  };

  const handleImportSpecs = async () => {
    setImporting(true);
    setImportResult(null);
    try {
      const result = await api.roadmap.importSpecs(props.projectId);
      setImportResult(result);
      refetch();
      toast("success", t("roadmap.toast.specsImported"));
    } catch (e) {
      const msg = e instanceof Error ? e.message : t("roadmap.toast.importSpecsFailed");
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
      toast("success", t("roadmap.toast.pmImported"));
    } catch (e) {
      const msg = e instanceof Error ? e.message : t("roadmap.toast.importPMFailed");
      props.onError(msg);
      toast("error", msg);
    } finally {
      setImporting(false);
    }
  };

  const handleSyncToFile = async () => {
    setSyncing(true);
    try {
      await api.roadmap.syncToFile(props.projectId);
      toast("success", t("roadmap.toast.synced"));
    } catch (e) {
      const msg = e instanceof Error ? e.message : t("roadmap.toast.syncFailed");
      props.onError(msg);
      toast("error", msg);
    } finally {
      setSyncing(false);
    }
  };

  return (
    <div class="h-full overflow-y-auto rounded-cf-md border border-cf-border bg-cf-bg-surface p-4">
      <h3 class="mb-3 text-lg font-semibold">{t("roadmap.title")}</h3>

      <Show
        when={roadmap()}
        fallback={
          <div>
            <p class="mb-3 text-sm text-cf-text-tertiary">{t("roadmap.empty")}</p>
            <div class="flex flex-col gap-2">
              <div>
                <label for="roadmap-title" class="sr-only">
                  {t("roadmap.titleLabel")}
                </label>
                <Input
                  id="roadmap-title"
                  type="text"
                  placeholder={t("roadmap.form.titlePlaceholder")}
                  value={title()}
                  onInput={(e) => setTitle(e.currentTarget.value)}
                  aria-required="true"
                />
              </div>
              <div>
                <label for="roadmap-description" class="sr-only">
                  {t("roadmap.descriptionLabel")}
                </label>
                <Input
                  id="roadmap-description"
                  type="text"
                  placeholder={t("roadmap.form.descriptionPlaceholder")}
                  value={description()}
                  onInput={(e) => setDescription(e.currentTarget.value)}
                />
              </div>
              <div class="flex gap-2">
                <Button
                  variant="primary"
                  size="sm"
                  onClick={handleCreate}
                  disabled={creating() || !title()}
                  loading={creating()}
                >
                  {creating() ? t("common.creating") : t("roadmap.createRoadmap")}
                </Button>
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={handleDetect}
                  disabled={detecting()}
                  loading={detecting()}
                >
                  {detecting() ? t("roadmap.detecting") : t("roadmap.autoDetect")}
                </Button>
              </div>
              <Show when={detectionResult()}>
                {(dr) => (
                  <div class="mt-3 rounded-cf-sm border border-emerald-200 bg-emerald-50 p-3 dark:border-emerald-800 dark:bg-emerald-900/30">
                    <div class="mb-1 flex items-center justify-between">
                      <span class="text-xs font-medium text-emerald-700 dark:text-emerald-400">
                        {t("roadmap.toast.detected", { format: dr().format, path: dr().path })}
                      </span>
                      <Button variant="ghost" size="sm" onClick={() => setDetectionResult(null)}>
                        {t("common.dismiss")}
                      </Button>
                    </div>
                    <Show when={(dr().file_markers ?? []).length > 0}>
                      <ul class="mt-1 list-disc pl-4 text-xs text-emerald-600 dark:text-emerald-400">
                        <For each={dr().file_markers}>{(marker) => <li>{marker}</li>}</For>
                      </ul>
                    </Show>
                  </div>
                )}
              </Show>
            </div>
          </div>
        }
      >
        {(rm) => (
          <>
            <div class="mb-4 flex items-center justify-between">
              <div class="flex items-center gap-2">
                <span class="text-base font-medium">{rm().title}</span>
                <Badge variant={roadmapStatusVariant(rm().status)} pill>
                  {rm().status}
                </Badge>
              </div>
              <div class="flex gap-2">
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={handleImportSpecs}
                  disabled={importing()}
                  loading={importing()}
                >
                  {importing() ? t("common.importing") : t("roadmap.importSpecs")}
                </Button>
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => setShowPMImport(!showPMImport())}
                >
                  {t("roadmap.importPM")}
                </Button>
                <Button variant="ghost" size="sm" onClick={handleAIView}>
                  {t("roadmap.aiView")}
                </Button>
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={handleSyncToFile}
                  disabled={syncing()}
                  loading={syncing()}
                >
                  {syncing() ? t("roadmap.syncing") : t("roadmap.syncToFile")}
                </Button>
                <Button
                  variant="danger"
                  size="sm"
                  onClick={() => setShowDeleteConfirm(true)}
                  aria-label={t("roadmap.deleteAria")}
                >
                  {t("common.delete")}
                </Button>
                <ConfirmDialog
                  open={showDeleteConfirm()}
                  title={t("roadmap.deleteTitle")}
                  message={t("roadmap.confirmDelete")}
                  variant="danger"
                  confirmLabel={t("common.delete")}
                  cancelLabel={t("common.cancel")}
                  onConfirm={handleDeleteRoadmap}
                  onCancel={() => setShowDeleteConfirm(false)}
                />
              </div>
            </div>

            <Show when={rm().description}>
              <p class="mb-3 text-sm text-cf-text-tertiary">{rm().description}</p>
            </Show>

            {/* AI Preview */}
            <Show when={aiPreview()}>
              <div class="mb-4 rounded-cf-sm border border-purple-200 bg-purple-50 p-3 dark:border-purple-800 dark:bg-purple-900/30">
                <div class="mb-2 flex items-center justify-between">
                  <span class="text-xs font-medium text-purple-700 dark:text-purple-400">
                    {t("roadmap.aiViewMarkdown")}
                  </span>
                  <Button variant="ghost" size="sm" onClick={() => setAiPreview(null)}>
                    {t("common.close")}
                  </Button>
                </div>
                <pre class="max-h-48 overflow-auto whitespace-pre-wrap text-xs text-cf-text-secondary">
                  {aiPreview()}
                </pre>
              </div>
            </Show>

            {/* PM Import Form */}
            <Show when={showPMImport()}>
              <div class="mb-4 rounded-cf-sm border border-indigo-200 bg-indigo-50 p-3 dark:border-indigo-800 dark:bg-indigo-900/30">
                <div class="mb-2 text-xs font-medium text-indigo-700 dark:text-indigo-400">
                  {t("roadmap.importPMTool")}
                </div>
                <div class="flex flex-col gap-2">
                  <Select
                    value={selectedPM()}
                    onChange={(e) => setSelectedPM(e.currentTarget.value)}
                    aria-label={t("roadmap.pmProviderLabel")}
                  >
                    <option value="">{t("roadmap.selectProvider")}</option>
                    <For each={pmProviders() ?? []}>
                      {(p: ProviderInfo) => <option value={p.name}>{p.name}</option>}
                    </For>
                  </Select>
                  <Input
                    type="text"
                    placeholder={t("roadmap.projectRefPlaceholder")}
                    value={pmProjectRef()}
                    onInput={(e) => setPmProjectRef(e.currentTarget.value)}
                    aria-label={t("roadmap.pmProjectRefLabel")}
                  />
                  <div class="flex gap-2">
                    <Button
                      variant="primary"
                      size="sm"
                      onClick={handleImportPM}
                      disabled={importing() || !selectedPM() || !pmProjectRef()}
                      loading={importing()}
                    >
                      {importing() ? t("common.importing") : t("common.import")}
                    </Button>
                    <Button variant="ghost" size="sm" onClick={() => setShowPMImport(false)}>
                      {t("common.cancel")}
                    </Button>
                  </div>
                </div>
              </div>
            </Show>

            {/* Import Result */}
            <Show when={importResult()}>
              {(result: () => ImportResult) => (
                <div class="mb-4 rounded-cf-sm border border-cf-success-border bg-cf-success-bg p-3">
                  <div class="mb-1 flex items-center justify-between">
                    <span class="text-xs font-medium text-cf-success-fg">
                      {t("roadmap.importComplete", { source: result().source })}
                    </span>
                    <Button variant="ghost" size="sm" onClick={() => setImportResult(null)}>
                      {t("common.dismiss")}
                    </Button>
                  </div>
                  <p class="text-xs text-cf-success-fg">
                    {t("roadmap.importStats", {
                      milestones: result().milestones_created,
                      features: result().features_created,
                    })}
                  </p>
                  <Show when={(result().errors ?? []).length > 0}>
                    <div class="mt-1 text-xs text-cf-danger-fg">
                      <For each={result().errors ?? []}>{(err) => <p>{err}</p>}</For>
                    </div>
                  </Show>
                </div>
              )}
            </Show>

            {/* Milestones (drag-to-reorder) */}
            <DragList
              items={rm().milestones ?? []}
              getId={(m: Milestone) => m.id}
              onReorder={async (reordered: Milestone[]) => {
                try {
                  for (let i = 0; i < reordered.length; i++) {
                    if (reordered[i].sort_order !== i) {
                      await api.roadmap.updateMilestone(reordered[i].id, {
                        sort_order: i,
                        version: reordered[i].version,
                      });
                    }
                  }
                  refetch();
                } catch (e) {
                  toast("error", e instanceof Error ? e.message : "Reorder failed");
                }
              }}
              renderItem={(m: Milestone, dragHandleProps: DragHandleProps) => (
                <div class="mb-3 rounded-cf-sm border border-cf-border-subtle bg-cf-bg-surface-alt p-3">
                  <div class="mb-2 flex items-center gap-2">
                    <span
                      class="cursor-grab text-cf-text-muted hover:text-cf-text-secondary"
                      {...dragHandleProps}
                      title={t("roadmap.dragToReorder")}
                    >
                      &#x2630;
                    </span>
                    <span class="text-sm font-medium">{m.title}</span>
                    <Badge variant={roadmapStatusVariant(m.status)} pill>
                      {m.status}
                    </Badge>
                  </div>

                  <Show when={m.description}>
                    <p class="mb-2 text-xs text-cf-text-tertiary">{m.description}</p>
                  </Show>

                  {/* Features with inline status toggle */}
                  <div class="space-y-1">
                    <For each={m.features ?? []}>
                      {(f: RoadmapFeature) => (
                        <div class="flex items-center justify-between rounded-cf-sm bg-cf-bg-surface px-2 py-1.5 text-sm">
                          <div class="flex items-center gap-2">
                            <button
                              class={`flex h-4 w-4 items-center justify-center rounded border text-xs ${
                                f.status === "done"
                                  ? "border-cf-success bg-cf-success text-white"
                                  : "border-cf-border text-transparent hover:border-cf-success"
                              }`}
                              title={
                                f.status === "done" ? t("roadmap.markTodo") : t("roadmap.markDone")
                              }
                              onClick={async () => {
                                const newStatus: FeatureStatus =
                                  f.status === "done" ? "backlog" : "done";
                                try {
                                  await api.roadmap.updateFeature(f.id, {
                                    status: newStatus,
                                    version: f.version,
                                  });
                                  refetch();
                                } catch (e) {
                                  toast(
                                    "error",
                                    e instanceof Error ? e.message : "Status update failed",
                                  );
                                }
                              }}
                            >
                              {f.status === "done" ? "\u2713" : "\u00A0"}
                            </button>
                            <span
                              class={f.status === "done" ? "text-cf-text-muted line-through" : ""}
                            >
                              {f.title}
                            </span>
                          </div>
                          <div class="flex items-center gap-1">
                            <Show when={(f.labels ?? []).length > 0}>
                              <For each={f.labels}>
                                {(label) => <Badge variant="default">{label}</Badge>}
                              </For>
                            </Show>
                            <Badge variant={featureStatusVariant(f.status)}>{f.status}</Badge>
                          </div>
                        </div>
                      )}
                    </For>
                  </div>

                  {/* Add Feature */}
                  <Show
                    when={featureMilestoneId() === m.id}
                    fallback={
                      <button
                        class="mt-2 text-xs text-cf-accent hover:underline"
                        onClick={() => setFeatureMilestoneId(m.id)}
                      >
                        {t("roadmap.addFeature")}
                      </button>
                    }
                  >
                    <div class="mt-2 flex gap-2">
                      <Input
                        type="text"
                        class="flex-1"
                        placeholder={t("roadmap.featurePlaceholder")}
                        value={featureTitle()}
                        onInput={(e) => setFeatureTitle(e.currentTarget.value)}
                        aria-label={t("roadmap.featureLabel")}
                      />
                      <Button variant="primary" size="sm" onClick={() => handleAddFeature(m.id)}>
                        {t("common.add")}
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => {
                          setFeatureMilestoneId(null);
                          setFeatureTitle("");
                        }}
                      >
                        {t("common.cancel")}
                      </Button>
                    </div>
                  </Show>
                </div>
              )}
            />

            {/* Add Milestone */}
            <Show
              when={showMilestoneForm()}
              fallback={
                <button
                  class="mt-2 text-sm text-cf-accent hover:underline"
                  onClick={() => setShowMilestoneForm(true)}
                >
                  {t("roadmap.addMilestone")}
                </button>
              }
            >
              <div class="mt-2 flex gap-2">
                <Input
                  type="text"
                  class="flex-1"
                  placeholder={t("roadmap.milestonePlaceholder")}
                  value={milestoneTitle()}
                  onInput={(e) => setMilestoneTitle(e.currentTarget.value)}
                  aria-label={t("roadmap.milestoneLabel")}
                />
                <Button variant="primary" size="sm" onClick={handleAddMilestone}>
                  {t("common.add")}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    setShowMilestoneForm(false);
                    setMilestoneTitle("");
                  }}
                >
                  {t("common.cancel")}
                </Button>
              </div>
            </Show>
          </>
        )}
      </Show>
    </div>
  );
}
