import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { ProjectHealth } from "~/api/types";
import { useConfirm } from "~/components/ConfirmProvider";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { Alert, Button, EmptyState, GridLayout, LoadingState, PageLayout } from "~/ui";

import ActivityTimeline from "./ActivityTimeline";
import ChartsPanel from "./ChartsPanel";
import { CreateProjectModal } from "./CreateProjectModal";
import KpiStrip from "./KpiStrip";
import ProjectCard from "./ProjectCard";

export default function DashboardPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const { confirm } = useConfirm();

  const [projects, { refetch }] = createResource(() => api.projects.list());
  const [stats] = createResource(() => api.dashboard.stats());
  const [showModal, setShowModal] = createSignal(false);

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
    window.location.href = `/projects/${id}`;
  }

  return (
    <PageLayout
      title={t("dashboard.title")}
      action={
        <Button variant="primary" onClick={() => setShowModal(true)}>
          {t("dashboard.newProject")}
        </Button>
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
          <div class="mt-4">
            <GridLayout>
              <For each={projects()}>
                {(p) => (
                  <ProjectCard
                    project={p}
                    health={healthMap()?.[p.id]}
                    onDelete={handleDelete}
                    onEdit={handleEdit}
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
    </PageLayout>
  );
}
