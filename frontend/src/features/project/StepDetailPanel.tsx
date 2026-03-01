import { Show } from "solid-js";

import type { DebateStatusEvent, PlanGraphNode, ReviewDecisionSnapshot } from "~/api/types";
import { getVariant, nodeStatusVariant } from "~/config/statusVariants";
import { useI18n } from "~/i18n";
import { Badge, Card } from "~/ui";

interface StepDetailPanelProps {
  step: PlanGraphNode;
  taskName: string;
  agentName: string;
  reviewDecision?: ReviewDecisionSnapshot;
  debateStatus?: DebateStatusEvent;
  onClose: () => void;
}

export default function StepDetailPanel(props: StepDetailPanelProps) {
  const { t } = useI18n();

  return (
    <Card>
      <Card.Header>
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <h4 class="text-sm font-semibold">{props.taskName}</h4>
            <Badge variant={getVariant(nodeStatusVariant, props.step.status)}>
              {props.step.status}
            </Badge>
          </div>
          <button
            class="text-cf-text-muted hover:text-cf-text-primary text-xs"
            onClick={() => props.onClose()}
            aria-label={t("common.close")}
          >
            {t("common.close")}
          </button>
        </div>
      </Card.Header>
      <Card.Body>
        <div class="space-y-3 text-xs">
          {/* Step metadata */}
          <div class="grid grid-cols-2 gap-2">
            <div>
              <span class="font-medium text-cf-text-tertiary">Agent</span>
              <p class="text-cf-text-secondary">{props.agentName}</p>
            </div>
            <Show when={props.step.mode_id}>
              <div>
                <span class="font-medium text-cf-text-tertiary">Mode</span>
                <p class="text-cf-text-secondary">{props.step.mode_id}</p>
              </div>
            </Show>
            <Show when={props.step.run_id}>
              <div>
                <span class="font-medium text-cf-text-tertiary">Run ID</span>
                <p class="font-mono text-cf-text-secondary">
                  {(props.step.run_id ?? "").slice(0, 12)}
                </p>
              </div>
            </Show>
            <Show when={props.step.round > 0}>
              <div>
                <span class="font-medium text-cf-text-tertiary">Round</span>
                <p class="text-cf-text-secondary">{props.step.round}</p>
              </div>
            </Show>
          </div>

          {/* Dependencies */}
          <Show when={props.step.depends_on && props.step.depends_on.length > 0}>
            <div>
              <span class="font-medium text-cf-text-tertiary">Dependencies</span>
              <p class="text-cf-text-secondary">
                {(props.step.depends_on ?? []).map((id) => id.slice(0, 8)).join(", ")}
              </p>
            </div>
          </Show>

          {/* Review Decision */}
          <Show when={props.reviewDecision}>
            {(rd) => (
              <div class="rounded-cf-sm border border-cf-border-subtle p-2">
                <div class="mb-1 flex items-center gap-2">
                  <span class="font-medium text-cf-text-tertiary">
                    {t("plan.step.reviewDecision")}
                  </span>
                  <Badge variant={rd().routed ? "warning" : "success"} pill>
                    {rd().routed ? t("plan.review.routedToReview") : t("plan.review.autoProceed")}
                  </Badge>
                </div>
                <div class="space-y-0.5">
                  <p class="text-cf-text-secondary">
                    {t("plan.review.confidence")}: {Math.round(rd().confidence * 100)}%
                  </p>
                  <p class="text-cf-text-secondary">{rd().reason}</p>
                </div>
              </div>
            )}
          </Show>

          {/* Debate Status */}
          <Show when={props.debateStatus}>
            {(debate) => (
              <div class="rounded-cf-sm border border-cf-border-subtle p-2">
                <div class="mb-1 flex items-center gap-2">
                  <span class="font-medium text-cf-text-tertiary">{t("plan.step.debate")}</span>
                  <Badge
                    variant={
                      debate().status === "completed"
                        ? "success"
                        : debate().status === "failed"
                          ? "danger"
                          : "info"
                    }
                    pill
                  >
                    {debate().status}
                  </Badge>
                </div>
                <div class="space-y-1">
                  <div class="flex gap-2 text-cf-text-secondary">
                    <span class="font-medium text-cf-text-tertiary">
                      {t("plan.step.proponent")}:
                    </span>
                    <span>Defends approach with evidence</span>
                  </div>
                  <div class="flex gap-2 text-cf-text-secondary">
                    <span class="font-medium text-cf-text-tertiary">
                      {t("plan.step.moderator")}:
                    </span>
                    <span>Synthesizes critique and revision</span>
                  </div>
                  <Show when={debate().synthesis}>
                    <div class="mt-1 rounded bg-cf-bg-tertiary p-1.5">
                      <span class="font-medium text-cf-text-tertiary">
                        {t("plan.step.synthesis")}:
                      </span>
                      <p class="mt-0.5 whitespace-pre-wrap text-cf-text-secondary">
                        {debate().synthesis}
                      </p>
                    </div>
                  </Show>
                </div>
              </div>
            )}
          </Show>

          {/* Error */}
          <Show when={props.step.error}>
            <div class="rounded-cf-sm border border-cf-danger-border bg-cf-danger-bg p-2">
              <span class="font-medium text-cf-danger-fg">Error</span>
              <p class="text-cf-danger-fg">{props.step.error}</p>
            </div>
          </Show>
        </div>
      </Card.Body>
    </Card>
  );
}
