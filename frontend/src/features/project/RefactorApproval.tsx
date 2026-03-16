import { createSignal, onCleanup, Show } from "solid-js";

import { useWebSocket } from "~/components/WebSocketProvider";
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

export default function RefactorApproval() {
  const [request, setRequest] = createSignal<ApprovalRequest | null>(null);
  const [loading, setLoading] = createSignal(false);
  const { onMessage } = useWebSocket();

  const cleanup = onMessage((msg) => {
    if (msg.type === "refactor.approval_required") {
      setRequest(msg.payload as ApprovalRequest);
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
        <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div class="mx-4 w-full max-w-lg rounded-lg bg-white p-6 shadow-xl dark:bg-zinc-800">
            <h3 class="mb-4 text-lg font-semibold text-zinc-900 dark:text-zinc-100">
              Refactoring Approval Required
            </h3>

            <div class="mb-4 space-y-2 text-sm text-zinc-600 dark:text-zinc-300">
              <div class="flex justify-between">
                <span>Files changed:</span>
                <span class="font-mono">{req().files_changed}</span>
              </div>
              <div class="flex justify-between">
                <span>Lines added:</span>
                <span class="font-mono text-green-600">+{req().lines_added}</span>
              </div>
              <div class="flex justify-between">
                <span>Lines removed:</span>
                <span class="font-mono text-red-600">-{req().lines_removed}</span>
              </div>
              <Show when={req().cross_layer}>
                <div class="rounded bg-amber-100 px-2 py-1 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300">
                  Cross-layer changes detected
                </div>
              </Show>
              <Show when={req().structural}>
                <div class="rounded bg-red-100 px-2 py-1 text-red-800 dark:bg-red-900/30 dark:text-red-300">
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
                class="flex-1 bg-green-600 hover:bg-green-700"
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
