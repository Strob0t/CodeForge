import { createResource, createSignal, For, onCleanup, Show } from "solid-js";

import { api } from "~/api/client";
import type { CreateProjectRequest, StackDetectionResult } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import type { TranslationKey } from "~/i18n/en";
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
  Tabs,
  Textarea,
} from "~/ui";

import ProjectCard from "./ProjectCard";

const AUTONOMY_LEVELS = [
  { value: "1", labelKey: "dashboard.form.autonomy.1" as const },
  { value: "2", labelKey: "dashboard.form.autonomy.2" as const },
  { value: "3", labelKey: "dashboard.form.autonomy.3" as const },
  { value: "4", labelKey: "dashboard.form.autonomy.4" as const },
  { value: "5", labelKey: "dashboard.form.autonomy.5" as const },
];

const emptyForm: CreateProjectRequest = {
  name: "",
  description: "",
  repo_url: "",
  provider: "",
  config: {},
};

const categoryLabels: Record<string, string> = {
  mode: "dashboard.detect.category.mode",
  pipeline: "dashboard.detect.category.pipeline",
  linter: "dashboard.detect.category.linter",
  formatter: "dashboard.detect.category.formatter",
};

export default function DashboardPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [projects, { refetch }] = createResource(() => api.projects.list());
  const [providers] = createResource(() => api.providers.git().then((r) => r.providers));
  const [showForm, setShowForm] = createSignal(false);
  const [form, setForm] = createSignal<CreateProjectRequest>({ ...emptyForm });
  const [error, setError] = createSignal("");
  const [detecting, setDetecting] = createSignal(false);
  const [stackResult, setStackResult] = createSignal<StackDetectionResult | null>(null);
  const [editingId, setEditingId] = createSignal<string | null>(null);
  const [parsingUrl, setParsingUrl] = createSignal(false);
  const [formMode, setFormMode] = createSignal<"remote" | "local">("remote");
  const [localPath, setLocalPath] = createSignal("");
  const [showAdvanced, setShowAdvanced] = createSignal(false);
  const [selectedAutonomy, setSelectedAutonomy] = createSignal("");
  const [branches, setBranches] = createSignal<string[]>([]);
  const [selectedBranch, setSelectedBranch] = createSignal("");
  const [loadingBranches, setLoadingBranches] = createSignal(false);

  let urlDebounceTimer: ReturnType<typeof setTimeout> | undefined;
  onCleanup(() => clearTimeout(urlDebounceTimer));

  function isEditing() {
    return editingId() !== null;
  }

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    setError("");

    const data = form();
    const isLocal = formMode() === "local" && !isEditing();

    if (isLocal) {
      const path = localPath().trim();
      if (!path) {
        setError(t("dashboard.toast.nameRequired"));
        return;
      }
      const derivedName = data.name.trim() || path.split("/").filter(Boolean).pop() || "";
      try {
        const created = await api.projects.create({
          name: derivedName,
          description: data.description,
          repo_url: "",
          provider: "",
          config: buildAdvancedConfig(),
        });
        toast("success", t("dashboard.toast.created"));
        await api.projects.adopt(created.id, { path });
        toast("info", t("dashboard.toast.setupStarted"));
        setForm({ ...emptyForm });
        setLocalPath("");
        setShowForm(false);
        setEditingId(null);
        setStackResult(null);
        setShowAdvanced(false);
        setSelectedAutonomy("");
        setBranches([]);
        setSelectedBranch("");
        await refetch();
      } catch (err) {
        const msg = err instanceof Error ? err.message : t("dashboard.toast.createFailed");
        setError(msg);
        toast("error", msg);
      }
      return;
    }

    if (!data.name.trim() && !data.repo_url.trim()) {
      setError(t("dashboard.toast.nameRequired"));
      return;
    }

    try {
      const eid = editingId();
      if (isEditing() && eid) {
        await api.projects.update(eid, {
          name: data.name || undefined,
          description: data.description || undefined,
          repo_url: data.repo_url || undefined,
          provider: data.provider || undefined,
          config: buildAdvancedConfig(),
        });
        toast("success", t("dashboard.toast.updated"));
      } else {
        const branch = selectedBranch() || undefined;
        const created = await api.projects.create({
          ...data,
          branch,
          config: buildAdvancedConfig(),
        });
        toast("success", t("dashboard.toast.created"));

        // Fire-and-forget: auto-setup (clone + detect stack + import specs)
        if (created.repo_url) {
          toast("info", t("dashboard.toast.setupStarted"));
          api.projects.setup(created.id, branch).catch((setupErr) => {
            const setupMsg = setupErr instanceof Error ? setupErr.message : "setup failed";
            toast("error", setupMsg);
          });
        }
      }
      setForm({ ...emptyForm });
      setShowForm(false);
      setEditingId(null);
      setStackResult(null);
      setShowAdvanced(false);
      setSelectedAutonomy("");
      setBranches([]);
      setSelectedBranch("");
      await refetch();
    } catch (err) {
      const msg = err instanceof Error ? err.message : t("dashboard.toast.createFailed");
      setError(msg);
      toast("error", msg);
    }
  }

  function handleEdit(id: string) {
    const p = projects()?.find((proj) => proj.id === id);
    if (!p) return;
    const cfg = p.config ?? {};
    setForm({
      name: p.name,
      description: p.description,
      repo_url: p.repo_url,
      provider: p.provider,
      config: cfg,
    });
    setSelectedAutonomy(cfg["autonomy_level"] ?? "");
    if (cfg["autonomy_level"]) {
      setShowAdvanced(true);
    }
    setEditingId(id);
    setShowForm(true);
  }

  function handleCancelForm() {
    setShowForm(false);
    setEditingId(null);
    setForm({ ...emptyForm });
    setLocalPath("");
    setFormMode("remote");
    setError("");
    setShowAdvanced(false);
    setSelectedAutonomy("");
    setBranches([]);
    setSelectedBranch("");
  }

  async function handleDelete(id: string) {
    try {
      await api.projects.delete(id);
      await refetch();
      toast("success", t("dashboard.toast.deleted"));
    } catch (err) {
      const msg = err instanceof Error ? err.message : t("dashboard.toast.deleteFailed");
      setError(msg);
      toast("error", msg);
    }
  }

  async function handleDetectStack(projectId: string) {
    setDetecting(true);
    setStackResult(null);
    try {
      const result = await api.projects.detectStack(projectId);
      setStackResult(result);
    } catch (err) {
      const msg = err instanceof Error ? err.message : t("dashboard.detect.error");
      toast("error", msg);
    } finally {
      setDetecting(false);
    }
  }

  function updateField<K extends keyof CreateProjectRequest>(
    field: K,
    value: CreateProjectRequest[K],
  ) {
    setForm((prev) => ({ ...prev, [field]: value }));
  }

  function buildAdvancedConfig(): Record<string, string> {
    const config: Record<string, string> = {};
    const autonomy = selectedAutonomy();
    if (autonomy) config["autonomy_level"] = autonomy;
    return config;
  }

  function handleRepoUrlInput(url: string) {
    updateField("repo_url", url);
    clearTimeout(urlDebounceTimer);
    if (!url.trim()) {
      setBranches([]);
      setSelectedBranch("");
      return;
    }
    urlDebounceTimer = setTimeout(async () => {
      try {
        setParsingUrl(true);
        const parsed = await api.projects.parseRepoURL(url);
        setForm((prev) => ({
          ...prev,
          name: prev.name || parsed.repo,
          provider: prev.provider || parsed.provider,
        }));
      } catch {
        // silently ignore parse errors during typing
      } finally {
        setParsingUrl(false);
      }

      // Fetch remote branches
      try {
        setLoadingBranches(true);
        const branchList = await api.projects.remoteBranches(url);
        setBranches(branchList);
        setSelectedBranch("");
      } catch {
        setBranches([]);
      } finally {
        setLoadingBranches(false);
      }
    }, 500);
  }

  const formModeTabs = () => [
    { value: "remote", label: t("dashboard.form.modeRemote") },
    { value: "local", label: t("dashboard.form.modeLocal") },
  ];

  return (
    <PageLayout
      title={t("dashboard.title")}
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
          {showForm() ? t("common.cancel") : t("dashboard.addProject")}
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
            <form onSubmit={handleSubmit}>
              {/* Mode tab toggle (hidden when editing) */}
              <Show when={!isEditing()}>
                <div class="mb-4">
                  <Tabs
                    items={formModeTabs()}
                    value={formMode()}
                    onChange={(v) => setFormMode(v as "remote" | "local")}
                    variant="pills"
                  />
                </div>
              </Show>

              <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* Local mode: path field */}
                <Show when={formMode() === "local" && !isEditing()}>
                  <FormField
                    label={t("dashboard.form.path")}
                    id="local_path"
                    required
                    class="sm:col-span-2"
                  >
                    <Input
                      id="local_path"
                      type="text"
                      value={localPath()}
                      onInput={(e) => setLocalPath(e.currentTarget.value)}
                      placeholder={t("dashboard.form.pathPlaceholder")}
                      aria-required="true"
                    />
                  </FormField>
                </Show>

                <FormField
                  label={t("dashboard.form.name")}
                  id="name"
                  required={formMode() === "remote" && !form().repo_url.trim()}
                >
                  <Input
                    id="name"
                    type="text"
                    value={form().name}
                    onInput={(e) => updateField("name", e.currentTarget.value)}
                    placeholder={t("dashboard.form.namePlaceholder")}
                    aria-required={
                      formMode() === "remote" && !form().repo_url.trim() ? "true" : "false"
                    }
                  />
                </FormField>

                {/* Remote mode: provider dropdown */}
                <Show when={(formMode() === "remote" || isEditing()) && providers()}>
                  <FormField label={t("dashboard.form.provider")} id="provider">
                    <Select
                      id="provider"
                      value={form().provider}
                      onChange={(e) => updateField("provider", e.currentTarget.value)}
                    >
                      <option value="">{t("dashboard.form.providerPlaceholder")}</option>
                      <For each={providers() ?? []}>{(p) => <option value={p}>{p}</option>}</For>
                    </Select>
                  </FormField>
                </Show>

                {/* Remote mode: repo URL field */}
                <Show when={formMode() === "remote" || isEditing()}>
                  <FormField
                    label={t("dashboard.form.repoUrl")}
                    id="repo_url"
                    class="sm:col-span-2"
                    help={parsingUrl() ? "detecting..." : undefined}
                  >
                    <Input
                      id="repo_url"
                      type="text"
                      value={form().repo_url}
                      onInput={(e) => handleRepoUrlInput(e.currentTarget.value)}
                      placeholder={t("dashboard.form.repoUrlPlaceholder")}
                    />
                  </FormField>
                </Show>

                {/* Branch selector (visible when branches loaded or loading) */}
                <Show
                  when={
                    (formMode() === "remote" || isEditing()) &&
                    (branches().length > 0 || loadingBranches())
                  }
                >
                  <FormField
                    label={t("dashboard.form.branch")}
                    id="branch"
                    class="sm:col-span-2"
                    help={loadingBranches() ? t("dashboard.form.branchLoading") : undefined}
                  >
                    <Select
                      id="branch"
                      value={selectedBranch()}
                      onChange={(e) => setSelectedBranch(e.currentTarget.value)}
                      disabled={loadingBranches()}
                    >
                      <option value="">{t("dashboard.form.branchPlaceholder")}</option>
                      <For each={branches()}>{(b) => <option value={b}>{b}</option>}</For>
                    </Select>
                  </FormField>
                </Show>

                <FormField
                  label={t("dashboard.form.description")}
                  id="description"
                  class="sm:col-span-2"
                >
                  <Textarea
                    id="description"
                    value={form().description}
                    onInput={(e) => updateField("description", e.currentTarget.value)}
                    rows={2}
                    placeholder={t("dashboard.form.descriptionPlaceholder")}
                  />
                </FormField>
              </div>

              {/* Advanced Settings Toggle */}
              <div class="mt-4 border-t border-cf-border pt-3">
                <button
                  type="button"
                  class="flex items-center gap-1 text-sm font-medium text-cf-text-secondary hover:text-cf-text-primary transition-colors"
                  onClick={() => setShowAdvanced(!showAdvanced())}
                  aria-expanded={showAdvanced()}
                >
                  <span
                    class="inline-block transition-transform"
                    classList={{ "rotate-90": showAdvanced() }}
                  >
                    &#9654;
                  </span>
                  {t("dashboard.form.advanced")}
                </button>

                <Show when={showAdvanced()}>
                  <div class="mt-3 grid grid-cols-1 gap-4 sm:grid-cols-2">
                    {/* Autonomy level */}
                    <FormField label={t("dashboard.form.autonomyLevel")} id="adv_autonomy">
                      <Select
                        id="adv_autonomy"
                        value={selectedAutonomy()}
                        onChange={(e) => setSelectedAutonomy(e.currentTarget.value)}
                      >
                        <option value="">{t("dashboard.form.autonomyPlaceholder")}</option>
                        <For each={AUTONOMY_LEVELS}>
                          {(level) => <option value={level.value}>{t(level.labelKey)}</option>}
                        </For>
                      </Select>
                    </FormField>
                  </div>
                </Show>
              </div>

              <div class="mt-4 flex justify-end">
                <Button type="submit">
                  {isEditing() ? t("common.save") : t("dashboard.form.create")}
                </Button>
              </div>
            </form>
          </Card.Body>
        </Card>
      </Show>

      <Show when={projects.loading}>
        <LoadingState message={t("dashboard.loading")} />
      </Show>

      <Show when={projects.error}>
        <Alert variant="error">{t("dashboard.loadError")}</Alert>
      </Show>

      <Show when={!projects.loading && !projects.error}>
        <Show when={projects()?.length} fallback={<EmptyState title={t("dashboard.empty")} />}>
          <div class="grid grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-3">
            <For each={projects()}>
              {(p) => (
                <ProjectCard
                  project={p}
                  onDelete={handleDelete}
                  onEdit={handleEdit}
                  onDetectStack={handleDetectStack}
                  detecting={detecting()}
                />
              )}
            </For>
          </div>
        </Show>
      </Show>

      {/* Stack Detection Results Panel */}
      <Show when={stackResult()}>
        {(result) => (
          <Card class="mt-6">
            <Card.Body>
              <h3 class="text-lg font-semibold text-cf-text-primary mb-4">
                {t("dashboard.detect.languages")}
              </h3>

              <Show
                when={(result().languages ?? []).length > 0}
                fallback={
                  <p class="text-sm text-cf-text-muted">{t("dashboard.detect.noLanguages")}</p>
                }
              >
                <div class="flex flex-wrap gap-2 mb-4">
                  <For each={result().languages ?? []}>
                    {(lang) => (
                      <Badge variant="info" pill>
                        <span class="font-medium">{lang.name}</span>
                        <span class="ml-2 text-xs opacity-75">
                          {Math.round(lang.confidence * 100)}%
                        </span>
                        <Show when={(lang.frameworks ?? []).length > 0}>
                          <span class="ml-1 text-xs opacity-60">
                            ({(lang.frameworks ?? []).join(", ")})
                          </span>
                        </Show>
                      </Badge>
                    )}
                  </For>
                </div>
              </Show>

              <Show when={(result().recommendations ?? []).length > 0}>
                <h4 class="text-sm font-semibold text-cf-text-secondary mb-2">
                  {t("dashboard.detect.recommendations")}
                </h4>
                <div class="flex flex-wrap gap-2">
                  <For each={result().recommendations}>
                    {(rec) => (
                      <Badge>
                        <span class="font-medium text-cf-text-muted">
                          {t((categoryLabels[rec.category] ?? rec.category) as TranslationKey)}
                        </span>
                        <span class="ml-1 text-cf-text-primary">{rec.name}</span>
                      </Badge>
                    )}
                  </For>
                </div>
              </Show>
            </Card.Body>
          </Card>
        )}
      </Show>
    </PageLayout>
  );
}
