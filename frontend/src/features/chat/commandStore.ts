import { createResource } from "solid-js";

import type { Item } from "./fuzzySearch";

interface CommandResponse {
  id: string;
  label: string;
  category: string;
  description: string;
}

/** Built-in fallback commands shown when the backend returns none. */
const FALLBACK_COMMANDS: Item[] = [
  { id: "compact", label: "compact", category: "command" },
  { id: "clear", label: "clear", category: "command" },
  { id: "diff", label: "diff", category: "command" },
  { id: "help", label: "help", category: "command" },
  { id: "mode", label: "mode", category: "command" },
  { id: "model", label: "model", category: "command" },
  { id: "rewind", label: "rewind", category: "command" },
];

/** Fetch commands from the backend and expose them as autocomplete Items. */
export function useCommandStore() {
  const [commands] = createResource(async (): Promise<Item[]> => {
    try {
      const response = await fetch("/api/v1/commands");
      if (!response.ok) return FALLBACK_COMMANDS;
      const data = (await response.json()) as CommandResponse[];
      if (data.length === 0) return FALLBACK_COMMANDS;
      return data.map(
        (cmd): Item => ({
          id: cmd.id,
          label: cmd.label,
          category: cmd.category,
        }),
      );
    } catch {
      return FALLBACK_COMMANDS;
    }
  });

  return { commands: (): Item[] => commands() ?? FALLBACK_COMMANDS };
}
