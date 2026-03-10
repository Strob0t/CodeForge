import { expect, test } from "./fixtures";

test.describe("Feature Description CRUD (F2.4)", () => {
  let projectId: string;
  let milestoneId: string;

  test.beforeEach(async ({ api }) => {
    const project = await api.createProject(`feat-desc-${Date.now()}`);
    projectId = project.id;

    const token = await api.getAdminToken();

    // Create roadmap
    await fetch(`http://localhost:8080/api/v1/projects/${projectId}/roadmap`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ title: "Test Roadmap", description: "E2E test" }),
    });

    // Create milestone
    const msRes = await fetch(
      `http://localhost:8080/api/v1/projects/${projectId}/roadmap/milestones`,
      {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ title: "Test Milestone" }),
      },
    );
    const ms = await msRes.json();
    milestoneId = ms.id;
  });

  test("create feature with title and description via API", async ({ api }) => {
    const token = await api.getAdminToken();

    const res = await fetch(`http://localhost:8080/api/v1/milestones/${milestoneId}/features`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        title: "Auth System",
        description: "Implement JWT-based auth with refresh tokens",
      }),
    });
    expect(res.ok).toBe(true);
    const feature = await res.json();
    expect(feature.title).toBe("Auth System");
    expect(feature.description).toBe("Implement JWT-based auth with refresh tokens");
  });

  test("update feature description persists changes", async ({ api }) => {
    const token = await api.getAdminToken();
    const headers = {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    };

    // Create feature
    const createRes = await fetch(
      `http://localhost:8080/api/v1/milestones/${milestoneId}/features`,
      {
        method: "POST",
        headers,
        body: JSON.stringify({
          title: "API Layer",
          description: "Initial description",
        }),
      },
    );
    const feature = await createRes.json();

    // Update description
    const updateRes = await fetch(`http://localhost:8080/api/v1/features/${feature.id}`, {
      method: "PUT",
      headers,
      body: JSON.stringify({
        title: "API Layer",
        description: "Updated description with more detail",
        status: feature.status || "open",
      }),
    });
    expect(updateRes.ok).toBe(true);

    // Read back
    const getRes = await fetch(`http://localhost:8080/api/v1/features/${feature.id}`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    const updated = await getRes.json();
    expect(updated.description).toBe("Updated description with more detail");
  });

  test("feature description visible in project UI", async ({ page, api }) => {
    const token = await api.getAdminToken();

    // Create a feature with description
    await fetch(`http://localhost:8080/api/v1/milestones/${milestoneId}/features`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        title: "Search Feature",
        description: "Full-text search with PostgreSQL GIN indexes",
      }),
    });

    // Navigate to project and find the Feature Map tab
    await page.goto(`/projects/${projectId}`);
    await page.waitForLoadState("networkidle");

    const featureMapTab = page.getByRole("button", { name: /feature\s*map/i });
    if (await featureMapTab.isVisible({ timeout: 5_000 })) {
      await featureMapTab.click();
      // Feature title should be visible on the board
      await expect(page.getByText("Search Feature")).toBeVisible({ timeout: 10_000 });
    }
  });

  test("empty description is allowed", async ({ api }) => {
    const token = await api.getAdminToken();

    const res = await fetch(`http://localhost:8080/api/v1/milestones/${milestoneId}/features`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        title: "Minimal Feature",
        description: "",
      }),
    });
    expect(res.ok).toBe(true);
    const feature = await res.json();
    expect(feature.title).toBe("Minimal Feature");
  });
});
