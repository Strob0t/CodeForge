import { createSignal, Show } from "solid-js";
import { For } from "solid-js";

import type { FeatureStatus, RoadmapFeature } from "~/api/types";
import { featureStatusVariant, getVariant } from "~/config/statusVariants";
import { useI18n } from "~/i18n";
import { Badge } from "~/ui";

import PanelChatLink from "../PanelChatLink";
import { encodeDragPayload, FEATURE_MIME } from "./featuremap-dnd";

interface FeatureCardProps {
  feature: RoadmapFeature;
  index: number;
  milestoneId: string;
  onStatusToggle: (featureId: string, currentStatus: FeatureStatus) => void;
  onEdit: (feature: RoadmapFeature) => void;
  onSendChatMessage?: (msg: string) => void;
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
      <div class="flex items-start justify-between gap-2">
        <div class="flex items-start gap-2 min-w-0">
          {/* Status toggle checkbox */}
          <button
            role="checkbox"
            aria-checked={props.feature.status === "done"}
            class={`mt-0.5 flex h-4 w-4 flex-shrink-0 items-center justify-center rounded border text-xs ${
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

          {/* Title (click to edit): two lines at most, in full on hover (KI-129) */}
          <span
            class={`text-sm line-clamp-2 break-words cursor-pointer hover:underline ${
              props.feature.status === "done"
                ? "text-cf-text-muted line-through"
                : "text-cf-text-primary"
            }`}
            onClick={(e) => {
              e.stopPropagation();
              props.onEdit(props.feature);
            }}
            title={props.feature.title}
          >
            {props.feature.title}
          </span>
        </div>

        <div class="flex items-center gap-1 flex-shrink-0">
          <Show when={props.onSendChatMessage}>
            <PanelChatLink
              type="feature"
              id={props.feature.id}
              title={props.feature.title}
              onDiscuss={(msg) => props.onSendChatMessage?.(msg)}
            />
          </Show>
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
      {/* How the auto-agent's verification ended (KI-152) */}
      <Show when={props.feature.result}>
        {(result) => (
          <p class="mt-1 text-xs text-cf-text-muted line-clamp-2" title={result()}>
            {result()}
          </p>
        )}
      </Show>
    </div>
  );
}
