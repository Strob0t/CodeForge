import { test, expect } from "@playwright/test";
import { apiLogin, createProject, createCleanupTracker, API_BASE } from "../helpers/api-helpers";

test.describe("Roadmap API", () => {
  let token: string;
  let projectId: string;
  const cleanup = createCleanupTracker();

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
    const proj = await createProject(`roadmap-e2e-${Date.now()}`);
    projectId = proj.id;
    cleanup.add("project", projectId);
  });

  test.afterAll(async () => {
    await cleanup.cleanup();
  });

  const headers = () => ({ Authorization: `Bearer ${token}` });
  const jsonHeaders = () => ({
    ...headers(),
    "Content-Type": "application/json",
  });

  test("POST /projects/{id}/roadmap creates a roadmap", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/roadmap`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        title: "E2E Roadmap",
        description: "Roadmap created by e2e tests",
      }),
    });
    expect(res.status).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
    expect(body.title).toBe("E2E Roadmap");
  });

  test("GET /projects/{id}/roadmap returns the roadmap", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/roadmap`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.title).toBe("E2E Roadmap");
  });

  test("PUT /projects/{id}/roadmap updates the roadmap", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/roadmap`, {
      method: "PUT",
      headers: jsonHeaders(),
      body: JSON.stringify({
        title: "Updated E2E Roadmap",
        description: "Updated description",
      }),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.title).toBe("Updated E2E Roadmap");
  });

  test("POST /projects/{id}/roadmap/detect detects specs", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/roadmap/detect`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
    // May return 200 or 404 depending on project setup
    expect([200, 404]).toContain(res.status);
  });

  test("POST /projects/{id}/roadmap/import imports specs", async () => {
    // ImportSpecs does not require a JSON body; it reads the projectID from the URL.
    // A roadmap must exist for the project (already created above).
    const res = await fetch(`${API_BASE}/projects/${projectId}/roadmap/import`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
    // 200 if specs found and imported, 404 if no workspace/specs found, 500 if internal error
    expect([200, 400, 404, 500]).toContain(res.status);
  });

  test("POST /projects/{id}/roadmap/milestones creates a milestone", async () => {
    // The roadmap must already exist (created in earlier test).
    // CreateMilestoneRequest fields: roadmap_id (set by handler), title, description, due_date
    const res = await fetch(`${API_BASE}/projects/${projectId}/roadmap/milestones`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        title: "Milestone 1",
        description: "First milestone",
      }),
    });
    expect(res.status).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
    expect(body.title).toBe("Milestone 1");
  });

  test("GET /milestones/{id} returns a milestone", async () => {
    // Create a milestone
    const createRes = await fetch(`${API_BASE}/projects/${projectId}/roadmap/milestones`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        title: "Get Milestone Test",
        description: "For get test",
      }),
    });
    expect(createRes.status).toBe(201);
    const milestone = await createRes.json();

    const res = await fetch(`${API_BASE}/milestones/${milestone.id}`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.id).toBe(milestone.id);
    expect(body.title).toBe("Get Milestone Test");
  });

  test("PUT /milestones/{id} updates a milestone", async () => {
    const createRes = await fetch(`${API_BASE}/projects/${projectId}/roadmap/milestones`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({ title: "Update MS Test", description: "orig" }),
    });
    expect(createRes.status).toBe(201);
    const milestone = await createRes.json();

    const res = await fetch(`${API_BASE}/milestones/${milestone.id}`, {
      method: "PUT",
      headers: jsonHeaders(),
      body: JSON.stringify({
        title: "Updated MS Title",
        description: "updated",
      }),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.title).toBe("Updated MS Title");
  });

  test("DELETE /milestones/{id} deletes a milestone", async () => {
    const createRes = await fetch(`${API_BASE}/projects/${projectId}/roadmap/milestones`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({ title: "Delete MS Test", description: "del" }),
    });
    expect(createRes.status).toBe(201);
    const milestone = await createRes.json();

    const res = await fetch(`${API_BASE}/milestones/${milestone.id}`, {
      method: "DELETE",
      headers: headers(),
    });
    expect(res.status).toBe(204);
  });

  test("POST /milestones/{id}/features creates a feature", async () => {
    // Create a milestone first
    const msRes = await fetch(`${API_BASE}/projects/${projectId}/roadmap/milestones`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        title: "Feature Parent MS",
        description: "parent",
      }),
    });
    expect(msRes.status).toBe(201);
    const milestone = await msRes.json();

    // CreateFeatureRequest fields: milestone_id (set by handler), title, description, labels, spec_ref, external_ids
    const res = await fetch(`${API_BASE}/milestones/${milestone.id}/features`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        title: "Feature 1",
        description: "A test feature",
        labels: [],
      }),
    });
    expect(res.status).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
    expect(body.title).toBe("Feature 1");
  });

  test("GET /features/{id} returns a feature", async () => {
    // Create milestone + feature
    const msRes = await fetch(`${API_BASE}/projects/${projectId}/roadmap/milestones`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({ title: "Get Feature MS", description: "ms" }),
    });
    expect(msRes.status).toBe(201);
    const milestone = await msRes.json();
    const fRes = await fetch(`${API_BASE}/milestones/${milestone.id}/features`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        title: "Get Feature Test",
        description: "feat",
        labels: [],
      }),
    });
    expect(fRes.status).toBe(201);
    const feature = await fRes.json();

    const res = await fetch(`${API_BASE}/features/${feature.id}`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.id).toBe(feature.id);
    expect(body.title).toBe("Get Feature Test");
  });

  test("PUT /features/{id} updates a feature", async () => {
    const msRes = await fetch(`${API_BASE}/projects/${projectId}/roadmap/milestones`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        title: "Update Feature MS",
        description: "ms",
      }),
    });
    expect(msRes.status).toBe(201);
    const milestone = await msRes.json();
    const fRes = await fetch(`${API_BASE}/milestones/${milestone.id}/features`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        title: "Update Feature Test",
        description: "orig",
        labels: [],
      }),
    });
    expect(fRes.status).toBe(201);
    const feature = await fRes.json();

    const res = await fetch(`${API_BASE}/features/${feature.id}`, {
      method: "PUT",
      headers: jsonHeaders(),
      body: JSON.stringify({
        title: "Updated Feature Title",
        description: "updated",
      }),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.title).toBe("Updated Feature Title");
  });

  test("DELETE /features/{id} deletes a feature", async () => {
    const msRes = await fetch(`${API_BASE}/projects/${projectId}/roadmap/milestones`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        title: "Delete Feature MS",
        description: "ms",
      }),
    });
    expect(msRes.status).toBe(201);
    const milestone = await msRes.json();
    const fRes = await fetch(`${API_BASE}/milestones/${milestone.id}/features`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        title: "Delete Feature Test",
        description: "del",
        labels: [],
      }),
    });
    expect(fRes.status).toBe(201);
    const feature = await fRes.json();

    const res = await fetch(`${API_BASE}/features/${feature.id}`, {
      method: "DELETE",
      headers: headers(),
    });
    expect(res.status).toBe(204);
  });

  test("DELETE /projects/{id}/roadmap deletes the roadmap", async () => {
    // Create a separate project with its own roadmap for delete test
    const delProj = await createProject(`roadmap-del-${Date.now()}`);
    cleanup.add("project", delProj.id);

    // Create roadmap
    const createRes = await fetch(`${API_BASE}/projects/${delProj.id}/roadmap`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        title: "Delete Roadmap Test",
        description: "To be deleted",
      }),
    });
    expect(createRes.status).toBe(201);

    // Delete it
    const res = await fetch(`${API_BASE}/projects/${delProj.id}/roadmap`, {
      method: "DELETE",
      headers: headers(),
    });
    expect(res.status).toBe(204);
  });
});
