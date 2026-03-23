import type { CoreClient } from "../core";
import { url } from "../factory";
import type {
  BoundaryConfig,
  Branch,
  CreateProjectRequest,
  GitStatus,
  ParsedRepoURL,
  Project,
  RepoInfo,
  SetupResult,
  UpdateProjectRequest,
} from "../types";

export function createProjectsResource(c: CoreClient) {
  return {
    list: () => c.get<Project[]>("/projects"),

    get: (id: string) => c.get<Project>(url`/projects/${id}`),

    create: (data: CreateProjectRequest) => c.post<Project>("/projects", data),

    update: (id: string, data: UpdateProjectRequest) => c.put<Project>(url`/projects/${id}`, data),

    delete: (id: string) => c.del<undefined>(url`/projects/${id}`),

    parseRepoURL: (repoUrl: string) => c.post<ParsedRepoURL>("/parse-repo-url", { url: repoUrl }),

    repoInfo: (repoUrl: string) =>
      c.get<RepoInfo>(`/repos/info?url=${encodeURIComponent(repoUrl)}`),

    clone: (id: string) => c.post<Project>(url`/projects/${id}/clone`),

    gitStatus: (id: string) => c.get<GitStatus>(url`/projects/${id}/git/status`),

    pull: (id: string) => c.post<{ status: string }>(url`/projects/${id}/git/pull`),

    branches: (id: string) => c.get<Branch[]>(url`/projects/${id}/git/branches`),

    checkout: (id: string, branch: string) =>
      c.post<{ status: string; branch: string }>(url`/projects/${id}/git/checkout`, { branch }),

    setup: (id: string, branch?: string) =>
      c.post<SetupResult>(url`/projects/${id}/setup`, branch ? { branch } : {}),

    adopt: (id: string, body: { path: string }) =>
      c.post<Project>(url`/projects/${id}/adopt`, body),

    initWorkspace: (id: string) => c.post<Project>(url`/projects/${id}/init-workspace`),

    remoteBranches: (repoUrl: string) =>
      c
        .get<{ branches: string[] }>(url`/projects/remote-branches?url=${repoUrl}`)
        .then((r) => r.branches),

    getBoundaries: (id: string) => c.get<BoundaryConfig>(url`/projects/${id}/boundaries`),

    triggerBoundaryAnalysis: (id: string) =>
      c.post<undefined>(url`/projects/${id}/boundaries/analyze`),
  };
}

export function createBatchResource(c: CoreClient) {
  return {
    deleteProjects: (ids: string[]) =>
      c.post<{ id: string; ok: boolean; error?: string }[]>("/projects/batch/delete", { ids }),

    pullProjects: (ids: string[]) =>
      c.post<{ id: string; ok: boolean; error?: string }[]>("/projects/batch/pull", { ids }),

    statusProjects: (ids: string[]) =>
      c.post<{ id: string; ok: boolean; error?: string; status?: GitStatus }[]>(
        "/projects/batch/status",
        { ids },
      ),
  };
}
