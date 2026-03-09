import { createSignal, For, Show } from "solid-js";

import type { ProviderConfig } from "~/api/types";
import { useI18n } from "~/i18n";
import { Card, Checkbox, FormField, Input, Select } from "~/ui";

const DIFFICULTIES = ["easy", "medium", "hard"];

interface ProviderField {
  key: string;
  label: string;
  type: "text" | "select";
  options?: string[];
}

const PROVIDER_SETTINGS: Record<string, ProviderField[]> = {
  swebench: [
    { key: "variant", label: "Variant", type: "select", options: ["full", "lite", "verified"] },
    { key: "repo_filter", label: "Repository Filter", type: "text" },
  ],
  bigcodebench: [
    { key: "subset", label: "Subset", type: "select", options: ["complete", "hard", "instruct"] },
  ],
  cruxeval: [
    {
      key: "task_type",
      label: "Task Type",
      type: "select",
      options: ["input_prediction", "output_prediction"],
    },
  ],
  livecodebench: [
    { key: "date_range", label: "Date Range", type: "text" },
    { key: "contest_filter", label: "Contest Filter", type: "text" },
  ],
  sparcbench: [{ key: "category_filter", label: "Category Filter", type: "text" }],
  aider_polyglot: [
    {
      key: "language_filter",
      label: "Language",
      type: "select",
      options: ["python", "javascript", "typescript", "java", "go", "rust"],
    },
  ],
};

export interface TaskSettingsProps {
  providerName: string;
  config: ProviderConfig;
  onChange: (config: ProviderConfig) => void;
  taskCount?: number;
}

type LimitMode = "all" | "max" | "percent";

export function TaskSettings(props: TaskSettingsProps): ReturnType<typeof Card> {
  const { t } = useI18n();
  const [expanded, setExpanded] = createSignal(true);

  const limitMode = (): LimitMode => {
    if (props.config.max_tasks != null) return "max";
    if (props.config.task_percentage != null) return "percent";
    return "all";
  };

  const setLimitMode = (mode: LimitMode) => {
    const next = { ...props.config };
    delete next.max_tasks;
    delete next.task_percentage;
    if (mode === "max") next.max_tasks = 50;
    if (mode === "percent") next.task_percentage = 50;
    props.onChange(next);
  };

  const setField = (key: string, value: unknown) => {
    props.onChange({ ...props.config, [key]: value });
  };

  const toggleDifficulty = (d: string) => {
    const prev = props.config.difficulty_filter ?? [];
    const next = prev.includes(d) ? prev.filter((x) => x !== d) : [...prev, d];
    setField("difficulty_filter", next.length > 0 ? next : undefined);
  };

  const providerFields = () => PROVIDER_SETTINGS[props.providerName] ?? [];

  return (
    <Card class="mt-3">
      <button
        type="button"
        class="flex w-full items-center justify-between px-4 py-2 text-left text-sm font-semibold text-cf-text-primary"
        onClick={() => setExpanded(!expanded())}
      >
        {t("benchmark.taskSettings")}
        <span class="text-xs text-cf-text-muted">{expanded() ? "\u25B2" : "\u25BC"}</span>
      </button>

      <Show when={expanded()}>
        <div class="space-y-4 px-4 pb-4">
          {/* ---- Limit mode ---- */}
          <FormField
            label={
              limitMode() === "all"
                ? t("benchmark.allTasks").replace("{count}", String(props.taskCount ?? "?"))
                : limitMode() === "max"
                  ? t("benchmark.limitTasks")
                  : t("benchmark.percentTasks")
            }
            id="task-limit"
          >
            <div class="flex items-center gap-3">
              <Select
                value={limitMode()}
                onChange={(e) => setLimitMode(e.currentTarget.value as LimitMode)}
                class="w-36"
              >
                <option value="all">
                  {t("benchmark.allTasks").replace("{count}", String(props.taskCount ?? "?"))}
                </option>
                <option value="max">{t("benchmark.limitTasks")}</option>
                <option value="percent">{t("benchmark.percentTasks")}</option>
              </Select>

              <Show when={limitMode() === "max"}>
                <Input
                  type="number"
                  min={1}
                  max={props.taskCount ?? 10000}
                  value={props.config.max_tasks ?? 50}
                  onInput={(e) => setField("max_tasks", parseInt(e.currentTarget.value, 10) || 1)}
                  class="w-24"
                />
              </Show>

              <Show when={limitMode() === "percent"}>
                <Input
                  type="number"
                  min={1}
                  max={100}
                  value={props.config.task_percentage ?? 50}
                  onInput={(e) =>
                    setField("task_percentage", parseInt(e.currentTarget.value, 10) || 1)
                  }
                  class="w-24"
                />
                <span class="text-sm text-cf-text-muted">%</span>
              </Show>
            </div>
          </FormField>

          {/* ---- Difficulty filter ---- */}
          <FormField label={t("benchmark.difficulty")} id="task-difficulty">
            <div class="flex gap-3">
              <For each={DIFFICULTIES}>
                {(d) => (
                  <Checkbox
                    label={d}
                    checked={props.config.difficulty_filter?.includes(d) ?? false}
                    onChange={() => toggleDifficulty(d)}
                  />
                )}
              </For>
            </div>
          </FormField>

          {/* ---- Shuffle & Seed ---- */}
          <div class="flex items-center gap-4">
            <Checkbox
              label={t("benchmark.shuffle")}
              checked={props.config.shuffle ?? false}
              onChange={(v) => setField("shuffle", v || undefined)}
            />
            <Show when={props.config.shuffle}>
              <FormField label={t("benchmark.seed")} id="task-seed">
                <Input
                  type="number"
                  min={0}
                  value={props.config.seed ?? ""}
                  onInput={(e) => {
                    const v = e.currentTarget.value;
                    setField("seed", v ? parseInt(v, 10) : undefined);
                  }}
                  placeholder="optional"
                  class="w-32"
                />
              </FormField>
            </Show>
          </div>

          {/* ---- Provider-specific settings ---- */}
          <Show when={providerFields().length > 0}>
            <div class="border-t border-cf-border pt-3">
              <p class="mb-2 text-xs font-semibold text-cf-text-muted">
                {t("benchmark.providerSettings")}
              </p>
              <div class="space-y-3">
                <For each={providerFields()}>
                  {(field) => (
                    <FormField label={field.label} id={`provider-${field.key}`}>
                      <Show
                        when={field.type === "select" && field.options}
                        fallback={
                          <Input
                            value={(props.config[field.key] as string) ?? ""}
                            onInput={(e) => setField(field.key, e.currentTarget.value || undefined)}
                            placeholder={field.label}
                          />
                        }
                      >
                        <Select
                          value={(props.config[field.key] as string) ?? ""}
                          onChange={(e) => setField(field.key, e.currentTarget.value || undefined)}
                        >
                          <option value="">--</option>
                          <For each={field.options ?? []}>
                            {(opt) => <option value={opt}>{opt}</option>}
                          </For>
                        </Select>
                      </Show>
                    </FormField>
                  )}
                </For>
              </div>
            </div>
          </Show>
        </div>
      </Show>
    </Card>
  );
}
