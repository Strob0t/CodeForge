import { createSignal, Show } from "solid-js";

import { api } from "~/api/client";
import type { AGUIRoadmapProposal } from "~/api/websocket";
import { useI18n } from "~/i18n";
import { Badge, Button } from "~/ui";
import type { BadgeVariant } from "~/ui/primitives/Badge";

interface Props {
  proposal: AGUIRoadmapProposal;
  projectId: string;
  onApprove: (title: string) => void;
  onReject: (title: string) => void;
}

const COMPLEXITY_BADGE_VARIANTS: Record<string, BadgeVariant> = {
  trivial: "success",
  simple: "info",
  medium: "warning",
  complex: "danger",
};

export default function RoadmapProposalCard(props: Props) {
  const { t } = useI18n();
  const [status, setStatus] = createSignal<"pending" | "approved" | "rejected">("pending");
  const [saving, setSaving] = createSignal(false);

  const displayTitle = (): string =>
    props.proposal.action === "create_step" && props.proposal.step_title
      ? props.proposal.step_title
      : props.proposal.milestone_title;

  const handleApprove = async (): Promise<void> => {
    setSaving(true);
    try {
      if (props.proposal.action === "create_milestone") {
        await api.roadmap.createMilestone(props.projectId, {
          title: props.proposal.milestone_title,
          description: props.proposal.milestone_description,
          sort_order: props.proposal.milestone_sort_order,
        });
      } else {
        // create_step: find milestone by title, then create feature
        const roadmap = await api.roadmap.get(props.projectId);
        const milestone = roadmap.milestones?.find(
          (m) => m.title === props.proposal.milestone_title,
        );
        if (!milestone) {
          throw new Error(`Milestone "${props.proposal.milestone_title}" not found`);
        }
        await api.roadmap.createFeature(milestone.id, {
          title: props.proposal.step_title ?? props.proposal.milestone_title,
          description: props.proposal.step_description,
          sort_order: props.proposal.step_sort_order,
        });
      }
      setStatus("approved");
      props.onApprove(displayTitle());
    } catch {
      setSaving(false);
    }
  };

  const handleReject = (): void => {
    setStatus("rejected");
    props.onReject(displayTitle());
  };

  const cardBorder = (): string => {
    switch (status()) {
      case "approved":
        return "border-cf-success-border bg-cf-success-bg/30";
      case "rejected":
        return "border-cf-danger-border/30 bg-cf-danger-bg/20 opacity-60";
      default:
        return "border-cf-border bg-cf-bg-secondary";
    }
  };

  return (
    <div class={`rounded-cf-sm border p-3 my-2 ${cardBorder()}`}>
      <div class="flex items-center gap-2 mb-2">
        <Badge variant={props.proposal.action === "create_milestone" ? "info" : "neutral"} pill>
          {props.proposal.action === "create_milestone" ? "Milestone" : "Work Step"}
        </Badge>
        <Show when={props.proposal.step_complexity}>
          <Badge
            variant={COMPLEXITY_BADGE_VARIANTS[props.proposal.step_complexity ?? ""] ?? "neutral"}
            pill
          >
            {props.proposal.step_complexity}
          </Badge>
        </Show>
        <Show when={props.proposal.step_model_tier}>
          <span class="text-xs text-cf-text-tertiary">Model: {props.proposal.step_model_tier}</span>
        </Show>
      </div>

      <h4 class="text-sm font-semibold text-cf-text-primary mb-1">{displayTitle()}</h4>

      <Show when={props.proposal.action === "create_step" && props.proposal.step_description}>
        <p class="text-xs text-cf-text-secondary whitespace-pre-wrap mb-3 line-clamp-6">
          {props.proposal.step_description}
        </p>
      </Show>
      <Show
        when={props.proposal.action === "create_milestone" && props.proposal.milestone_description}
      >
        <p class="text-xs text-cf-text-secondary whitespace-pre-wrap mb-3 line-clamp-6">
          {props.proposal.milestone_description}
        </p>
      </Show>

      <Show
        when={status() === "pending"}
        fallback={
          <span
            class={`text-xs font-medium ${
              status() === "approved" ? "text-cf-success-fg" : "text-cf-danger-fg"
            }`}
          >
            {status() === "approved" ? "\u2713 Approved" : "\u2717 Rejected"}
          </span>
        }
      >
        <div class="flex items-center gap-2">
          <Button
            variant="primary"
            size="xs"
            loading={saving()}
            disabled={saving()}
            onClick={handleApprove}
          >
            {saving() ? t("common.saving") : t("common.approve")}
          </Button>
          <Button variant="secondary" size="xs" onClick={handleReject}>
            {t("common.reject")}
          </Button>
        </div>
      </Show>
    </div>
  );
}
