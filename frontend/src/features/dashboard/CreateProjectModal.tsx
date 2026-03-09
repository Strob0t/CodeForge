import { batch, createMemo, createResource, createSignal, For, onCleanup, Show } from "solid-js";

import { api } from "~/api/client";
import { useToast } from "~/components/Toast";
import { useAsyncAction } from "~/hooks/useAsyncAction";
import { useFormState } from "~/hooks/useFormState";
import { useI18n } from "~/i18n";
import {
  Button,
  ErrorBanner,
  FormField,
  Input,
  Modal,
  Select,
  Tabs,
  Textarea,
} from "~/ui";

const AUTONOMY_LEVELS = [
  { value: "1", labelKey: "dashboard.form.autonomy.1" as const },
  { value: "2", labelKey: "dashboard.form.autonomy.2" as const },
  { value: "3", labelKey: "dashboard.form.autonomy.3" as const },
  { value: "4", labelKey: "dashboard.form.autonomy.4" as const },
  { value: "5", labelKey: "dashboard.form.autonomy.5" as const },
];

const formDefaults = {
  name: "",
  description: "",
  repo_url: "",
  provider: "",
  localPath: "",
  selectedAutonomy: "",
  selectedBranch: "",
  formMode: "remote" as "remote" | "local" | "empty",
};

export interface CreateProjectModalProps {
  open: boolean;
  onClose: () => void;
  onCreated: () => void;
}

