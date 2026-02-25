import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { CreateKnowledgeBaseRequest, KnowledgeBase } from "~/api/types";
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
} from "~/ui";
import type { BadgeVariant } from "~/ui/primitives/Badge";

const CATEGORIES = ["framework", "paradigm", "language", "security", "custom"] as const;

export default function KnowledgeBasesPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [kbs, { refetch }] = createResource(() => api.knowledgeBases.list());
  const [showForm, setShowForm] = createSignal(false);

  // -- Form state --
  const [formName, setFormName] = createSignal("");
  const [formDesc, setFormDesc] = createSignal("");
  const [formCategory, setFormCategory] = createSignal<string>("custom");
  const [formTags, setFormTags] = createSignal("");
  const [formContentPath, setFormContentPath] = createSignal("");

  const resetForm = () => {
    setFormName("");
    setFormDesc("");
    setFormCategory("custom");
    setFormTags("");
    setFormContentPath("");
  };

  const handleCreate = async (e: SubmitEvent) => {
    e.preventDefault();
    const name = formName().trim();
    if (!name) return;
    try {
      const req: CreateKnowledgeBaseRequest = {
        name,
        description: formDesc().trim(),
        category: formCategory(),
        tags: formTags()
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean),
        content_path: formContentPath().trim(),
      };
      await api.knowledgeBases.create(req);
      resetForm();
      setShowForm(false);
      refetch();
      toast("success", t("kb.toast.created"));
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to create knowledge base");
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await api.knowledgeBases.delete(id);
      refetch();
      toast("success", t("kb.toast.deleted"));
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete knowledge base");
    }
  };

  const [indexingId, setIndexingId] = createSignal<string | null>(null);

  const handleIndex = async (id: string) => {
    try {
      setIndexingId(id);
      await api.knowledgeBases.index(id);
      refetch();
      toast("success", t("kb.toast.indexed"));
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to index knowledge base");
    } finally {
      setIndexingId(null);
    }
  };

  const sorted = () => {
    const list = kbs() ?? [];
    return [...list].sort((a, b) => a.name.localeCompare(b.name));
  };

  return (
    <PageLayout
      title={t("kb.title")}
      description={t("kb.description")}
      action={
        <Button onClick={() => setShowForm((v) => !v)}>
          {showForm() ? "Cancel" : t("kb.form.create")}
        </Button>
      }
    >
      <Show when={showForm()}>
        <form onSubmit={handleCreate} class="mb-6" aria-label={t("kb.form.create")}>
          <Card>
            <Card.Body>
              <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <FormField label={t("kb.form.name")} id="kb-name" required>
                  <Input
                    id="kb-name"
                    type="text"
                    value={formName()}
                    onInput={(e) => setFormName(e.currentTarget.value)}
                    required
                  />
                </FormField>
                <FormField label={t("kb.form.category")} id="kb-category">
                  <Select
                    id="kb-category"
                    value={formCategory()}
                    onChange={(e) => setFormCategory(e.currentTarget.value)}
                  >
                    <For each={[...CATEGORIES]}>
                      {(cat) => (
                        <option value={cat}>
                          {t(`kb.category.${cat}` as keyof typeof import("~/i18n/en").default)}
                        </option>
                      )}
                    </For>
                  </Select>
                </FormField>
                <FormField label={t("kb.form.description")} id="kb-desc" class="sm:col-span-2">
                  <Input
                    id="kb-desc"
                    type="text"
                    value={formDesc()}
                    onInput={(e) => setFormDesc(e.currentTarget.value)}
                  />
                </FormField>
                <FormField label={t("kb.form.tags")} id="kb-tags">
                  <Input
                    id="kb-tags"
                    type="text"
                    value={formTags()}
                    onInput={(e) => setFormTags(e.currentTarget.value)}
                    placeholder="tag1, tag2, tag3"
                  />
                </FormField>
                <FormField label={t("kb.form.contentPath")} id="kb-content-path" required>
                  <Input
                    id="kb-content-path"
                    type="text"
                    value={formContentPath()}
                    onInput={(e) => setFormContentPath(e.currentTarget.value)}
                    placeholder="/absolute/path/to/content"
                    required
                  />
                </FormField>
              </div>
              <div class="mt-4 flex justify-end">
                <Button type="submit">{t("kb.form.create")}</Button>
              </div>
            </Card.Body>
          </Card>
        </form>
      </Show>

      <Show when={kbs.loading}>
        <LoadingState message={t("kb.loading")} />
      </Show>

      <Show when={!kbs.loading}>
        <Show when={sorted().length} fallback={<EmptyState title={t("kb.empty")} />}>
          <div class="grid grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-3">
            <For each={sorted()}>
              {(kb) => (
                <KBCard
                  kb={kb}
                  onDelete={handleDelete}
                  onIndex={handleIndex}
                  indexing={indexingId() === kb.id}
                />
              )}
            </For>
          </div>
        </Show>
      </Show>
    </PageLayout>
  );
}

const statusVariants: Record<string, BadgeVariant> = {
  pending: "warning",
  indexed: "success",
  error: "danger",
};

const categoryVariants: Record<string, BadgeVariant> = {
  framework: "info",
  paradigm: "primary",
  language: "success",
  security: "danger",
  custom: "default",
};

function KBCard(props: {
  kb: KnowledgeBase;
  onDelete: (id: string) => void;
  onIndex: (id: string) => void;
  indexing: boolean;
}) {
  const { t } = useI18n();

  const statusKey = () =>
    `kb.status.${props.kb.status}` as keyof typeof import("~/i18n/en").default;
  const categoryKey = () =>
    `kb.category.${props.kb.category}` as keyof typeof import("~/i18n/en").default;

  return (
    <Card class="transition-shadow hover:shadow-md">
      <Card.Body>
        <div class="flex items-start justify-between">
          <div class="min-w-0 flex-1">
            <h3 class="text-lg font-semibold text-cf-text-primary">{props.kb.name}</h3>
            <p class="mt-1 text-sm text-cf-text-muted">{props.kb.description}</p>
          </div>
        </div>

        <Show when={props.kb.content_path}>
          <p class="mt-1 truncate text-xs text-cf-text-muted" title={props.kb.content_path}>
            {props.kb.content_path}
          </p>
        </Show>

        <div class="mt-3 flex flex-wrap items-center gap-2">
          <Badge variant={categoryVariants[props.kb.category] ?? "default"} pill>
            {t(categoryKey())}
          </Badge>
          <Badge variant={statusVariants[props.kb.status] ?? "warning"} pill>
            {t(statusKey())}
          </Badge>
          <Show when={props.kb.chunk_count > 0}>
            <span class="text-xs text-cf-text-muted">
              {props.kb.chunk_count} {t("kb.chunks")}
            </span>
          </Show>
        </div>

        <Show when={props.kb.tags?.length}>
          <div class="mt-2 flex flex-wrap gap-1">
            <For each={props.kb.tags}>{(tag) => <Badge variant="default">{tag}</Badge>}</For>
          </div>
        </Show>

        <div class="mt-4 flex gap-2">
          <Button
            variant="secondary"
            size="sm"
            onClick={() => props.onIndex(props.kb.id)}
            disabled={props.indexing}
          >
            {props.indexing
              ? "Indexing..."
              : props.kb.status === "indexed"
                ? t("kb.index.reindex")
                : t("kb.index.button")}
          </Button>
          <Button variant="danger" size="sm" onClick={() => props.onDelete(props.kb.id)}>
            Delete
          </Button>
        </div>
      </Card.Body>
    </Card>
  );
}
