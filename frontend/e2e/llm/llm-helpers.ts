/**
 * LLM-specific test utilities for the comprehensive E2E LLM test suite.
 * Provides provider discovery, conversation helpers, and assertion utilities.
 *
 * All helpers reuse the existing api-helpers.ts and ws-helpers.ts infrastructure.
 *
 * TODO: FIX-103: Some E2E tests still use hardcoded localhost URLs instead of
 * relying on the `baseURL` from playwright config. Audit all test files and
 * replace `http://localhost:8080` with the config-driven base URL.
 */

import {
  apiFetch,
  apiGet,
  apiPost,
  createCleanupTracker,
  createProject,
  type CleanupTracker,
  type ProjectData,
} from "../helpers/api-helpers";
import { type TestWSClient, type WSMessage } from "../helpers/ws-helpers";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface DiscoveredModel {
  model_name: string;
  provider: string;
  status: "reachable" | "unreachable";
  source: string;
  model_info?: Record<string, unknown>;
}

export interface DiscoverResponse {
  models: DiscoveredModel[];
  count: number;
  ollama_url: string;
}

export interface AvailableResponse {
  models: Array<{ model_name: string }>;
  best_model: string;
}

export interface LLMHealthResponse {
  status: "healthy" | "unhealthy";
}

export interface ConversationMessage {
  id: string;
  conversation_id: string;
  role: "user" | "assistant" | "system" | "tool";
  content: string;
  tool_calls?: unknown;
  tokens_in?: number;
  tokens_out?: number;
  model?: string;
  created_at: string;
}

export interface CostSummary {
  total_cost_usd: number;
  total_tokens_in: number;
  total_tokens_out: number;
  run_count: number;
}

export interface CostByModel {
  model: string;
  total_cost_usd: number;
  total_tokens_in: number;
  total_tokens_out: number;
  run_count: number;
}

export interface CostRun {
  id: string;
  model?: string;
  cost_usd: number;
  tokens_in: number;
  tokens_out: number;
  created_at: string;
}

export interface RoutingOutcome {
  model: string;
  task_type: string;
  tier: string;
  quality_score: number;
  latency_ms: number;
  cost_usd: number;
}

// ---------------------------------------------------------------------------
// Provider discovery
// ---------------------------------------------------------------------------

/** Discover all available LLM models from LiteLLM + Ollama. */
export async function discoverAvailableModels(): Promise<DiscoverResponse> {
  return apiGet<DiscoverResponse>("/llm/discover");
}

/** Check LiteLLM proxy health. */
export async function checkLLMHealth(): Promise<LLMHealthResponse> {
  return apiGet<LLMHealthResponse>("/llm/health");
}

/** Check if LiteLLM is healthy (non-throwing). */
export async function isLiteLLMHealthy(): Promise<boolean> {
  try {
    const h = await checkLLMHealth();
    return h.status === "healthy";
  } catch {
    return false;
  }
}

/** Get unique provider names from discovered models. */
export function extractProviders(models: DiscoveredModel[]): string[] {
  const providers = new Set(models.map((m) => m.provider).filter(Boolean));
  return [...providers].sort();
}

/** Check if a specific provider has reachable models. */
export function hasProvider(models: DiscoveredModel[], provider: string): boolean {
  return models.some((m) => m.provider === provider && m.status === "reachable");
}

/** Pick the fastest available model (prefer groq > openai-mini > any). */
export function pickFastModel(models: DiscoveredModel[]): string | null {
  const reachable = models.filter((m) => m.status === "reachable");
  const priority = [
    (m: DiscoveredModel) => m.model_name.includes("groq/") && m.model_name.includes("8b"),
    (m: DiscoveredModel) => m.model_name.includes("groq/"),
    (m: DiscoveredModel) => m.model_name.includes("gpt-4o-mini"),
    (m: DiscoveredModel) => m.model_name.includes("gemini") && m.model_name.includes("flash"),
    (m: DiscoveredModel) => m.model_name.includes("claude-haiku"),
  ];
  for (const pred of priority) {
    const match = reachable.find(pred);
    if (match) return match.model_name;
  }
  return reachable[0]?.model_name ?? null;
}

