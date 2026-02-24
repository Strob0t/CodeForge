import { createEffect, createSignal, For, onCleanup, Show } from "solid-js";

import { api } from "~/api/client";
import type { Mode } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";

import { ProjectCostSection } from "../costs/CostDashboardPage";

const AGENT_BACKENDS = ["aider", "goose", "opencode", "openhands", "plandex"] as const;

const AUTONOMY_LEVELS = [
  { value: "1", labelKey: "dashboard.form.autonomy.1" as const },
  { value: "2", labelKey: "dashboard.form.autonomy.2" as const },
  { value: "3", labelKey: "dashboard.form.autonomy.3" as const },
  { value: "4", labelKey: "dashboard.form.autonomy.4" as const },
  { value: "5", labelKey: "dashboard.form.autonomy.5" as const },
];

interface CompactSettingsPopoverProps {
  projectId: string;
  config: Record<string, string>;
  allModes: Mode[];
  open: boolean;
  onClose: () => void;
  onSaved: () => void;
}

export default function CompactSettingsPopover(props: CompactSettingsPopoverProps) {
  const { t } = useI18n();
  const { show: toast } = useToast();

  const [mode, setMode] = createSignal("");
  const [backends, setBackends] = createSignal<string[]>([]);
  const [autonomy, setAutonomy] = createSignal("");
  const [saving, setSaving] = createSignal(false);

  let popoverRef: HTMLDivElement | undefined;

  // Sync from props when popover opens
  createEffect(() => {
    if (props.open) {
      const cfg = props.config ?? {};
      setMode(cfg["default_mode"] ?? "");
      setBackends(cfg["agent_backends"] ? cfg["agent_backends"].split(",").filter(Boolean) : []);
      setAutonomy(cfg["autonomy_level"] ?? "");
    }
  });

  // Dismiss: click-outside and Escape key.
  // Listeners are registered once on mount and check props.open in the handler
  // to avoid SolidJS createEffect timing issues with addEventListener/removeEventListener.
  const handleClickOutside = (e: MouseEvent) => {
    // Check the parent container (which also holds the toggle button) so that
    // clicking the gear icon counts as "inside" and lets the toggle handler work.
    const container = popoverRef?.parentElement;
    if (props.open && container && !container.contains(e.target as Node)) {
      props.onClose();
    }
  };

  const handleKeyDown = (e: KeyboardEvent) => {
    if (props.open && e.key === "Escape") {
      e.preventDefault();
      props.onClose();
    }
  };

  document.addEventListener("mousedown", handleClickOutside);
  document.addEventListener("keydown", handleKeyDown);

  onCleanup(() => {
    document.removeEventListener("mousedown", handleClickOutside);
    document.removeEventListener("keydown", handleKeyDown);
  });

  function toggleBackend(backend: string) {
    setBackends((prev) =>
      prev.includes(backend) ? prev.filter((b) => b !== backend) : [...prev, backend],
    );
  }

  const handleSave = async () => {
    setSaving(true);
    try {
      const config: Record<string, string> = {};
      const m = mode();
      if (m) config["default_mode"] = m;
      const b = backends();
      if (b.length > 0) config["agent_backends"] = b.join(",");
      const a = autonomy();
      if (a) config["autonomy_level"] = a;

      await api.projects.update(props.projectId, { config });
      toast("success", t("detail.toast.settingsSaved"));
      props.onSaved();
    } catch (e) {
      const msg = e instanceof Error ? e.message : t("detail.toast.settingsFailed");
      toast("error", msg);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Show when={props.open}>
      <div
        ref={popoverRef}
        class="absolute right-0 top-full mt-2 w-96 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 shadow-lg z-50 p-4"
      >
        <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100 mb-3">
          {t("detail.settings.title")}
        </h3>

        {/* Mode Selection */}
        <div class="mb-3">
          <label
            for="popover_mode"
            class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1"
          >
            {t("detail.settings.defaultMode")}
          </label>
          <select
            id="popover_mode"
            value={mode()}
            onChange={(e) => setMode(e.currentTarget.value)}
            class="block w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-700 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            <option value="">{t("detail.settings.defaultModePlaceholder")}</option>
            <For each={props.allModes}>
              {(m: Mode) => (
                <option value={m.id}>
                  {m.name} {m.builtin ? `(${t("modes.builtin")})` : ""}
                </option>
              )}
            </For>
          </select>
        </div>

        {/* Autonomy Level */}
        <div class="mb-3">
          <label
            for="popover_autonomy"
            class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1"
          >
            {t("detail.settings.autonomyLevel")}
          </label>
          <select
            id="popover_autonomy"
            value={autonomy()}
            onChange={(e) => setAutonomy(e.currentTarget.value)}
            class="block w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-700 px-2 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            <option value="">{t("detail.settings.autonomyPlaceholder")}</option>
            <For each={AUTONOMY_LEVELS}>
              {(level) => <option value={level.value}>{t(level.labelKey)}</option>}
            </For>
          </select>
        </div>

        {/* Agent Backends */}
        <div class="mb-3">
          <span class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
            {t("detail.settings.agentBackends")}
          </span>
          <div class="flex flex-wrap gap-2">
            <For each={AGENT_BACKENDS}>
              {(backend) => (
                <label class="inline-flex items-center gap-1 text-xs text-gray-700 dark:text-gray-300 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={backends().includes(backend)}
                    onChange={() => toggleBackend(backend)}
                    class="rounded border-gray-300 dark:border-gray-600 text-blue-600 focus:ring-blue-500"
                  />
                  {backend}
                </label>
              )}
            </For>
          </div>
        </div>

        {/* Save Button */}
        <div class="mb-4 flex justify-end">
          <button
            type="button"
            class="rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            onClick={handleSave}
            disabled={saving()}
          >
            {saving() ? t("detail.settings.saving") : t("detail.settings.save")}
          </button>
        </div>

        {/* Cost Summary */}
        <div class="border-t border-gray-200 dark:border-gray-700 pt-3">
          <h4 class="text-xs font-medium text-gray-500 dark:text-gray-400 mb-2">
            {t("detail.settings.costSummary")}
          </h4>
          <ProjectCostSection projectId={props.projectId} />
        </div>
      </div>
    </Show>
  );
}
