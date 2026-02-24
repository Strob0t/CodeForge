import { createResource, createSignal, For, onMount, Show } from "solid-js";

import { api } from "~/api/client";
import type {
  APIKeyInfo,
  CreateAPIKeyRequest,
  CreateVCSAccountRequest,
  User,
  VCSAccount,
  VCSProvider,
} from "~/api/types";
import { useAuth } from "~/components/AuthProvider";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";

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
      if (data.default_provider) setDefaultProvider(data.default_provider as string);
      if (data.default_autonomy) setDefaultAutonomy(data.default_autonomy as string);
      if (data.auto_clone !== undefined) setAutoClone(data.auto_clone as boolean);
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
    if (!confirm(t("settings.vcs.deleteConfirm"))) return;
    try {
      await api.vcsAccounts.delete(id);
      refetchVCS();
      toast("success", t("settings.vcs.deleted"));
    } catch {
      toast("error", t("settings.vcs.deleteFailed"));
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

  return (
    <div>
      <h2 class="mb-6 text-2xl font-bold">{t("settings.title")}</h2>

      {/* General Settings Section */}
      <section class="mb-8">
        <h3 class="mb-4 text-lg font-semibold">{t("settings.general.title")}</h3>
        <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
          <div class="space-y-4">
            {/* Default Provider */}
            <div>
              <label
                for="default-provider"
                class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("settings.general.defaultProvider")}
              </label>
              <input
                id="default-provider"
                type="text"
                value={defaultProvider()}
                onInput={(e) => setDefaultProvider(e.currentTarget.value)}
                placeholder="e.g. openai/gpt-4o"
                class="w-full max-w-md rounded border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-gray-200"
              />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {t("settings.general.defaultProviderHelp")}
              </p>
            </div>

            {/* Default Autonomy */}
            <div>
              <label
                for="default-autonomy"
                class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("settings.general.defaultAutonomy")}
              </label>
              <select
                id="default-autonomy"
                value={defaultAutonomy()}
                onChange={(e) => setDefaultAutonomy(e.currentTarget.value)}
                class="w-full max-w-md rounded border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-gray-200"
              >
                <For each={AUTONOMY_LEVELS}>
                  {(level) => <option value={level.value}>{level.label}</option>}
                </For>
              </select>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {t("settings.general.defaultAutonomyHelp")}
              </p>
            </div>

            {/* Auto Clone */}
            <div class="flex items-center gap-3">
              <input
                id="auto-clone"
                type="checkbox"
                checked={autoClone()}
                onChange={(e) => setAutoClone(e.currentTarget.checked)}
                class="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              <div>
                <label
                  for="auto-clone"
                  class="text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {t("settings.general.autoClone")}
                </label>
                <p class="text-xs text-gray-500 dark:text-gray-400">
                  {t("settings.general.autoCloneHelp")}
                </p>
              </div>
            </div>

            {/* Save Button */}
            <div class="pt-2">
              <button
                type="button"
                class="rounded bg-blue-600 px-4 py-2 text-sm text-white hover:bg-blue-700 disabled:opacity-50"
                onClick={handleSaveGeneral}
                disabled={saving()}
              >
                {t("settings.general.save")}
              </button>
            </div>
          </div>
        </div>
      </section>

      {/* VCS Accounts Section */}
      <section class="mb-8">
        <h3 class="mb-4 text-lg font-semibold">{t("settings.vcs.title")}</h3>
        <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
          {/* Add new account form */}
          <div class="mb-4 space-y-3">
            <div class="flex gap-2">
              <select
                value={vcsProvider()}
                onChange={(e) => setVcsProvider(e.currentTarget.value as VCSProvider)}
                class="rounded border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-gray-200"
                aria-label={t("settings.vcs.provider")}
              >
                <option value="github">GitHub</option>
                <option value="gitlab">GitLab</option>
                <option value="gitea">Gitea</option>
                <option value="bitbucket">Bitbucket</option>
              </select>
              <input
                type="text"
                value={vcsLabel()}
                onInput={(e) => setVcsLabel(e.currentTarget.value)}
                placeholder={t("settings.vcs.labelPlaceholder")}
                class="flex-1 rounded border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-gray-200"
                aria-label={t("settings.vcs.label")}
              />
            </div>
            <div class="flex gap-2">
              <input
                type="password"
                value={vcsToken()}
                onInput={(e) => setVcsToken(e.currentTarget.value)}
                placeholder={t("settings.vcs.tokenPlaceholder")}
                class="flex-1 rounded border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-gray-200"
                aria-label={t("settings.vcs.token")}
              />
              <input
                type="text"
                value={vcsServerUrl()}
                onInput={(e) => setVcsServerUrl(e.currentTarget.value)}
                placeholder={t("settings.vcs.serverUrlPlaceholder")}
                class="flex-1 rounded border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-gray-200"
                aria-label={t("settings.vcs.serverUrl")}
              />
            </div>
            <button
              type="button"
              class="rounded bg-blue-600 px-4 py-2 text-sm text-white hover:bg-blue-700 disabled:opacity-50"
              onClick={handleCreateVCS}
              disabled={!vcsLabel().trim() || !vcsToken().trim()}
            >
              {t("settings.vcs.add")}
            </button>
          </div>

          {/* Account list */}
          <Show
            when={(vcsAccounts() ?? []).length > 0}
            fallback={
              <p class="text-sm text-gray-500 dark:text-gray-400">{t("settings.vcs.empty")}</p>
            }
          >
            <ul class="divide-y divide-gray-200 dark:divide-gray-700">
              <For each={vcsAccounts() ?? []}>
                {(acct) => (
                  <li class="flex items-center justify-between py-3">
                    <div class="flex items-center gap-3">
                      <span
                        class={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${
                          acct.provider === "github"
                            ? "bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300"
                            : acct.provider === "gitlab"
                              ? "bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400"
                              : acct.provider === "gitea"
                                ? "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400"
                                : "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400"
                        }`}
                      >
                        {acct.provider}
                      </span>
                      <div>
                        <span class="text-sm font-medium">{acct.label}</span>
                        <Show when={acct.server_url}>
                          <span class="ml-2 text-xs text-gray-500 dark:text-gray-400">
                            {acct.server_url}
                          </span>
                        </Show>
                      </div>
                    </div>
                    <div class="flex items-center gap-2">
                      <span class="text-xs text-gray-400 dark:text-gray-500">
                        {new Date(acct.created_at).toLocaleDateString()}
                      </span>
                      <button
                        type="button"
                        class="rounded border border-gray-300 px-2 py-1 text-xs text-gray-600 hover:bg-gray-50 disabled:opacity-50 dark:border-gray-600 dark:text-gray-400 dark:hover:bg-gray-700"
                        onClick={() => handleTestVCS(acct.id)}
                        disabled={testingId() === acct.id}
                        aria-label={t("settings.vcs.testAria", { name: acct.label })}
                      >
                        {testingId() === acct.id
                          ? t("settings.vcs.testing")
                          : t("settings.vcs.test")}
                      </button>
                      <button
                        type="button"
                        class="text-xs text-red-600 hover:underline dark:text-red-400"
                        onClick={() => handleDeleteVCS(acct.id)}
                        aria-label={t("settings.vcs.deleteAria", { name: acct.label })}
                      >
                        {t("common.delete")}
                      </button>
                    </div>
                  </li>
                )}
              </For>
            </ul>
          </Show>
        </div>
      </section>

      {/* Providers Section */}
      <section class="mb-8">
        <h3 class="mb-4 text-lg font-semibold">{t("settings.providers.title")}</h3>
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
      </section>

      {/* LLM Health */}
      <section class="mb-8">
        <h3 class="mb-4 text-lg font-semibold">{t("settings.llm.title")}</h3>
        <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
          <Show
            when={!llmHealth.loading}
            fallback={
              <p class="text-sm text-gray-500 dark:text-gray-400">{t("settings.llm.checking")}</p>
            }
          >
            <Show
              when={llmHealth()}
              fallback={
                <p class="text-sm text-red-600 dark:text-red-400">
                  {t("settings.llm.unavailable")}
                </p>
              }
            >
              <p class="text-sm text-green-600 dark:text-green-400">
                {t("settings.llm.connected")}
              </p>
            </Show>
          </Show>
        </div>
      </section>

      {/* API Keys Section */}
      <section class="mb-8">
        <h3 class="mb-4 text-lg font-semibold">{t("settings.apiKey.title")}</h3>
        <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
          {/* Create new key */}
          <div class="mb-4 flex gap-2">
            <input
              type="text"
              value={newKeyName()}
              onInput={(e) => setNewKeyName(e.currentTarget.value)}
              placeholder={t("settings.apiKey.namePlaceholder")}
              class="flex-1 rounded border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-gray-200"
              aria-label={t("settings.apiKey.nameLabel")}
            />
            <button
              type="button"
              class="rounded bg-blue-600 px-4 py-2 text-sm text-white hover:bg-blue-700 disabled:opacity-50"
              onClick={handleCreateKey}
              disabled={!newKeyName().trim()}
            >
              {t("settings.apiKey.create")}
            </button>
          </div>

          {/* Show newly created key */}
          <Show when={createdKey()}>
            {(key) => (
              <div class="mb-4 rounded bg-green-50 p-3 text-sm dark:bg-green-900/20" role="alert">
                <p class="mb-1 font-medium text-green-800 dark:text-green-300">
                  {t("settings.apiKey.copyWarning")}
                </p>
                <code class="block break-all rounded bg-white p-2 font-mono text-xs dark:bg-gray-800">
                  {key()}
                </code>
                <button
                  type="button"
                  class="mt-2 text-xs text-green-600 hover:underline dark:text-green-400"
                  onClick={() => setCreatedKey(null)}
                >
                  {t("common.dismiss")}
                </button>
              </div>
            )}
          </Show>

          {/* Key list */}
          <Show
            when={(apiKeys() ?? []).length > 0}
            fallback={
              <p class="text-sm text-gray-500 dark:text-gray-400">{t("settings.apiKey.empty")}</p>
            }
          >
            <ul class="divide-y divide-gray-200 dark:divide-gray-700">
              <For each={apiKeys() ?? []}>
                {(key) => (
                  <li class="flex items-center justify-between py-2">
                    <div>
                      <span class="text-sm font-medium">{key.name}</span>
                      <span class="ml-2 font-mono text-xs text-gray-500 dark:text-gray-400">
                        {key.prefix}...
                      </span>
                    </div>
                    <button
                      type="button"
                      class="text-xs text-red-600 hover:underline dark:text-red-400"
                      onClick={() => handleDeleteKey(key.id)}
                      aria-label={t("settings.apiKey.deleteAria", { name: key.name })}
                    >
                      {t("common.delete")}
                    </button>
                  </li>
                )}
              </For>
            </ul>
          </Show>
        </div>
      </section>

      {/* User Management (admin only) */}
      <Show when={auth.user()?.role === "admin"}>
        <section class="mb-8">
          <h3 class="mb-4 text-lg font-semibold">{t("settings.users.title")}</h3>
          <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
            <Show
              when={(users() ?? []).length > 0}
              fallback={
                <p class="text-sm text-gray-500 dark:text-gray-400">{t("settings.users.empty")}</p>
              }
            >
              <table class="w-full text-left text-sm">
                <thead>
                  <tr class="border-b border-gray-200 dark:border-gray-700">
                    <th scope="col" class="pb-2 font-medium">
                      {t("settings.users.email")}
                    </th>
                    <th scope="col" class="pb-2 font-medium">
                      {t("settings.users.name")}
                    </th>
                    <th scope="col" class="pb-2 font-medium">
                      {t("settings.users.role")}
                    </th>
                    <th scope="col" class="pb-2 font-medium">
                      {t("common.status")}
                    </th>
                    <th scope="col" class="pb-2 font-medium">
                      {t("settings.users.actions")}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  <For each={users() ?? []}>
                    {(u) => (
                      <tr class="border-b border-gray-100 dark:border-gray-700/50">
                        <td class="py-2">{u.email}</td>
                        <td class="py-2">{u.name}</td>
                        <td class="py-2">
                          <span
                            class={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${
                              u.role === "admin"
                                ? "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400"
                                : u.role === "editor"
                                  ? "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400"
                                  : "bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-400"
                            }`}
                          >
                            {u.role}
                          </span>
                        </td>
                        <td class="py-2">
                          <button
                            type="button"
                            class={`text-xs ${
                              u.enabled
                                ? "text-green-600 dark:text-green-400"
                                : "text-gray-400 dark:text-gray-500"
                            }`}
                            onClick={() => handleToggleUser(u)}
                            aria-label={
                              u.enabled
                                ? t("settings.users.disableAria", { name: u.name })
                                : t("settings.users.enableAria", { name: u.name })
                            }
                          >
                            {u.enabled ? t("settings.users.enabled") : t("settings.users.disabled")}
                          </button>
                        </td>
                        <td class="py-2">
                          <button
                            type="button"
                            class="text-xs text-red-600 hover:underline dark:text-red-400"
                            onClick={() => handleDeleteUser(u.id)}
                            aria-label={t("settings.users.deleteAria", { name: u.name })}
                          >
                            {t("common.delete")}
                          </button>
                        </td>
                      </tr>
                    )}
                  </For>
                </tbody>
              </table>
            </Show>
          </div>
        </section>
      </Show>
    </div>
  );
}

function ProviderCard(props: { label: string; items: string[]; loading: boolean }) {
  const { t } = useI18n();
  return (
    <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <h4 class="mb-2 text-sm font-medium text-gray-500 dark:text-gray-400">{props.label}</h4>
      <Show
        when={!props.loading}
        fallback={
          <p class="text-xs text-gray-400 dark:text-gray-500">{t("settings.providers.loading")}</p>
        }
      >
        <Show
          when={props.items.length > 0}
          fallback={
            <p class="text-xs text-gray-400 dark:text-gray-500">{t("settings.providers.none")}</p>
          }
        >
          <ul class="space-y-1">
            <For each={props.items}>
              {(item) => (
                <li class="flex items-center gap-1.5 text-sm">
                  <span class="h-1.5 w-1.5 rounded-full bg-green-500" aria-hidden="true" />
                  {item}
                </li>
              )}
            </For>
          </ul>
        </Show>
      </Show>
    </div>
  );
}
