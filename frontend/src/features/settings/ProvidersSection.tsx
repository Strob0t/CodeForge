import { createResource, For, Show } from "solid-js";

import { api } from "~/api/client";
import { useI18n } from "~/i18n";
import { Card, Section } from "~/ui";

function ProviderCard(props: { label: string; items: string[]; loading: boolean }) {
  const { t } = useI18n();
  return (
    <Card>
      <Card.Body>
        <h4 class="mb-2 text-sm font-medium text-cf-text-tertiary">{props.label}</h4>
        <Show
          when={!props.loading}
          fallback={<p class="text-xs text-cf-text-muted">{t("settings.providers.loading")}</p>}
        >
          <Show
            when={props.items.length > 0}
            fallback={<p class="text-xs text-cf-text-muted">{t("settings.providers.none")}</p>}
          >
            <ul class="space-y-1">
              <For each={props.items}>
                {(item) => (
                  <li class="flex items-center gap-1.5 text-sm text-cf-text-primary">
                    <span class="h-1.5 w-1.5 rounded-full bg-cf-success-fg" aria-hidden="true" />
                    {item}
                  </li>
                )}
              </For>
            </ul>
          </Show>
        </Show>
      </Card.Body>
    </Card>
  );
}

export default function ProvidersSection() {
  const { t } = useI18n();

  const [gitProviders] = createResource(() => api.providers.git().then((r) => r.providers));
  const [agentProviders] = createResource(() => api.providers.agent().then((r) => r.backends));
  const [specProviders] = createResource(() =>
    api.providers.spec().then((list) => list.map((p) => p.name)),
  );
  const [pmProviders] = createResource(() =>
    api.providers.pm().then((list) => list.map((p) => p.name)),
  );

  return (
    <Section id="settings-providers" title={t("settings.providers.title")} class="mb-8">
      <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <ProviderCard
          label={t("settings.providers.git")}
          items={gitProviders() ?? []}
          loading={gitProviders.loading}
        />
        <ProviderCard
          label={t("settings.providers.agent")}
          items={agentProviders() ?? []}
          loading={agentProviders.loading}
        />
        <ProviderCard
          label={t("settings.providers.spec")}
          items={specProviders() ?? []}
          loading={specProviders.loading}
        />
        <ProviderCard
          label={t("settings.providers.pm")}
          items={pmProviders() ?? []}
          loading={pmProviders.loading}
        />
      </div>
    </Section>
  );
}
