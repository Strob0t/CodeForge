import type { CoreClient } from "../core";
import { url } from "../factory";
import type {
  AIRoadmapView,
  CreateFeatureRequest,
  CreateMilestoneRequest,
  CreateRoadmapRequest,
  DetectionResult,
  ImportResult,
  Milestone,
  PMImportRequest,
  Roadmap,
  RoadmapFeature,
} from "../types";

export function createRoadmapResource(c: CoreClient) {
  return {
    get: (projectId: string) => c.get<Roadmap>(url`/projects/${projectId}/roadmap`),

    create: (projectId: string, data: CreateRoadmapRequest) =>
      c.post<Roadmap>(url`/projects/${projectId}/roadmap`, data),

    update: (projectId: string, data: Partial<Roadmap> & { version: number }) =>
      c.put<Roadmap>(url`/projects/${projectId}/roadmap`, data),

    delete: (projectId: string) => c.del<undefined>(url`/projects/${projectId}/roadmap`),

    ai: (projectId: string, format: "json" | "yaml" | "markdown" = "markdown") =>
      c.get<AIRoadmapView>(url`/projects/${projectId}/roadmap/ai?format=${format}`),

    detect: (projectId: string) =>
      c.post<DetectionResult>(url`/projects/${projectId}/roadmap/detect`),

    createMilestone: (projectId: string, data: CreateMilestoneRequest) =>
      c.post<Milestone>(url`/projects/${projectId}/roadmap/milestones`, data),

    updateMilestone: (id: string, data: Partial<Milestone> & { version: number }) =>
      c.put<Milestone>(url`/milestones/${id}`, data),

    createFeature: (milestoneId: string, data: CreateFeatureRequest) =>
      c.post<RoadmapFeature>(url`/milestones/${milestoneId}/features`, data),

    updateFeature: (id: string, data: Partial<RoadmapFeature> & { version: number }) =>
      c.put<RoadmapFeature>(url`/features/${id}`, data),

    importSpecs: (projectId: string) =>
      c.post<ImportResult>(url`/projects/${projectId}/roadmap/import`),

    importPMItems: (projectId: string, data: PMImportRequest) =>
      c.post<ImportResult>(url`/projects/${projectId}/roadmap/import/pm`, data),

    syncToFile: (projectId: string) =>
      c.post<{ status: string }>(url`/projects/${projectId}/roadmap/sync-to-file`),
  };
}
