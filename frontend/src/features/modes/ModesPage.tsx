import { createMemo, createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { CreateModeRequest, Mode } from "~/api/types";
import { useConfirm } from "~/components/ConfirmProvider";
import { useToast } from "~/components/Toast";
import { COMMON_DENIED_ACTIONS } from "~/config/domain-constants";
import { useAsyncAction, useCRUDForm } from "~/hooks";
import { useI18n } from "~/i18n";
import {
  Alert,
  Badge,
  Button,
  Card,
  EmptyState,
  ErrorBanner,
  FormField,
  GridLayout,
  Input,
  LoadingState,
  PageLayout,
  Select,
  TagInput,
  Textarea,
} from "~/ui";

interface ModeFormState {
  id: string;
  name: string;
  desc: string;
  tools: string[];
  deniedTools: string[];
  deniedActions: string[];
  requiredArtifact: string;
  scenario: string;
  autonomy: number;
  prompt: string;
}

const FORM_DEFAULTS: ModeFormState = {
  id: "",
  name: "",
  desc: "",
  tools: [],
  deniedTools: [],
  deniedActions: [],
  requiredArtifact: "",
  scenario: "default",
  autonomy: 3,
  prompt: "",
};

export default function ModesPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const { confirm } = useConfirm();
  const [modes, { refetch }] = createResource(() => api.modes.list());
  const [scenarios] = createResource(() => api.modes.scenarios());
  const [toolSuggestions] = createResource(() => api.modes.tools());
  const [artifactTypes] = createResource(() => api.modes.artifactTypes());

  const crud = useCRUDForm<ModeFormState>(FORM_DEFAULTS);

  function handleCancelForm() {
    crud.cancelForm();
    clearError();
  }

  function handleEdit(mode: Mode) {
    crud.startEdit(mode.id, {
      id: mode.id,
      name: mode.name,
      desc: mode.description,
      tools: [...mode.tools],
      deniedTools: [...(mode.denied_tools ?? [])],
      deniedActions: [...(mode.denied_actions ?? [])],
      requiredArtifact: mode.required_artifact ?? "",
      scenario: mode.llm_scenario || "default",
      autonomy: mode.autonomy,
      prompt: mode.prompt_prefix ?? "",
    });
  }

  const {
    run: handleSubmit,
    error,
    clearError,
  } = useAsyncAction(
    async () => {
      const id = crud.form.state.id.trim();
      const name = crud.form.state.name.trim();
      if (!id) {
        toast("error", t("modes.toast.idRequired"));
        return;
      }
      if (!name) {
        toast("error", t("modes.toast.nameRequired"));
        return;
      }
      const req: CreateModeRequest = {
        id,
        name,
        description: crud.form.state.desc.trim() || undefined,
        tools: crud.form.state.tools.filter(Boolean),
        denied_tools: crud.form.state.deniedTools.filter(Boolean),
        denied_actions: crud.form.state.deniedActions.filter(Boolean),
        required_artifact: crud.form.state.requiredArtifact.trim() || undefined,
        llm_scenario: crud.form.state.scenario.trim() || undefined,
        autonomy: crud.form.state.autonomy,
        prompt_prefix: crud.form.state.prompt.trim() || undefined,
      };
      const eid = crud.editingId();
      if (crud.isEditing() && eid) {
        await api.modes.update(eid, req);
        toast("success", t("modes.toast.updated"));
      } else {
        await api.modes.create(req);
        toast("success", t("modes.toast.created"));
      }
      crud.cancelForm();
      refetch();
    },
    {
      onError: (err) => {
        const msg =
          err instanceof Error
            ? err.message
            : crud.isEditing()
              ? t("modes.toast.updateFailed")
              : t("modes.toast.createFailed");
        toast("error", msg);
      },
    },
  );

  const { run: handleDelete } = useAsyncAction(
    async (mode: Mode) => {
      const ok = await confirm({
        title: t("common.delete"),
        message: t("modes.confirm.delete"),
        variant: "danger",
        confirmLabel: t("common.delete"),
      });
      if (!ok) return;
      await api.modes.delete(mode.id);
      toast("success", t("modes.toast.deleted"));
      refetch();
    },
    {
      onError: (err) => {
        const msg = err instanceof Error ? err.message : t("modes.toast.deleteFailed");
        toast("error", msg);
      },
    },
  );

  const sorted = createMemo(() => {
    const list = modes() ?? [];
    return [...list].sort((a, b) => {
      if (a.builtin !== b.builtin) return a.builtin ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
  });

  return (
    <PageLayout
      title={t("modes.title")}
      action={
        <Button
          variant={crud.showForm() ? "secondary" : "primary"}
          onClick={() => {
            if (crud.showForm()) {
              handleCancelForm();
            } else {
              crud.startCreate();
            }
          }}
        >
          {crud.showForm() ? t("common.cancel") : t("modes.addMode")}
        </Button>
      }
    >
      <ErrorBanner error={error} onDismiss={clearError} />

      <Show when={crud.showForm()}>
        <Card class="mb-6">
          <Card.Body>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void handleSubmit();
              }}
              aria-label={crud.isEditing() ? t("modes.edit") : t("modes.addMode")}
            >
              <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <FormField label={t("modes.form.id")} id="mode-id" required>
                  <Input
                    id="mode-id"
                    type="text"
                    value={crud.form.state.id}
                    onInput={(e) => crud.form.setState("id", e.currentTarget.value)}
                    placeholder={t("modes.form.idPlaceholder")}
                    aria-required="true"
                    disabled={crud.isEditing()}
                  />
                </FormField>
                <FormField label={t("modes.form.name")} id="mode-name" required>
                  <Input
                    id="mode-name"
                    type="text"
                    value={crud.form.state.name}
                    onInput={(e) => crud.form.setState("name", e.currentTarget.value)}
                    placeholder={t("modes.form.namePlaceholder")}
                    aria-required="true"
                  />
                </FormField>
                <FormField label={t("modes.form.description")} id="mode-desc" class="sm:col-span-2">
                  <Input
                    id="mode-desc"
                    type="text"
                    value={crud.form.state.desc}
                    onInput={(e) => crud.form.setState("desc", e.currentTarget.value)}
                    placeholder={t("modes.form.descriptionPlaceholder")}
                  />
                </FormField>
                <FormField label={t("modes.form.tools")} id="mode-tools">
                  <TagInput
                    id="mode-tools"
                    values={crud.form.state.tools}
                    onChange={(v) => crud.form.setState("tools", v)}
                    suggestions={toolSuggestions() ?? []}
                    placeholder={t("modes.form.toolsPlaceholder")}
                  />
                </FormField>
                <FormField label={t("modes.form.deniedTools")} id="mode-denied-tools">
                  <TagInput
                    id="mode-denied-tools"
                    values={crud.form.state.deniedTools}
                    onChange={(v) => crud.form.setState("deniedTools", v)}
                    suggestions={toolSuggestions() ?? []}
                    placeholder={t("modes.form.deniedToolsPlaceholder")}
                  />
                </FormField>
                <FormField label={t("modes.form.deniedActions")} id="mode-denied-actions">
                  <TagInput
                    id="mode-denied-actions"
                    values={crud.form.state.deniedActions}
                    onChange={(v) => crud.form.setState("deniedActions", v)}
                    suggestions={[...COMMON_DENIED_ACTIONS]}
                    placeholder={t("modes.form.deniedActionsPlaceholder")}
                  />
                </FormField>
                <FormField label={t("modes.form.requiredArtifact")} id="mode-required-artifact">
                  <Select
                    id="mode-required-artifact"
                    value={crud.form.state.requiredArtifact}
                    onChange={(e) => crud.form.setState("requiredArtifact", e.currentTarget.value)}
                  >
                    <option value="">{t("modes.form.requiredArtifactPlaceholder")}</option>
                    <For each={artifactTypes() ?? []}>
                      {(at) => <option value={at}>{at}</option>}
                    </For>
                  </Select>
                </FormField>
                <FormField label={t("modes.form.scenario")} id="mode-scenario">
                  <Select
                    id="mode-scenario"
                    value={crud.form.state.scenario}
                    onChange={(e) => crud.form.setState("scenario", e.currentTarget.value)}
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
                    value={crud.form.state.autonomy}
                    onInput={(e) => crud.form.setState("autonomy", Number(e.currentTarget.value))}
                  />
                </FormField>
                <FormField label={t("modes.form.prompt")} id="mode-prompt" class="sm:col-span-2">
                  <Textarea
                    id="mode-prompt"
                    value={crud.form.state.prompt}
                    onInput={(e) => crud.form.setState("prompt", e.currentTarget.value)}
                    rows={3}
                    placeholder={t("modes.form.promptPlaceholder")}
                  />
                </FormField>
              </div>
              <div class="mt-4 flex justify-end">
                <Button type="submit">
                  {crud.isEditing() ? t("common.save") : t("modes.form.create")}
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
          <GridLayout>
            <For each={sorted()}>
              {(mode) => <ModeCard mode={mode} onEdit={handleEdit} onDelete={handleDelete} />}
            </For>
          </GridLayout>
        </Show>
      </Show>
    </PageLayout>
  );
}

function ModeCard(props: {
  mode: Mode;
  onEdit: (mode: Mode) => void;
  onDelete: (mode: Mode) => Promise<void>;
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
                onClick={() => void props.onDelete(props.mode)}
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
