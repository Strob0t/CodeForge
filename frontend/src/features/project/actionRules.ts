export interface ActionRule {
  label: string;
  action: "send_message" | "run_tool" | "navigate";
  value: string;
}

/** Derive action suggestions from the last tool call's name and result */
export function deriveActions(toolName: string, result: string): ActionRule[] {
  const lower = toolName.toLowerCase();
  const actions: ActionRule[] = [];

  // After file edits, suggest running tests
  if (lower.includes("edit") || lower.includes("write")) {
    actions.push({
      label: "Run tests",
      action: "send_message",
      value: "Run the test suite to verify the changes",
    });
    actions.push({
      label: "Show diff",
      action: "send_message",
      value: "Show me the full diff of recent changes",
    });
  }

  // After bash/exec, suggest re-running if it failed
  if (
    (lower.includes("bash") || lower.includes("exec")) &&
    result.toLowerCase().includes("error")
  ) {
    actions.push({
      label: "Fix & retry",
      action: "send_message",
      value: "The previous command failed. Please fix the issue and retry.",
    });
  }

  // After search/grep, suggest narrowing or expanding
  if (lower.includes("search") || lower.includes("grep") || lower.includes("glob")) {
    actions.push({
      label: "Refine search",
      action: "send_message",
      value: "Refine the search with more specific criteria",
    });
  }

  // After read, suggest editing
  if (lower.includes("read")) {
    actions.push({
      label: "Edit this file",
      action: "send_message",
      value: "Make changes to this file",
    });
  }

  return actions;
}
