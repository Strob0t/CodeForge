import type { AGUIPermissionRequest } from "~/api/websocket";

/**
 * The approval cards after a reload restored the running turn (KI-148): the
 * cards already shown stay, restored approvals not yet shown are added.
 */
export function mergePermissionRequests(
  shown: AGUIPermissionRequest[],
  restored: AGUIPermissionRequest[],
): AGUIPermissionRequest[] {
  const ids = new Set(shown.map((pr) => pr.call_id));
  return [...shown, ...restored.filter((pr) => !ids.has(pr.call_id))];
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