/** Pick the strongest available model (prefer claude-sonnet > gpt-4o > any). */
export function pickStrongModel(models: DiscoveredModel[]): string | null {
  const reachable = models.filter((m) => m.status === "reachable");
  const priority = [
    (m: DiscoveredModel) => m.model_name.includes("claude-sonnet"),
    (m: DiscoveredModel) => m.model_name.includes("claude-opus"),
    (m: DiscoveredModel) => m.model_name.includes("gpt-4o") && !m.model_name.includes("mini"),
    (m: DiscoveredModel) => m.model_name.includes("gemini") && m.model_name.includes("pro"),
    (m: DiscoveredModel) => m.model_name.includes("groq/") && m.model_name.includes("70b"),
  ];
  for (const pred of priority) {
    const match = reachable.find(pred);
    if (match) return match.model_name;
  }
  return reachable[0]?.model_name ?? null;
}

/** Pick a model that supports tool calling. */
export function pickToolCapableModel(models: DiscoveredModel[]): string | null {
  const reachable = models.filter((m) => m.status === "reachable");
  // Models known to support tool calling
  const priority = [
    (m: DiscoveredModel) => m.model_name.includes("claude-sonnet"),
    (m: DiscoveredModel) => m.model_name.includes("claude-opus"),
    (m: DiscoveredModel) => m.model_name.includes("gpt-4o"),
    (m: DiscoveredModel) => m.model_name.includes("claude-haiku"),
    (m: DiscoveredModel) => m.model_name.includes("gemini"),
  ];
  for (const pred of priority) {
    const match = reachable.find(pred);
    if (match) return match.model_name;
  }
  return reachable[0]?.model_name ?? null;
}

/** Pick the best model for a specific provider. */
export function pickModelForProvider(models: DiscoveredModel[], provider: string): string | null {
  const providerModels = models.filter((m) => m.provider === provider && m.status === "reachable");
  return providerModels[0]?.model_name ?? null;
}

// ---------------------------------------------------------------------------
// Worker availability
// ---------------------------------------------------------------------------

/**
 * Check if the Python agent worker is running and processing NATS messages.
 * Sends a quick agentic message and polls for a response within a short timeout.
 * Returns true only if we get an assistant reply back (proving the worker is active).
 */
export async function isWorkerAvailable(): Promise<boolean> {
  try {
    // Create a throwaway project + conversation
    const proj = await createProject(`e2e-llm-worker-check-${Date.now()}`);
    const conv = await apiPost<{ id: string }>(`/projects/${proj.id}/conversations`, {});

    // Send an agentic message
    const res = await apiFetch(`/conversations/${conv.id}/messages`, {
      method: "POST",
      body: JSON.stringify({ content: "Say OK", agentic: true }),
    });

    if (res.status !== 202) {
      // Cleanup
      await apiFetch(`/projects/${proj.id}`, { method: "DELETE" }).catch(() => {});
      return false;
    }

    // Poll for a response with a short timeout (15 seconds)
    const reply = await waitForAssistantMessage(conv.id, 0, 15_000, 2_000);

    // Cleanup
    await apiFetch(`/projects/${proj.id}`, { method: "DELETE" }).catch(() => {});

    return reply !== null;
  } catch {
    return false;
  }
}

// ---------------------------------------------------------------------------
// Conversation helpers
// ---------------------------------------------------------------------------

/** Create a project and a conversation inside it. Returns both IDs. */
export async function createProjectWithConversation(
  name: string,
  cleanup?: CleanupTracker,
): Promise<{ projectId: string; conversationId: string }> {
  const proj = await createProject(name);
  if (cleanup) cleanup.add("project", proj.id);

  const conv = await apiPost<{ id: string }>(`/projects/${proj.id}/conversations`, {});
  if (cleanup) cleanup.add("conversation", conv.id);

  return { projectId: proj.id, conversationId: conv.id };
}

/** Send a message (simple mode) and wait for the assistant reply by polling. */
export async function sendAndWaitForReply(
  conversationId: string,
  content: string,
  options: { agentic?: boolean; timeoutMs?: number; pollIntervalMs?: number } = {},
): Promise<ConversationMessage | null> {
  const timeout = options.timeoutMs ?? 60_000;
  const poll = options.pollIntervalMs ?? 2_000;
  const agentic = options.agentic ?? false;

  // Get current message count
  const beforeMessages = await apiGet<ConversationMessage[]>(
    `/conversations/${conversationId}/messages`,
  );
  const beforeCount = beforeMessages.length;

  // Send the message
  const res = await apiFetch(`/conversations/${conversationId}/messages`, {
    method: "POST",
    body: JSON.stringify({ content, agentic }),
  });

  // For simple (non-agentic) messages, the response includes the assistant message
  if (res.status === 201) {
    const msg = (await res.json()) as ConversationMessage;
    if (msg.role === "assistant") return msg;
  }

  // For agentic messages (202) or if simple didn't return assistant, poll
  return waitForAssistantMessage(conversationId, beforeCount, timeout, poll);
}

