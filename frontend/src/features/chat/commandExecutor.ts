import { getAccessToken } from "~/api/client";

const BASE = "/api/v1";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface CommandContext {
  conversationId: string;
  messages: readonly { role: string; content: string }[];
  sessionCostUsd: number;
  sessionTokensIn: number;
  sessionTokensOut: number;
  sessionSteps: number;
  sessionModel: string;
}

export interface CommandResult {
  type: "display" | "api_call" | "modal";
  content?: string;
  action?: string;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatUsd(value: number): string {
  return `$${value.toFixed(4)}`;
}

function formatNumber(value: number): string {
  return value.toLocaleString("en-US");
}

async function post(path: string, body?: Record<string, string>): Promise<Response> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };

  const token = getAccessToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  return fetch(`${BASE}${path}`, {
    method: "POST",
    headers,
    credentials: "include",
    body: body ? JSON.stringify(body) : undefined,
  });
}

// ---------------------------------------------------------------------------
// Command handlers
// ---------------------------------------------------------------------------

function handleCost(ctx: CommandContext): CommandResult {
  const lines = [
    `Model: ${ctx.sessionModel || "unknown"}`,
    `Cost: ${formatUsd(ctx.sessionCostUsd)}`,
    `Tokens in: ${formatNumber(ctx.sessionTokensIn)}`,
    `Tokens out: ${formatNumber(ctx.sessionTokensOut)}`,
    `Steps: ${formatNumber(ctx.sessionSteps)}`,
  ];
  return { type: "display", content: lines.join("\n") };
}

function handleHelp(): CommandResult {
  const lines = [
    "Available commands:",
    "  /cost    — Show session cost, tokens, and model info",
    "  /help    — Show this help message",
    "  /diff    — Open the diff summary view",
    "  /compact — Compact conversation history",
    "  /clear   — Clear conversation messages",
    "  /mode <name>  — Switch agent mode",
    "  /model <name> — Switch LLM model",
    "  /rewind  — Open the rewind timeline",
  ];
  return { type: "display", content: lines.join("\n") };
}

function handleDiff(): CommandResult {
  return { type: "modal", action: "show_diff" };
}

function handleRewind(): CommandResult {
  return { type: "modal", action: "show_rewind" };
}

async function handleCompact(ctx: CommandContext): Promise<CommandResult> {
  await post(`/conversations/${ctx.conversationId}/compact`);
  return { type: "api_call", content: "Compacting conversation..." };
}

async function handleClear(ctx: CommandContext): Promise<CommandResult> {
  await post(`/conversations/${ctx.conversationId}/clear`);
  return { type: "api_call", content: "Conversation cleared." };
}

async function handleMode(ctx: CommandContext, args: string): Promise<CommandResult> {
  const mode = args.trim();
  if (!mode) {
    return { type: "display", content: "Usage: /mode <name>" };
  }
  await post(`/conversations/${ctx.conversationId}/mode`, { mode });
  return { type: "api_call", content: `Mode switched to "${mode}".` };
}

async function handleModel(ctx: CommandContext, args: string): Promise<CommandResult> {
  const model = args.trim();
  if (!model) {
    return { type: "display", content: "Usage: /model <name>" };
  }
  await post(`/conversations/${ctx.conversationId}/model`, { model });
  return { type: "api_call", content: `Model switched to "${model}".` };
}

// ---------------------------------------------------------------------------
// Dispatch
// ---------------------------------------------------------------------------

export async function executeCommand(
  commandId: string,
  args: string,
  context: CommandContext,
): Promise<CommandResult> {
  switch (commandId) {
    case "cost":
      return handleCost(context);
    case "help":
      return handleHelp();
    case "diff":
      return handleDiff();
    case "compact":
      return handleCompact(context);
    case "clear":
      return handleClear(context);
    case "mode":
      return handleMode(context, args);
    case "model":
      return handleModel(context, args);
    case "rewind":
      return handleRewind();
    default:
      return { type: "display", content: `Unknown command: /${commandId}` };
  }
}
