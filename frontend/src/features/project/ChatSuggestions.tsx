import { createMemo, For } from "solid-js";

import { useI18n } from "~/i18n";
import type { TranslationKey } from "~/i18n/en";

interface ChatSuggestionsProps {
  activeTab: string;
  onSelect: (text: string) => void;
}

const SUGGESTIONS: Record<string, TranslationKey[]> = {
  files: ["chat.suggestion.explainStructure", "chat.suggestion.findEntryPoints"],
  goals: ["chat.suggestion.defineGoals", "chat.suggestion.setPriorities"],
  roadmap: ["chat.suggestion.createRoadmap", "chat.suggestion.planMvp"],
  featuremap: ["chat.suggestion.analyzeFeatures", "chat.suggestion.showDeps"],
  warroom: ["chat.suggestion.startAgent", "chat.suggestion.explainStatus"],
  sessions: ["chat.suggestion.summarizeSession", "chat.suggestion.continueWork"],
  trajectory: ["chat.suggestion.explainTrajectory", "chat.suggestion.whatWentWrong"],
  audit: ["chat.suggestion.summarizeChanges", "chat.suggestion.showSecurityEvents"],
};

export default function ChatSuggestions(props: ChatSuggestionsProps) {
  const { t } = useI18n();

  const suggestions = createMemo(() => SUGGESTIONS[props.activeTab] ?? []);

  return (
    <div class="flex gap-1.5 overflow-x-auto scrollbar-none px-3 py-1.5">
      <For each={suggestions()}>
        {(key) => (
          <button
            class="flex-shrink-0 rounded-full border border-cf-border bg-cf-bg-surface px-3 py-1 text-xs text-cf-text-muted hover:border-cf-accent hover:text-cf-accent transition-colors"
            onClick={() => props.onSelect(t(key))}
          >
            {t(key)}
          </button>
        )}
      </For>
    </div>
  );
}
