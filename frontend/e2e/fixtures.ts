import { test as base } from "@playwright/test";

const API_BASE = "http://localhost:8080/api/v1";

interface Project {
  id: string;
  name: string;
}

interface ApiHelper {
  createProject(name: string, description?: string): Promise<Project>;
  deleteProject(id: string): Promise<void>;
  listProjects(): Promise<Project[]>;
  deleteAllProjects(): Promise<void>;
}

export const test = base.extend<{ api: ApiHelper }>({
  api: async ({}, use) => {
    const createdIds: string[] = [];

    const helper: ApiHelper = {
      async createProject(name: string, description = ""): Promise<Project> {
        const res = await fetch(`${API_BASE}/projects`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ name, description, repo_url: "", provider: "", config: {} }),
        });
        if (!res.ok) throw new Error(`Failed to create project: ${res.status}`);
        const project = (await res.json()) as Project;
        createdIds.push(project.id);
        return project;
      },

      async deleteProject(id: string): Promise<void> {
        const res = await fetch(`${API_BASE}/projects/${encodeURIComponent(id)}`, {
          method: "DELETE",
        });
        if (!res.ok && res.status !== 404) {
          throw new Error(`Failed to delete project: ${res.status}`);
        }
      },

      async listProjects(): Promise<Project[]> {
        const res = await fetch(`${API_BASE}/projects`);
        if (!res.ok) throw new Error(`Failed to list projects: ${res.status}`);
        return (await res.json()) as Project[];
      },

      async deleteAllProjects(): Promise<void> {
        const projects = await helper.listProjects();
        await Promise.all(projects.map((p) => helper.deleteProject(p.id)));
      },
    };

    await use(helper);

    // Teardown: clean up any projects created during the test
    for (const id of createdIds) {
      await helper.deleteProject(id).catch(() => {});
    }
  },
});

export { expect } from "@playwright/test";
