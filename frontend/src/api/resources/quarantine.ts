import type { CoreClient } from "../core";
import { url } from "../factory";
import type {
  QuarantineMessage,
  QuarantineReviewRequest,
  QuarantineStats,
  QuarantineStatus,
} from "../types";

export function createQuarantineResource(c: CoreClient) {
  return {
    list: (projectId: string, status?: QuarantineStatus, limit?: number, offset?: number) => {
      const params = new URLSearchParams({ project_id: projectId });
      if (status) params.set("status", status);
      if (limit !== undefined) params.set("limit", String(limit));
      if (offset !== undefined) params.set("offset", String(offset));
      return c.get<QuarantineMessage[]>(`/quarantine?${params.toString()}`);
    },

    get: (id: string) => c.get<QuarantineMessage>(url`/quarantine/${id}`),

    approve: (id: string, data: QuarantineReviewRequest) =>
      c.post<{ status: string }>(url`/quarantine/${id}/approve`, data),

    reject: (id: string, data: QuarantineReviewRequest) =>
      c.post<{ status: string }>(url`/quarantine/${id}/reject`, data),

    stats: (projectId: string) =>
      c.get<QuarantineStats>(`/quarantine/stats?project_id=${encodeURIComponent(projectId)}`),
  };
}
