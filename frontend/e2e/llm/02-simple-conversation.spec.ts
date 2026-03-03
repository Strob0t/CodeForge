import { test, expect } from "@playwright/test";
import { apiLogin, createCleanupTracker, API_BASE } from "../helpers/api-helpers";
import { discoverAvailableModels } from "./llm-helpers";

test.describe("LLM E2E — Simple Conversation", () => {
  test.setTimeout(60_000);

  let token: string;
  let projectId: string;
  const cleanup = createCleanupTracker();

  const headers = () => ({ Authorization: `Bearer ${token}` });

  /** Create a fresh conversation inside the shared project. */
  async function newConversation(
    request: { post: Function },
    title = "E2E test conversation",
  ): Promise<string> {
    const res = await request.post(`${API_BASE}/projects/${projectId}/conversations`, {
      headers: headers(),
      data: { title },
    });
    expect(res.status()).toBe(201);
    const body = await res.json();
    cleanup.add("conversation", body.id);
    return body.id;
  }

  /** Send a user message and return the assistant response. */
  async function sendMessage(
    request: { post: Function },
    convId: string,
    content: string,
  ): Promise<{
    role: string;
    content: string;
    tokens_in: number;
    tokens_out: number;
    model: string;
  }> {
    const res = await request.post(`${API_BASE}/conversations/${convId}/messages`, {
      headers: headers(),
      data: { content },
    });
    expect(res.status()).toBe(201);
    return res.json();
  }

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;

    // Verify at least one model is reachable (prerequisite for all tests)
    const discovery = await discoverAvailableModels();
    const reachable = discovery.models.filter((m) => m.status === "reachable");
    expect(reachable.length).toBeGreaterThan(0);

    // Create shared project
    const projRes = await fetch(`${API_BASE}/projects`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      body: JSON.stringify({ name: `e2e-llm-conv-${Date.now()}` }),
    });
    expect(projRes.status).toBe(201);
    const proj = (await projRes.json()) as { id: string };
    projectId = proj.id;
    cleanup.add("project", projectId);
  });

  test.afterAll(async () => {
    await cleanup.cleanup();
  });

  test("create conversation returns 201 with id", async ({ request }) => {
    const convId = await newConversation(request, "creation test");
    expect(convId).toBeTruthy();
  });

  test("send simple greeting gets LLM response", async ({ request }) => {
    const convId = await newConversation(request);
    const reply = await sendMessage(request, convId, "Hello, respond with exactly one word.");

    expect(reply.role).toBe("assistant");
    expect(typeof reply.content).toBe("string");
    expect(reply.content.trim().length).toBeGreaterThan(0);
  });

  test("response has non-zero token counts", async ({ request }) => {
    const convId = await newConversation(request);
    const reply = await sendMessage(request, convId, "Say hi.");

    expect(reply.tokens_in).toBeGreaterThan(0);
    expect(reply.tokens_out).toBeGreaterThan(0);
  });

  test("response has model field", async ({ request }) => {
    const convId = await newConversation(request);
    const reply = await sendMessage(request, convId, "Say hi.");

    expect(typeof reply.model).toBe("string");
    expect(reply.model.length).toBeGreaterThan(0);
  });

  test("send follow-up tests conversation context", async ({ request }) => {
    const convId = await newConversation(request, "context test");

    // First message: establish context
    await sendMessage(request, convId, "Remember this word: PINEAPPLE");

    // Follow-up: ask about context
    const reply = await sendMessage(
      request,
      convId,
      "What word did I ask you to remember? Reply with just the word.",
    );

    expect(reply.role).toBe("assistant");
    expect(reply.content.toUpperCase()).toContain("PINEAPPLE");
  });

  test("send code question gets code response", async ({ request }) => {
    const convId = await newConversation(request, "code gen test");
    const reply = await sendMessage(
      request,
      convId,
      "Write a Python function that adds two numbers. Only output the code.",
    );

    expect(reply.role).toBe("assistant");
    const content = reply.content.toLowerCase();
    const hasCode = content.includes("def") || content.includes("return");
    expect(hasCode).toBe(true);
  });

  test("send long prompt gets coherent response", async ({ request }) => {
    const convId = await newConversation(request, "long prompt test");

    const words = Array.from(
      { length: 40 },
      (_, i) =>
        `This is sentence number ${i + 1} in a longer prompt designed to test that the LLM can handle substantial input.`,
    ).join(" ");

    const reply = await sendMessage(
      request,
      convId,
      `${words} Please summarize what I just said in one sentence.`,
    );

    expect(reply.role).toBe("assistant");
    expect(reply.content.trim().length).toBeGreaterThan(0);
  });

  test("messages list grows with each exchange", async ({ request }) => {
    const convId = await newConversation(request);
    await sendMessage(request, convId, "Say hello.");

    const res = await request.get(`${API_BASE}/conversations/${convId}/messages`, {
      headers: headers(),
    });
    expect(res.status()).toBe(200);
    const messages = await res.json();
    expect(Array.isArray(messages)).toBe(true);
    // user message + assistant response = at least 2
    expect(messages.length).toBeGreaterThanOrEqual(2);
  });

  test("multiple conversations are independent", async ({ request }) => {
    const conv1Id = await newConversation(request, "independent 1");
    const conv2Id = await newConversation(request, "independent 2");

    await sendMessage(request, conv1Id, "The secret code is ALPHA.");
    await sendMessage(request, conv2Id, "The secret code is BRAVO.");

    const list1 = await request.get(`${API_BASE}/conversations/${conv1Id}/messages`, {
      headers: headers(),
    });
    const list2 = await request.get(`${API_BASE}/conversations/${conv2Id}/messages`, {
      headers: headers(),
    });
    expect(list1.status()).toBe(200);
    expect(list2.status()).toBe(200);

    const msgs1 = await list1.json();
    const msgs2 = await list2.json();

    const userMsgs1 = msgs1.filter((m: { role: string }) => m.role === "user");
    const userMsgs2 = msgs2.filter((m: { role: string }) => m.role === "user");

    expect(userMsgs1.some((m: { content: string }) => m.content.includes("ALPHA"))).toBe(true);
    expect(userMsgs1.some((m: { content: string }) => m.content.includes("BRAVO"))).toBe(false);
    expect(userMsgs2.some((m: { content: string }) => m.content.includes("BRAVO"))).toBe(true);
    expect(userMsgs2.some((m: { content: string }) => m.content.includes("ALPHA"))).toBe(false);
  });

  test("send empty content returns error", async ({ request }) => {
    const convId = await newConversation(request);
    const res = await request.post(`${API_BASE}/conversations/${convId}/messages`, {
      headers: headers(),
      data: { content: "" },
    });
    expect([400, 422, 500]).toContain(res.status());
  });

  test("assistant response is listed via messages endpoint", async ({ request }) => {
    const convId = await newConversation(request);
    await sendMessage(request, convId, "Hi there.");

    const res = await request.get(`${API_BASE}/conversations/${convId}/messages`, {
      headers: headers(),
    });
    expect(res.status()).toBe(200);
    const messages = await res.json();

    const assistant = messages.find((m: { role: string }) => m.role === "assistant");
    expect(assistant).toBeTruthy();
    expect(assistant.role).toBe("assistant");
    expect(assistant.content.length).toBeGreaterThan(0);
  });
});
