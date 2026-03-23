import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { PromptVariant } from "~/api/types";
import { useConfirm } from "~/components/ConfirmProvider";
import { useToast } from "~/components/Toast";
import { useAsyncAction } from "~/hooks";
import { useI18n } from "~/i18n";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  FormField,
  Input,
  LoadingState,
  Select,
  Textarea,
} from "~/ui";
import { DocumentPenIcon } from "~/ui/icons/EmptyStateIcons";

const MODEL_FAMILIES = ["openai", "anthropic", "google", "meta", "local"];

function statusBadgeVariant(status: string): "neutral" | "success" | "warning" {
  if (status === "promoted") return "success";
  if (status === "retired") return "warning";
  return "neutral";
}

export default function EvolutionTab() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const { confirm } = useConfirm();

  // --- Status section ---
  const [status] = createResource(() => api.promptEvolution.status());

  // --- Variants table ---
  const [filterMode, setFilterMode] = createSignal("");
  const [filterStatus, setFilterStatus] = createSignal("");

  const [variants, { refetch: refetchVariants }] = createResource(
    () => ({ mode: filterMode(), status: filterStatus() }),
    (filters) =>
      api.promptEvolution.variants(filters.mode || undefined, filters.status || undefined),
  );

  // --- Promote action ---
  const { run: handlePromote } = useAsyncAction(
    async (variant: PromptVariant) => {
      const ok = await confirm({
        title: t("prompts.evolution.promote"),
        message: t("prompts.evolution.confirm.promote"),
        variant: "warning",
        confirmLabel: t("prompts.evolution.promote"),
      });
      if (!ok) return;
      await api.promptEvolution.promote(variant.id);
      toast("success", t("prompts.evolution.promoted"));
      refetchVariants();
    },
    { onError: () => toast("error", t("prompts.evolution.error.promoteFailed")) },
  );

  // --- Revert action ---
  const { run: handleRevert } = useAsyncAction(
    async (variant: PromptVariant) => {
      const ok = await confirm({
        title: t("prompts.evolution.revert"),
        message: t("prompts.evolution.confirm.revert"),
        variant: "danger",
        confirmLabel: t("prompts.evolution.revert"),
      });
      if (!ok) return;
      await api.promptEvolution.revert(variant.mode_id);
      toast("success", t("prompts.evolution.reverted"));
      refetchVariants();
    },
    { onError: () => toast("error", t("prompts.evolution.error.revertFailed")) },
  );

  // --- Trigger Reflection form ---
  const [reflectModeId, setReflectModeId] = createSignal("");
  const [reflectModelFamily, setReflectModelFamily] = createSignal("openai");
  const [reflectPrompt, setReflectPrompt] = createSignal("");

  const { run: handleReflect, loading: reflecting } = useAsyncAction(
    async () => {
      if (!reflectModeId().trim()) {
        toast("error", t("prompts.evolution.error.reflectFailed"));
        return;
      }
      if (!reflectPrompt().trim()) {
        toast("error", t("prompts.evolution.error.reflectFailed"));
        return;
      }
      await api.promptEvolution.reflect({
        mode_id: reflectModeId().trim(),
        model_family: reflectModelFamily(),
        current_prompt: reflectPrompt(),
      });
      toast("success", t("prompts.evolution.reflectionTriggered"));
      setReflectModeId("");
      setReflectPrompt("");
    },
    { onError: () => toast("error", t("prompts.evolution.error.reflectFailed")) },
  );

  return (
    <div class="space-y-6">
      {/* --- Status section --- */}
      <Card class="p-4">
        <h3 class="mb-3 text-sm font-semibold">{t("prompts.evolution.status")}</h3>
        <Show when={!status.loading} fallback={<LoadingState />}>
          <Show when={status()}>
            {(st) => (
              <div class="flex flex-wrap items-center gap-3">
                <Badge variant={st().enabled ? "success" : "neutral"}>
                  {st().enabled ? t("prompts.evolution.enabled") : "Disabled"}
                </Badge>
                <span class="text-xs text-cf-text-muted">
                  {t("prompts.evolution.trigger")}: {st().trigger}
                </span>
                <span class="text-xs text-cf-text-muted">
                  {t("prompts.evolution.strategy")}: {st().strategy}
                </span>
              </div>
            )}
          </Show>
          <Show when={status()?.mode_status}>
            {(modeStatus) => (
              <div class="mt-3 space-y-1">
                <For each={Object.values(modeStatus())}>
                  {(ms) => (
                    <div class="flex items-center gap-3 rounded bg-cf-bg-secondary px-3 py-1.5 text-xs">
                      <span class="font-medium">{ms.mode_id}</span>
                      <span class="text-cf-text-muted">Candidates: {ms.candidate_count}</span>
                      <span class="text-cf-text-muted">Trials: {ms.total_trials}</span>
                      <span class="text-cf-text-muted">Avg: {ms.avg_score.toFixed(2)}</span>
                    </div>
                  )}
                </For>
              </div>
            )}
          </Show>
        </Show>
      </Card>

      {/* --- Variants table --- */}
      <Card class="p-4">
        <div class="mb-3 flex items-center justify-between">
          <h3 class="text-sm font-semibold">{t("prompts.evolution.variants")}</h3>
          <div class="flex gap-2">
            <Input
              value={filterMode()}
              onInput={(e) => setFilterMode(e.currentTarget.value)}
              placeholder="Filter by mode..."
              class="w-36 text-xs"
            />
            <Select
              value={filterStatus()}
              onChange={(e) => setFilterStatus(e.currentTarget.value)}
              class="w-32 text-xs"
            >
              <option value="">All</option>
              <option value="candidate">Candidate</option>
              <option value="promoted">Promoted</option>
              <option value="retired">Retired</option>
            </Select>
          </div>
        </div>

        <Show when={!variants.loading} fallback={<LoadingState />}>
          <Show
            when={(variants() ?? []).length > 0}
            fallback={
              <EmptyState
                illustration={<DocumentPenIcon />}
                title={t("prompts.evolution.empty")}
                description={t("prompts.evolution.emptyDescription")}
              />
            }
          >
            <div class="overflow-x-auto">
              <table class="w-full text-left text-xs">
                <thead>
                  <tr class="border-b border-cf-border text-cf-text-muted">
                    <th class="px-2 py-1.5 font-medium">Mode</th>
                    <th class="px-2 py-1.5 font-medium">Model Family</th>
                    <th class="px-2 py-1.5 font-medium">{t("common.status")}</th>
                    <th class="px-2 py-1.5 font-medium">Version</th>
                    <th class="px-2 py-1.5 font-medium">Avg Score</th>
                    <th class="px-2 py-1.5 font-medium">Trials</th>
                    <th class="px-2 py-1.5 font-medium">Content</th>
                    <th class="px-2 py-1.5 font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  <For each={variants()}>
                    {(v) => (
                      <tr class="border-b border-cf-border/50">
                        <td class="px-2 py-1.5 font-medium">{v.mode_id}</td>
                        <td class="px-2 py-1.5">{v.model_family}</td>
                        <td class="px-2 py-1.5">
                          <Badge variant={statusBadgeVariant(v.promotion_status)}>
                            {v.promotion_status}
                          </Badge>
                        </td>
                        <td class="px-2 py-1.5">{v.version}</td>
                        <td class="px-2 py-1.5">{v.avg_score.toFixed(2)}</td>
                        <td class="px-2 py-1.5">{v.trial_count}</td>
                        <td class="max-w-xs truncate px-2 py-1.5 text-cf-text-muted">
                          {v.content.slice(0, 80)}
                          {v.content.length > 80 ? "..." : ""}
                        </td>
                        <td class="px-2 py-1.5">
                          <div class="flex gap-1">
                            <Show when={v.promotion_status === "candidate"}>
                              <Button
                                onClick={() => void handlePromote(v)}
                                size="sm"
                                variant="ghost"
                              >
                                {t("prompts.evolution.promote")}
                              </Button>
                            </Show>
                            <Show when={v.promotion_status === "promoted"}>
                              <Button
                                onClick={() => void handleRevert(v)}
                                size="sm"
                                variant="ghost"
                              >
                                {t("prompts.evolution.revert")}
                              </Button>
                            </Show>
                          </div>
                        </td>
                      </tr>
                    )}
                  </For>
                </tbody>
              </table>
            </div>
          </Show>
        </Show>
      </Card>

      {/* --- Trigger Reflection form --- */}
      <Card class="p-4">
        <h3 class="mb-3 text-sm font-semibold">{t("prompts.evolution.triggerReflection")}</h3>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <FormField label={t("prompts.evolution.field.modeId")}>
            <Input
              value={reflectModeId()}
              onInput={(e) => setReflectModeId(e.currentTarget.value)}
              placeholder="e.g. coder"
            />
          </FormField>
          <FormField label={t("prompts.evolution.field.modelFamily")}>
            <Select
              value={reflectModelFamily()}
              onChange={(e) => setReflectModelFamily(e.currentTarget.value)}
            >
              <For each={MODEL_FAMILIES}>
                {(f) => <option value={f}>{f.charAt(0).toUpperCase() + f.slice(1)}</option>}
              </For>
            </Select>
          </FormField>
        </div>
        <FormField label={t("prompts.evolution.field.currentPrompt")} class="mt-3">
          <Textarea
            value={reflectPrompt()}
            onInput={(e) => setReflectPrompt(e.currentTarget.value)}
            rows={6}
            class="font-mono text-xs"
            placeholder="Paste the current prompt text here..."
          />
        </FormField>
        <div class="mt-4">
          <Button onClick={() => void handleReflect()} size="sm" disabled={reflecting()}>
            {reflecting() ? t("common.loading") : t("prompts.evolution.triggerReflection")}
          </Button>
        </div>
      </Card>
    </div>
  );
}
