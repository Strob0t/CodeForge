import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { APIKeyInfo, CreateAPIKeyRequest, User } from "~/api/types";
import { useAuth } from "~/components/AuthProvider";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";

export default function SettingsPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const auth = useAuth();

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

  return (
    <div>
      <h2 class="mb-6 text-2xl font-bold">{t("settings.title")}</h2>

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
