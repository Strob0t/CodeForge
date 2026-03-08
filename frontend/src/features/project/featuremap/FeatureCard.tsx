import { createSignal } from "solid-js";
import { For, Show } from "solid-js";

import type { FeatureStatus, RoadmapFeature } from "~/api/types";
import { featureStatusVariant, getVariant } from "~/config/statusVariants";
import { useI18n } from "~/i18n";
import { Badge } from "~/ui";

import { encodeDragPayload, FEATURE_MIME } from "./featuremap-dnd";

interface FeatureCardProps {
  feature: RoadmapFeature;
  index: number;
  milestoneId: string;
  onStatusToggle: (featureId: string, currentStatus: FeatureStatus) => void;
  onEdit: (feature: RoadmapFeature) => void;
}

export default function FeatureCard(props: FeatureCardProps) {
  const { t } = useI18n();
  const [isDragging, setIsDragging] = createSignal(false);

  const handleDragStart = (e: DragEvent) => {
    if (!e.dataTransfer) return;
    e.dataTransfer.effectAllowed = "move";
    e.dataTransfer.setData(
      FEATURE_MIME,
      encodeDragPayload({
        featureId: props.feature.id,
        sourceMilestoneId: props.milestoneId,
        sourceIndex: props.index,
      }),
    );
    setIsDragging(true);
  };

  const handleDragEnd = () => {
    setIsDragging(false);
  };

  return (
    <div
      draggable="true"
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
      class={`rounded-cf-sm border border-cf-border bg-cf-bg-surface px-3 py-2 cursor-grab transition-opacity ${
        isDragging() ? "opacity-50" : ""
      }`}
      title={t("featuremap.dragToMove")}
    >
      <div class="flex items-center justify-between gap-2">
        <div class="flex items-center gap-2 min-w-0">
          {/* Status toggle checkbox */}
          <button
            class={`flex h-4 w-4 flex-shrink-0 items-center justify-center rounded border text-xs ${
              props.feature.status === "done"
                ? "border-cf-success bg-cf-success text-white"
                : "border-cf-border text-transparent hover:border-cf-success"
            }`}
            title={props.feature.status === "done" ? t("roadmap.markTodo") : t("roadmap.markDone")}
            onClick={(e) => {
              e.stopPropagation();
              props.onStatusToggle(props.feature.id, props.feature.status);
            }}
          >
            {props.feature.status === "done" ? "\u2713" : "\u00A0"}
          </button>

          {/* Title (click to edit) */}
          <span
            class={`text-sm truncate cursor-pointer hover:underline ${
              props.feature.status === "done"
                ? "text-cf-text-muted line-through"
                : "text-cf-text-primary"
            }`}
            onClick={(e) => {
              e.stopPropagation();
              props.onEdit(props.feature);
            }}
            title={t("featuremap.editFeature")}
          >
            {props.feature.title}
          </span>
        </div>

        <div class="flex items-center gap-1 flex-shrink-0">
          <Show when={(props.feature.labels ?? []).length > 0}>
            <For each={props.feature.labels}>
              {(label) => <Badge variant="default">{label}</Badge>}
            </For>
          </Show>
          <Badge variant={getVariant(featureStatusVariant, props.feature.status)}>
            {props.feature.status}
          </Badge>
        </div>
      </div>
    </div>
  );
}
