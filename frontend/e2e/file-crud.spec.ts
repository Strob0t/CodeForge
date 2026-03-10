import { expect, test } from "./fixtures";

test.describe("File Create and Upload (F1.4)", () => {
  let projectId: string;

  test.beforeEach(async ({ api }) => {
    const project = await api.createProject(`file-crud-${Date.now()}`);
    projectId = project.id;

    // Initialize workspace so files can be written
    const token = await api.getAdminToken();
    await fetch(`http://localhost:8080/api/v1/projects/${projectId}/init-workspace`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}` },
    });
  });

  test("create file via API and verify it appears in file tree", async ({ page, api }) => {
    const token = await api.getAdminToken();

    // Create a file via the API (same as the "Create File" modal does)
    const res = await fetch(`http://localhost:8080/api/v1/projects/${projectId}/files/content`, {
      method: "PUT",
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ path: "hello.py", content: 'print("hello world")\n' }),
    });
    expect(res.ok).toBe(true);

    // Navigate to the project detail page
    await page.goto(`/projects/${projectId}`);
    await page.waitForLoadState("networkidle");

    // Click the "Files" tab
    const filesTab = page.getByRole("button", { name: /files/i });
    if (await filesTab.isVisible()) {
      await filesTab.click();
    }

    // The file should appear in the file tree
    await expect(page.getByText("hello.py")).toBeVisible({ timeout: 10_000 });
  });

  test("read back created file content via API", async ({ api }) => {
    const token = await api.getAdminToken();

    // Write a file
    const writeRes = await fetch(
      `http://localhost:8080/api/v1/projects/${projectId}/files/content`,
      {
        method: "PUT",
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ path: "test.txt", content: "Test content 123" }),
      },
    );
    expect(writeRes.ok).toBe(true);

    // Read it back
    const readRes = await fetch(
      `http://localhost:8080/api/v1/projects/${projectId}/files/content?path=test.txt`,
      {
        headers: { Authorization: `Bearer ${token}` },
      },
    );
    expect(readRes.ok).toBe(true);
    const body = await readRes.json();
    expect(body.content).toBe("Test content 123");
  });

  test("overwrite file replaces content", async ({ api }) => {
    const token = await api.getAdminToken();
    const url = `http://localhost:8080/api/v1/projects/${projectId}/files/content`;
    const headers = {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    };

    // Write v1
    await fetch(url, {
      method: "PUT",
      headers,
      body: JSON.stringify({ path: "config.yaml", content: "version: 1" }),
    });

    // Overwrite with v2
    await fetch(url, {
      method: "PUT",
      headers,
      body: JSON.stringify({ path: "config.yaml", content: "version: 2" }),
    });

    // Read back
    const res = await fetch(
      `http://localhost:8080/api/v1/projects/${projectId}/files/content?path=config.yaml`,
      { headers: { Authorization: `Bearer ${token}` } },
    );
    const body = await res.json();
    expect(body.content).toBe("version: 2");
  });

  test("create file with special characters in name", async ({ api }) => {
    const token = await api.getAdminToken();

    const res = await fetch(`http://localhost:8080/api/v1/projects/${projectId}/files/content`, {
      method: "PUT",
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ path: "my-file_v2.txt", content: "special chars ok\n" }),
    });
    expect(res.ok).toBe(true);

    // Read back
    const readRes = await fetch(
      `http://localhost:8080/api/v1/projects/${projectId}/files/content?path=my-file_v2.txt`,
      { headers: { Authorization: `Bearer ${token}` } },
    );
    expect(readRes.ok).toBe(true);
    const body = await readRes.json();
    expect(body.content).toBe("special chars ok\n");
  });
});
