import { createEffect, createSignal, For, on, onCleanup, onMount, Show } from "solid-js";

import { api } from "~/api/client";
import type { PendingReviewDecision, ReviewImpactEvent, ReviewUserEdit } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useWebSocket } from "~/components/WebSocketProvider";
import { useFocusTrap } from "~/hooks/useFocusTrap";
import { useI18n } from "~/i18n";
import { Alert, Button } from "~/ui";

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

/** A refactoring waiting for keep or undo; the WS event lacks the step status. */
type Pending = ReviewImpactEvent & Partial<Pick<PendingReviewDecision, "step_status">>;

const key = (p: Pending): string => `${p.run_id}/${p.step_id}`;

/**
 * Files users changed through the editor or the file API while the
 * refactorer ran (KI-94): the workspace records no writer, so they count as
 * the refactoring's change and an undo sets them back too.
 */
function UserEdits(props: { edits: ReviewUserEdit[]; total: number }) {
  const { t, fmt } = useI18n();
  const more = (): number => props.total - props.edits.length;
  return (
    <Alert variant="warning">
      <p class="font-medium">{t("reviewApproval.userEdits.title")}</p>
      <ul class="mt-1 max-h-40 space-y-1 overflow-y-auto">
        <For each={props.edits}>
          {(edit) => (
            <li>
              <span class="break-all font-mono text-xs">{edit.path}</span>{" "}
              <span class="text-xs opacity-80">
                {t("reviewApproval.userEdits.by", {
                  operation: t(`reviewApproval.userEdits.op.${edit.operation}`),
                  user: edit.user_name || t("reviewApproval.userEdits.unknownUser"),
                  time: fmt.dateTime(edit.edited_at),
                })}
              </span>
            </li>
          )}
        </For>
      </ul>
      <Show when={more() > 0}>
        <p class="mt-1 text-xs">{t("reviewApproval.userEdits.more", { count: more() })}</p>
      </Show>
      <p class="mt-1 text-xs">{t("reviewApproval.userEdits.hint")}</p>
    </Alert>
  );
}

/**
 * Threshold HITL of the review pipeline (KI-17): refactorings that wait for
 * keep or undo - high impact (review.approval_required), or a refactoring
 * step that failed or was cancelled after changing the workspace - are
 * decided here one after the other. The queue is loaded from the server when
 * the dialog mounts and when the WebSocket reconnects, so a missed event
 * loses no decision (S6-F 6); there is no timeout. A medium-impact
 * refactoring was applied and is announced (review.refactor_applied).
 */
export default function RefactorApproval(props: { projectId: string }) {
  const [queue, setQueue] = createSignal<Pending[]>([]);
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal("");
  const { onMessage, connected } = useWebSocket();
  const { show: toast } = useToast();
  const { t } = useI18n();
  let dialogRef: HTMLDivElement | undefined;

  const current = (): Pending | undefined => queue()[0];

  const { onKeyDown: trapKeyDown } = useFocusTrap(
    () => dialogRef,
    () => current() !== undefined,
  );

  // Requests announced while a load runs are kept: the server's answer may
  // predate them.
  let announced = new Map<string, Pending>();

  const load = async (): Promise<void> => {
    announced = new Map();
    try {
      const pending: Pending[] = await api.projects.pendingReviewDecisions(props.projectId);
      const loaded = new Set(pending.map(key));
      setQueue([...pending, ...[...announced.values()].filter((p) => !loaded.has(key(p)))]);
    } catch {
      // Keep what is queued; the next reconnect loads again.
    }
  };

  onMount(() => void load());
  createEffect(
    on(
      () => connected(),
      (isConnected, wasConnected) => {
        if (isConnected && wasConnected === false) void load();
      },
      { defer: true },
    ),
  );

  // eslint-disable-next-line solid/reactivity -- subscription callback, not a reactive computation
  const cleanup = onMessage((msg) => {
    if (!isReviewImpact(msg.payload) || msg.payload.project_id !== props.projectId) return;
    if (msg.type === "review.approval_required") {
      const req = msg.payload;
      // The server could not read the files users changed (KI-94 review F2):
      // load them with the pending decisions. load() starts a new announced
      // set, so this request is added after it and kept if the load fails.
      if (req.user_edits_unavailable) void load();
      announced.set(key(req), req);
      setQueue((q) => (q.some((p) => key(p) === key(req)) ? q : [...q, req]));
    } else if (msg.type === "review.refactor_applied") {
      const p = msg.payload;
      toast(
        "info",
        `Review refactoring applied: ${p.files_changed} file(s), +${p.lines_added} -${p.lines_removed}`,
      );
    }
  });
  onCleanup(cleanup);

  const ended = (p: Pending): boolean =>
    p.step_status === "failed" || p.step_status === "cancelled";

  const decide = async (approve: boolean) => {
    const req = current();
    if (!req) return;
    setLoading(true);
    setError("");
    const step = { plan_id: req.plan_id, step_id: req.step_id };
    try {
      const res = approve
        ? await api.runs.approveRefactor(req.run_id, step)
        : await api.runs.rejectRefactor(req.run_id, step);
      announced.delete(key(req));
      setQueue((q) => q.filter((p) => key(p) !== key(req)));
      if (res.message) toast("warning", res.message);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Show when={current()}>
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
              {ended(req())
                ? `Refactoring ${req().step_status}: keep or undo its change?`
                : "Refactoring Approval Required"}
            </h3>
            <Show when={queue().length > 1}>
              <p class="mb-2 text-xs text-cf-text-muted">
                1 of {queue().length} refactorings waiting for a decision
              </p>
            </Show>

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
              <Show when={req().user_edits_unavailable}>
                <Alert variant="warning">{t("reviewApproval.userEdits.unavailable")}</Alert>
              </Show>
              <Show when={req().user_edits?.length}>
                <UserEdits
                  edits={req().user_edits ?? []}
                  total={Math.max(req().user_edits_total ?? 0, req().user_edits?.length ?? 0)}
                />
              </Show>
              <p class="text-xs text-cf-text-muted">
                Undoing reverts only the refactoring's changes; HEAD moves back only if it still
                points at the refactoring's commit.
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
                {ended(req()) ? "Keep" : "Approve"}
              </Button>
              <Button
                variant="danger"
                size="sm"
                onClick={() => void decide(false)}
                disabled={loading()}
                loading={loading()}
                class="flex-1"
              >
                {ended(req()) ? "Undo" : "Reject"}
              </Button>
            </div>
          </div>
        </div>
      )}
    </Show>
  );
}
