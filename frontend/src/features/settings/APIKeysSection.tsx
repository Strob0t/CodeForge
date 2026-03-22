import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { APIKeyInfo, CreateAPIKeyRequest } from "~/api/types";
import { useConfirm } from "~/components/ConfirmProvider";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { Alert, Button, Input, Section } from "~/ui";

export default function APIKeysSection() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const { confirm } = useConfirm();

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

  return (
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
  );
}
