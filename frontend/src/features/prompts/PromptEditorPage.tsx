import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { PromptSectionRow } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  FormField,
  Input,
  LoadingState,
  PageLayout,
  Select,
  Textarea,
} from "~/ui";

const MERGE_OPTIONS = ["replace", "prepend", "append"];

const SCOPE_OPTIONS = ["global"];

export default function PromptEditorPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();

  const [scope, setScope] = createSignal("global");
  const [sections, { refetch }] = createResource(scope, (s) => api.promptSections.list(s));
  const [showForm, setShowForm] = createSignal(false);
  const [editingId, setEditingId] = createSignal<string | null>(null);
  const [previewText, setPreviewText] = createSignal("");
  const [previewTokens, setPreviewTokens] = createSignal(0);

  // Form state
  const [formName, setFormName] = createSignal("");
  const [formContent, setFormContent] = createSignal("");
  const [formPriority, setFormPriority] = createSignal(50);
  const [formSortOrder, setFormSortOrder] = createSignal(0);
  const [formEnabled, setFormEnabled] = createSignal(true);
  const [formMerge, setFormMerge] = createSignal("replace");
  const [saving, setSaving] = createSignal(false);

  function resetForm() {
    setFormName("");
    setFormContent("");
    setFormPriority(50);
    setFormSortOrder(0);
    setFormEnabled(true);
    setFormMerge("replace");
    setEditingId(null);
  }

  function handleEdit(row: PromptSectionRow) {
    setFormName(row.name);
    setFormContent(row.content);
    setFormPriority(row.priority);
    setFormSortOrder(row.sort_order);
    setFormEnabled(row.enabled);
    setFormMerge(row.merge);
    setEditingId(row.id);
    setShowForm(true);
  }

  async function handleSave() {
    if (!formName().trim()) {
      toast("error", t("prompts.error.nameRequired"));
      return;
    }
    setSaving(true);
    try {
      await api.promptSections.upsert({
        id: editingId() ?? "",
        name: formName().trim(),
        scope: scope(),
        content: formContent(),
        priority: formPriority(),
        sort_order: formSortOrder(),
        enabled: formEnabled(),
        merge: formMerge() as "replace" | "prepend" | "append",
      });
      toast("success", t("prompts.saved"));
      setShowForm(false);
      resetForm();
      refetch();
    } catch {
      toast("error", t("prompts.error.saveFailed"));
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete(id: string) {
    try {
      await api.promptSections.delete(id);
      toast("success", t("prompts.deleted"));
      refetch();
    } catch {
      toast("error", t("prompts.error.deleteFailed"));
    }
  }

  async function handlePreview() {
    const data = sections();
    if (!data || data.length === 0) {
      setPreviewText("");
      setPreviewTokens(0);
      return;
    }
    try {
      const previewSections = data
        .filter((s) => s.enabled)
        .map((s) => ({
          name: s.name,
          text: s.content,
          tokens: 0,
          priority: s.priority,
          source: "db_custom" as const,
          enabled: s.enabled,
        }));
      const res = await api.promptSections.preview({
        sections: previewSections,
        budget: 2048,
      });
      setPreviewText(res.text);
      setPreviewTokens(res.total_tokens);
    } catch {
      toast("error", t("prompts.error.previewFailed"));
    }
  }

  function estimateTokens(text: string): number {
    return Math.ceil(text.length / 4);
  }

  return (
    <PageLayout title={t("prompts.title")} description={t("prompts.subtitle")}>
      {/* Scope selector + actions */}
      <div class="mb-4 flex items-center gap-3">
        <Select value={scope()} onChange={(e) => setScope(e.currentTarget.value)} class="w-40">
          <For each={SCOPE_OPTIONS}>
            {(s) => <option value={s}>{s.charAt(0).toUpperCase() + s.slice(1)}</option>}
          </For>
        </Select>
        <Button
          onClick={() => {
            resetForm();
            setShowForm(true);
          }}
          size="sm"
        >
          {t("prompts.add")}
        </Button>
        <Button onClick={handlePreview} size="sm" variant="ghost">
          {t("prompts.preview")}
        </Button>
      </div>

      {/* Preview panel */}
      <Show when={previewText()}>
        <Card class="mb-4">
          <div class="flex items-center justify-between px-4 py-2">
            <span class="text-sm font-medium">{t("prompts.previewTitle")}</span>
            <Badge variant={previewTokens() > 2048 ? "error" : "success"}>
              {previewTokens()} tokens
            </Badge>
          </div>
          <pre class="max-h-60 overflow-auto whitespace-pre-wrap border-t border-cf-border px-4 py-2 font-mono text-xs text-cf-text-muted">
            {previewText()}
          </pre>
        </Card>
      </Show>

      {/* Section form */}
      <Show when={showForm()}>
        <Card class="mb-4 p-4">
          <h3 class="mb-3 text-sm font-semibold">
            {editingId() ? t("prompts.editSection") : t("prompts.newSection")}
          </h3>
          <div class="grid grid-cols-2 gap-3">
            <FormField label={t("prompts.field.name")}>
              <Input value={formName()} onInput={(e) => setFormName(e.currentTarget.value)} />
            </FormField>
            <FormField label={t("prompts.field.merge")}>
              <Select value={formMerge()} onChange={(e) => setFormMerge(e.currentTarget.value)}>
                <For each={MERGE_OPTIONS}>
                  {(m) => <option value={m}>{m.charAt(0).toUpperCase() + m.slice(1)}</option>}
                </For>
              </Select>
            </FormField>
            <FormField label={t("prompts.field.priority")}>
              <div class="flex items-center gap-2">
                <input
                  type="range"
                  min="0"
                  max="100"
                  value={formPriority()}
                  onInput={(e) => setFormPriority(Number(e.currentTarget.value))}
                  class="flex-1"
                />
                <span class="w-8 text-center text-xs text-cf-text-muted">{formPriority()}</span>
              </div>
            </FormField>
            <FormField label={t("prompts.field.sortOrder")}>
              <Input
                type="number"
                value={String(formSortOrder())}
                onInput={(e) => setFormSortOrder(Number(e.currentTarget.value))}
              />
            </FormField>
          </div>
          <FormField label={t("prompts.field.content")} class="mt-3">
            <Textarea
              value={formContent()}
              onInput={(e) => setFormContent(e.currentTarget.value)}
              rows={8}
              class="font-mono text-xs"
            />
            <span class="mt-1 text-xs text-cf-text-muted">
              ~{estimateTokens(formContent())} tokens
            </span>
          </FormField>
          <div class="mt-3 flex items-center gap-2">
            <label class="flex items-center gap-1 text-sm">
              <input
                type="checkbox"
                checked={formEnabled()}
                onChange={(e) => setFormEnabled(e.currentTarget.checked)}
              />
              {t("prompts.field.enabled")}
            </label>
          </div>
          <div class="mt-4 flex gap-2">
            <Button onClick={handleSave} size="sm" disabled={saving()}>
              {saving() ? t("common.saving") : t("common.save")}
            </Button>
            <Button
              onClick={() => {
                setShowForm(false);
                resetForm();
              }}
              size="sm"
              variant="ghost"
            >
              {t("common.cancel")}
            </Button>
          </div>
        </Card>
      </Show>

      {/* Section list */}
      <Show when={!sections.loading} fallback={<LoadingState />}>
        <Show
          when={(sections() ?? []).length > 0}
          fallback={<EmptyState title={t("prompts.empty")} />}
        >
          <div class="space-y-2">
            <For each={sections()}>
              {(row) => (
                <Card class="flex items-center gap-3 px-4 py-3">
                  <div class="flex-1">
                    <div class="flex items-center gap-2">
                      <span class="text-sm font-medium">{row.name}</span>
                      <Badge variant={row.enabled ? "success" : "neutral"}>
                        {row.enabled ? "on" : "off"}
                      </Badge>
                      <Badge variant="neutral">{row.merge}</Badge>
                      <span class="text-xs text-cf-text-muted">
                        P{row.priority} / S{row.sort_order}
                      </span>
                    </div>
                    <p class="mt-1 line-clamp-2 text-xs text-cf-text-muted">{row.content}</p>
                  </div>
                  <div class="flex gap-1">
                    <Button onClick={() => handleEdit(row)} size="sm" variant="ghost">
                      {t("common.edit")}
                    </Button>
                    <Button onClick={() => handleDelete(row.id)} size="sm" variant="ghost">
                      {t("common.delete")}
                    </Button>
                  </div>
                </Card>
              )}
            </For>
          </div>
        </Show>
      </Show>
    </PageLayout>
  );
}
