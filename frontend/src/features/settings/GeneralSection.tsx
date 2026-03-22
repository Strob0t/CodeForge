import { createSignal, For, onMount } from "solid-js";

import { api } from "~/api/client";
import { useToast } from "~/components/Toast";
import { AUTONOMY_LEVELS } from "~/config/domain-constants";
import { useAsyncAction } from "~/hooks";
import { useI18n } from "~/i18n";
import { Button, Checkbox, FormField, ModelCombobox, Section, Select } from "~/ui";

export default function GeneralSection() {
  const { t } = useI18n();
  const { show: toast } = useToast();

  const [defaultProvider, setDefaultProvider] = createSignal("");
  const [defaultAutonomy, setDefaultAutonomy] = createSignal("supervised");
  const [autoClone, setAutoClone] = createSignal(false);

  onMount(async () => {
    try {
      const data = await api.settings.get();
      if (data.default_provider) setDefaultProvider(data.default_provider);
      if (data.default_autonomy) setDefaultAutonomy(data.default_autonomy);
      if (data.auto_clone !== undefined) setAutoClone(data.auto_clone);
    } catch {
      // Settings may not exist yet, use defaults
    }
  });

  const { run: handleSaveGeneral, loading: saving } = useAsyncAction(
    async () => {
      await api.settings.update({
        settings: {
          default_provider: defaultProvider(),
          default_autonomy: defaultAutonomy(),
          auto_clone: autoClone(),
        },
      });
      toast("success", t("settings.general.saved"));
    },
    {
      onError: () => {
        toast("error", t("settings.general.saveFailed"));
      },
    },
  );

  return (
    <Section id="settings-general" title={t("settings.general.title")} class="mb-8">
      <div class="space-y-4">
        <FormField
          label={t("settings.general.defaultProvider")}
          id="default-provider"
          help={t("settings.general.defaultProviderHelp")}
        >
          <ModelCombobox
            id="default-provider"
            value={defaultProvider()}
            onInput={setDefaultProvider}
            class="max-w-md"
          />
        </FormField>

        <FormField
          label={t("settings.general.defaultAutonomy")}
          id="default-autonomy"
          help={t("settings.general.defaultAutonomyHelp")}
        >
          <Select
            id="default-autonomy"
            value={defaultAutonomy()}
            onChange={(e) => setDefaultAutonomy(e.currentTarget.value)}
            class="max-w-md"
          >
            <For each={AUTONOMY_LEVELS}>
              {(level) => <option value={level.value}>{level.label}</option>}
            </For>
          </Select>
        </FormField>

        <div class="flex items-center gap-3">
          <Checkbox
            id="auto-clone"
            checked={autoClone()}
            onChange={(checked) => setAutoClone(checked)}
          />
          <div>
            <label for="auto-clone" class="text-sm font-medium text-cf-text-secondary">
              {t("settings.general.autoClone")}
            </label>
            <p class="text-xs text-cf-text-muted">{t("settings.general.autoCloneHelp")}</p>
          </div>
        </div>

        <div class="pt-2">
          <Button onClick={handleSaveGeneral} loading={saving()}>
            {t("settings.general.save")}
          </Button>
        </div>
      </div>
    </Section>
  );
}
