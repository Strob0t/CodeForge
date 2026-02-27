import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { CreateModeRequest, Mode } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import {
  Alert,
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

export default function ModesPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [modes, { refetch }] = createResource(() => api.modes.list());
  const [scenarios] = createResource(() => api.modes.scenarios());
  const [showForm, setShowForm] = createSignal(false);
  const [error, setError] = createSignal("");
  const [editingId, setEditingId] = createSignal<string | null>(null);

  // -- Form state --
  const [formId, setFormId] = createSignal("");
  const [formName, setFormName] = createSignal("");
  const [formDesc, setFormDesc] = createSignal("");
  const [formTools, setFormTools] = createSignal("");
  const [formDeniedTools, setFormDeniedTools] = createSignal("");
  const [formDeniedActions, setFormDeniedActions] = createSignal("");
  const [formRequiredArtifact, setFormRequiredArtifact] = createSignal("");
  const [formScenario, setFormScenario] = createSignal("default");
  const [formAutonomy, setFormAutonomy] = createSignal(3);
  const [formPrompt, setFormPrompt] = createSignal("");

  function isEditing() {
    return editingId() !== null;
  }

  const resetForm = () => {
    setFormId("");
    setFormName("");
    setFormDesc("");
    setFormTools("");
    setFormDeniedTools("");
    setFormDeniedActions("");
    setFormRequiredArtifact("");
    setFormScenario("default");
    setFormAutonomy(3);
    setFormPrompt("");
    setEditingId(null);
  };

  function handleCancelForm() {
    setShowForm(false);
    resetForm();
    setError("");
  }

  function handleEdit(mode: Mode) {
    setFormId(mode.id);
    setFormName(mode.name);
    setFormDesc(mode.description);
    setFormTools(mode.tools.join(", "));
    setFormDeniedTools((mode.denied_tools ?? []).join(", "));
    setFormDeniedActions((mode.denied_actions ?? []).join(", "));
    setFormRequiredArtifact(mode.required_artifact ?? "");
    setFormScenario(mode.llm_scenario || "default");
    setFormAutonomy(mode.autonomy);
    setFormPrompt(mode.prompt_prefix ?? "");
    setEditingId(mode.id);
    setShowForm(true);
  }

  const handleSubmit = async (e: SubmitEvent) => {
    e.preventDefault();
    const id = formId().trim();
    const name = formName().trim();
    if (!id) {
      toast("error", t("modes.toast.idRequired"));
      return;
    }
    if (!name) {
      toast("error", t("modes.toast.nameRequired"));
      return;
    }
    setError("");
    try {
      const req: CreateModeRequest = {
        id,
        name,
        description: formDesc().trim() || undefined,
        tools: formTools()
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean),
        denied_tools: formDeniedTools()
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean),
        denied_actions: formDeniedActions()
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean),
        required_artifact: formRequiredArtifact().trim() || undefined,
        llm_scenario: formScenario().trim() || undefined,
        autonomy: formAutonomy(),
        prompt_prefix: formPrompt().trim() || undefined,
      };
      const eid = editingId();
      if (isEditing() && eid) {
        await api.modes.update(eid, req);
        toast("success", t("modes.toast.updated"));
      } else {
        await api.modes.create(req);
        toast("success", t("modes.toast.created"));
      }
      resetForm();
      setShowForm(false);
      refetch();
    } catch (err) {
      const msg =
        err instanceof Error
          ? err.message
          : isEditing()
            ? t("modes.toast.updateFailed")
            : t("modes.toast.createFailed");
      setError(msg);
      toast("error", msg);
    }
  };

  const handleDelete = async (mode: Mode) => {
    try {
      await api.modes.delete(mode.id);
      toast("success", t("modes.toast.deleted"));
      refetch();
    } catch (err) {
      const msg = err instanceof Error ? err.message : t("modes.toast.deleteFailed");
      toast("error", msg);
    }
  };

  const sorted = () => {
    const list = modes() ?? [];
    return [...list].sort((a, b) => {
      if (a.builtin !== b.builtin) return a.builtin ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
  };

  return (
    <PageLayout
      title={t("modes.title")}
      action={
        <Button
          variant={showForm() ? "secondary" : "primary"}
          onClick={() => {
            if (showForm()) {
              handleCancelForm();
            } else {
              setShowForm(true);
            }
          }}
        >
          {showForm() ? t("common.cancel") : t("modes.addMode")}
        </Button>
      }
    >
      <Show when={error()}>
        <Alert variant="error" class="mb-4" onDismiss={() => setError("")}>
          {error()}
        </Alert>
      </Show>

      <Show when={showForm()}>
        <Card class="mb-6">
          <Card.Body>
            <form
              onSubmit={handleSubmit}
              aria-label={isEditing() ? t("modes.edit") : t("modes.addMode")}
            >
              <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <FormField label={t("modes.form.id")} id="mode-id" required>
                  <Input
                    id="mode-id"
                    type="text"
                    value={formId()}
                    onInput={(e) => setFormId(e.currentTarget.value)}
                    placeholder={t("modes.form.idPlaceholder")}
                    aria-required="true"
                    disabled={isEditing()}
                  />
                </FormField>
                <FormField label={t("modes.form.name")} id="mode-name" required>
                  <Input
                    id="mode-name"
                    type="text"
                    value={formName()}
                    onInput={(e) => setFormName(e.currentTarget.value)}
                    placeholder={t("modes.form.namePlaceholder")}
                    aria-required="true"
                  />
                </FormField>
                <FormField label={t("modes.form.description")} id="mode-desc" class="sm:col-span-2">
                  <Input
                    id="mode-desc"
                    type="text"
                    value={formDesc()}
                    onInput={(e) => setFormDesc(e.currentTarget.value)}
                    placeholder={t("modes.form.descriptionPlaceholder")}
                  />
                </FormField>
                <FormField label={t("modes.form.tools")} id="mode-tools">
                  <Input
                    id="mode-tools"
                    type="text"
                    value={formTools()}
                    onInput={(e) => setFormTools(e.currentTarget.value)}
                    placeholder={t("modes.form.toolsPlaceholder")}
                  />
                </FormField>
                <FormField label={t("modes.form.deniedTools")} id="mode-denied-tools">
                  <Input
                    id="mode-denied-tools"
                    type="text"
                    value={formDeniedTools()}
                    onInput={(e) => setFormDeniedTools(e.currentTarget.value)}
                    placeholder={t("modes.form.deniedToolsPlaceholder")}
                  />
                </FormField>
                <FormField label={t("modes.form.deniedActions")} id="mode-denied-actions">
                  <Input
                    id="mode-denied-actions"
                    type="text"
                    value={formDeniedActions()}
                    onInput={(e) => setFormDeniedActions(e.currentTarget.value)}
                    placeholder={t("modes.form.deniedActionsPlaceholder")}
                  />
                </FormField>
                <FormField label={t("modes.form.requiredArtifact")} id="mode-required-artifact">
                  <Input
                    id="mode-required-artifact"
                    type="text"
                    value={formRequiredArtifact()}
                    onInput={(e) => setFormRequiredArtifact(e.currentTarget.value)}
                    placeholder={t("modes.form.requiredArtifactPlaceholder")}
                  />
                </FormField>
                <FormField label={t("modes.form.scenario")} id="mode-scenario">
                  <Select
                    id="mode-scenario"
                    value={formScenario()}
                    onChange={(e) => setFormScenario(e.currentTarget.value)}
                  >
                    <option value="">{t("modes.form.scenarioPlaceholder")}</option>
                    <For each={scenarios() ?? []}>{(s) => <option value={s}>{s}</option>}</For>
                  </Select>
                </FormField>
                <FormField label={t("modes.form.autonomy")} id="mode-autonomy">
                  <Input
                    id="mode-autonomy"
                    type="number"
                    min="1"
                    max="5"
                    value={formAutonomy()}
                    onInput={(e) => setFormAutonomy(Number(e.currentTarget.value))}
                  />
                </FormField>
                <FormField label={t("modes.form.prompt")} id="mode-prompt" class="sm:col-span-2">
                  <Textarea
                    id="mode-prompt"
                    value={formPrompt()}
                    onInput={(e) => setFormPrompt(e.currentTarget.value)}
                    rows={3}
                    placeholder={t("modes.form.promptPlaceholder")}
                  />
                </FormField>
              </div>
              <div class="mt-4 flex justify-end">
                <Button type="submit">
                  {isEditing() ? t("common.save") : t("modes.form.create")}
                </Button>
              </div>
            </form>
          </Card.Body>
        </Card>
      </Show>

      <Show when={modes.loading}>
        <LoadingState message={t("modes.loading")} />
      </Show>

      <Show when={modes.error}>
        <Alert variant="error">{t("modes.loadError")}</Alert>
      </Show>

      <Show when={!modes.loading && !modes.error}>
        <Show when={sorted().length} fallback={<EmptyState title={t("modes.empty")} />}>
          <div class="grid grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-3">
            <For each={sorted()}>
              {(mode) => <ModeCard mode={mode} onEdit={handleEdit} onDelete={handleDelete} />}
            </For>
          </div>
        </Show>
      </Show>
    </PageLayout>
  );
}

function ModeCard(props: {
  mode: Mode;
  onEdit: (mode: Mode) => void;
  onDelete: (mode: Mode) => void;
}) {
  const { t } = useI18n();
  const [showPrompt, setShowPrompt] = createSignal(false);

  const autonomyVariant = (): "success" | "info" | "warning" | "danger" | "default" => {
    switch (props.mode.autonomy) {
      case 1:
        return "success";
      case 2:
        return "info";
      case 3:
        return "warning";
      case 4:
        return "warning";
      case 5:
        return "danger";
      default:
        return "default";
    }
  };

  return (
    <Card class="transition-shadow hover:shadow-md">
      <Card.Body>
        <div class="flex items-start justify-between">
          <div>
            <h3 class="text-lg font-semibold text-cf-text-primary">{props.mode.name}</h3>
            <p class="mt-1 text-sm text-cf-text-muted">{props.mode.description}</p>
          </div>
          <div class="flex items-center gap-2">
            <Show when={!props.mode.builtin}>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => props.onEdit(props.mode)}
                aria-label={t("modes.editAria", { name: props.mode.name })}
              >
                {t("modes.edit")}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => props.onDelete(props.mode)}
                aria-label={t("modes.deleteAria", { name: props.mode.name })}
              >
                {t("modes.delete")}
              </Button>
            </Show>
            <Badge variant={props.mode.builtin ? "default" : "primary"} pill>
              {props.mode.builtin ? t("modes.builtin") : t("modes.custom")}
            </Badge>
          </div>
        </div>

        <div class="mt-3 space-y-2">
          {/* Tools */}
          <div>
            <span class="text-xs font-medium text-cf-text-muted">{t("modes.tools")}</span>
            <div class="mt-1 flex flex-wrap gap-1">
              <For each={props.mode.tools}>{(tool) => <Badge class="font-mono">{tool}</Badge>}</For>
            </div>
          </div>

          {/* Denied Tools */}
          <Show when={props.mode.denied_tools?.length}>
            <div>
              <span class="text-xs font-medium text-cf-text-muted">{t("modes.deniedTools")}</span>
              <div class="mt-1 flex flex-wrap gap-1">
                <For each={props.mode.denied_tools}>
                  {(tool) => (
                    <Badge variant="danger" class="font-mono">
                      {tool}
                    </Badge>
                  )}
                </For>
              </div>
            </div>
          </Show>

          {/* Denied Actions */}
          <Show when={props.mode.denied_actions?.length}>
            <div>
              <span class="text-xs font-medium text-cf-text-muted">{t("modes.deniedActions")}</span>
              <div class="mt-1 flex flex-wrap gap-1">
                <For each={props.mode.denied_actions}>
                  {(action) => (
                    <Badge variant="danger" class="font-mono">
                      {action}
                    </Badge>
                  )}
                </For>
              </div>
            </div>
          </Show>

          {/* Scenario + Autonomy + Required Artifact */}
          <div class="flex flex-wrap items-center gap-3 text-xs">
            <div>
              <span class="font-medium text-cf-text-muted">{t("modes.scenario")}: </span>
              <Badge variant="info">{props.mode.llm_scenario}</Badge>
            </div>
            <div>
              <span class="font-medium text-cf-text-muted">{t("modes.autonomy")}: </span>
              <Badge variant={autonomyVariant()}>
                {t("modes.autonomyLabel", { level: String(props.mode.autonomy) })}
              </Badge>
            </div>
            <Show when={props.mode.required_artifact}>
              <div>
                <span class="font-medium text-cf-text-muted">{t("modes.requiredArtifact")}: </span>
                <Badge variant="warning">{props.mode.required_artifact}</Badge>
              </div>
            </Show>
          </div>

          {/* Prompt toggle */}
          <Show when={props.mode.prompt_prefix}>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setShowPrompt((v) => !v)}
              aria-label={
                showPrompt()
                  ? t("modes.hidePromptAria", { name: props.mode.name })
                  : t("modes.showPromptAria", { name: props.mode.name })
              }
            >
              {showPrompt() ? t("modes.hidePrompt") : t("modes.showPrompt")}
            </Button>
            <Show when={showPrompt()}>
              <p class="mt-1 rounded-cf-md bg-cf-bg-surface-alt p-2 text-xs text-cf-text-secondary">
                {props.mode.prompt_prefix}
              </p>
            </Show>
          </Show>
        </div>
      </Card.Body>
    </Card>
  );
}
