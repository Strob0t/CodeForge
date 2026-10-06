/**
 * The model the worker offers when the Claude Code CLI is available; it is
 * not configured in LiteLLM (workers/codeforge/consumer/_conversation_routing.py).
 */
const CLAUDE_CODE_PREFIX = "claudecode/";

/**
 * Whether the router can still pick a model: its name is configured in
 * LiteLLM, or a provider wildcard ("groq/*") covers it, as the worker expands
 * one (model_resolver.expand_wildcard_models). Routing stats of any other
 * model belong to a removed model (KI-129).
 */
export function isConfiguredModel(model: string, configured: readonly string[]): boolean {
  if (!model) return false;
  if (model.startsWith(CLAUDE_CODE_PREFIX)) return true;
  const provider = model.includes("/") ? model.slice(0, model.indexOf("/")) : "";
  return configured.some((name) => {
    if (!name.includes("*")) return name === model;
    const wildcardProvider = name.split("/")[0];
    return wildcardProvider !== "" && wildcardProvider === provider;
  });
}
