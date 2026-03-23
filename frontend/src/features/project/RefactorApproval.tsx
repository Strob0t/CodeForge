import { createSignal, onCleanup, Show } from "solid-js";

import { useWebSocket } from "~/components/WebSocketProvider";
import { useFocusTrap } from "~/hooks/useFocusTrap";
import { Button } from "~/ui";

interface ApprovalRequest {
  run_id: string;
  plan_id: string;
  step_id: string;
  files_changed: number;
  lines_added: number;
  lines_removed: number;
  cross_layer: boolean;
  structural: boolean;
}

function isApprovalRequest(p: unknown): p is ApprovalRequest {
  return typeof p === "object" && p !== null && "run_id" in p && "plan_id" in p && "step_id" in p;
}

export default function RefactorApproval() {
  const [request, setRequest] = createSignal<ApprovalRequest | null>(null);
  const [loading, setLoading] = createSignal(false);
  const { onMessage } = useWebSocket();
  let dialogRef: HTMLDivElement | undefined;

  const { onKeyDown: trapKeyDown } = useFocusTrap(
    () => dialogRef,
    () => request() !== null,
  );

  const cleanup = onMessage((msg) => {
    if (msg.type === "refactor.approval_required" && isApprovalRequest(msg.payload)) {
      setRequest(msg.payload);
    }
  });
  onCleanup(cleanup);

  const handleApprove = async () => {
    const req = request();
    if (!req) return;
    setLoading(true);
    try {
      await fetch(`/api/v1/runs/${req.run_id}/approve`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ plan_id: req.plan_id, step_id: req.step_id }),
      });
      setRequest(null);
    } finally {
      setLoading(false);
    }
  };

  const handleReject = async () => {
    const req = request();
    if (!req) return;
    setLoading(true);
    try {
      await fetch(`/api/v1/runs/${req.run_id}/reject`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ plan_id: req.plan_id, step_id: req.step_id }),
      });
      setRequest(null);
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
                  Structural changes (file moves/deletes)
                </div>
              </Show>
            </div>

            <div class="flex gap-3">
              <Button
                variant="primary"
                size="sm"
                onClick={handleApprove}
                disabled={loading()}
                loading={loading()}
                class="flex-1 bg-cf-success hover:opacity-90"
              >
                {loading() ? "..." : "Approve"}
              </Button>
              <Button
                variant="danger"
                size="sm"
                onClick={handleReject}
                disabled={loading()}
                loading={loading()}
                class="flex-1"
              >
                {loading() ? "..." : "Reject"}
              </Button>
            </div>
          </div>
        </div>
      )}
    </Show>
  );
}
