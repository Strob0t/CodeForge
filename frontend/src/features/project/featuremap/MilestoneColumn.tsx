import { createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { FeatureStatus, Milestone, RoadmapFeature } from "~/api/types";
import { useToast } from "~/components/Toast";
import { getVariant, roadmapStatusVariant } from "~/config/statusVariants";
import { useI18n } from "~/i18n";
import { extractErrorMessage } from "~/lib/errorUtils";
import { Badge, Button } from "~/ui";

import FeatureCard from "./FeatureCard";
import FeatureCardForm from "./FeatureCardForm";
import { decodeDragPayload, FEATURE_MIME } from "./featuremap-dnd";

interface MilestoneColumnProps {
  milestone: Milestone;
  onFeatureDropped: (featureId: string, targetMilestoneId: string, targetIndex: number) => void;
  onRefetch: () => void;
}

export default function MilestoneColumn(props: MilestoneColumnProps) {
  const { t } = useI18n();
  const { show: toast } = useToast();

  const [isDragOver, setIsDragOver] = createSignal(false);
  const [dropIndex, setDropIndex] = createSignal<number | null>(null);
  const [showAddForm, setShowAddForm] = createSignal(false);
  const [editingFeature, setEditingFeature] = createSignal<RoadmapFeature | null>(null);

  const features = (): RoadmapFeature[] => props.milestone.features ?? [];

  const handleDragOver = (e: DragEvent) => {
    if (!e.dataTransfer?.types.includes(FEATURE_MIME)) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    setIsDragOver(true);

    // Calculate insertion index from mouse position
    const column = e.currentTarget as HTMLElement;
    const featureEls = column.querySelectorAll("[data-feature-index]");
    let insertIdx = features().length;

    for (const el of featureEls) {
      const rect = el.getBoundingClientRect();
      const midY = rect.top + rect.height / 2;
      if (e.clientY < midY) {
        insertIdx = Number(el.getAttribute("data-feature-index"));
        break;
      }
    }
    setDropIndex(insertIdx);
  };

  const handleDragLeave = (e: DragEvent) => {
    // Only reset if we actually left the column (not entering a child)
    const related = e.relatedTarget as Node | null;
    const current = e.currentTarget as HTMLElement;
    if (!related || !current.contains(related)) {
      setIsDragOver(false);
      setDropIndex(null);
    }
  };

  const handleDrop = (e: DragEvent) => {
    e.preventDefault();
    setIsDragOver(false);

    const data = e.dataTransfer?.getData(FEATURE_MIME);
    if (!data) return;

    const payload = decodeDragPayload(data);
    if (!payload) return;

    const idx = dropIndex() ?? features().length;
    setDropIndex(null);
    props.onFeatureDropped(payload.featureId, props.milestone.id, idx);
  };

  const handleStatusToggle = async (featureId: string, currentStatus: FeatureStatus) => {
    const newStatus: FeatureStatus = currentStatus === "done" ? "backlog" : "done";
    const feature = features().find((f) => f.id === featureId);
    if (!feature) return;

    try {
      await api.roadmap.updateFeature(featureId, {
        status: newStatus,
        version: feature.version,
      });
      toast("success", t("featuremap.statusToggled"));
      props.onRefetch();
    } catch (e) {
      const msg = extractErrorMessage(e, t("featuremap.updateFailed"));
      toast("error", msg);
    }
  };

  const handleEditFeature = (feature: RoadmapFeature) => {
    setEditingFeature(feature);
    setShowAddForm(false);
  };

  return (
    <div class="flex flex-col w-72 flex-shrink-0 rounded-cf-md border border-cf-border-subtle bg-cf-bg-surface-alt h-full">
      {/* Column Header */}
      <div class="flex items-center justify-between gap-2 px-3 py-2.5 border-b border-cf-border-subtle">
        <div class="flex items-center gap-2 min-w-0">
          <span class="text-sm font-semibold text-cf-text-primary truncate">
            {props.milestone.title}
          </span>
          <Badge variant={getVariant(roadmapStatusVariant, props.milestone.status)} pill>
            {props.milestone.status}
          </Badge>
        </div>
        <span class="text-xs text-cf-text-muted flex-shrink-0">{features().length}</span>
      </div>

      {/* Drop Zone / Feature List */}
      <div
        class={`flex-1 overflow-y-auto p-2 space-y-2 transition-colors ${
          isDragOver()
            ? "border-2 border-dashed border-cf-accent bg-cf-accent/5"
            : "border-2 border-transparent"
        }`}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
      >
        <For each={features()}>
          {(feature, index) => (
            <>
              {/* Drop indicator line */}
              <Show when={isDragOver() && dropIndex() === index()}>
                <div class="h-0.5 bg-cf-accent rounded-full mx-1" />
              </Show>

              <Show
                when={editingFeature()?.id !== feature.id}
                fallback={
                  <FeatureCardForm
                    milestoneId={props.milestone.id}
                    feature={feature}
                    onSave={() => {
                      setEditingFeature(null);
                      props.onRefetch();
                    }}
                    onCancel={() => setEditingFeature(null)}
                  />
                }
              >
                <div data-feature-index={index()}>
                  <FeatureCard
                    feature={feature}
                    index={index()}
                    milestoneId={props.milestone.id}
                    onStatusToggle={handleStatusToggle}
                    onEdit={handleEditFeature}
                  />
                </div>
              </Show>
            </>
          )}
        </For>

        {/* Drop indicator at end of list */}
        <Show when={isDragOver() && dropIndex() === features().length}>
          <div class="h-0.5 bg-cf-accent rounded-full mx-1" />
        </Show>

        {/* Empty state text when dragging over empty column */}
        <Show when={isDragOver() && features().length === 0}>
          <div class="flex items-center justify-center py-6 text-xs text-cf-text-muted">
            {t("featuremap.dropHere")}
          </div>
        </Show>
      </div>

      {/* Add Feature Section */}
      <div class="border-t border-cf-border-subtle p-2">
        <Show
          when={showAddForm()}
          fallback={
            <Button
              variant="ghost"
              size="sm"
              class="w-full text-cf-text-muted hover:text-cf-text-primary"
              onClick={() => {
                setShowAddForm(true);
                setEditingFeature(null);
              }}
            >
              {t("featuremap.addFeature")}
            </Button>
          }
        >
          <FeatureCardForm
            milestoneId={props.milestone.id}
            onSave={() => {
              setShowAddForm(false);
              props.onRefetch();
            }}
            onCancel={() => setShowAddForm(false)}
          />
        </Show>
      </div>
    </div>
  );
}
