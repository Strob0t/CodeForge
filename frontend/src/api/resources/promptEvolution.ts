import type { CoreClient } from "../core";
import { url } from "../factory";
import type { PromptEvolutionStatus, PromptVariant } from "../types";

export function createPromptEvolutionResource(c: CoreClient) {
  return {
    /** GET /prompt-evolution/status — evolution config + per-mode stats. */
    status: () => c.get<PromptEvolutionStatus>("/prompt-evolution/status"),

    /** GET /prompt-evolution/variants — all variants, optionally filtered. */
    variants: (modeId?: string, status?: string) => {
      const params = new URLSearchParams();
      if (modeId) params.set("mode_id", modeId);
      if (status) params.set("status", status);
      const qs = params.toString();
      return c.get<PromptVariant[]>(`/prompt-evolution/variants${qs ? `?${qs}` : ""}`);
    },

    /** POST /prompt-evolution/promote/{variantId} — promote a candidate variant. */
    promote: (variantId: string) =>
      c.post<{ status: string; variant_id: string }>(url`/prompt-evolution/promote/${variantId}`),

    /** POST /prompt-evolution/revert/{modeId} — revert mode to base prompts. */
    revert: (modeId: string) =>
      c.post<{ status: string; mode_id: string }>(url`/prompt-evolution/revert/${modeId}`),

    /** POST /prompt-evolution/reflect — trigger a reflection loop. */
    reflect: (data: {
      mode_id: string;
      model_family: string;
      current_prompt: string;
      failures?: Record<string, unknown>[];
    }) => c.post<{ status: string }>("/prompt-evolution/reflect", data),
  };
}
