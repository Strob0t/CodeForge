import { createResource, createSignal, For, onMount, Show } from "solid-js";

import { api } from "~/api/client";
import type {
  CreateMicroagentRequest,
  CreateSkillRequest,
  Microagent,
  MicroagentType,
  Project,
  Skill,
  UpdateMicroagentRequest,
  UpdateSkillRequest,
} from "~/api/types";
import { useToast } from "~/components/Toast";
import { useAsyncAction, useCRUDForm } from "~/hooks";
import { useI18n } from "~/i18n";
import {
  Badge,
  Button,
  Card,
  Checkbox,
  ConfirmDialog,
  EmptyState,
  ErrorBanner,
  FormField,
  Input,
  LoadingState,
  PageLayout,
  Select,
  Table,
  Textarea,
} from "~/ui";
import type { TableColumn } from "~/ui/composites/Table";

// ---------------------------------------------------------------------------
// Tab type
// ---------------------------------------------------------------------------

type MicroagentsTab = "microagents" | "skills";

// ---------------------------------------------------------------------------
// MicroagentsPage
// ---------------------------------------------------------------------------

export default function MicroagentsPage() {
  onMount(() => {
    document.title = "Microagents & Skills - CodeForge";
  });
  const { t } = useI18n();
  const [activeTab, setActiveTab] = createSignal<MicroagentsTab>("microagents");
  const [selectedProjectId, setSelectedProjectId] = createSignal("");

  const [projects] = createResource(() => api.projects.list());

  return (
    <PageLayout title={t("microagents.title")} description={t("microagents.description")}>
      {/* Project selector */}
      <div class="mb-4">
        <Select
          value={selectedProjectId()}
          onChange={(e) => setSelectedProjectId(e.currentTarget.value)}
        >
          <option value="">{t("microagents.selectProject")}</option>
          <For each={projects() ?? []}>
            {(p: Project) => <option value={p.id}>{p.name}</option>}
          </For>
        </Select>
      </div>

      {/* Tabs */}
      <div class="flex gap-2 mb-4 border-b border-cf-border">
        <For each={["microagents", "skills"] as const}>
          {(tab) => (
            <button
              class={`px-4 py-2 text-sm font-medium border-b-2 transition-colors ${
                activeTab() === tab
                  ? "border-cf-accent text-cf-accent"
                  : "border-transparent text-cf-text-muted hover:text-cf-text-primary"
              }`}
              onClick={() => setActiveTab(tab)}
            >
              {t(`microagents.tab.${tab}`)}
            </button>
          )}
        </For>
      </div>

      <Show when={selectedProjectId()}>
        <Show when={activeTab() === "microagents"}>
          <MicroagentsTab projectId={selectedProjectId()} />
        </Show>
        <Show when={activeTab() === "skills"}>
          <SkillsTab projectId={selectedProjectId()} />
        </Show>
      </Show>
      <Show when={!selectedProjectId()}>
        <EmptyState title={t("microagents.selectProjectFirst")} />
      </Show>
    </PageLayout>
  );
}

// ---------------------------------------------------------------------------
// MicroagentsTab
// ---------------------------------------------------------------------------

const MICROAGENT_TYPES: MicroagentType[] = ["knowledge", "repo", "task"];

interface MicroagentFormState {
  name: string;
  type: MicroagentType;
  trigger_pattern: string;
  description: string;
  prompt: string;
  enabled: boolean;
}

const MA_FORM_DEFAULTS: MicroagentFormState = {
  name: "",
  type: "knowledge",
  trigger_pattern: "",
  description: "",
  prompt: "",
  enabled: true,
};

