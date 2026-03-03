import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { BenchmarkSuite } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { Badge, Button, Card, EmptyState, FormField, Input, LoadingState } from "~/ui";

export function SuiteManagement() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [suites, { refetch }] = createResource(() => api.benchmarks.listSuites());
  const [showForm, setShowForm] = createSignal(false);
  const [editingSuite, setEditingSuite] = createSignal<BenchmarkSuite | null>(null);
  const [name, setName] = createSignal("");
  const [description, setDescription] = createSignal("");
  const [type, setType] = createSignal("deepeval");
  const [provider, setProvider] = createSignal("deepeval");

  const resetForm = () => {
    setName("");
    setDescription("");
    setType("deepeval");
    setProvider("deepeval");
    setEditingSuite(null);
  };

  const startEdit = (suite: BenchmarkSuite) => {
    setEditingSuite(suite);
    setName(suite.name);
    setDescription(suite.description || "");
    setType(String(suite.type));
    setProvider(suite.provider_name);
    setShowForm(true);
  };

  const handleCreate = async (e: SubmitEvent) => {
    e.preventDefault();
    try {
      await api.benchmarks.createSuite({
        name: name(),
        description: description() || undefined,
        type: type(),
        provider_name: provider(),
      });
      toast("success", t("benchmark.suites.toast.created"));
      setShowForm(false);
      resetForm();
      refetch();
    } catch {
      toast("error", t("benchmark.suites.toast.createError"));
    }
  };

  const handleUpdate = async (e: SubmitEvent) => {
    e.preventDefault();
    const suite = editingSuite();
    if (!suite) return;
    try {
      await api.benchmarks.updateSuite(suite.id, {
        name: name(),
        description: description() || undefined,
        type: type(),
        provider_name: provider(),
      });
      toast("success", t("benchmark.suites.toast.updated"));
      setShowForm(false);
      resetForm();
      refetch();
    } catch {
      toast("error", t("benchmark.suites.toast.updateError"));
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await api.benchmarks.deleteSuite(id);
      toast("success", t("benchmark.suites.toast.deleted"));
      refetch();
    } catch {
      toast("error", t("benchmark.suites.toast.deleteError"));
    }
  };

  return (
    <div class="space-y-4">
      <div class="flex gap-2">
        <Button
          size="sm"
          onClick={() => {
            if (showForm()) {
              setShowForm(false);
              resetForm();
            } else {
              resetForm();
              setShowForm(true);
            }
          }}
        >
          {showForm() ? t("common.cancel") : t("benchmark.suites.createBtn")}
        </Button>
      </div>

      <Show when={showForm()}>
        <Card class="p-4">
          <form onSubmit={editingSuite() ? handleUpdate : handleCreate} class="space-y-3">
            <Show when={editingSuite()}>
              <div class="mb-2 text-sm font-medium text-blue-600 dark:text-blue-400">
                Editing: {editingSuite()?.name}
              </div>
            </Show>
            <FormField label={t("benchmark.suites.name")} id="suite-name">
              <Input
                value={name()}
                onInput={(e) => setName(e.currentTarget.value)}
                placeholder="e.g. Code Quality Suite"
                required
              />
            </FormField>
            <FormField label="Description" id="suite-desc">
              <Input
                value={description()}
                onInput={(e) => setDescription(e.currentTarget.value)}
                placeholder="Optional description"
              />
            </FormField>
            <FormField label={t("benchmark.suites.type")} id="suite-type">
              <Input
                value={type()}
                onInput={(e) => setType(e.currentTarget.value)}
                placeholder="deepeval"
                required
              />
            </FormField>
            <FormField label={t("benchmark.suites.provider")} id="suite-provider">
              <Input
                value={provider()}
                onInput={(e) => setProvider(e.currentTarget.value)}
                placeholder="deepeval"
                required
              />
            </FormField>
            <Button type="submit" variant="primary" size="sm">
              {editingSuite() ? t("common.save") : t("benchmark.suites.createBtn")}
            </Button>
          </form>
        </Card>
      </Show>

      <Show when={!suites.loading} fallback={<LoadingState />}>
        <Show when={suites()?.length} fallback={<EmptyState title={t("benchmark.suites.empty")} />}>
          <div class="space-y-2">
            <For each={suites()}>
              {(suite: BenchmarkSuite) => (
                <Card class="flex items-center justify-between p-4">
                  <div>
                    <div class="font-medium">{suite.name}</div>
                    <Show when={suite.description}>
                      <div class="text-sm text-gray-500">{suite.description}</div>
                    </Show>
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
              )}
            </For>
          </div>
        </Show>
      </Show>
    </div>
  );
}
