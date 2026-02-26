import { createResource, createSignal, For, onMount, Show } from "solid-js";

import { api, FetchError } from "~/api/client";
import type {
  APIKeyInfo,
  BenchmarkResult,
  CreateAPIKeyRequest,
  CreateVCSAccountRequest,
  User,
  VCSAccount,
  VCSProvider,
} from "~/api/types";
import { useAuth } from "~/components/AuthProvider";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import {
  Alert,
  Badge,
  Button,
  Card,
  Checkbox,
  ConfirmDialog,
  FormField,
  Input,
  PageLayout,
  Section,
  Select,
  Table,
  Textarea,
} from "~/ui";
import type { TableColumn } from "~/ui/composites/Table";

const AUTONOMY_LEVELS = [
  { value: "supervised", label: "1 - Supervised" },
  { value: "semi-auto", label: "2 - Semi-Auto" },
  { value: "auto-edit", label: "3 - Auto-Edit" },
  { value: "full-auto", label: "4 - Full-Auto" },
  { value: "headless", label: "5 - Headless" },
];

export default function SettingsPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const auth = useAuth();

  // -- General settings -------------------------------------------------------
  const [defaultProvider, setDefaultProvider] = createSignal("");
  const [defaultAutonomy, setDefaultAutonomy] = createSignal("supervised");
  const [autoClone, setAutoClone] = createSignal(false);
  const [saving, setSaving] = createSignal(false);

  onMount(async () => {
    try {
      const data = await api.settings.get();
      if (data.default_provider) setDefaultProvider(data.default_provider);
      if (data.default_autonomy) setDefaultAutonomy(data.default_autonomy);
      if (data.auto_clone !== undefined) setAutoClone(data.auto_clone);
    } catch {
      // Settings may not exist yet, use defaults
    }
  });

  const handleSaveGeneral = async () => {
    setSaving(true);
    try {
      await api.settings.update({
        settings: {
          default_provider: defaultProvider(),
          default_autonomy: defaultAutonomy(),
          auto_clone: autoClone(),
        },
      });
      toast("success", t("settings.general.saved"));
    } catch {
      toast("error", t("settings.general.saveFailed"));
    } finally {
      setSaving(false);
    }
  };

  // -- Provider info (read-only) -------------------------------------------
  const [gitProviders] = createResource(() => api.providers.git().then((r) => r.providers));
  const [agentProviders] = createResource(() => api.providers.agent().then((r) => r.backends));
  const [specProviders] = createResource(() =>
    api.providers.spec().then((list) => list.map((p) => p.name)),
  );
  const [pmProviders] = createResource(() =>
    api.providers.pm().then((list) => list.map((p) => p.name)),
  );
  const [llmHealth] = createResource(() => api.llm.health());

  // -- API Keys (personal) -------------------------------------------------
  const [apiKeys, { refetch: refetchKeys }] = createResource<APIKeyInfo[]>(() =>
    api.auth.listAPIKeys(),
  );
  const [newKeyName, setNewKeyName] = createSignal("");
  const [createdKey, setCreatedKey] = createSignal<string | null>(null);

  const handleCreateKey = async () => {
    const name = newKeyName().trim();
    if (!name) return;
    try {
      const req: CreateAPIKeyRequest = { name };
      const res = await api.auth.createAPIKey(req);
      setCreatedKey(res.plain_key);
      setNewKeyName("");
      refetchKeys();
      toast("success", t("settings.apiKey.created"));
    } catch {
      toast("error", t("settings.apiKey.createFailed"));
    }
  };

  const handleDeleteKey = async (id: string) => {
    try {
      await api.auth.deleteAPIKey(id);
      refetchKeys();
      toast("success", t("settings.apiKey.deleted"));
    } catch {
      toast("error", t("settings.apiKey.deleteFailed"));
    }
  };

  // -- User management (admin only) ----------------------------------------
  const [users, { refetch: refetchUsers }] = createResource<User[]>(() => {
    if (auth.user()?.role === "admin") return api.users.list();
    return Promise.resolve([]);
  });

  const handleDeleteUser = async (id: string) => {
    try {
      await api.users.delete(id);
      refetchUsers();
      toast("success", t("settings.users.deleted"));
    } catch {
      toast("error", t("settings.users.deleteFailed"));
    }
  };

  const handleToggleUser = async (u: User) => {
    try {
      await api.users.update(u.id, { enabled: !u.enabled });
      refetchUsers();
    } catch {
      toast("error", t("settings.users.updateFailed"));
    }
  };

  // -- VCS Accounts ----------------------------------------------------------
  const [vcsAccounts, { refetch: refetchVCS }] = createResource<VCSAccount[]>(() =>
    api.vcsAccounts.list(),
  );
  const [vcsProvider, setVcsProvider] = createSignal<VCSProvider>("github");
  const [vcsLabel, setVcsLabel] = createSignal("");
  const [vcsToken, setVcsToken] = createSignal("");
  const [vcsServerUrl, setVcsServerUrl] = createSignal("");
  const [testingId, setTestingId] = createSignal<string | null>(null);
  const [vcsDeleteId, setVcsDeleteId] = createSignal<string | null>(null);

  const handleCreateVCS = async () => {
    const label = vcsLabel().trim();
    const token = vcsToken().trim();
    if (!label || !token) return;
    try {
      const req: CreateVCSAccountRequest = {
        provider: vcsProvider(),
        label,
        token,
        server_url: vcsServerUrl().trim() || undefined,
      };
      await api.vcsAccounts.create(req);
      setVcsLabel("");
      setVcsToken("");
      setVcsServerUrl("");
      refetchVCS();
      toast("success", t("settings.vcs.created"));
    } catch {
      toast("error", t("settings.vcs.createFailed"));
    }
  };

  const handleDeleteVCS = async (id: string) => {
    try {
      await api.vcsAccounts.delete(id);
      refetchVCS();
      toast("success", t("settings.vcs.deleted"));
    } catch {
      toast("error", t("settings.vcs.deleteFailed"));
    } finally {
      setVcsDeleteId(null);
    }
  };

  const handleTestVCS = async (id: string) => {
    setTestingId(id);
    try {
      await api.vcsAccounts.test(id);
      toast("success", t("settings.vcs.testSuccess"));
    } catch {
      toast("error", t("settings.vcs.testFailed"));
    } finally {
      setTestingId(null);
    }
  };

  // -- Benchmark (dev tools) -------------------------------------------------
  const [benchModel, setBenchModel] = createSignal("");
  const [benchSystemPrompt, setBenchSystemPrompt] = createSignal("");
  const [benchPrompt, setBenchPrompt] = createSignal("");
  const [benchTemp, setBenchTemp] = createSignal(0.7);
  const [benchMaxTokens, setBenchMaxTokens] = createSignal(1000);
  const [benchRunning, setBenchRunning] = createSignal(false);
  const [benchResult, setBenchResult] = createSignal<BenchmarkResult | null>(null);
  const [benchError, setBenchError] = createSignal<string | null>(null);

  const handleRunBenchmark = async () => {
    setBenchRunning(true);
    setBenchResult(null);
    setBenchError(null);
    try {
      const result = await api.dev.benchmark({
        model: benchModel(),
        prompt: benchPrompt(),
        system_prompt: benchSystemPrompt() || undefined,
        temperature: benchTemp(),
        max_tokens: benchMaxTokens(),
      });
      setBenchResult(result);
    } catch (err) {
      if (err instanceof FetchError && err.status === 403) {
        setBenchError(t("settings.benchmark.devModeRequired"));
      } else {
        setBenchError(err instanceof Error ? err.message : "Benchmark failed");
      }
    } finally {
      setBenchRunning(false);
    }
  };

  // -- User table columns --
  const userColumns: TableColumn<User>[] = [
    { key: "email", header: t("settings.users.email") },
    { key: "name", header: t("settings.users.name") },
    {
      key: "role",
      header: t("settings.users.role"),
      render: (u) => (
        <Badge
          variant={u.role === "admin" ? "danger" : u.role === "editor" ? "primary" : "default"}
          pill
        >
          {u.role}
        </Badge>
      ),
    },
    {
      key: "status",
      header: t("common.status"),
      render: (u) => (
        <button
          type="button"
          class="text-xs"
          onClick={() => handleToggleUser(u)}
          aria-label={
            u.enabled
              ? t("settings.users.disableAria", { name: u.name })
              : t("settings.users.enableAria", { name: u.name })
          }
        >
          <Badge variant={u.enabled ? "success" : "default"} pill>
            {u.enabled ? t("settings.users.enabled") : t("settings.users.disabled")}
          </Badge>
        </button>
      ),
    },
    {
      key: "actions",
      header: t("settings.users.actions"),
      render: (u) => (
        <Button
          variant="danger"
          size="sm"
          onClick={() => handleDeleteUser(u.id)}
          aria-label={t("settings.users.deleteAria", { name: u.name })}
        >
          {t("common.delete")}
        </Button>
      ),
    },
  ];

  const providerBadgeVariant = (provider: string) => {
    switch (provider) {
      case "github":
        return "default" as const;
      case "gitlab":
        return "warning" as const;
      case "gitea":
        return "success" as const;
      default:
        return "info" as const;
    }
  };

  return (
    <PageLayout title={t("settings.title")}>
      {/* General Settings Section */}
      <Section title={t("settings.general.title")} class="mb-8">
        <div class="space-y-4">
          <FormField
            label={t("settings.general.defaultProvider")}
            id="default-provider"
            help={t("settings.general.defaultProviderHelp")}
          >
            <Input
              id="default-provider"
              type="text"
              value={defaultProvider()}
              onInput={(e) => setDefaultProvider(e.currentTarget.value)}
              placeholder="e.g. openai/gpt-4o"
              class="max-w-md"
            />
          </FormField>

          <FormField
            label={t("settings.general.defaultAutonomy")}
            id="default-autonomy"
            help={t("settings.general.defaultAutonomyHelp")}
          >
            <Select
              id="default-autonomy"
              value={defaultAutonomy()}
              onChange={(e) => setDefaultAutonomy(e.currentTarget.value)}
              class="max-w-md"
            >
              <For each={AUTONOMY_LEVELS}>
                {(level) => <option value={level.value}>{level.label}</option>}
              </For>
            </Select>
          </FormField>

          <div class="flex items-center gap-3">
            <Checkbox
              id="auto-clone"
              checked={autoClone()}
              onChange={(checked) => setAutoClone(checked)}
            />
            <div>
              <label for="auto-clone" class="text-sm font-medium text-cf-text-secondary">
                {t("settings.general.autoClone")}
              </label>
              <p class="text-xs text-cf-text-muted">{t("settings.general.autoCloneHelp")}</p>
            </div>
          </div>

          <div class="pt-2">
            <Button onClick={handleSaveGeneral} loading={saving()}>
              {t("settings.general.save")}
            </Button>
          </div>
        </div>
      </Section>

      {/* VCS Accounts Section */}
      <Section title={t("settings.vcs.title")} class="mb-8">
        {/* Add new account form */}
        <div class="mb-4 space-y-3">
          <div class="flex gap-2">
            <Select
              value={vcsProvider()}
              onChange={(e) => setVcsProvider(e.currentTarget.value as VCSProvider)}
              aria-label={t("settings.vcs.provider")}
              class="w-auto"
            >
              <option value="github">GitHub</option>
              <option value="gitlab">GitLab</option>
              <option value="gitea">Gitea</option>
              <option value="bitbucket">Bitbucket</option>
            </Select>
            <Input
              type="text"
              value={vcsLabel()}
              onInput={(e) => setVcsLabel(e.currentTarget.value)}
              placeholder={t("settings.vcs.labelPlaceholder")}
              aria-label={t("settings.vcs.label")}
              class="flex-1"
            />
          </div>
          <div class="flex gap-2">
            <Input
              type="password"
              value={vcsToken()}
              onInput={(e) => setVcsToken(e.currentTarget.value)}
              placeholder={t("settings.vcs.tokenPlaceholder")}
              aria-label={t("settings.vcs.token")}
              class="flex-1"
            />
            <Input
              type="text"
              value={vcsServerUrl()}
              onInput={(e) => setVcsServerUrl(e.currentTarget.value)}
              placeholder={t("settings.vcs.serverUrlPlaceholder")}
              aria-label={t("settings.vcs.serverUrl")}
              class="flex-1"
            />
          </div>
          <Button onClick={handleCreateVCS} disabled={!vcsLabel().trim() || !vcsToken().trim()}>
            {t("settings.vcs.add")}
          </Button>
        </div>

        {/* Account list */}
        <Show
          when={(vcsAccounts() ?? []).length > 0}
          fallback={<p class="text-sm text-cf-text-muted">{t("settings.vcs.empty")}</p>}
        >
          <ul class="divide-y divide-cf-border">
            <For each={vcsAccounts() ?? []}>
              {(acct) => (
                <li class="flex items-center justify-between py-3">
                  <div class="flex items-center gap-3">
                    <Badge variant={providerBadgeVariant(acct.provider)} pill>
                      {acct.provider}
                    </Badge>
                    <div>
                      <span class="text-sm font-medium text-cf-text-primary">{acct.label}</span>
                      <Show when={acct.server_url}>
                        <span class="ml-2 text-xs text-cf-text-muted">{acct.server_url}</span>
                      </Show>
                    </div>
                  </div>
                  <div class="flex items-center gap-2">
                    <span class="text-xs text-cf-text-muted">
                      {new Date(acct.created_at).toLocaleDateString()}
                    </span>
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={() => handleTestVCS(acct.id)}
                      loading={testingId() === acct.id}
                      aria-label={t("settings.vcs.testAria", { name: acct.label })}
                    >
                      {testingId() === acct.id ? t("settings.vcs.testing") : t("settings.vcs.test")}
                    </Button>
                    <Button
                      variant="danger"
                      size="sm"
                      onClick={() => setVcsDeleteId(acct.id)}
                      aria-label={t("settings.vcs.deleteAria", { name: acct.label })}
                    >
                      {t("common.delete")}
                    </Button>
                  </div>
                </li>
              )}
            </For>
          </ul>
        </Show>
      </Section>

      {/* VCS Delete Confirm Dialog */}
      <ConfirmDialog
        open={vcsDeleteId() !== null}
        title={t("common.delete")}
        message={t("settings.vcs.deleteConfirm")}
        variant="danger"
        onConfirm={() => {
          const id = vcsDeleteId();
          if (id) handleDeleteVCS(id);
        }}
        onCancel={() => setVcsDeleteId(null)}
      />

      {/* Providers Section */}
      <Section title={t("settings.providers.title")} class="mb-8">
        <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <ProviderCard
            label={t("settings.providers.git")}
            items={gitProviders() ?? []}
            loading={gitProviders.loading}
          />
          <ProviderCard
            label={t("settings.providers.agent")}
            items={agentProviders() ?? []}
            loading={agentProviders.loading}
          />
          <ProviderCard
            label={t("settings.providers.spec")}
            items={specProviders() ?? []}
            loading={specProviders.loading}
          />
          <ProviderCard
            label={t("settings.providers.pm")}
            items={pmProviders() ?? []}
            loading={pmProviders.loading}
          />
        </div>
      </Section>

      {/* LLM Health */}
      <Section title={t("settings.llm.title")} class="mb-8">
        <Show
          when={!llmHealth.loading}
          fallback={<p class="text-sm text-cf-text-muted">{t("settings.llm.checking")}</p>}
        >
          <Show
            when={llmHealth()}
            fallback={<Alert variant="error">{t("settings.llm.unavailable")}</Alert>}
          >
            <Alert variant="success">{t("settings.llm.connected")}</Alert>
          </Show>
        </Show>
      </Section>

      {/* API Keys Section */}
      <Section title={t("settings.apiKey.title")} class="mb-8">
        {/* Create new key */}
        <div class="mb-4 flex gap-2">
          <Input
            type="text"
            value={newKeyName()}
            onInput={(e) => setNewKeyName(e.currentTarget.value)}
            placeholder={t("settings.apiKey.namePlaceholder")}
            aria-label={t("settings.apiKey.nameLabel")}
            class="flex-1"
          />
          <Button onClick={handleCreateKey} disabled={!newKeyName().trim()}>
            {t("settings.apiKey.create")}
          </Button>
        </div>

        {/* Show newly created key */}
        <Show when={createdKey()}>
          {(key) => (
            <Alert variant="success" onDismiss={() => setCreatedKey(null)} class="mb-4">
              <p class="mb-1 font-medium">{t("settings.apiKey.copyWarning")}</p>
              <code class="block break-all rounded bg-cf-bg-surface p-2 font-mono text-xs">
                {key()}
              </code>
            </Alert>
          )}
        </Show>

        {/* Key list */}
        <Show
          when={(apiKeys() ?? []).length > 0}
          fallback={<p class="text-sm text-cf-text-muted">{t("settings.apiKey.empty")}</p>}
        >
          <ul class="divide-y divide-cf-border">
            <For each={apiKeys() ?? []}>
              {(key) => (
                <li class="flex items-center justify-between py-2">
                  <div>
                    <span class="text-sm font-medium text-cf-text-primary">{key.name}</span>
                    <span class="ml-2 font-mono text-xs text-cf-text-muted">{key.prefix}...</span>
                  </div>
                  <Button
                    variant="danger"
                    size="sm"
                    onClick={() => handleDeleteKey(key.id)}
                    aria-label={t("settings.apiKey.deleteAria", { name: key.name })}
                  >
                    {t("common.delete")}
                  </Button>
                </li>
              )}
            </For>
          </ul>
        </Show>
      </Section>

      {/* User Management (admin only) */}
      <Show when={auth.user()?.role === "admin"}>
        <Section title={t("settings.users.title")} class="mb-8">
          <Table<User>
            columns={userColumns}
            data={users() ?? []}
            rowKey={(u) => u.id}
            emptyMessage={t("settings.users.empty")}
          />
        </Section>
      </Show>

      {/* Developer Tools Section */}
      <Section title={t("settings.devTools")} class="mb-8">
        <h4 class="mb-3 text-sm font-medium text-cf-text-secondary">
          {t("settings.benchmark.title")}
        </h4>
        <div class="space-y-3">
          <FormField label={t("settings.benchmark.model")} id="bench-model">
            <Input
              id="bench-model"
              type="text"
              value={benchModel()}
              onInput={(e) => setBenchModel(e.currentTarget.value)}
              placeholder="e.g. openai/gpt-4o"
              class="max-w-md"
            />
          </FormField>

          <FormField label={t("settings.benchmark.systemPrompt")} id="bench-system">
            <Textarea
              id="bench-system"
              value={benchSystemPrompt()}
              onInput={(e) => setBenchSystemPrompt(e.currentTarget.value)}
              placeholder="Optional system instructions..."
              rows={2}
            />
          </FormField>

          <FormField label={t("settings.benchmark.prompt")} id="bench-prompt">
            <Textarea
              id="bench-prompt"
              value={benchPrompt()}
              onInput={(e) => setBenchPrompt(e.currentTarget.value)}
              placeholder="Enter your prompt..."
              rows={4}
            />
          </FormField>

          {/* Temperature + Max Tokens */}
          <div class="flex gap-4">
            <div class="flex-1">
              <label for="bench-temp" class="mb-1 block text-sm font-medium text-cf-text-secondary">
                {t("settings.benchmark.temperature")}: {benchTemp().toFixed(1)}
              </label>
              <input
                id="bench-temp"
                type="range"
                min="0"
                max="2"
                step="0.1"
                value={benchTemp()}
                onInput={(e) => setBenchTemp(parseFloat(e.currentTarget.value))}
                class="w-full max-w-md"
              />
            </div>
            <div class="w-40">
              <FormField label={t("settings.benchmark.maxTokens")} id="bench-tokens">
                <Input
                  id="bench-tokens"
                  type="number"
                  min="1"
                  max="128000"
                  value={benchMaxTokens()}
                  onInput={(e) => setBenchMaxTokens(parseInt(e.currentTarget.value, 10) || 1000)}
                />
              </FormField>
            </div>
          </div>

          {/* Run button */}
          <div class="pt-2">
            <Button
              onClick={handleRunBenchmark}
              loading={benchRunning()}
              disabled={!benchModel().trim() || !benchPrompt().trim()}
            >
              {benchRunning() ? t("settings.benchmark.running") : t("settings.benchmark.run")}
            </Button>
          </div>

          {/* Error */}
          <Show when={benchError()}>{(err) => <Alert variant="error">{err()}</Alert>}</Show>

          {/* Results */}
          <Show when={benchResult()}>
            {(result) => (
              <Card>
                <Card.Body>
                  <div class="flex flex-wrap gap-4 text-sm">
                    <div>
                      <span class="font-medium text-cf-text-tertiary">
                        {t("settings.benchmark.model")}:
                      </span>{" "}
                      <span class="font-mono">{result().model}</span>
                    </div>
                    <div>
                      <span class="font-medium text-cf-text-tertiary">
                        {t("settings.benchmark.latency")}:
                      </span>{" "}
                      {result().latency_ms} ms
                    </div>
                    <div>
                      <span class="font-medium text-cf-text-tertiary">
                        {t("settings.benchmark.tokensIn")}:
                      </span>{" "}
                      {result().tokens_in}
                    </div>
                    <div>
                      <span class="font-medium text-cf-text-tertiary">
                        {t("settings.benchmark.tokensOut")}:
                      </span>{" "}
                      {result().tokens_out}
                    </div>
                  </div>
                  <div class="mt-3">
                    <p class="mb-1 text-sm font-medium text-cf-text-tertiary">
                      {t("settings.benchmark.response")}:
                    </p>
                    <pre class="max-h-64 overflow-auto whitespace-pre-wrap rounded bg-cf-bg-surface-alt p-3 text-sm">
                      {result().content}
                    </pre>
                  </div>
                </Card.Body>
              </Card>
            )}
          </Show>
        </div>
      </Section>
    </PageLayout>
  );
}

function ProviderCard(props: { label: string; items: string[]; loading: boolean }) {
  const { t } = useI18n();
  return (
    <Card>
      <Card.Body>
        <h4 class="mb-2 text-sm font-medium text-cf-text-tertiary">{props.label}</h4>
        <Show
          when={!props.loading}
          fallback={<p class="text-xs text-cf-text-muted">{t("settings.providers.loading")}</p>}
        >
          <Show
            when={props.items.length > 0}
            fallback={<p class="text-xs text-cf-text-muted">{t("settings.providers.none")}</p>}
          >
            <ul class="space-y-1">
              <For each={props.items}>
                {(item) => (
                  <li class="flex items-center gap-1.5 text-sm text-cf-text-primary">
                    <span class="h-1.5 w-1.5 rounded-full bg-cf-success-fg" aria-hidden="true" />
                    {item}
                  </li>
                )}
              </For>
            </ul>
          </Show>
        </Show>
      </Card.Body>
    </Card>
  );
}
