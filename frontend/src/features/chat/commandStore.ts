import { createResource } from "solid-js";

import type { Item } from "./fuzzySearch";

interface CommandResponse {
  id: string;
  label: string;
  category: string;
  description: string;
}

/** Fetch commands from the backend and expose them as autocomplete Items. */
export function useCommandStore() {
  const [commands] = createResource(async (): Promise<Item[]> => {
    try {
      const response = await fetch("/api/v1/commands");
      if (!response.ok) return [];
      const data = (await response.json()) as CommandResponse[];
      return data.map(
        (cmd): Item => ({
          id: cmd.id,
          label: cmd.label,
          category: cmd.category,
        }),
      );
    } catch {
      return [];
    }
  });

  return { commands: (): Item[] => commands() ?? [] };
}
