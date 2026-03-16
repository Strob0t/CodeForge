import { useSearchParams } from "@solidjs/router";
import { Show } from "solid-js";

import { KnowledgeBasesContent } from "~/features/knowledgebases/KnowledgeBasesPage";
import { ScopesContent } from "~/features/scopes/ScopesPage";
import { useI18n } from "~/i18n";
import { PageLayout, Tabs } from "~/ui";

export default function KnowledgePage() {
  const { t } = useI18n();
  const [params, setParams] = useSearchParams();
  const activeTab = (): string => {
    const tab = params.tab;
    if (Array.isArray(tab)) return tab[0] ?? "bases";
    return tab ?? "bases";
  };

  return (
    <PageLayout title={t("app.nav.knowledge")} description={t("kb.description")}>
      <Tabs
        items={[
          { value: "bases", label: t("knowledge.tab.bases") },
          { value: "scopes", label: t("knowledge.tab.scopes") },
        ]}
        value={activeTab()}
        onChange={(v) => setParams({ tab: v === "bases" ? undefined : v })}
        variant="underline"
        class="mb-4"
      />
      <Show when={activeTab() === "bases"}>
        <KnowledgeBasesContent />
      </Show>
      <Show when={activeTab() === "scopes"}>
        <ScopesContent />
      </Show>
    </PageLayout>
  );
}