function MicroagentsTab(props: { projectId: string }) {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [microagents, { refetch }] = createResource(
    () => props.projectId,
    (pid) => api.microagents.list(pid),
  );

  const crud = useCRUDForm(MA_FORM_DEFAULTS, async (ma: Microagent) => {
    await api.microagents.delete(ma.id);
    toast("success", t("microagents.toast.deleted"));
    refetch();
  });

  function handleEdit(ma: Microagent): void {
    crud.startEdit(ma.id, {
      name: ma.name,
      type: ma.type,
      trigger_pattern: ma.trigger_pattern,
      description: ma.description,
      prompt: ma.prompt,
      enabled: ma.enabled,
    });
  }

  const {
    run: handleSubmit,
    error,
    clearError,
  } = useAsyncAction(
    async () => {
      const name = crud.form.state.name.trim();
      if (!name) return;
      const eid = crud.editingId();
      if (crud.isEditing() && eid) {
        const data: UpdateMicroagentRequest = {
          name,
          trigger_pattern: crud.form.state.trigger_pattern,
          description: crud.form.state.description,
          prompt: crud.form.state.prompt,
          enabled: crud.form.state.enabled,
        };
        await api.microagents.update(eid, data);
        toast("success", t("microagents.toast.updated"));
      } else {
        const data: CreateMicroagentRequest = {
          name,
          type: crud.form.state.type,
          trigger_pattern: crud.form.state.trigger_pattern,
          description: crud.form.state.description,
          prompt: crud.form.state.prompt,
        };
        await api.microagents.create(props.projectId, data);
        toast("success", t("microagents.toast.created"));
      }
      crud.cancelForm();
      refetch();
    },
    {
      onError: (err) => {
        const msg = err instanceof Error ? err.message : "Failed";
        toast("error", msg);
      },
    },
  );

  const maColumns: TableColumn<Microagent>[] = [
    {
      key: "name",
      header: t("microagents.col.name"),
      render: (ma) => <span class="font-medium text-cf-text-primary">{ma.name}</span>,
    },
    {
      key: "type",
      header: t("microagents.col.type"),
      render: (ma) => <Badge>{ma.type}</Badge>,
    },
    {
      key: "trigger_pattern",
      header: t("microagents.col.triggerPattern"),
      render: (ma) => <span class="font-mono text-xs">{ma.trigger_pattern}</span>,
    },
    {
      key: "enabled",
      header: t("microagents.col.enabled"),
      render: (ma) => (
        <Badge variant={ma.enabled ? "success" : "default"}>{ma.enabled ? "Yes" : "No"}</Badge>
      ),
    },
    {
      key: "description",
      header: t("microagents.col.description"),
      render: (ma) => (
        <span class="text-xs text-cf-text-muted truncate max-w-[200px] inline-block">
          {ma.description}
        </span>
      ),
    },
    {
      key: "created_at",
      header: t("microagents.col.createdAt"),
      render: (ma) => (
        <span class="text-xs text-cf-text-muted">
          {new Date(ma.created_at).toLocaleDateString()}
        </span>
      ),
    },
    {
      key: "actions",
      header: "",
      render: (ma) => (
        <div class="flex items-center gap-2">
          <Button variant="ghost" size="sm" onClick={() => handleEdit(ma)}>
            {t("common.edit")}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            class="text-cf-danger-fg hover:text-cf-danger-fg"
            onClick={() => crud.del.requestConfirm(ma)}
          >
            {t("common.delete")}
          </Button>
        </div>
      ),
    },
  ];

  return (
    <>
      <div class="mb-4 flex justify-end">
        <Button
          variant={crud.showForm() ? "secondary" : "primary"}
          onClick={() => {
            if (crud.showForm()) {
              crud.cancelForm();
              clearError();
            } else {
              crud.startCreate();
            }
          }}
        >
          {crud.showForm() ? t("common.cancel") : t("microagents.create")}
        </Button>
      </div>

      <ErrorBanner error={error} onDismiss={clearError} />

      <Show when={crud.showForm()}>
        <Card class="mb-6">
          <Card.Body>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void handleSubmit();
              }}
              aria-label={crud.isEditing() ? t("common.edit") : t("microagents.create")}
            >
              <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <FormField label={t("microagents.form.name")} id="ma-name" required>
                  <Input
                    id="ma-name"
                    type="text"
                    value={crud.form.state.name}
                    onInput={(e) => crud.form.setState("name", e.currentTarget.value)}
                    aria-required="true"
                  />
                </FormField>

                <FormField label={t("microagents.form.type")} id="ma-type" required>
                  <Select
                    id="ma-type"
                    value={crud.form.state.type}
                    onChange={(e) =>
                      crud.form.setState("type", e.currentTarget.value as MicroagentType)
                    }
                    disabled={crud.isEditing()}
                  >
                    <For each={MICROAGENT_TYPES}>{(tp) => <option value={tp}>{tp}</option>}</For>
                  </Select>
                </FormField>

                <FormField
                  label={t("microagents.form.triggerPattern")}
                  id="ma-trigger"
                  required
                  help={t("microagents.form.triggerPatternHint")}
                  class="sm:col-span-2"
                >
                  <Input
                    id="ma-trigger"
                    type="text"
                    value={crud.form.state.trigger_pattern}
                    onInput={(e) => crud.form.setState("trigger_pattern", e.currentTarget.value)}
                    mono
                    aria-required="true"
                  />
                </FormField>

                <FormField
                  label={t("microagents.form.description")}
                  id="ma-desc"
                  class="sm:col-span-2"
                >
                  <Textarea
                    id="ma-desc"
                    value={crud.form.state.description}
                    onInput={(e) => crud.form.setState("description", e.currentTarget.value)}
                    rows={2}
                  />
                </FormField>

                <FormField
                  label={t("microagents.form.prompt")}
                  id="ma-prompt"
                  required
                  help={t("microagents.form.promptHint")}
                  class="sm:col-span-2"
                >
                  <Textarea
                    id="ma-prompt"
                    value={crud.form.state.prompt}
                    onInput={(e) => crud.form.setState("prompt", e.currentTarget.value)}
                    rows={6}
                    mono
                    aria-required="true"
                  />
                </FormField>

                <Show when={crud.isEditing()}>
                  <div class="flex items-center gap-3 sm:col-span-2">
                    <Checkbox
                      id="ma-enabled"
                      checked={crud.form.state.enabled}
                      onChange={(checked) => crud.form.setState("enabled", checked)}
                    />
                    <label for="ma-enabled" class="text-sm font-medium text-cf-text-secondary">
                      {t("microagents.col.enabled")}
                    </label>
                  </div>
                </Show>
              </div>

              <div class="mt-4 flex justify-end gap-2">
                <Button
                  variant="secondary"
                  onClick={() => {
                    crud.cancelForm();
                    clearError();
                  }}
                >
                  {t("common.cancel")}
                </Button>
                <Button type="submit">
                  {crud.isEditing() ? t("common.save") : t("microagents.create")}
                </Button>
              </div>
            </form>
          </Card.Body>
        </Card>
      </Show>

      <Show when={microagents.loading}>
        <LoadingState message={t("common.loading")} />
      </Show>

      <Show when={!microagents.loading && !microagents.error}>
        <Show
          when={(microagents() ?? []).length > 0}
          fallback={<EmptyState title={t("microagents.empty")} />}
        >
          <Table<Microagent>
            columns={maColumns}
            data={microagents() ?? []}
            rowKey={(ma) => ma.id}
          />
        </Show>
      </Show>

      <ConfirmDialog
        open={crud.del.target() !== null}
        title={t("common.delete")}
        message={t("microagents.toast.deleted")}
        variant="danger"
        confirmLabel={t("common.delete")}
        cancelLabel={t("common.cancel")}
        onConfirm={() => void crud.del.confirm()}
        onCancel={crud.del.cancel}
      />
    </>
  );
}

