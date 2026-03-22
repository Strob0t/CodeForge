import { createSignal, For, onCleanup, onMount } from "solid-js";

import { useI18n } from "~/i18n";
import { PageLayout, Section } from "~/ui";
import { cx } from "~/utils/cx";

import APIKeysSection from "./APIKeysSection";
import DevToolsSection from "./DevToolsSection";
import GeneralSection from "./GeneralSection";
import ProvidersSection from "./ProvidersSection";
import ProxySection from "./ProxySection";
import { SETTINGS_SECTIONS } from "./settingsTypes";
import { ShortcutsSection } from "./ShortcutsSection";
import SubscriptionsSection from "./SubscriptionsSection";
import UsersSection from "./UsersSection";
import VCSSection from "./VCSSection";

export default function SettingsPage() {
  onMount(() => {
    document.title = "Settings - CodeForge";
  });
  const { t } = useI18n();

  // -- Section navigation ------------------------------------------------------
  const sections = SETTINGS_SECTIONS;

  const [activeSection, setActiveSection] = createSignal("settings-general");

  onMount(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) setActiveSection(entry.target.id);
        }
      },
      { rootMargin: "-20% 0px -80% 0px" },
    );
    for (const s of sections) {
      const el = document.getElementById(s.id);
      if (el) observer.observe(el);
    }
    onCleanup(() => observer.disconnect());
  });

  return (
    <PageLayout title={t("settings.title")}>
      <nav class="sticky top-0 z-10 bg-cf-bg-primary/95 backdrop-blur-sm border-b border-cf-border overflow-x-auto whitespace-nowrap flex gap-1 py-2 mb-4">
        <For each={sections}>
          {(s) => (
            <button
              class={cx(
                "px-3 py-1.5 text-xs font-medium rounded-cf-sm transition-colors shrink-0",
                activeSection() === s.id
                  ? "bg-cf-accent text-cf-accent-fg"
                  : "text-cf-text-secondary hover:bg-cf-bg-surface-alt",
              )}
              onClick={() => document.getElementById(s.id)?.scrollIntoView({ behavior: "smooth" })}
            >
              {s.label}
            </button>
          )}
        </For>
      </nav>

      <GeneralSection />

      <Section id="settings-shortcuts" title={t("settings.shortcuts.title")} class="mb-8">
        <ShortcutsSection />
      </Section>

      <VCSSection />
      <ProvidersSection />
      <ProxySection />
      <SubscriptionsSection />
      <APIKeysSection />
      <UsersSection />
      <DevToolsSection />
    </PageLayout>
  );
}
