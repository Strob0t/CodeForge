import type { AGUIPermissionRequest } from "~/api/websocket";

/** An approval is one call of one run: call IDs repeat across runs. */
function approvalKey(pr: AGUIPermissionRequest): string {
  return `${pr.run_id}\u0000${pr.call_id}`;
}

/**
 * The approval cards after a reload restored the running turn (KI-148): the
 * cards already shown stay, restored approvals not yet shown are added.
 */
export function mergePermissionRequests(
  shown: AGUIPermissionRequest[],
  restored: AGUIPermissionRequest[],
): AGUIPermissionRequest[] {
  const keys = new Set(shown.map(approvalKey));
  return [...shown, ...restored.filter((pr) => !keys.has(approvalKey(pr)))];
}

/**
 * The approval cards after a live permission_request event, which can
 * arrive after a restore already showed its card: shown at most once.
 */
export function addPermissionRequest(
  shown: AGUIPermissionRequest[],
  pr: AGUIPermissionRequest,
): AGUIPermissionRequest[] {
  const key = approvalKey(pr);
  return shown.some((s) => approvalKey(s) === key) ? shown : [...shown, pr];
}

/**
 * The streamed text after a reload restored the running turn (KI-148).
 * `snapshot` is what the Core saw the turn stream until it answered;
 * `sinceRequest` is what arrived over the WebSocket after the request was
 * sent, whose start may already be in the snapshot.
 */
export function mergeStreamedText(snapshot: string, sinceRequest: string): string {
  for (let k = Math.min(snapshot.length, sinceRequest.length); k > 0; k--) {
    if (snapshot.endsWith(sinceRequest.slice(0, k))) {
      return snapshot + sinceRequest.slice(k);
    }
  }
  return snapshot + sinceRequest;
}
