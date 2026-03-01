import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { CreateKnowledgeBaseRequest, KnowledgeBase } from "~/api/types";
import { useToast } from "~/components/Toast";
import { kbCategoryVariant, kbStatusVariant } from "~/config/statusVariants";
import { useAsyncAction, useFormState } from "~/hooks";
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

const CATEGORIES = ["framework", "paradigm", "language", "security", "custom"] as const;

const KB_FORM_DEFAULTS = {
  name: "",
  desc: "",
  category: "custom",
  tags: "",
  contentPath: "",
};

export default function KnowledgeBasesPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [kbs, { refetch }] = createResource(() => api.knowledgeBases.list());
  const [showForm, setShowForm] = createSignal(false);
  const [indexingId, setIndexingId] = createSignal<string | null>(null);

  const form = useFormState(KB_FORM_DEFAULTS);

  const { run: handleCreate } = useAsyncAction(
    async () => {
      const name = form.state.name.trim();
      if (!name) return;
      const req: CreateKnowledgeBaseRequest = {
        name,
        description: form.state.desc.trim(),
        category: form.state.category,
        tags: form.state.tags
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean),
        content_path: form.state.contentPath.trim(),
      };
      await api.knowledgeBases.create(req);
      form.reset();
      setShowForm(false);
      refetch();
      toast("success", t("kb.toast.created"));
    },
    {
      onError: (err) => {
        toast("error", err instanceof Error ? err.message : "Failed to create knowledge base");
      },
    },
  );

  const { run: handleDelete } = useAsyncAction(
    async (id: string) => {
      await api.knowledgeBases.delete(id);
      refetch();
      toast("success", t("kb.toast.deleted"));
    },
    {
      onError: (err) => {
        toast("error", err instanceof Error ? err.message : "Failed to delete knowledge base");
      },
    },
  );

  const { run: handleIndex } = useAsyncAction(
    async (id: string) => {
      setIndexingId(id);
      try {
        await api.knowledgeBases.index(id);
        refetch();
        toast("success", t("kb.toast.indexed"));
      } finally {
        setIndexingId(null);
      }
    },
    {
      onError: (err) => {
        toast("error", err instanceof Error ? err.message : "Failed to index knowledge base");
      },
    },
  );

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
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void handleCreate();
          }}
          class="mb-6"
          aria-label={t("kb.form.create")}
        >
          <Card>
            <Card.Body>
              <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <FormField label={t("kb.form.name")} id="kb-name" required>
                  <Input
                    id="kb-name"
                    type="text"
                    value={form.state.name}
                    onInput={(e) => form.setState("name", e.currentTarget.value)}
                    required
                  />
                </FormField>
                <FormField label={t("kb.form.category")} id="kb-category">
                  <Select
                    id="kb-category"
                    value={form.state.category}
                    onChange={(e) => form.setState("category", e.currentTarget.value)}
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
                    value={form.state.desc}
                    onInput={(e) => form.setState("desc", e.currentTarget.value)}
                  />
                </FormField>
                <FormField label={t("kb.form.tags")} id="kb-tags">
                  <Input
                    id="kb-tags"
                    type="text"
                    value={form.state.tags}
                    onInput={(e) => form.setState("tags", e.currentTarget.value)}
                    placeholder="tag1, tag2, tag3"
                  />
                </FormField>
                <FormField label={t("kb.form.contentPath")} id="kb-content-path" required>
                  <Input
                    id="kb-content-path"
                    type="text"
                    value={form.state.contentPath}
                    onInput={(e) => form.setState("contentPath", e.currentTarget.value)}
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

function KBCard(props: {
  kb: KnowledgeBase;
  onDelete: (id: string) => Promise<void>;
  onIndex: (id: string) => Promise<void>;
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
          <Badge variant={kbCategoryVariant[props.kb.category] ?? "default"} pill>
            {t(categoryKey())}
          </Badge>
          <Badge variant={kbStatusVariant[props.kb.status] ?? "warning"} pill>
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
            onClick={() => void props.onIndex(props.kb.id)}
            disabled={props.indexing}
          >
            {props.indexing
              ? "Indexing..."
              : props.kb.status === "indexed"
                ? t("kb.index.reindex")
                : t("kb.index.button")}
          </Button>
          <Button variant="danger" size="sm" onClick={() => void props.onDelete(props.kb.id)}>
            Delete
          </Button>
        </div>
      </Card.Body>
    </Card>
  );
}