/** Poll messages until a new assistant message appears. */
export async function waitForAssistantMessage(
  conversationId: string,
  afterIndex: number,
  timeoutMs = 60_000,
  pollIntervalMs = 2_000,
): Promise<ConversationMessage | null> {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    const messages = await apiGet<ConversationMessage[]>(
      `/conversations/${conversationId}/messages`,
    );
    // Look for assistant messages after the index
    const newAssistant = messages.slice(afterIndex).find((m) => m.role === "assistant");
    if (newAssistant) return newAssistant;

    await new Promise((r) => setTimeout(r, pollIntervalMs));
  }
  return null;
}

/** Send an agentic message (fire-and-forget, returns 202). */
export async function sendAgenticMessage(
  conversationId: string,
  content: string,
): Promise<{ status: number; body: unknown }> {
  const res = await apiFetch(`/conversations/${conversationId}/messages`, {
    method: "POST",
    body: JSON.stringify({ content, agentic: true }),
  });
  const text = await res.text();
  return {
    status: res.status,
    body: text ? JSON.parse(text) : {},
  };
}

/** Get the most recent assistant message in a conversation. */
export async function getLastAssistantMessage(
  conversationId: string,
): Promise<ConversationMessage | null> {
  const messages = await apiGet<ConversationMessage[]>(`/conversations/${conversationId}/messages`);
  return [...messages].reverse().find((m) => m.role === "assistant") ?? null;
}

// ---------------------------------------------------------------------------
// WebSocket / AG-UI helpers
// ---------------------------------------------------------------------------

/** Collect AG-UI events until agui.run_finished or timeout. */
export async function collectAGUIEventsUntilDone(
  ws: TestWSClient,
  timeoutMs = 60_000,
): Promise<WSMessage[]> {
  const events: WSMessage[] = [];

  return new Promise<WSMessage[]>((resolve) => {
    const timeout = setTimeout(() => {
      resolve(events);
    }, timeoutMs);

    // Collect events as they come in
    const check = setInterval(() => {
      const all = ws.getMessages();
      // Add any AG-UI events we haven't seen
      for (const msg of all) {
        if (msg.type.startsWith("agui.") && !events.some((e) => e === msg)) {
          events.push(msg);
        }
      }
      // Check if run_finished arrived
      if (all.some((m) => m.type === "agui.run_finished")) {
        clearInterval(check);
        clearTimeout(timeout);
        resolve(events);
      }
    }, 500);
  });
}

// ---------------------------------------------------------------------------
// Cost helpers
// ---------------------------------------------------------------------------

/** Get project cost summary. */
export async function getProjectCost(projectId: string): Promise<CostSummary> {
  return apiGet<CostSummary>(`/projects/${projectId}/costs`);
}

/** Get project cost breakdown by model. */
export async function getProjectCostByModel(projectId: string): Promise<CostByModel[]> {
  return apiGet<CostByModel[]>(`/projects/${projectId}/costs/by-model`);
}

/** Get recent runs with costs. */
export async function getProjectCostRuns(projectId: string, limit = 10): Promise<CostRun[]> {
  return apiGet<CostRun[]>(`/projects/${projectId}/costs/runs?limit=${limit}`);
}

// ---------------------------------------------------------------------------
// Routing helpers
// ---------------------------------------------------------------------------

/** Record a routing outcome. */
export async function recordRoutingOutcome(outcome: RoutingOutcome): Promise<void> {
  await apiPost("/routing/outcomes", outcome);
}

// ---------------------------------------------------------------------------
// Assertions
// ---------------------------------------------------------------------------

/** Assert a response is a valid, non-empty LLM response. */
export function assertValidLLMResponse(content: string | undefined | null): boolean {
  if (!content) return false;
  if (content.trim().length === 0) return false;
  // Check it's not just an error message
  const errorPatterns = ["error:", "failed:", "exception:", "traceback"];
  const lower = content.toLowerCase();
  return !errorPatterns.some((p) => lower.startsWith(p));
}

/** Assert cost values are positive and reasonable. */
export function assertCostPositive(cost: CostSummary): boolean {
  return cost.total_cost_usd >= 0 && cost.total_tokens_in >= 0 && cost.total_tokens_out >= 0;
}

// ---------------------------------------------------------------------------
// Re-exports for convenience
// ---------------------------------------------------------------------------

export { createCleanupTracker, createProject, type CleanupTracker, type ProjectData };
