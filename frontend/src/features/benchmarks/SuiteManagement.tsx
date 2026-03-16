import { createResource, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { BenchmarkSuite } from "~/api/types";
import { useConfirm } from "~/components/ConfirmProvider";
import { useToast } from "~/components/Toast";
import { useCRUDForm } from "~/hooks";
import { useI18n } from "~/i18n";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  FormField,
  Input,
  LoadingState,
  ResourceView,
  Select,
} from "~/ui";

const KNOWN_PROVIDERS: { value: string; label: string; type: string }[] = [
  { value: "codeforge_simple", label: "CodeForge Simple", type: "simple" },
  { value: "codeforge_tool_use", label: "CodeForge Tool Use", type: "tool_use" },
  { value: "codeforge_agent", label: "CodeForge Agent", type: "agent" },
  { value: "humaneval", label: "HumanEval", type: "simple" },
  { value: "mbpp", label: "MBPP", type: "simple" },
  { value: "swebench", label: "SWE-bench", type: "agent" },
  { value: "bigcodebench", label: "BigCodeBench", type: "simple" },
  { value: "cruxeval", label: "CRUXEval", type: "simple" },
  { value: "livecodebench", label: "LiveCodeBench", type: "simple" },
  { value: "sparcbench", label: "SPARCBench", type: "agent" },
  { value: "aider_polyglot", label: "Aider Polyglot", type: "agent" },
];

/** External provider names that indicate a seeded/built-in suite. */
const SEEDED_PROVIDERS = new Set<string>(
  KNOWN_PROVIDERS.filter((p) => !p.value.startsWith("codeforge_")).map((p) => p.value),
);

function isSeededSuite(providerName: string): boolean {
  return SEEDED_PROVIDERS.has(providerName);
}

