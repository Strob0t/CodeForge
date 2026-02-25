import { createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { BenchmarkRun } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { Button, Card, FormField, Select } from "~/ui";

interface BenchmarkCompareProps {
  runs: BenchmarkRun[];
}

export function BenchmarkCompare(props: BenchmarkCompareProps) {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [compareA, setCompareA] = createSignal<string>("");
  const [compareB, setCompareB] = createSignal<string>("");

  return (
    <Show when={props.runs.length >= 2}>
      <Card class="mt-6 p-4">
        <h3 class="mb-3 text-sm font-semibold">{t("benchmark.compare")}</h3>
        <div class="flex items-end gap-3">
          <FormField label={t("benchmark.runA")} id="benchmark-compare-a">
            <Select value={compareA()} onChange={(e) => setCompareA(e.currentTarget.value)}>
              <option value="">{t("common.select")}</option>
              <For each={props.runs}>
                {(r: BenchmarkRun) => (
                  <option value={r.id}>
                    {r.dataset} / {r.model} ({r.status})
                  </option>
                )}
              </For>
            </Select>
          </FormField>
          <FormField label={t("benchmark.runB")} id="benchmark-compare-b">
            <Select value={compareB()} onChange={(e) => setCompareB(e.currentTarget.value)}>
              <option value="">{t("common.select")}</option>
              <For each={props.runs}>
                {(r: BenchmarkRun) => (
                  <option value={r.id}>
                    {r.dataset} / {r.model} ({r.status})
                  </option>
                )}
              </For>
            </Select>
          </FormField>
          <Button
            size="sm"
            variant="primary"
            disabled={!compareA() || !compareB() || compareA() === compareB()}
            onClick={async () => {
              try {
                const result = await api.benchmarks.compare(compareA(), compareB());
                console.log("Compare result:", result);
                toast("success", t("benchmark.toast.compareReady"));
              } catch {
                toast("error", t("benchmark.toast.compareError"));
              }
            }}
          >
            {t("benchmark.compareBtn")}
          </Button>
        </div>
      </Card>
    </Show>
  );
}
