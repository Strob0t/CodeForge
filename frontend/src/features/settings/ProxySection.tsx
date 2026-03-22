import { createResource, Show } from "solid-js";

import { api } from "~/api/client";
import { useI18n } from "~/i18n";
import { Alert, Section } from "~/ui";
import { SkeletonCard } from "~/ui/composites/SkeletonCard";

export default function ProxySection() {
  const { t } = useI18n();

  const [llmHealth] = createResource(() => api.llm.health());

  return (
    <Section id="settings-proxy" title={t("settings.llm.title")} class="mb-8">
      <Show when={!llmHealth.loading} fallback={<SkeletonCard variant="stat" />}>
        <Show
          when={llmHealth()}
          fallback={<Alert variant="error">{t("settings.llm.unavailable")}</Alert>}
        >
          <Alert variant="success">{t("settings.llm.connected")}</Alert>
        </Show>
      </Show>
    </Section>
  );
}