export function SuiteManagement() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const { confirm } = useConfirm();
  const [suites, { refetch }] = createResource(() => api.benchmarks.listSuites());

  const crud = useCRUDForm({
    name: "",
    description: "",
    type: KNOWN_PROVIDERS[0].type,
    provider: KNOWN_PROVIDERS[0].value,
  });

  const startEdit = (suite: BenchmarkSuite) => {
    crud.startEdit(suite.id, {
      name: suite.name,
      description: suite.description || "",
      type: String(suite.type),
      provider: suite.provider_name,
    });
  };

  const handleSubmit = async (e: SubmitEvent) => {
    e.preventDefault();
    const payload = {
      name: crud.form.state.name,
      description: crud.form.state.description || undefined,
      type: crud.form.state.type,
      provider_name: crud.form.state.provider,
    };
    try {
      const eid = crud.editingId();
      if (crud.isEditing() && eid) {
        await api.benchmarks.updateSuite(eid, payload);
        toast("success", t("benchmark.suites.toast.updated"));
      } else {
        await api.benchmarks.createSuite(payload);
        toast("success", t("benchmark.suites.toast.created"));
      }
      crud.cancelForm();
      refetch();
    } catch {
      toast(
        "error",
        crud.isEditing()
          ? t("benchmark.suites.toast.updateError")
          : t("benchmark.suites.toast.createError"),
      );
    }
  };

  const handleDelete = async (id: string) => {
    const ok = await confirm({
      title: t("common.delete"),
      message: t("benchmark.suites.confirm.delete"),
      variant: "danger",
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    try {
      await api.benchmarks.deleteSuite(id);
      toast("success", t("benchmark.suites.toast.deleted"));
      refetch();
    } catch {
      toast("error", t("benchmark.suites.toast.deleteError"));
    }
  };

  const suiteData = () => {
    const items = suites();
    return items?.length ? items : undefined;
  };

  return (
    <div class="space-y-4">
      <div class="flex gap-2">
        <Button
          size="sm"
          onClick={() => {
            if (crud.showForm()) {
              crud.cancelForm();
            } else {
              crud.startCreate();
            }
          }}
        >
          {crud.showForm() ? t("common.cancel") : t("benchmark.suites.createBtn")}
        </Button>
      </div>

      <Show when={crud.showForm()}>
        <Card class="p-4">
          <form onSubmit={handleSubmit} class="space-y-3">
            <Show when={crud.isEditing()}>
              <div class="mb-2 text-sm font-medium text-blue-600 dark:text-blue-400">
                Editing: {crud.form.state.name}
              </div>
            </Show>
            <FormField label={t("benchmark.suites.name")} id="suite-name">
              <Input
                value={crud.form.state.name}
                onInput={(e) => crud.form.setState("name", e.currentTarget.value)}
                placeholder="e.g. Code Quality Suite"
                required
              />
            </FormField>
            <FormField label="Description" id="suite-desc">
              <Input
                value={crud.form.state.description}
                onInput={(e) => crud.form.setState("description", e.currentTarget.value)}
                placeholder="Optional description"
              />
            </FormField>
            <FormField label={t("benchmark.suites.provider")} id="suite-provider">
              <Select
                value={crud.form.state.provider}
                onChange={(e) => {
                  const val = e.currentTarget.value;
                  crud.form.setState("provider", val);
                  const known = KNOWN_PROVIDERS.find((p) => p.value === val);
                  if (known) {
                    crud.form.setState("type", known.type);
                  }
                }}
                required
              >
                <For each={KNOWN_PROVIDERS}>
                  {(p) => <option value={p.value}>{p.label}</option>}
                </For>
              </Select>
            </FormField>
            <FormField label={t("benchmark.suites.type")} id="suite-type">
              <Input
                value={crud.form.state.type}
                onInput={(e) => crud.form.setState("type", e.currentTarget.value)}
                placeholder="simple"
                required
              />
            </FormField>
            <Button type="submit" variant="primary" size="sm">
              {crud.isEditing() ? t("common.save") : t("benchmark.suites.createBtn")}
            </Button>
          </form>
        </Card>
      </Show>

      <ResourceView
        loading={suites.loading}
        data={suiteData()}
        loadingFallback={<LoadingState />}
        emptyFallback={<EmptyState title={t("benchmark.suites.empty")} />}
      >
        {(items) => (
          <div class="space-y-2">
            <For each={items}>
              {(suite: BenchmarkSuite) => {
                const seeded = isSeededSuite(suite.provider_name);
                return (
                  <Card class="flex items-center justify-between p-4">
                    <div class="flex items-center gap-2">
                      <Show when={seeded}>
                        <svg
                          class="h-4 w-4 shrink-0 text-gray-400"
                          viewBox="0 0 20 20"
                          fill="currentColor"
                          aria-label="Built-in suite"
                        >
                          <path
                            fill-rule="evenodd"
                            d="M10 1a4.5 4.5 0 00-4.5 4.5V9H5a2 2 0 00-2 2v6a2 2 0 002 2h10a2 2 0 002-2v-6a2 2 0 00-2-2h-.5V5.5A4.5 4.5 0 0010 1zm3 8V5.5a3 3 0 10-6 0V9h6z"
                            clip-rule="evenodd"
                          />
                        </svg>
                      </Show>
                      <div>
                        <div class="font-medium">{suite.name}</div>
                        <Show when={suite.description}>
                          <div class="text-sm text-gray-500">{suite.description}</div>
                        </Show>
                      </div>
                    </div>
                    <div class="flex items-center gap-2">
                      <Badge variant="default">{suite.type}</Badge>
                      <span class="text-xs text-gray-500">{suite.provider_name}</span>
                      <Badge variant="default">
                        {suite.task_count} {t("benchmark.tasks")}
                      </Badge>
                      <Button size="sm" variant="secondary" onClick={() => startEdit(suite)}>
                        {t("common.edit")}
                      </Button>
                      <Button size="sm" variant="danger" onClick={() => handleDelete(suite.id)}>
                        {t("common.delete")}
                      </Button>
                    </div>
                  </Card>
                );
              }}
            </For>
          </div>
        )}
      </ResourceView>
    </div>
  );
}
