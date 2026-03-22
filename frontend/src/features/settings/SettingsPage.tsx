import { createResource, createSignal, For, onCleanup, onMount, Show } from "solid-js";

import { api, FetchError } from "~/api/client";
import type {
  APIKeyInfo,
  BenchmarkResult,
  CreateAPIKeyRequest,
  DeviceFlowResponse,
  SubscriptionProvider,
  User,
} from "~/api/types";
import { useAuth } from "~/components/AuthProvider";
import { useConfirm } from "~/components/ConfirmProvider";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import {
  Alert,
  Badge,
  Button,
  Card,
  FormField,
  Input,
  ModelCombobox,
  PageLayout,
  Section,
  Table,
  Textarea,
} from "~/ui";
import { SkeletonCard } from "~/ui/composites/SkeletonCard";
import type { TableColumn } from "~/ui/composites/Table";
import { cx } from "~/utils/cx";

import GeneralSection from "./GeneralSection";
import { SETTINGS_SECTIONS } from "./settingsTypes";
import { ShortcutsSection } from "./ShortcutsSection";
import VCSSection from "./VCSSection";

export default function SettingsPage() {
  onMount(() => {
    document.title = "Settings - CodeForge";
  });
  const { t } = useI18n();
  const { show: toast } = useToast();
  const { confirm } = useConfirm();
  const auth = useAuth();

  // -- Section navigation ------------------------------------------------------
  const sections = SETTINGS_SECTIONS;

  const [activeSection, setActiveSection] = createSignal("settings-general");

  onMount(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) setActiveSection(entry.target.id);
        }
      },
      { rootMargin: "-20% 0px -80% 0px" },
    );
    for (const s of sections) {
      const el = document.getElementById(s.id);
      if (el) observer.observe(el);
    }
    onCleanup(() => observer.disconnect());
  });

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
    const ok = await confirm({
      title: t("common.delete"),
      message: t("settings.apiKey.confirm.delete"),
      variant: "danger",
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
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
    const ok = await confirm({
      title: t("common.delete"),
      message: t("settings.users.confirm.delete"),
      variant: "danger",
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
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

  // -- Subscription Providers ---------------------------------------------------
  const [subProviders, { refetch: refetchSubProviders }] = createResource(() =>
    api.subscriptionProviders.list().then((r) => r.providers),
  );
  const [deviceFlow, setDeviceFlow] = createSignal<{
    provider: string;
    data: DeviceFlowResponse;
  } | null>(null);
  const [connectingProvider, setConnectingProvider] = createSignal<string | null>(null);
  const [pollTimer, setPollTimer] = createSignal<ReturnType<typeof setInterval> | null>(null);

  onCleanup(() => {
    const timer = pollTimer();
    if (timer) clearInterval(timer);
  });

  const handleConnectProvider = async (provider: SubscriptionProvider) => {
    setConnectingProvider(provider.name);
    try {
      const data = await api.subscriptionProviders.connect(provider.name);
      setDeviceFlow({ provider: provider.name, data });
      // Start polling for status
      const interval = Math.max(data.interval, 2) * 1000;
      const timer = setInterval(async () => {
        try {
          const result = await api.subscriptionProviders.status(provider.name);
          if (result.status === "complete") {
            clearInterval(timer);
            setPollTimer(null);
            setDeviceFlow(null);
            setConnectingProvider(null);
            refetchSubProviders();
            toast("success", t("settings.subscriptionProviders.connectSuccess"));
          } else if (result.status === "error") {
            clearInterval(timer);
            setPollTimer(null);
            setDeviceFlow(null);
            setConnectingProvider(null);
            toast("error", result.error ?? t("settings.subscriptionProviders.connectFailed"));
          }
        } catch {
          clearInterval(timer);
          setPollTimer(null);
          setDeviceFlow(null);
          setConnectingProvider(null);
          toast("error", t("settings.subscriptionProviders.connectFailed"));
        }
      }, interval);
      setPollTimer(timer);
    } catch {
      setConnectingProvider(null);
      toast("error", t("settings.subscriptionProviders.connectFailed"));
    }
  };

  const handleDisconnectProvider = async (provider: SubscriptionProvider) => {
    const ok = await confirm({
      title: t("settings.subscriptionProviders.disconnect"),
      message: t("settings.subscriptionProviders.confirmDisconnect"),
      variant: "danger",
      confirmLabel: t("settings.subscriptionProviders.disconnect"),
    });
    if (!ok) return;
    try {
      await api.subscriptionProviders.disconnect(provider.name);
      refetchSubProviders();
      toast("success", t("settings.subscriptionProviders.disconnectSuccess"));
    } catch {
      toast("error", t("settings.subscriptionProviders.disconnectFailed"));
    }
  };

  // -- Dev mode detection (from /health endpoint) ----------------------------
  const [devMode] = createResource(() => api.health.check().then((h) => h.dev_mode === true));

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
        <Button
          variant="ghost"
          size="xs"
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
        </Button>
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

  return (
    <PageLayout title={t("settings.title")}>
      <nav class="sticky top-0 z-10 bg-cf-bg-primary/95 backdrop-blur-sm border-b border-cf-border overflow-x-auto whitespace-nowrap flex gap-1 py-2 mb-4">
        <For each={sections}>
          {(s) => (
            <button
              class={cx(
                "px-3 py-1.5 text-xs font-medium rounded-cf-sm transition-colors shrink-0",
                activeSection() === s.id
                  ? "bg-cf-accent text-cf-accent-fg"
                  : "text-cf-text-secondary hover:bg-cf-bg-surface-alt",
              )}
              onClick={() => document.getElementById(s.id)?.scrollIntoView({ behavior: "smooth" })}
            >
              {s.label}
            </button>
          )}
        </For>
      </nav>

      {/* General Settings Section */}
      <GeneralSection />

      {/* Keyboard Shortcuts Section */}
      <Section id="settings-shortcuts" title={t("settings.shortcuts.title")} class="mb-8">
        <ShortcutsSection />
      </Section>

      {/* VCS Accounts Section */}
      <VCSSection />

      {/* Providers Section */}
      <Section id="settings-providers" title={t("settings.providers.title")} class="mb-8">
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
      <Section id="settings-proxy" title={t("settings.llm.title")} class="mb-8">
        <Show when={!llmHealth.loading} fallback={<SkeletonCard variant="stat" />}>
          <Show
            when={llmHealth()}
            fallback={<Alert variant="error">{t("settings.llm.unavailable")}</Alert>}
          >
            <Alert variant="success">{t("settings.llm.connected")}</Alert>
          </Show>
        </Show>
      </Section>

      {/* Subscription Providers Section */}
      <Section
        id="settings-subscriptions"
        title={t("settings.subscriptionProviders.title")}
        class="mb-8"
      >
        <p class="mb-4 text-sm text-cf-text-muted">
          {t("settings.subscriptionProviders.subtitle")}
        </p>
        <Show
          when={!subProviders.loading}
          fallback={<p class="text-sm text-cf-text-muted">{t("common.loading")}</p>}
        >
          <Show
            when={(subProviders() ?? []).length > 0}
            fallback={
              <p class="text-sm text-cf-text-muted">{t("settings.subscriptionProviders.empty")}</p>
            }
          >
            <div class="space-y-4">
              <For each={subProviders() ?? []}>
                {(provider) => (
                  <Card>
                    <Card.Body>
                      <div class="flex items-start justify-between">
                        <div class="flex-1">
                          <div class="flex items-center gap-2">
                            <h4 class="text-sm font-medium text-cf-text-primary">
                              {provider.display_name}
                            </h4>
                            <Badge variant={provider.connected ? "success" : "default"} pill>
                              {provider.connected
                                ? t("settings.subscriptionProviders.connected")
                                : t("settings.subscriptionProviders.disconnected")}
                            </Badge>
                          </div>
                          <p class="mt-1 text-xs text-cf-text-muted">{provider.description}</p>
                          <div class="mt-2">
                            <span class="text-xs font-medium text-cf-text-tertiary">
                              {t("settings.subscriptionProviders.models")}:
                            </span>
                            <div class="mt-1 flex flex-wrap gap-1">
                              <For each={provider.models}>
                                {(model) => (
                                  <Badge variant="info" pill>
                                    {model}
                                  </Badge>
                                )}
                              </For>
                            </div>
                          </div>
                        </div>
                        <div class="ml-4 flex-shrink-0">
                          <Show
                            when={provider.connected}
                            fallback={
                              <Button
                                variant="primary"
                                size="sm"
                                onClick={() => handleConnectProvider(provider)}
                                loading={connectingProvider() === provider.name}
                                disabled={connectingProvider() !== null}
                              >
                                {connectingProvider() === provider.name
                                  ? t("settings.subscriptionProviders.connecting")
                                  : t("settings.subscriptionProviders.connect")}
                              </Button>
                            }
                          >
                            <Button
                              variant="danger"
                              size="sm"
                              onClick={() => handleDisconnectProvider(provider)}
                            >
                              {t("settings.subscriptionProviders.disconnect")}
                            </Button>
                          </Show>
                        </div>
                      </div>

                      {/* Device flow UI */}
                      <Show
                        when={deviceFlow()?.provider === provider.name ? deviceFlow() : undefined}
                      >
                        {(flow) => (
                          <div class="mt-4 rounded-lg border border-cf-border bg-cf-bg-surface-alt p-4">
                            <p class="mb-2 text-sm font-medium text-cf-text-secondary">
                              {t("settings.subscriptionProviders.deviceCode")}:
                            </p>
                            <p class="mb-3 select-all text-center font-mono text-2xl font-bold tracking-widest text-cf-text-primary">
                              {flow().data.user_code}
                            </p>
                            <div class="flex items-center justify-center gap-2">
                              <a
                                href={flow().data.verification_uri}
                                target="_blank"
                                rel="noopener noreferrer"
                                class="text-sm font-medium text-cf-primary-fg underline hover:text-cf-primary-hover"
                              >
                                {t("settings.subscriptionProviders.openBrowser")}
                              </a>
                            </div>
                            <p class="mt-3 text-center text-xs text-cf-text-muted">
                              {t("settings.subscriptionProviders.waitingAuth")}
                            </p>
                          </div>
                        )}
                      </Show>
                    </Card.Body>
                  </Card>
                )}
              </For>
            </div>
          </Show>
        </Show>
      </Section>

      {/* API Keys Section */}
      <Section id="settings-apikeys" title={t("settings.apiKey.title")} class="mb-8">
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
        <Section id="settings-users" title={t("settings.users.title")} class="mb-8">
          <Table<User>
            columns={userColumns}
            data={users() ?? []}
            rowKey={(u) => u.id}
            emptyMessage={t("settings.users.empty")}
          />
        </Section>
      </Show>

      {/* Developer Tools Section (only visible in dev mode) */}
      <Show when={devMode()}>
        <Section id="settings-devtools" title={t("settings.devTools")} class="mb-8">
          <h4 class="mb-3 text-sm font-medium text-cf-text-secondary">
            {t("settings.benchmark.title")}
          </h4>
          <div class="space-y-3">
            <FormField label={t("settings.benchmark.model")} id="bench-model">
              <ModelCombobox
                id="bench-model"
                value={benchModel()}
                onInput={setBenchModel}
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
                <label
                  for="bench-temp"
                  class="mb-1 block text-sm font-medium text-cf-text-secondary"
                >
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
      </Show>
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
