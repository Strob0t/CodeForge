import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { Project, ProjectHealth } from "~/api/types";
import { useConfirm } from "~/components/ConfirmProvider";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { Alert, Button, EmptyState, GridLayout, LoadingState, PageLayout } from "~/ui";

import ActivityTimeline from "./ActivityTimeline";
import ChartsPanel from "./ChartsPanel";
import { CreateProjectModal } from "./CreateProjectModal";
import { EditProjectModal } from "./EditProjectModal";
import KpiStrip from "./KpiStrip";
import ProjectCard from "./ProjectCard";

export default function DashboardPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const { confirm } = useConfirm();

  const [projects, { refetch }] = createResource(() => api.projects.list());
  const [stats] = createResource(() => api.dashboard.stats());
  const [showModal, setShowModal] = createSignal(false);
  const [editingProject, setEditingProject] = createSignal<Project | null>(null);
  const [batchMode, setBatchMode] = createSignal(false);
  const [selectedIds, setSelectedIds] = createSignal<Set<string>>(new Set());
  const [batchLoading, setBatchLoading] = createSignal(false);

  // Fetch health for each project in parallel once the project list loads
  const [healthMap] = createResource(
    () => projects(),
    async (projs) => {
      const entries = await Promise.all(
        projs.map(async (p) => {
          try {
            const h = await api.dashboard.projectHealth(p.id);
            return [p.id, h] as const;
          } catch {
            return [p.id, undefined] as const;
          }
        }),
      );
      return Object.fromEntries(entries) as Record<string, ProjectHealth | undefined>;
    },
  );

  async function handleDelete(id: string) {
    const ok = await confirm({
      title: t("common.delete"),
      message: t("dashboard.confirm.delete"),
      variant: "danger",
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    try {
      await api.projects.delete(id);
      await refetch();
      toast("success", t("dashboard.toast.deleted"));
    } catch (err) {
      const msg = err instanceof Error ? err.message : t("dashboard.toast.deleteFailed");
      toast("error", msg);
    }
  }

  function handleEdit(id: string) {
    const p = projects()?.find((proj) => proj.id === id);
    if (p) setEditingProject(p);
  }

  function toggleSelect(id: string) {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function selectAll() {
    const all = projects() ?? [];
    setSelectedIds(new Set(all.map((p) => p.id)));
  }

  function deselectAll() {
    setSelectedIds(new Set<string>());
  }

  function exitBatchMode() {
    setBatchMode(false);
    setSelectedIds(new Set<string>());
  }

  async function handleBatchDelete() {
    const ids = [...selectedIds()];
    if (ids.length === 0) return;
    const ok = await confirm({
      title: t("dashboard.batch.delete"),
      message: t("dashboard.batch.confirmDelete", { count: ids.length }),
      variant: "danger",
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    setBatchLoading(true);
    try {
      await api.batch.deleteProjects(ids);
      toast("success", t("dashboard.batch.deleteSuccess"));
      exitBatchMode();
      await refetch();
    } catch {
      toast("error", t("dashboard.batch.failed"));
    } finally {
      setBatchLoading(false);
    }
  }

  async function handleBatchPull() {
    const ids = [...selectedIds()];
    if (ids.length === 0) return;
    setBatchLoading(true);
    try {
      await api.batch.pullProjects(ids);
      toast("success", t("dashboard.batch.pullSuccess"));
    } catch {
      toast("error", t("dashboard.batch.failed"));
    } finally {
      setBatchLoading(false);
    }
  }

  async function handleBatchStatus() {
    const ids = [...selectedIds()];
    if (ids.length === 0) return;
    setBatchLoading(true);
    try {
      await api.batch.statusProjects(ids);
      toast("info", t("dashboard.batch.statusSuccess"));
    } catch {
      toast("error", t("dashboard.batch.failed"));
    } finally {
      setBatchLoading(false);
    }
  }

  return (
    <PageLayout
      title={t("dashboard.title")}
      action={
        <div class="flex items-center gap-2">
          <Show
            when={batchMode()}
            fallback={
              <Button variant="secondary" onClick={() => setBatchMode(true)}>
                {t("dashboard.batch.select")}
              </Button>
            }
          >
            <Button variant="ghost" onClick={exitBatchMode}>
              {t("dashboard.batch.cancel")}
            </Button>
          </Show>
          <Button variant="primary" onClick={() => setShowModal(true)}>
            {t("dashboard.newProject")}
          </Button>
        </div>
      }
    >
      {/* KPI Strip */}
      <Show when={stats()}>{(s) => <KpiStrip stats={s()} />}</Show>

      {/* Project Grid */}
      <Show when={projects.loading}>
        <LoadingState message={t("dashboard.loading")} />
      </Show>

      <Show when={projects.error}>
        <Alert variant="error">{t("dashboard.loadError")}</Alert>
      </Show>

      <Show when={!projects.loading && !projects.error}>
        <Show when={projects()?.length} fallback={<EmptyState title={t("dashboard.empty")} />}>
          {/* Batch action bar */}
          <Show when={batchMode()}>
            <div class="mt-3 flex flex-wrap items-center gap-2 rounded-cf-sm bg-cf-bg-inset px-4 py-2">
              <span class="text-sm text-cf-text-primary font-medium">
                {t("dashboard.batch.selected", { count: selectedIds().size })}
              </span>
              <span class="flex-1" />
              <Button
                variant="ghost"
                size="sm"
                onClick={selectedIds().size === (projects()?.length ?? 0) ? deselectAll : selectAll}
              >
                {selectedIds().size === (projects()?.length ?? 0)
                  ? t("dashboard.batch.deselectAll")
                  : t("dashboard.batch.selectAll")}
              </Button>
              <Button
                variant="secondary"
                size="sm"
                disabled={selectedIds().size === 0 || batchLoading()}
                onClick={() => void handleBatchStatus()}
              >
                {t("dashboard.batch.status")}
              </Button>
              <Button
                variant="secondary"
                size="sm"
                disabled={selectedIds().size === 0 || batchLoading()}
                onClick={() => void handleBatchPull()}
              >
                {t("dashboard.batch.pull")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                class="bg-red-600 hover:bg-red-700 text-white"
                disabled={selectedIds().size === 0 || batchLoading()}
                onClick={() => void handleBatchDelete()}
              >
                {t("dashboard.batch.delete")}
              </Button>
            </div>
          </Show>

          <div class="mt-4">
            <GridLayout>
              <For each={projects()}>
                {(p) => (
                  <ProjectCard
                    project={p}
                    health={healthMap()?.[p.id]}
                    onDelete={handleDelete}
                    onEdit={handleEdit}
                    batchMode={batchMode()}
                    selected={selectedIds().has(p.id)}
                    onToggleSelect={toggleSelect}
                  />
                )}
              </For>
            </GridLayout>
          </div>
        </Show>
      </Show>

      {/* Bottom section: Activity Timeline + Charts */}
      <div class="mt-6 grid grid-cols-1 gap-6 lg:grid-cols-5">
        <div class="lg:col-span-2">
          <ActivityTimeline />
        </div>
        <div class="lg:col-span-3">
          <ChartsPanel />
        </div>
      </div>

      {/* Create Project Modal */}
      <CreateProjectModal
        open={showModal()}
        onClose={() => setShowModal(false)}
        onCreated={() => {
          setShowModal(false);
          refetch();
        }}
      />

      {/* Edit Project Modal */}
      <EditProjectModal
        project={editingProject()}
        onClose={() => setEditingProject(null)}
        onUpdated={() => {
          setEditingProject(null);
          refetch();
        }}
      />
    </PageLayout>
  );
}
