import { For } from "solid-js";

import { useI18n } from "~/i18n";
import { Badge, Card } from "~/ui";
import type { BadgeVariant } from "~/ui/primitives/Badge";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface ConversationResult {
  conversation_id: string;
  message_id: string;
  role: string;
  content: string;
  model: string;
  created_at: string;
}

export interface ConversationResultsProps {
  results: ConversationResult[];
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const ROLE_VARIANTS: Record<string, BadgeVariant> = {
  user: "primary",
  assistant: "success",
  tool: "warning",
  system: "neutral",
};

function roleBadgeVariant(role: string): BadgeVariant {
  return ROLE_VARIANTS[role] ?? "default";
}

function truncate(text: string, maxLen: number): string {
  if (text.length <= maxLen) return text;
  return text.slice(0, maxLen).trimEnd() + "...";
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function ConversationResults(props: ConversationResultsProps) {
  const { fmt } = useI18n();

  return (
    <div class="mt-2 space-y-2">
      <For each={props.results}>
        {(result) => (
          <a href={`/chat?conversation=${result.conversation_id}`} class="block">
            <Card class="transition-shadow hover:shadow-md">
              <Card.Body>
                <div class="flex flex-wrap items-center gap-2">
                  <Badge variant={roleBadgeVariant(result.role)} pill>
                    {result.role}
                  </Badge>
                  {result.model && (
                    <Badge variant="info" pill>
                      {result.model}
                    </Badge>
                  )}
                  <span class="ml-auto text-xs text-cf-text-muted">
                    {fmt.dateTime(result.created_at)}
                  </span>
                </div>
                <p class="mt-2 text-sm text-cf-text-secondary whitespace-pre-wrap break-words">
                  {truncate(result.content, 200)}
                </p>
              </Card.Body>
            </Card>
          </a>
        )}
      </For>
    </div>
  );
}
