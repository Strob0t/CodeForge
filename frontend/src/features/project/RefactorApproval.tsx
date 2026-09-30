import { createSignal, onCleanup, Show } from "solid-js";

import { api } from "~/api/client";
import type { ReviewImpactEvent } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useWebSocket } from "~/components/WebSocketProvider";
import { useFocusTrap } from "~/hooks/useFocusTrap";
import { Button } from "~/ui";

function isReviewImpact(p: unknown): p is ReviewImpactEvent {
  return (
    typeof p === "object" &&
    p !== null &&
    "run_id" in p &&
    "plan_id" in p &&
    "step_id" in p &&
    "project_id" in p
  );
}

/**
 * Threshold HITL of the review pipeline (KI-17): a high-impact refactoring
 * (review.approval_required) waits here for approval or rejection; a
 * medium-impact one was applied and is announced (review.refactor_applied).
 */
export default function RefactorApproval(props: { projectId: string }) {
  const [request, setRequest] = createSignal<ReviewImpactEvent | null>(null);
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal("");
  const { onMessage } = useWebSocket();
  const { show: toast } = useToast();
  let dialogRef: HTMLDivElement | undefined;

  const { onKeyDown: trapKeyDown } = useFocusTrap(
    () => dialogRef,
    () => request() !== null,
  );

  // eslint-disable-next-line solid/reactivity -- subscription callback, not a reactive computation
  const cleanup = onMessage((msg) => {
    if (!isReviewImpact(msg.payload) || msg.payload.project_id !== props.projectId) return;
    if (msg.type === "review.approval_required") {
      setError("");
      setRequest(msg.payload);
    } else if (msg.type === "review.refactor_applied") {
      const p = msg.payload;
      toast(
        "info",
        `Review refactoring applied: ${p.files_changed} file(s), +${p.lines_added} -${p.lines_removed}`,
      );
    }
  });
  onCleanup(cleanup);

  const decide = async (approve: boolean) => {
    const req = request();
    if (!req) return;
    setLoading(true);
    setError("");
    const step = { plan_id: req.plan_id, step_id: req.step_id };
    try {
      if (approve) {
        await api.runs.approveRefactor(req.run_id, step);
      } else {
        await api.runs.rejectRefactor(req.run_id, step);
      }
      setRequest(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Show when={request()}>
      {(req) => (
        <div
          class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
          role="dialog"
          aria-modal="true"
          aria-label="Refactor approval"
          tabIndex={-1}
          onKeyDown={trapKeyDown}
        >
          <div
            ref={dialogRef}
            class="mx-4 w-full max-w-lg rounded-lg bg-cf-bg-surface p-6 shadow-xl"
          >
            <h3 class="mb-4 text-lg font-semibold text-cf-text-primary">
              Refactoring Approval Required
            </h3>

            <div class="mb-4 space-y-2 text-sm text-cf-text-secondary">
              <Show when={req().reason}>
                <div class="rounded bg-cf-warning-bg px-2 py-1 text-cf-warning-fg">
                  {req().reason}
                </div>
              </Show>
              <div class="flex justify-between">
                <span>Files changed:</span>
                <span class="font-mono">{req().files_changed}</span>
              </div>
              <div class="flex justify-between">
                <span>Lines added:</span>
                <span class="font-mono text-cf-success-fg">+{req().lines_added}</span>
              </div>
              <div class="flex justify-between">
                <span>Lines removed:</span>
                <span class="font-mono text-cf-danger-fg">-{req().lines_removed}</span>
              </div>
              <Show when={req().cross_layer}>
                <div class="rounded bg-cf-warning-bg px-2 py-1 text-cf-warning-fg">
                  Cross-layer changes detected
                </div>
              </Show>
              <Show when={req().structural}>
                <div class="rounded bg-cf-danger-bg px-2 py-1 text-cf-danger-fg">
                  Structural changes (files added, deleted or renamed)
                </div>
              </Show>
              <p class="text-xs text-cf-text-muted">
                Rejecting restores the workspace to its state before the review.
              </p>
            </div>

            <Show when={error()}>
              <p class="mb-3 text-sm text-cf-danger-fg" role="alert">
                {error()}
              </p>
            </Show>

            <div class="flex gap-3">
              <Button
                variant="primary"
                size="sm"
                onClick={() => void decide(true)}
                disabled={loading()}
                loading={loading()}
                class="flex-1 bg-cf-success hover:opacity-90"
              >
                Approve
              </Button>
              <Button
                variant="danger"
                size="sm"
                onClick={() => void decide(false)}
                disabled={loading()}
                loading={loading()}
                class="flex-1"
              >
                Reject
              </Button>
            </div>
          </div>
        </div>
      )}
    </Show>
  );
}
