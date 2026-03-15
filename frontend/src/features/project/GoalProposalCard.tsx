import { createSignal, Show } from "solid-js";

import { api } from "~/api/client";
import type { GoalKind } from "~/api/types";
import type { AGUIGoalProposal } from "~/api/websocket";
import { useI18n } from "~/i18n";
import { Badge, Button } from "~/ui";
import type { BadgeVariant } from "~/ui/primitives/Badge";

interface Props {
  proposal: AGUIGoalProposal;
  projectId: string;
  onApprove: (title: string) => void;
  onReject: (title: string) => void;
}

const KIND_LABELS: Record<GoalKind, string> = {
  vision: "Vision",
  requirement: "Requirement",
  constraint: "Constraint",
  state: "Current State",
  context: "Context",
};

const KIND_BADGE_VARIANTS: Record<GoalKind, BadgeVariant> = {
  vision: "success",
  requirement: "info",
  constraint: "warning",
  state: "neutral",
  context: "neutral",
};

/** Maximum characters shown in the content preview before truncation. */
const CONTENT_PREVIEW_LIMIT = 500;

export default function GoalProposalCard(props: Props) {
  const { t } = useI18n();
  const [status, setStatus] = createSignal<"pending" | "approved" | "rejected">("pending");
  const [saving, setSaving] = createSignal(false);

  const handleApprove = async (): Promise<void> => {
    setSaving(true);
    try {
      await api.goals.create(props.projectId, {
        kind: props.proposal.kind,
        title: props.proposal.title,
        content: props.proposal.content,
        priority: props.proposal.priority,
        source: "agent",
      });
      setStatus("approved");
      props.onApprove(props.proposal.title);
    } catch {
      setSaving(false);
    }
  };

  const handleReject = (): void => {
    setStatus("rejected");
    props.onReject(props.proposal.title);
  };

  const contentPreview = (): string => {
    const raw = props.proposal.content;
    if (raw.length <= CONTENT_PREVIEW_LIMIT) return raw;
    return raw.slice(0, CONTENT_PREVIEW_LIMIT) + "...";
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
        <Badge variant={KIND_BADGE_VARIANTS[props.proposal.kind]} pill>
          {KIND_LABELS[props.proposal.kind]}
        </Badge>
        <span class="text-xs text-cf-text-tertiary">
          {props.proposal.action === "create" ? "New Goal" : props.proposal.action}
        </span>
      </div>

      <h4 class="text-sm font-semibold text-cf-text-primary mb-1">{props.proposal.title}</h4>

      <Show when={props.proposal.content}>
        <p class="text-xs text-cf-text-secondary whitespace-pre-wrap mb-3 line-clamp-6">
          {contentPreview()}
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
