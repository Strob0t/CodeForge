import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { Milestone, RoadmapFeature } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";

import MilestoneColumn from "./featuremap/MilestoneColumn";
import MilestoneForm from "./featuremap/MilestoneForm";

interface FeatureMapPanelProps {
  projectId: string;
  onError?: (msg: string) => void;
  onNavigate?: (target: string) => void;
  onSendChatMessage?: (msg: string) => void;
}

export default function FeatureMapPanel(props: FeatureMapPanelProps) {
  const { t } = useI18n();
  const { show: toast } = useToast();

  const [roadmap, { refetch }] = createResource(
    () => props.projectId,
    (id) => api.roadmap.get(id).catch(() => null),
  );

  const [showMilestoneForm, setShowMilestoneForm] = createSignal(false);

  const milestones = (): Milestone[] => roadmap()?.milestones ?? [];

  /** Find a feature by ID across all milestones. */
  const findFeature = (featureId: string): RoadmapFeature | undefined => {
    for (const ms of milestones()) {
      const found = (ms.features ?? []).find((f) => f.id === featureId);
      if (found) return found;
    }
    return undefined;
  };

  /**
   * Handle a feature being dropped into a milestone column.
   * Two cases:
   *   1. Cross-milestone move: update milestone_id
   *   2. Within-milestone reorder: update sort_order
   */
  const handleFeatureDropped = async (
    featureId: string,
    targetMilestoneId: string,
    targetIndex: number,
  ) => {
    const feature = findFeature(featureId);
    if (!feature) return;

    const isMovingAcrossMilestones = feature.milestone_id !== targetMilestoneId;

    try {
      if (isMovingAcrossMilestones) {
        await api.roadmap.updateFeature(featureId, {
          milestone_id: targetMilestoneId,
          sort_order: targetIndex,
          version: feature.version,
        });
        toast("success", t("featuremap.featureMoved"));
      } else {
        // Same milestone, just reorder
        if (feature.sort_order === targetIndex) return;
        await api.roadmap.updateFeature(featureId, {
          sort_order: targetIndex,
          version: feature.version,
        });
        toast("success", t("featuremap.featureReordered"));
      }
      refetch();
    } catch (e) {
      const msg =
        e instanceof Error
          ? e.message
          : isMovingAcrossMilestones
            ? t("featuremap.moveFailed")
            : t("featuremap.reorderFailed");
      toast("error", msg);
      props.onError?.(msg);
    }
  };

  return (
    <Show
      when={roadmap() !== undefined && roadmap() !== null}
      fallback={
        <div class="flex flex-col items-center justify-center gap-3 py-16 text-center">
          <p class="text-sm text-cf-text-muted">{t("empty.featuremap")}</p>
          <button
            class="text-sm text-cf-accent hover:underline"
            onClick={() => props.onNavigate?.("roadmap")}
          >
            {t("empty.featuremap.action")}
          </button>
        </div>
      }
    >
      <div class="h-full overflow-x-auto">
        <div class="flex gap-4 p-4 min-w-max h-full">
          {/* Milestone Columns */}
          <For each={milestones()}>
            {(milestone) => (
              <MilestoneColumn
                milestone={milestone}
                onFeatureDropped={handleFeatureDropped}
                onRefetch={refetch}
                onSendChatMessage={props.onSendChatMessage}
              />
            )}
          </For>

          {/* Add Milestone Column */}
          <div class="flex flex-col w-72 flex-shrink-0 rounded-cf-md border border-dashed border-cf-border-subtle bg-cf-bg-surface-alt/50 h-full">
            <Show
              when={showMilestoneForm()}
              fallback={
                <button
                  class="flex items-center justify-center w-full h-full min-h-[120px] text-sm text-cf-text-muted hover:text-cf-text-primary hover:bg-cf-bg-surface-alt transition-colors rounded-cf-md"
                  onClick={() => setShowMilestoneForm(true)}
                >
                  {t("featuremap.addMilestone")}
                </button>
              }
            >
              <MilestoneForm
                projectId={props.projectId}
                onSave={() => {
                  setShowMilestoneForm(false);
                  refetch();
                }}
                onCancel={() => setShowMilestoneForm(false)}
              />
            </Show>
          </div>
        </div>
      </div>
    </Show>
  );
}
