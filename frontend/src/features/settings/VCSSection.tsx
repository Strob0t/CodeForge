import { createEffect, createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { CreateVCSAccountRequest, VCSAccount, VCSProvider } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { logError } from "~/lib/errorUtils";
import { Badge, Button, ConfirmDialog, FormField, Input, Section, Select } from "~/ui";

function providerBadgeVariant(provider: string) {
  switch (provider) {
    case "github":
      return "default" as const;
    case "gitlab":
      return "warning" as const;
    case "gitea":
    case "forgejo":
    case "codeberg":
      return "success" as const;
    default:
      return "info" as const;
  }
}

export default function VCSSection() {
  const { t } = useI18n();
  const { show: toast } = useToast();

  // -- Git providers (for the dropdown) --
  const [gitProviders] = createResource(() => api.providers.git().then((r) => r.providers));

  // -- VCS Accounts --
  const [vcsAccounts, { refetch: refetchVCS }] = createResource<VCSAccount[]>(() =>
    api.vcsAccounts.list(),
  );
  const [vcsProvider, setVcsProvider] = createSignal<VCSProvider>("github");
  const [vcsLabel, setVcsLabel] = createSignal("");
  const [vcsToken, setVcsToken] = createSignal("");
  const [vcsServerUrl, setVcsServerUrl] = createSignal("");

  // Sync default provider with first loaded git provider
  createEffect(() => {
    const providers = gitProviders();
    if (providers?.length && !vcsLabel().trim()) {
      setVcsProvider(providers[0] as VCSProvider);
    }
  });

  // Auto-fill server_url based on selected VCS provider
  createEffect(() => {
    const provider = vcsProvider();
    if (provider === "codeberg") {
      setVcsServerUrl("https://codeberg.org");
    } else if (provider === "forgejo") {
      setVcsServerUrl("");
    }
  });

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
    } catch (err) {
      logError("VCSSection.handleCreateVCS", err);
      toast("error", t("settings.vcs.createFailed"));
    }
  };

  const handleDeleteVCS = async (id: string) => {
    try {
      await api.vcsAccounts.delete(id);
      refetchVCS();
      toast("success", t("settings.vcs.deleted"));
    } catch (err) {
      logError("VCSSection.handleDeleteVCS", err);
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
    } catch (err) {
      logError("VCSSection.handleTestVCS", err);
      toast("error", t("settings.vcs.testFailed"));
    } finally {
      setTestingId(null);
    }
  };

  return (
    <>
      <Section id="settings-vcs" title={t("settings.vcs.title")} class="mb-8">
        {/* Add new account form */}
        <div class="mb-4 space-y-3 max-w-2xl">
          <div class="grid grid-cols-[180px_1fr] gap-3">
            <FormField label={t("settings.vcs.provider")} id="vcs-provider">
              <Select
                id="vcs-provider"
                value={vcsProvider()}
                onChange={(e) => setVcsProvider(e.currentTarget.value as VCSProvider)}
              >
                <For each={gitProviders() ?? []}>{(p) => <option value={p}>{p}</option>}</For>
                <option value="forgejo">{t("settings.vcs.providerForgejo")}</option>
                <option value="codeberg">{t("settings.vcs.providerCodeberg")}</option>
              </Select>
            </FormField>
            <FormField label={t("settings.vcs.label")} id="vcs-label">
              <Input
                id="vcs-label"
                type="text"
                value={vcsLabel()}
                onInput={(e) => setVcsLabel(e.currentTarget.value)}
                placeholder={t("settings.vcs.labelPlaceholder")}
              />
            </FormField>
          </div>
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <FormField label={t("settings.vcs.token")} id="vcs-token">
              <Input
                id="vcs-token"
                type="password"
                value={vcsToken()}
                onInput={(e) => setVcsToken(e.currentTarget.value)}
                placeholder={t("settings.vcs.tokenPlaceholder")}
              />
            </FormField>
            <FormField label={t("settings.vcs.serverUrl")} id="vcs-server-url">
              <Input
                id="vcs-server-url"
                type="text"
                value={vcsServerUrl()}
                onInput={(e) => setVcsServerUrl(e.currentTarget.value)}
                placeholder={t("settings.vcs.serverUrlPlaceholder")}
              />
            </FormField>
          </div>
          <div class="flex items-center gap-2">
            <Button onClick={handleCreateVCS} disabled={!vcsLabel().trim() || !vcsToken().trim()}>
              {t("settings.vcs.add")}
            </Button>
            <span class="text-xs text-cf-text-muted">{t("settings.vcs.orOAuth")}</span>
            <Button
              variant="secondary"
              onClick={() => {
                void (async () => {
                  try {
                    const { url } = await api.auth.githubOAuth();
                    window.location.href = url;
                  } catch (err) {
                    logError("VCSSection.githubOAuth", err);
                    toast("error", t("settings.vcs.oauthFailed"));
                  }
                })();
              }}
            >
              {t("settings.vcs.connectGitHub")}
            </Button>
          </div>
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
    </>
  );
}
