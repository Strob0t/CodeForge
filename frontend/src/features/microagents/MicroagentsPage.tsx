import { For, onMount, Show } from "solid-js";

import type { Microagent, MicroagentType, Project, Skill } from "~/api/types";
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

import { useMicroagentsPage, useMicroagentsTab, useSkillsTab } from "./useMicroagentsPage";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const MICROAGENT_TYPES: MicroagentType[] = ["knowledge", "repo", "task"];
const SKILL_TYPES = ["workflow", "pattern"] as const;
const SKILL_STATUSES = ["draft", "active", "disabled"] as const;

// ---------------------------------------------------------------------------
// MicroagentsPage
// ---------------------------------------------------------------------------

export default function MicroagentsPage() {
  onMount(() => {
    document.title = "Microagents & Skills - CodeForge";
  });
  const { t } = useI18n();
  const page = useMicroagentsPage();

  return (
    <PageLayout title={t("microagents.title")} description={t("microagents.description")}>
      {/* Project selector */}
      <div class="mb-4">
        <Select
          value={page.selectedProjectId()}
          onChange={(e) => page.setSelectedProjectId(e.currentTarget.value)}
        >
          <option value="">{t("microagents.selectProject")}</option>
          <For each={page.projects() ?? []}>
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
                page.activeTab() === tab
                  ? "border-cf-accent text-cf-accent"
                  : "border-transparent text-cf-text-muted hover:text-cf-text-primary"
              }`}
              onClick={() => page.setActiveTab(tab)}
            >
              {t(`microagents.tab.${tab}`)}
            </button>
          )}
        </For>
      </div>

      <Show when={page.selectedProjectId()}>
        <Show when={page.activeTab() === "microagents"}>
          <MicroagentsTabView projectId={page.selectedProjectId()} />
        </Show>
        <Show when={page.activeTab() === "skills"}>
          <SkillsTabView projectId={page.selectedProjectId()} />
        </Show>
      </Show>
      <Show when={!page.selectedProjectId()}>
        <EmptyState title={t("microagents.selectProjectFirst")} />
      </Show>
    </PageLayout>
  );
}

// ---------------------------------------------------------------------------
// MicroagentsTabView
// ---------------------------------------------------------------------------

function MicroagentsTabView(props: { projectId: string }) {
  const { t } = useI18n();
  const state = useMicroagentsTab(() => props.projectId);

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
          <Button variant="ghost" size="sm" onClick={() => state.handleEdit(ma)}>
            {t("common.edit")}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            class="text-cf-danger-fg hover:text-cf-danger-fg"
            onClick={() => state.crud.del.requestConfirm(ma)}
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
          variant={state.crud.showForm() ? "secondary" : "primary"}
          onClick={() => {
            if (state.crud.showForm()) {
              state.crud.cancelForm();
              state.clearError();
            } else {
              state.crud.startCreate();
            }
          }}
        >
          {state.crud.showForm() ? t("common.cancel") : t("microagents.create")}
        </Button>
      </div>

      <ErrorBanner error={state.error} onDismiss={state.clearError} />

      <Show when={state.crud.showForm()}>
        <Card class="mb-6">
          <Card.Body>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void state.handleSubmit();
              }}
              aria-label={state.crud.isEditing() ? t("common.edit") : t("microagents.create")}
            >
              <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <FormField label={t("microagents.form.name")} id="ma-name" required>
                  <Input
                    id="ma-name"
                    type="text"
                    value={state.crud.form.state.name}
                    onInput={(e) => state.crud.form.setState("name", e.currentTarget.value)}
                    aria-required="true"
                  />
                </FormField>

                <FormField label={t("microagents.form.type")} id="ma-type" required>
                  <Select
                    id="ma-type"
                    value={state.crud.form.state.type}
                    onChange={(e) =>
                      state.crud.form.setState("type", e.currentTarget.value as MicroagentType)
                    }
                    disabled={state.crud.isEditing()}
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
                    value={state.crud.form.state.trigger_pattern}
                    onInput={(e) =>
                      state.crud.form.setState("trigger_pattern", e.currentTarget.value)
                    }
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
                    value={state.crud.form.state.description}
                    onInput={(e) => state.crud.form.setState("description", e.currentTarget.value)}
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
                    value={state.crud.form.state.prompt}
                    onInput={(e) => state.crud.form.setState("prompt", e.currentTarget.value)}
                    rows={6}
                    mono
                    aria-required="true"
                  />
                </FormField>

                <Show when={state.crud.isEditing()}>
                  <div class="flex items-center gap-3 sm:col-span-2">
                    <Checkbox
                      id="ma-enabled"
                      checked={state.crud.form.state.enabled}
                      onChange={(checked) => state.crud.form.setState("enabled", checked)}
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
                    state.crud.cancelForm();
                    state.clearError();
                  }}
                >
                  {t("common.cancel")}
                </Button>
                <Button type="submit">
                  {state.crud.isEditing() ? t("common.save") : t("microagents.create")}
                </Button>
              </div>
            </form>
          </Card.Body>
        </Card>
      </Show>

      <Show when={state.microagents.loading}>
        <LoadingState message={t("common.loading")} />
      </Show>

      <Show when={!state.microagents.loading && !state.microagents.error}>
        <Show
          when={(state.microagents() ?? []).length > 0}
          fallback={<EmptyState title={t("microagents.empty")} />}
        >
          <Table<Microagent>
            columns={maColumns}
            data={state.microagents() ?? []}
            rowKey={(ma) => ma.id}
          />
        </Show>
      </Show>

      <ConfirmDialog
        open={state.crud.del.target() !== null}
        title={t("common.delete")}
        message={t("microagents.toast.deleted")}
        variant="danger"
        confirmLabel={t("common.delete")}
        cancelLabel={t("common.cancel")}
        onConfirm={() => void state.crud.del.confirm()}
        onCancel={state.crud.del.cancel}
      />
    </>
  );
}

// ---------------------------------------------------------------------------
// SkillsTabView
// ---------------------------------------------------------------------------

function SkillsTabView(props: { projectId: string }) {
  const { t } = useI18n();
  const state = useSkillsTab(() => props.projectId);

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
          <Button variant="ghost" size="sm" onClick={() => state.handleEdit(sk)}>
            {t("common.edit")}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            class="text-cf-danger-fg hover:text-cf-danger-fg"
            onClick={() => state.crud.del.requestConfirm(sk)}
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
        <Button variant="secondary" onClick={() => state.setShowImport((v) => !v)}>
          {state.showImport() ? t("common.cancel") : t("skills.import")}
        </Button>
        <Button
          variant={state.crud.showForm() ? "secondary" : "primary"}
          onClick={() => {
            if (state.crud.showForm()) {
              state.crud.cancelForm();
              state.clearError();
            } else {
              state.crud.startCreate();
            }
          }}
        >
          {state.crud.showForm() ? t("common.cancel") : t("skills.create")}
        </Button>
      </div>

      <ErrorBanner error={state.error} onDismiss={state.clearError} />
      <ErrorBanner error={state.importError} onDismiss={state.clearImportError} />

      {/* Import from URL */}
      <Show when={state.showImport()}>
        <Card class="mb-6">
          <Card.Body>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void state.handleImport();
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
                  value={state.importUrl()}
                  onInput={(e) => state.setImportUrl(e.currentTarget.value)}
                  mono
                  aria-required="true"
                />
              </FormField>
              <div class="mt-4 flex justify-end gap-2">
                <Button
                  variant="secondary"
                  onClick={() => {
                    state.setShowImport(false);
                    state.setImportUrl("");
                    state.clearImportError();
                  }}
                >
                  {t("common.cancel")}
                </Button>
                <Button
                  type="submit"
                  disabled={state.importing() || !state.importUrl().trim()}
                  loading={state.importing()}
                >
                  {state.importing() ? t("common.importing") : t("skills.import")}
                </Button>
              </div>
            </form>
          </Card.Body>
        </Card>
      </Show>

      {/* Create / Edit form */}
      <Show when={state.crud.showForm()}>
        <Card class="mb-6">
          <Card.Body>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void state.handleSubmit();
              }}
              aria-label={state.crud.isEditing() ? t("common.edit") : t("skills.create")}
            >
              <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <FormField label={t("skills.form.name")} id="sk-name" required>
                  <Input
                    id="sk-name"
                    type="text"
                    value={state.crud.form.state.name}
                    onInput={(e) => state.crud.form.setState("name", e.currentTarget.value)}
                    aria-required="true"
                  />
                </FormField>

                <FormField label={t("skills.form.type")} id="sk-type">
                  <Select
                    id="sk-type"
                    value={state.crud.form.state.type}
                    onChange={(e) => state.crud.form.setState("type", e.currentTarget.value)}
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
                    value={state.crud.form.state.description}
                    onInput={(e) => state.crud.form.setState("description", e.currentTarget.value)}
                    rows={2}
                    aria-required="true"
                  />
                </FormField>

                <FormField label={t("skills.form.language")} id="sk-language">
                  <Input
                    id="sk-language"
                    type="text"
                    value={state.crud.form.state.language}
                    onInput={(e) => state.crud.form.setState("language", e.currentTarget.value)}
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
                    value={state.crud.form.state.tags}
                    onInput={(e) => state.crud.form.setState("tags", e.currentTarget.value)}
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
                    value={state.crud.form.state.content}
                    onInput={(e) => state.crud.form.setState("content", e.currentTarget.value)}
                    rows={8}
                    mono
                    aria-required="true"
                  />
                </FormField>

                <Show when={state.crud.isEditing()}>
                  <FormField label={t("skills.form.status")} id="sk-status">
                    <Select
                      id="sk-status"
                      value={state.crud.form.state.status}
                      onChange={(e) => state.crud.form.setState("status", e.currentTarget.value)}
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
                    state.crud.cancelForm();
                    state.clearError();
                  }}
                >
                  {t("common.cancel")}
                </Button>
                <Button type="submit">
                  {state.crud.isEditing() ? t("common.save") : t("skills.create")}
                </Button>
              </div>
            </form>
          </Card.Body>
        </Card>
      </Show>

      <Show when={state.skills.loading}>
        <LoadingState message={t("common.loading")} />
      </Show>

      <Show when={!state.skills.loading && !state.skills.error}>
        <Show
          when={(state.skills() ?? []).length > 0}
          fallback={<EmptyState title={t("skills.empty")} />}
        >
          <Table<Skill> columns={skColumns} data={state.skills() ?? []} rowKey={(sk) => sk.id} />
        </Show>
      </Show>

      <ConfirmDialog
        open={state.crud.del.target() !== null}
        title={t("common.delete")}
        message={t("skills.toast.deleted")}
        variant="danger"
        confirmLabel={t("common.delete")}
        cancelLabel={t("common.cancel")}
        onConfirm={() => void state.crud.del.confirm()}
        onCancel={state.crud.del.cancel}
      />
    </>
  );
}
