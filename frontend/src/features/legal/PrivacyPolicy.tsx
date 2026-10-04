import { For, type JSX, onMount } from "solid-js";

import { type TranslationKey, useI18n } from "~/i18n";
import { Button } from "~/ui";

function Section(props: { title: TranslationKey; children: JSX.Element }): JSX.Element {
  const { t } = useI18n();
  return (
    <section class="mb-6">
      <h2 class="mb-2 text-lg font-semibold text-cf-text-primary">{t(props.title)}</h2>
      {props.children}
    </section>
  );
}

function Items(props: { keys: TranslationKey[] }): JSX.Element {
  const { t } = useI18n();
  return (
    <ul class="list-inside list-disc space-y-1 text-sm text-cf-text-secondary">
      <For each={props.keys}>{(key) => <li>{t(key)}</li>}</For>
    </ul>
  );
}

export default function PrivacyPolicy(): JSX.Element {
  const { t } = useI18n();
  onMount(() => {
    document.title = `${t("privacy.title")} - CodeForge`;
  });

  return (
    <div class="flex min-h-screen items-center justify-center bg-cf-bg-primary">
      <div class="w-full max-w-2xl rounded-lg border border-cf-border bg-cf-bg-surface p-8">
        <h1 class="mb-6 text-2xl font-bold text-cf-text-primary">{t("privacy.title")}</h1>

        <Section title="privacy.controller.title">
          <p class="text-sm text-cf-text-secondary">{t("privacy.controller.body")}</p>
        </Section>

        <Section title="privacy.collect.title">
          <Items
            keys={[
              "privacy.collect.account",
              "privacy.collect.auth",
              "privacy.collect.projects",
              "privacy.collect.conversations",
              "privacy.collect.usage",
            ]}
          />
        </Section>

        <Section title="privacy.dpo.title">
          <p class="text-sm text-cf-text-secondary">{t("privacy.dpo.body")}</p>
        </Section>

        <Section title="privacy.basis.title">
          <Items
            keys={[
              "privacy.basis.service",
              "privacy.basis.llm",
              "privacy.basis.security",
              "privacy.basis.cost",
            ]}
          />
        </Section>

        <Section title="privacy.processors.title">
          <p class="mb-2 text-sm text-cf-text-secondary">{t("privacy.processors.body")}</p>
          <Items
            keys={[
              "privacy.processors.openai",
              "privacy.processors.anthropic",
              "privacy.processors.google",
              "privacy.processors.local",
            ]}
          />
          <p class="mt-2 text-xs text-cf-text-muted">{t("privacy.processors.note")}</p>
        </Section>

        <Section title="privacy.retention.title">
          <Items
            keys={[
              "privacy.retention.account",
              "privacy.retention.conversations",
              "privacy.retention.sessions",
              "privacy.retention.runs",
              "privacy.retention.audit",
              "privacy.retention.auditIp",
              "privacy.retention.consent",
              "privacy.retention.consentIp",
            ]}
          />
        </Section>

        <Section title="privacy.rights.title">
          <Items
            keys={[
              "privacy.rights.access",
              "privacy.rights.rectification",
              "privacy.rights.erasure",
              "privacy.rights.portability",
              "privacy.rights.object",
              "privacy.rights.complaint",
            ]}
          />
        </Section>

        <div class="mt-8 text-center">
          <Button variant="ghost" onClick={() => window.history.back()}>
            {t("common.back")}
          </Button>
        </div>
      </div>
    </div>
  );
}
