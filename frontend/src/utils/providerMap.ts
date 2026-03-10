const PROVIDER_MAP: Record<string, string> = {
  "gpt-": "OpenAI",
  o1: "OpenAI",
  o3: "OpenAI",
  o4: "OpenAI",
  "claude-": "Anthropic",
  gemini: "Google",
  mistral: "Mistral",
  llama: "Meta",
  "groq/": "Groq",
  deepseek: "DeepSeek",
  qwen: "Alibaba",
  command: "Cohere",
};

export function getProvider(model: string): string {
  const lower = model.toLowerCase();
  for (const [prefix, provider] of Object.entries(PROVIDER_MAP)) {
    if (lower.startsWith(prefix) || lower.includes(`/${prefix}`)) {
      return provider;
    }
  }
  return "Unknown";
}
