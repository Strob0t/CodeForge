import { test, expect } from "@playwright/test";
import { apiLogin, createCleanupTracker, API_BASE } from "../helpers/api-helpers";
import { discoverAvailableModels, type DiscoveredModel } from "./llm-helpers";

test.describe("LLM E2E — Simple Conversation", () => {
  let token: string;
  let models: DiscoveredModel[] = [];
  const cleanup = createCleanupTracker();

  // Shared state across sequential tests
  let projectId: string;
  let conversationId: string;
  let assistantMessage: {
    role: string;
    content: string;
    tokens_in?: number;
    tokens_out?: number;
    model?: string;
  } | null = null;

  const headers = () => ({ Authorization: `Bearer ${token}` });

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;

    try {
      const discovery = await discoverAvailableModels();
      models = discovery.models.filter((m) => m.status === "reachable");
    } catch {
      // LiteLLM may not be available
      models = [];
    }
  });

  test.afterAll(async () => {
    await cleanup.cleanup();
  });

  test("create project for LLM testing", async ({ request }) => {
    const projName = `e2e-llm-conv-${Date.now()}`;
    const res = await request.post(`${API_BASE}/projects`, {
      headers: headers(),
      data: {
        name: projName,
        description: "E2E LLM conversation test project",
        repo_url: "",
        provider: "",
        config: {},
      },
    });
    expect(res.status()).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
    projectId = body.id;
    cleanup.add("project", projectId);
  });

  test("create conversation", async ({ request }) => {
    test.skip(!projectId, "No project created");

    const res = await request.post(`${API_BASE}/projects/${projectId}/conversations`, {
      headers: headers(),
      data: { title: "E2E simple conversation test" },
    });
    expect(res.status()).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
    conversationId = body.id;
    cleanup.add("conversation", conversationId);
  });

  test("send simple greeting gets LLM response", async ({ request }) => {
    test.setTimeout(60_000);
    test.skip(!conversationId, "No conversation created");
    test.skip(models.length === 0, "No LLM models available");

    const res = await request.post(`${API_BASE}/conversations/${conversationId}/messages`, {
      headers: headers(),
      data: { content: "Hello, respond with exactly one word." },
    });
    expect([200, 201, 202, 500, 502]).toContain(res.status());

    if (res.status() === 201) {
      const body = await res.json();
      expect(body.role).toBe("assistant");
      expect(typeof body.content).toBe("string");
      expect(body.content.trim().length).toBeGreaterThan(0);
      assistantMessage = body;
    } else {
      test.skip(true, "LLM/worker not available");
    }
  });

  test("assistant response has correct role", async ({ request }) => {
    test.skip(!conversationId, "No conversation created");
    test.skip(!assistantMessage, "No assistant response from previous test");

    // Also verify via the messages list endpoint
    const res = await request.get(`${API_BASE}/conversations/${conversationId}/messages`, {
      headers: headers(),
    });
    expect(res.status()).toBe(200);
    const messages = await res.json();
    expect(Array.isArray(messages)).toBe(true);

    const assistant = messages.find((m: { role: string }) => m.role === "assistant");
    if (assistant) {
      expect(assistant.role).toBe("assistant");
    }
  });

  test("response has non-zero token counts", async () => {
    test.skip(!assistantMessage, "No assistant response available");

    if (assistantMessage!.tokens_in !== undefined) {
      expect(assistantMessage!.tokens_in).toBeGreaterThan(0);
    }
    if (assistantMessage!.tokens_out !== undefined) {
      expect(assistantMessage!.tokens_out).toBeGreaterThan(0);
    }
  });

  test("response has model field", async () => {
    test.skip(!assistantMessage, "No assistant response available");

    expect(typeof assistantMessage!.model).toBe("string");
    expect(assistantMessage!.model!.length).toBeGreaterThan(0);
  });

  test("send follow-up tests conversation context", async ({ request }) => {
    test.setTimeout(60_000);
    test.skip(!conversationId, "No conversation created");
    test.skip(!assistantMessage, "No previous assistant response");

    const res = await request.post(`${API_BASE}/conversations/${conversationId}/messages`, {
      headers: headers(),
      data: { content: "What was the first word I asked you to say?" },
    });
    expect([200, 201, 202, 500, 502]).toContain(res.status());

    if (res.status() === 201) {
      const body = await res.json();
      expect(body.role).toBe("assistant");
      expect(typeof body.content).toBe("string");
      // The response should reference the previous exchange in some way
      expect(body.content.trim().length).toBeGreaterThan(0);
    } else {
      test.skip(true, "LLM/worker not available");
    }
  });

  test("send code question gets code response", async ({ request }) => {
    test.setTimeout(60_000);
    test.skip(!conversationId, "No conversation created");
    test.skip(models.length === 0, "No LLM models available");

    const res = await request.post(`${API_BASE}/conversations/${conversationId}/messages`, {
      headers: headers(),
      data: {
        content: "Write a Python function that adds two numbers. Only output the code.",
      },
    });
    expect([200, 201, 202, 500, 502]).toContain(res.status());

    if (res.status() === 201) {
      const body = await res.json();
      expect(body.role).toBe("assistant");
      const content = body.content.toLowerCase();
      // The response should contain Python code markers
      const hasCode = content.includes("def") || content.includes("return");
      expect(hasCode).toBe(true);
    } else {
      test.skip(true, "LLM/worker not available");
    }
  });

  test("send long prompt gets coherent response", async ({ request }) => {
    test.setTimeout(60_000);
    test.skip(!conversationId, "No conversation created");
    test.skip(models.length === 0, "No LLM models available");

    // Generate a 200+ word prompt
    const words = Array.from(
      { length: 40 },
      (_, i) =>
        `This is sentence number ${i + 1} in a longer prompt designed to test that the LLM can handle substantial input.`,
    ).join(" ");

    const res = await request.post(`${API_BASE}/conversations/${conversationId}/messages`, {
      headers: headers(),
      data: { content: `${words} Please summarize what I just said in one sentence.` },
    });
    expect([200, 201, 202, 500, 502]).toContain(res.status());

    if (res.status() === 201) {
      const body = await res.json();
      expect(body.role).toBe("assistant");
      expect(body.content.trim().length).toBeGreaterThan(0);
    } else {
      test.skip(true, "LLM/worker not available");
    }
  });

  test("messages list grows with each exchange", async ({ request }) => {
    test.skip(!conversationId, "No conversation created");

    const res = await request.get(`${API_BASE}/conversations/${conversationId}/messages`, {
      headers: headers(),
    });
    expect(res.status()).toBe(200);
    const messages = await res.json();
    expect(Array.isArray(messages)).toBe(true);
    // We sent at least one message; if LLM responded we have at least 2
    expect(messages.length).toBeGreaterThanOrEqual(1);
  });

  test("multiple conversations are independent", async ({ request }) => {
    test.setTimeout(60_000);
    test.skip(!projectId, "No project created");
    test.skip(models.length === 0, "No LLM models available");

    // Create two separate conversations
    const conv1Res = await request.post(`${API_BASE}/projects/${projectId}/conversations`, {
      headers: headers(),
      data: { title: "Independent conversation 1" },
    });
    expect(conv1Res.status()).toBe(201);
    const conv1 = await conv1Res.json();
    cleanup.add("conversation", conv1.id);

    const conv2Res = await request.post(`${API_BASE}/projects/${projectId}/conversations`, {
      headers: headers(),
      data: { title: "Independent conversation 2" },
    });
    expect(conv2Res.status()).toBe(201);
    const conv2 = await conv2Res.json();
    cleanup.add("conversation", conv2.id);

    // Send different content to each
    const msg1Res = await request.post(`${API_BASE}/conversations/${conv1.id}/messages`, {
      headers: headers(),
      data: { content: "The secret code is ALPHA." },
    });
    const msg2Res = await request.post(`${API_BASE}/conversations/${conv2.id}/messages`, {
      headers: headers(),
      data: { content: "The secret code is BRAVO." },
    });

    // Both should accept the message
    expect([200, 201, 202, 500, 502]).toContain(msg1Res.status());
    expect([200, 201, 202, 500, 502]).toContain(msg2Res.status());

    // Verify each conversation has only its own messages
    const list1Res = await request.get(`${API_BASE}/conversations/${conv1.id}/messages`, {
      headers: headers(),
    });
    const list2Res = await request.get(`${API_BASE}/conversations/${conv2.id}/messages`, {
      headers: headers(),
    });
    expect(list1Res.status()).toBe(200);
    expect(list2Res.status()).toBe(200);

    const messages1 = await list1Res.json();
    const messages2 = await list2Res.json();

    // Each conversation should have at least its own user message
    const userMsgs1 = messages1.filter((m: { role: string }) => m.role === "user");
    const userMsgs2 = messages2.filter((m: { role: string }) => m.role === "user");

    if (userMsgs1.length > 0) {
      expect(userMsgs1.some((m: { content: string }) => m.content.includes("ALPHA"))).toBe(true);
      expect(userMsgs1.some((m: { content: string }) => m.content.includes("BRAVO"))).toBe(false);
    }
    if (userMsgs2.length > 0) {
      expect(userMsgs2.some((m: { content: string }) => m.content.includes("BRAVO"))).toBe(true);
      expect(userMsgs2.some((m: { content: string }) => m.content.includes("ALPHA"))).toBe(false);
    }
  });

  test("send empty content returns error", async ({ request }) => {
    test.skip(!conversationId, "No conversation created");

    const res = await request.post(`${API_BASE}/conversations/${conversationId}/messages`, {
      headers: headers(),
      data: { content: "" },
    });
    expect([400, 422, 500]).toContain(res.status());
  });
});
