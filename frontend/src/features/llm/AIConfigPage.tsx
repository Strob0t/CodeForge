import { useSearchParams } from "@solidjs/router";
import { onMount, Show } from "solid-js";

import { ModelsContent } from "~/features/llm/ModelsPage";
import { ModesContent } from "~/features/modes/ModesPage";
import { useI18n } from "~/i18n";
import { PageLayout, Tabs } from "~/ui";

export default function AIConfigPage() {
  onMount(() => {
    document.title = "AI Config - CodeForge";
  });
  const { t } = useI18n();
  const [params, setParams] = useSearchParams();
  const activeTab = (): string => {
    const tab = params.tab;
    if (Array.isArray(tab)) return tab[0] ?? "models";
    return tab ?? "models";
  };

  return (
    <PageLayout title={t("app.nav.aiConfig")}>
      <Tabs
        items={[
          { value: "models", label: t("ai.tab.models") },
          { value: "modes", label: t("ai.tab.modes") },
        ]}
        value={activeTab()}
        onChange={(v) => setParams({ tab: v === "models" ? undefined : v })}
        variant="underline"
        class="mb-4"
      />
      <Show when={activeTab() === "models"}>
        <ModelsContent />
      </Show>
      <Show when={activeTab() === "modes"}>
        <ModesContent />
      </Show>
    </PageLayout>
  );
}