// ---------------------------------------------------------------------------
// SkillsTab
// ---------------------------------------------------------------------------

const SKILL_TYPES = ["workflow", "pattern"] as const;
const SKILL_STATUSES = ["draft", "active", "disabled"] as const;

interface SkillFormState {
  name: string;
  type: string;
  description: string;
  language: string;
  content: string;
  tags: string;
  status: string;
}

const SK_FORM_DEFAULTS: SkillFormState = {
  name: "",
  type: "workflow",
  description: "",
  language: "",
  content: "",
  tags: "",
  status: "draft",
};

function SkillsTab(props: { projectId: string }) {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [skills, { refetch }] = createResource(
    () => props.projectId,
    (pid) => api.skills.list(pid),
  );

  const crud = useCRUDForm(SK_FORM_DEFAULTS, async (sk: Skill) => {
    await api.skills.delete(sk.id);
    toast("success", t("skills.toast.deleted"));
    refetch();
  });

  // Import state
  const [showImport, setShowImport] = createSignal(false);
  const [importUrl, setImportUrl] = createSignal("");

  const {
    run: handleImport,
    loading: importing,
    error: importError,
    clearError: clearImportError,
  } = useAsyncAction(
    async () => {
      await api.skills.import({
        source_url: importUrl(),
        project_id: props.projectId,
      });
      toast("success", t("skills.toast.imported"));
      setShowImport(false);
      setImportUrl("");
      refetch();
    },
    {
      onError: (err) => {
        const msg = err instanceof Error ? err.message : "Import failed";
        toast("error", msg);
      },
    },
  );

  function handleEdit(sk: Skill): void {
    crud.startEdit(sk.id, {
      name: sk.name,
      type: sk.type,
      description: sk.description,
      language: sk.language,
      content: sk.content,
      tags: (sk.tags ?? []).join(", "),
      status: sk.status,
    });
  }

  function parseTags(raw: string): string[] {
    return raw
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
  }

  const {
    run: handleSubmit,
    error,
    clearError,
  } = useAsyncAction(
    async () => {
      const name = crud.form.state.name.trim();
      if (!name) return;
      const eid = crud.editingId();
      if (crud.isEditing() && eid) {
        const data: UpdateSkillRequest = {
          name,
          type: crud.form.state.type,
          description: crud.form.state.description,
          language: crud.form.state.language,
          content: crud.form.state.content,
          tags: parseTags(crud.form.state.tags),
          status: crud.form.state.status,
        };
        await api.skills.update(eid, data);
        toast("success", t("skills.toast.updated"));
      } else {
        const data: CreateSkillRequest = {
          name,
          type: crud.form.state.type,
          description: crud.form.state.description,
          language: crud.form.state.language,
          content: crud.form.state.content,
          tags: parseTags(crud.form.state.tags),
        };
        await api.skills.create(props.projectId, data);
        toast("success", t("skills.toast.created"));
      }
      crud.cancelForm();
      refetch();
    },
    {
      onError: (err) => {
        const msg = err instanceof Error ? err.message : "Failed";
        toast("error", msg);
      },
    },
  );

  const skColumns: TableColumn<Skill>[] = [
    {
      key: "name",
      header: t("skills.col.name"),
      render: (sk) => <span class="font-medium text-cf-text-primary">{sk.name}</span>,
    },
    {
      key: "type",
      header: t("skills.col.type"),
      render: (sk) => <Badge>{sk.type}</Badge>,
    },
    {
      key: "language",
      header: t("skills.col.language"),
      render: (sk) => <span class="text-xs">{sk.language}</span>,
    },
    {
      key: "status",
      header: t("skills.col.status"),
      render: (sk) => (
        <Badge
          variant={
            sk.status === "active" ? "success" : sk.status === "disabled" ? "danger" : "default"
          }
        >
          {sk.status}
        </Badge>
      ),
    },
    {
      key: "source",
      header: t("skills.col.source"),
      render: (sk) => <span class="text-xs text-cf-text-muted">{sk.source}</span>,
    },
    {
      key: "tags",
      header: t("skills.col.tags"),
      render: (sk) => (
        <div class="flex flex-wrap gap-1">
          <For each={sk.tags ?? []}>{(tag) => <Badge variant="info">{tag}</Badge>}</For>
        </div>
      ),
    },
    {
      key: "usage_count",
      header: t("skills.col.usageCount"),
      render: (sk) => <span class="text-xs">{sk.usage_count}</span>,
    },
    {
      key: "created_at",
      header: t("skills.col.createdAt"),
      render: (sk) => (
        <span class="text-xs text-cf-text-muted">
          {new Date(sk.created_at).toLocaleDateString()}
        </span>
      ),
    },
    {
      key: "actions",
      header: "",
      render: (sk) => (
        <div class="flex items-center gap-2">
          <Button variant="ghost" size="sm" onClick={() => handleEdit(sk)}>
            {t("common.edit")}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            class="text-cf-danger-fg hover:text-cf-danger-fg"
            onClick={() => crud.del.requestConfirm(sk)}
          >
            {t("common.delete")}
          </Button>
        </div>
      ),
    },
  ];

  return (
    <>
      <div class="mb-4 flex items-center justify-end gap-2">
        <Button variant="secondary" onClick={() => setShowImport((v) => !v)}>
          {showImport() ? t("common.cancel") : t("skills.import")}
        </Button>
        <Button
          variant={crud.showForm() ? "secondary" : "primary"}
          onClick={() => {
            if (crud.showForm()) {
              crud.cancelForm();
              clearError();
            } else {
              crud.startCreate();
            }
          }}
        >
          {crud.showForm() ? t("common.cancel") : t("skills.create")}
        </Button>
      </div>

      <ErrorBanner error={error} onDismiss={clearError} />
      <ErrorBanner error={importError} onDismiss={clearImportError} />

      {/* Import from URL */}
      <Show when={showImport()}>
        <Card class="mb-6">
          <Card.Body>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void handleImport();
              }}
              aria-label={t("skills.import")}
            >
              <FormField
                label={t("skills.import.url")}
                id="sk-import-url"
                required
                help={t("skills.import.urlHint")}
              >
                <Input
                  id="sk-import-url"
                  type="url"
                  value={importUrl()}
                  onInput={(e) => setImportUrl(e.currentTarget.value)}
                  mono
                  aria-required="true"
                />
              </FormField>
              <div class="mt-4 flex justify-end gap-2">
                <Button
                  variant="secondary"
                  onClick={() => {
                    setShowImport(false);
                    setImportUrl("");
                    clearImportError();
                  }}
                >
                  {t("common.cancel")}
                </Button>
                <Button
                  type="submit"
                  disabled={importing() || !importUrl().trim()}
                  loading={importing()}
                >
                  {importing() ? t("common.importing") : t("skills.import")}
                </Button>
              </div>
            </form>
          </Card.Body>
        </Card>
      </Show>

      {/* Create / Edit form */}
      <Show when={crud.showForm()}>
        <Card class="mb-6">
          <Card.Body>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void handleSubmit();
              }}
              aria-label={crud.isEditing() ? t("common.edit") : t("skills.create")}
            >
              <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <FormField label={t("skills.form.name")} id="sk-name" required>
                  <Input
                    id="sk-name"
                    type="text"
                    value={crud.form.state.name}
                    onInput={(e) => crud.form.setState("name", e.currentTarget.value)}
                    aria-required="true"
                  />
                </FormField>

                <FormField label={t("skills.form.type")} id="sk-type">
                  <Select
                    id="sk-type"
                    value={crud.form.state.type}
                    onChange={(e) => crud.form.setState("type", e.currentTarget.value)}
                  >
                    <For each={[...SKILL_TYPES]}>{(tp) => <option value={tp}>{tp}</option>}</For>
                  </Select>
                </FormField>

                <FormField
                  label={t("skills.form.description")}
                  id="sk-desc"
                  required
                  class="sm:col-span-2"
                >
                  <Textarea
                    id="sk-desc"
                    value={crud.form.state.description}
                    onInput={(e) => crud.form.setState("description", e.currentTarget.value)}
                    rows={2}
                    aria-required="true"
                  />
                </FormField>

                <FormField label={t("skills.form.language")} id="sk-language">
                  <Input
                    id="sk-language"
                    type="text"
                    value={crud.form.state.language}
                    onInput={(e) => crud.form.setState("language", e.currentTarget.value)}
                    placeholder="python, go, typescript..."
                  />
                </FormField>

                <FormField
                  label={t("skills.form.tags")}
                  id="sk-tags"
                  help={t("skills.form.tagsHint")}
                >
                  <Input
                    id="sk-tags"
                    type="text"
                    value={crud.form.state.tags}
                    onInput={(e) => crud.form.setState("tags", e.currentTarget.value)}
                  />
                </FormField>

                <FormField
                  label={t("skills.form.content")}
                  id="sk-content"
                  required
                  help={t("skills.form.contentHint")}
                  class="sm:col-span-2"
                >
                  <Textarea
                    id="sk-content"
                    value={crud.form.state.content}
                    onInput={(e) => crud.form.setState("content", e.currentTarget.value)}
                    rows={8}
                    mono
                    aria-required="true"
                  />
                </FormField>

                <Show when={crud.isEditing()}>
                  <FormField label={t("skills.form.status")} id="sk-status">
                    <Select
                      id="sk-status"
                      value={crud.form.state.status}
                      onChange={(e) => crud.form.setState("status", e.currentTarget.value)}
                    >
                      <For each={[...SKILL_STATUSES]}>
                        {(st) => <option value={st}>{st}</option>}
                      </For>
                    </Select>
                  </FormField>
                </Show>
              </div>

              <div class="mt-4 flex justify-end gap-2">
                <Button
                  variant="secondary"
                  onClick={() => {
                    crud.cancelForm();
                    clearError();
                  }}
                >
                  {t("common.cancel")}
                </Button>
                <Button type="submit">
                  {crud.isEditing() ? t("common.save") : t("skills.create")}
                </Button>
              </div>
            </form>
          </Card.Body>
        </Card>
      </Show>

      <Show when={skills.loading}>
        <LoadingState message={t("common.loading")} />
      </Show>

      <Show when={!skills.loading && !skills.error}>
        <Show
          when={(skills() ?? []).length > 0}
          fallback={<EmptyState title={t("skills.empty")} />}
        >
          <Table<Skill> columns={skColumns} data={skills() ?? []} rowKey={(sk) => sk.id} />
        </Show>
      </Show>

      <ConfirmDialog
        open={crud.del.target() !== null}
        title={t("common.delete")}
        message={t("skills.toast.deleted")}
        variant="danger"
        confirmLabel={t("common.delete")}
        cancelLabel={t("common.cancel")}
        onConfirm={() => void crud.del.confirm()}
        onCancel={crud.del.cancel}
      />
    </>
  );
}