export function CreateProjectModal(props: CreateProjectModalProps) {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [providers] = createResource(() => api.providers.git().then((r) => r.providers));
  const [error, setError] = createSignal("");
  const [parsingUrl, setParsingUrl] = createSignal(false);
  const [showAdvanced, setShowAdvanced] = createSignal(false);
  const [branches, setBranches] = createSignal<string[]>([]);

  const form = useFormState(formDefaults);

  const { run: fetchBranches, loading: loadingBranches } = useAsyncAction(
    async (url: string) => {
      const branchList = await api.projects.remoteBranches(url);
      setBranches(branchList);
      form.setState("selectedBranch", "");
    },
    {
      onError: () => setBranches([]),
    },
  );

  let urlDebounceTimer: ReturnType<typeof setTimeout> | undefined;
  onCleanup(() => clearTimeout(urlDebounceTimer));

  function resetFormState() {
    batch(() => {
      form.reset();
      setError("");
      setShowAdvanced(false);
      setBranches([]);
    });
  }

  function handleClose() {
    resetFormState();
    props.onClose();
  }

  function buildAdvancedConfig(): Record<string, string> {
    const config: Record<string, string> = {};
    const autonomy = form.state.selectedAutonomy;
    if (autonomy) config["autonomy_level"] = autonomy;
    return config;
  }

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    setError("");

    const data = form.state;
    const isEmpty = data.formMode === "empty";
    const isLocal = data.formMode === "local";

    if (isEmpty) {
      if (!data.name.trim()) {
        setError(t("dashboard.toast.nameRequired"));
        return;
      }
      try {
        const created = await api.projects.create({
          name: data.name.trim(),
          description: data.description,
          repo_url: "",
          provider: "",
          config: buildAdvancedConfig(),
        });
        toast("success", t("dashboard.toast.created"));
        api.projects.initWorkspace(created.id).catch((initErr) => {
          const initMsg = initErr instanceof Error ? initErr.message : "init workspace failed";
          toast("error", initMsg);
        });
        toast("info", t("dashboard.toast.setupStarted"));
        resetFormState();
        props.onCreated();
      } catch (err) {
        const msg = err instanceof Error ? err.message : t("dashboard.toast.createFailed");
        setError(msg);
        toast("error", msg);
      }
      return;
    }

    if (isLocal) {
      const path = data.localPath.trim();
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
        resetFormState();
        props.onCreated();
      } catch (err) {
        const msg = err instanceof Error ? err.message : t("dashboard.toast.createFailed");
        setError(msg);
        toast("error", msg);
      }
      return;
    }

    // Remote mode
    if (!data.name.trim() && !data.repo_url.trim()) {
      setError(t("dashboard.toast.nameRequired"));
      return;
    }

    try {
      const branch = data.selectedBranch || undefined;
      const created = await api.projects.create({
        name: data.name,
        description: data.description,
        repo_url: data.repo_url,
        provider: data.provider,
        branch,
        config: buildAdvancedConfig(),
      });
      toast("success", t("dashboard.toast.created"));

      // Auto-setup (clone + detect stack + import specs)
      if (created.repo_url) {
        toast("info", t("dashboard.toast.setupStarted"));
        try {
          await api.projects.setup(created.id, branch);
        } catch (setupErr) {
          const setupMsg = setupErr instanceof Error ? setupErr.message : "setup failed";
          toast("error", setupMsg);
        }
      }
      resetFormState();
      props.onCreated();
    } catch (err) {
      const msg = err instanceof Error ? err.message : t("dashboard.toast.createFailed");
      setError(msg);
      toast("error", msg);
    }
  }

  function handleRepoUrlInput(url: string) {
    form.setState("repo_url", url);
    clearTimeout(urlDebounceTimer);
    if (!url.trim()) {
      setBranches([]);
      form.setState("selectedBranch", "");
      return;
    }
    urlDebounceTimer = setTimeout(async () => {
      try {
        setParsingUrl(true);
        const parsed = await api.projects.parseRepoURL(url);
        form.populate({
          name: form.state.name || parsed.repo,
          provider: form.state.provider || parsed.provider,
        });
      } catch {
        // silently ignore parse errors during typing
      } finally {
        setParsingUrl(false);
      }

      // Fetch repo metadata (description, language, etc.) from hosting API
      try {
        const info = await api.projects.repoInfo(url);
        form.populate({
          name: form.state.name || info.name,
          description: form.state.description || info.description,
        });
      } catch {
        // silently ignore -- repo may be private or API unavailable
      }

      // Fetch remote branches
      await fetchBranches(url);
    }, 500);
  }

  const formModeTabs = createMemo(() => [
    { value: "remote", label: t("dashboard.form.modeRemote") },
    { value: "local", label: t("dashboard.form.modeLocal") },
    { value: "empty", label: t("dashboard.form.modeEmpty") },
  ]);

  return (
    <Modal open={props.open} onClose={handleClose} title={t("dashboard.form.create")}>
      <ErrorBanner error={error} onDismiss={() => setError("")} />

      <form onSubmit={handleSubmit}>
        {/* Mode tab toggle */}
        <div class="mb-4">
          <Tabs
            items={formModeTabs()}
            value={form.state.formMode}
            onChange={(v) => form.setState("formMode", v as "remote" | "local" | "empty")}
            variant="pills"
          />
        </div>

        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {/* Local mode: path field */}
          <Show when={form.state.formMode === "local"}>
            <FormField
              label={t("dashboard.form.path")}
              id="create_local_path"
              required
              class="sm:col-span-2"
            >
              <Input
                id="create_local_path"
                type="text"
                value={form.state.localPath}
                onInput={(e) => form.setState("localPath", e.currentTarget.value)}
                placeholder={t("dashboard.form.pathPlaceholder")}
                aria-required="true"
              />
            </FormField>
          </Show>

          <FormField
            label={t("dashboard.form.name")}
            id="create_name"
            required={
              form.state.formMode === "empty" ||
              (form.state.formMode === "remote" && !form.state.repo_url.trim())
            }
          >
            <Input
              id="create_name"
              type="text"
              value={form.state.name}
              onInput={(e) => form.setState("name", e.currentTarget.value)}
              placeholder={t("dashboard.form.namePlaceholder")}
              aria-required={
                form.state.formMode === "empty" ||
                (form.state.formMode === "remote" && !form.state.repo_url.trim())
                  ? "true"
                  : "false"
              }
            />
          </FormField>

          {/* Remote mode: provider dropdown */}
          <Show when={form.state.formMode === "remote" && providers()}>
            <FormField label={t("dashboard.form.provider")} id="create_provider">
              <Select
                id="create_provider"
                value={form.state.provider}
                onChange={(e) => form.setState("provider", e.currentTarget.value)}
              >
                <option value="">{t("dashboard.form.providerPlaceholder")}</option>
                <For each={providers() ?? []}>{(p) => <option value={p}>{p}</option>}</For>
              </Select>
            </FormField>
          </Show>

          {/* Remote mode: repo URL field */}
          <Show when={form.state.formMode === "remote"}>
            <FormField
              label={t("dashboard.form.repoUrl")}
              id="create_repo_url"
              class="sm:col-span-2"
              help={parsingUrl() ? "detecting..." : undefined}
            >
              <Input
                id="create_repo_url"
                type="text"
                value={form.state.repo_url}
                onInput={(e) => handleRepoUrlInput(e.currentTarget.value)}
                placeholder={t("dashboard.form.repoUrlPlaceholder")}
              />
            </FormField>
          </Show>

          {/* Branch selector (visible when branches loaded or loading) */}
          <Show
            when={
              form.state.formMode === "remote" &&
              (branches().length > 0 || loadingBranches())
            }
          >
            <FormField
              label={t("dashboard.form.branch")}
              id="create_branch"
              class="sm:col-span-2"
              help={loadingBranches() ? t("dashboard.form.branchLoading") : undefined}
            >
              <Select
                id="create_branch"
                value={form.state.selectedBranch}
                onChange={(e) => form.setState("selectedBranch", e.currentTarget.value)}
                disabled={loadingBranches()}
              >
                <option value="">{t("dashboard.form.branchPlaceholder")}</option>
                <For each={branches()}>{(b) => <option value={b}>{b}</option>}</For>
              </Select>
            </FormField>
          </Show>

          <FormField
            label={t("dashboard.form.description")}
            id="create_description"
            class="sm:col-span-2"
          >
            <Textarea
              id="create_description"
              value={form.state.description}
              onInput={(e) => form.setState("description", e.currentTarget.value)}
              rows={2}
              placeholder={t("dashboard.form.descriptionPlaceholder")}
            />
          </FormField>
        </div>

        {/* Advanced Settings Toggle */}
        <div class="mt-4 border-t border-cf-border pt-3">
          <Button
            variant="link"
            size="sm"
            class="flex items-center gap-1"
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
          </Button>

          <Show when={showAdvanced()}>
            <div class="mt-3 grid grid-cols-1 gap-4 sm:grid-cols-2">
              {/* Autonomy level */}
              <FormField label={t("dashboard.form.autonomyLevel")} id="create_adv_autonomy">
                <Select
                  id="create_adv_autonomy"
                  value={form.state.selectedAutonomy}
                  onChange={(e) => form.setState("selectedAutonomy", e.currentTarget.value)}
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

        <div class="mt-4 flex justify-end gap-2">
          <Button variant="secondary" onClick={handleClose}>
            {t("common.cancel")}
          </Button>
          <Button type="submit">
            {t("dashboard.form.create")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
