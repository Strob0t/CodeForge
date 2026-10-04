import { createResource, createSignal, For, type JSX, Show } from "solid-js";

import { api } from "~/api/client";
import type { ConsentPurpose, ConsentStatus } from "~/api/types";
import { useAuth } from "~/components/AuthProvider";
import { useToast } from "~/components/Toast";
import { type TranslationKey, useI18n } from "~/i18n";
import { extractErrorMessage } from "~/lib/errorUtils";
import { Alert, Badge, Button, Checkbox, FormField, Input, Modal, Section } from "~/ui";

/**
 * Settings > Privacy: the signed-in user's GDPR self-service (export, Art. 15
 * and 20; erasure, Art. 17; consent, Art. 7). The privacy policy links here
 * (/settings?section=privacy).
 */
export default function PrivacySection(): JSX.Element {
  const { t } = useI18n();
  return (
    <Section
      id="settings-privacy"
      title={t("settings.privacy.title")}
      description={t("settings.privacy.description")}
      action={
        <a href="/privacy" class="text-sm text-cf-accent hover:underline">
          {t("settings.privacy.policyLink")}
        </a>
      }
      class="mb-8"
    >
      <div class="divide-y divide-cf-border">
        <ExportData />
        <DeleteAccount />
        <ConsentSettings />
      </div>
    </Section>
  );
}

function Block(props: {
  title: TranslationKey;
  description: TranslationKey;
  children: JSX.Element;
}): JSX.Element {
  const { t } = useI18n();
  return (
    <div class="py-4 first:pt-0 last:pb-0">
      <h3 class="text-sm font-semibold text-cf-text-primary">{t(props.title)}</h3>
      <p class="mb-3 mt-0.5 text-sm text-cf-text-muted">{t(props.description)}</p>
      {props.children}
    </div>
  );
}

/** Saves data as a JSON file in the browser's downloads. */
function saveJSON(filename: string, data: unknown): void {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
  const href = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = href;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  // Revoked after the click has handed the file to the download.
  setTimeout(() => URL.revokeObjectURL(href), 0);
}

function ExportData(): JSX.Element {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const { user } = useAuth();
  const [exporting, setExporting] = createSignal(false);

  const handleExport = async (): Promise<void> => {
    setExporting(true);
    try {
      const data = await api.privacy.exportMyData();
      // The same name the Go Core gives the file (Content-Disposition).
      saveJSON(`codeforge-export-${user()?.id ?? data.user.id}.json`, data);
      toast("success", t("settings.privacy.export.done"));
    } catch (err) {
      toast("error", extractErrorMessage(err, t("settings.privacy.export.failed")));
    } finally {
      setExporting(false);
    }
  };

  return (
    <Block title="settings.privacy.export.title" description="settings.privacy.export.description">
      <Button
        variant="secondary"
        size="sm"
        onClick={() => void handleExport()}
        loading={exporting()}
        disabled={exporting()}
      >
        {t("settings.privacy.export.button")}
      </Button>
    </Block>
  );
}

const DELETED: TranslationKey[] = [
  "settings.privacy.delete.deleted.account",
  "settings.privacy.delete.deleted.keys",
  "settings.privacy.delete.deleted.sessions",
  "settings.privacy.delete.deleted.channels",
];

const KEPT: TranslationKey[] = [
  "settings.privacy.delete.kept.audit",
  "settings.privacy.delete.kept.consent",
  "settings.privacy.delete.kept.channels",
  "settings.privacy.delete.kept.quarantine",
];

function DeleteAccount(): JSX.Element {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const { user, logout } = useAuth();
  const [open, setOpen] = createSignal(false);
  const [typed, setTyped] = createSignal("");
  const [deleting, setDeleting] = createSignal(false);
  const [error, setError] = createSignal("");

  const email = (): string => user()?.email ?? "";
  // A strong confirmation: the user types their own email address.
  const confirmed = (): boolean =>
    email() !== "" && typed().trim().toLowerCase() === email().toLowerCase();

  const close = (): void => {
    if (deleting()) return;
    setOpen(false);
    setTyped("");
    setError("");
  };

  const handleDelete = async (): Promise<void> => {
    if (!confirmed() || deleting()) return;
    setDeleting(true);
    setError("");
    try {
      await api.privacy.deleteMyData();
    } catch (err) {
      setError(extractErrorMessage(err, t("settings.privacy.delete.failed")));
      setDeleting(false);
      return;
    }
    setDeleting(false);
    close();
    toast("success", t("settings.privacy.delete.done"));
    try {
      await logout();
    } catch {
      // The account is gone, so the server may refuse to end its session;
      // logout has cleared the local session and gone to the login page anyway.
    }
  };

  return (
    <Block title="settings.privacy.delete.title" description="settings.privacy.delete.description">
      <Button variant="danger" size="sm" onClick={() => setOpen(true)}>
        {t("settings.privacy.delete.button")}
      </Button>

      <Modal open={open()} onClose={close} title={t("settings.privacy.delete.dialogTitle")}>
        <div class="space-y-3 text-sm text-cf-text-secondary">
          <Alert variant="warning">{t("settings.privacy.delete.irreversible")}</Alert>
          <div>
            <h4 class="font-medium text-cf-text-primary">
              {t("settings.privacy.delete.deletedTitle")}
            </h4>
            <ul class="mt-1 list-inside list-disc space-y-0.5">
              <For each={DELETED}>{(key) => <li>{t(key)}</li>}</For>
            </ul>
          </div>
          <div>
            <h4 class="font-medium text-cf-text-primary">
              {t("settings.privacy.delete.keptTitle")}
            </h4>
            <ul class="mt-1 list-inside list-disc space-y-0.5">
              <For each={KEPT}>{(key) => <li>{t(key)}</li>}</For>
            </ul>
          </div>
          <p>{t("settings.privacy.delete.notDeleted")}</p>
          <p class="text-xs text-cf-text-muted">{t("settings.privacy.delete.backups")}</p>
          <FormField
            id="privacy-delete-confirm"
            label={t("settings.privacy.delete.confirmLabel", { email: email() })}
          >
            <Input
              id="privacy-delete-confirm"
              type="text"
              autocomplete="off"
              spellcheck={false}
              value={typed()}
              onInput={(e) => setTyped(e.currentTarget.value)}
            />
          </FormField>
          <Show when={error()}>
            <Alert variant="error">{error()}</Alert>
          </Show>
        </div>
        <div class="mt-4 flex justify-end gap-2">
          <Button variant="secondary" onClick={close} disabled={deleting()}>
            {t("common.cancel")}
          </Button>
          <Button
            variant="danger"
            onClick={() => void handleDelete()}
            disabled={!confirmed() || deleting()}
            loading={deleting()}
          >
            {t("settings.privacy.delete.confirm")}
          </Button>
        </div>
      </Modal>
    </Block>
  );
}

function withConsent(list: ConsentStatus[], purposeId: string, granted: boolean): ConsentStatus[] {
  return [...list.filter((s) => s.purpose_id !== purposeId), { purpose_id: purposeId, granted }];
}

function ConsentSettings(): JSX.Element {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [purposes] = createResource(() => api.privacy.consentPurposes());
  const [status, { mutate }] = createResource(() => api.privacy.consentStatus());
  const [saving, setSaving] = createSignal<string | null>(null);

  const failed = (): boolean => purposes.state === "errored" || status.state === "errored";
  const ready = (): boolean => purposes.state === "ready" && status.state === "ready";
  const granted = (purposeId: string): boolean =>
    (status() ?? []).some((s) => s.purpose_id === purposeId && s.granted);

  // Shown at once; the stored state comes back when saving fails.
  const changeConsent = async (purpose: ConsentPurpose, value: boolean): Promise<void> => {
    const before = status() ?? [];
    mutate(withConsent(before, purpose.id, value));
    setSaving(purpose.id);
    try {
      await api.privacy.setConsent(purpose.id, value);
      toast("success", t("settings.privacy.consent.saved"));
    } catch (err) {
      mutate(before);
      toast("error", extractErrorMessage(err, t("settings.privacy.consent.failed")));
    } finally {
      setSaving(null);
    }
  };

  return (
    <Block
      title="settings.privacy.consent.title"
      description="settings.privacy.consent.description"
    >
      <Show when={failed()}>
        <Alert variant="error">{t("settings.privacy.consent.loadFailed")}</Alert>
      </Show>
      <Show when={!failed() && ready()}>
        <Show
          when={(purposes() ?? []).length > 0}
          fallback={<p class="text-sm text-cf-text-muted">{t("settings.privacy.consent.empty")}</p>}
        >
          <ul class="divide-y divide-cf-border-subtle">
            <For each={purposes() ?? []}>
              {(purpose) => (
                <li class="flex items-start justify-between gap-4 py-2">
                  <div>
                    <div class="flex flex-wrap items-center gap-2">
                      <span class="text-sm font-medium text-cf-text-primary">{purpose.label}</span>
                      <Badge variant="info" pill>
                        {t(`settings.privacy.consent.basis.${purpose.legal_basis}`)}
                      </Badge>
                      <Show when={purpose.required}>
                        <Badge variant="warning" pill>
                          {t("settings.privacy.consent.required")}
                        </Badge>
                      </Show>
                    </div>
                    <p class="mt-0.5 text-xs text-cf-text-muted">{purpose.description}</p>
                    <Show when={purpose.required && granted(purpose.id)}>
                      <p class="mt-0.5 text-xs text-cf-text-muted">
                        {t("settings.privacy.consent.requiredHint")}
                      </p>
                    </Show>
                  </div>
                  <Checkbox
                    label={t("settings.privacy.consent.toggle")}
                    aria-label={t("settings.privacy.consent.toggleAria", { label: purpose.label })}
                    checked={granted(purpose.id)}
                    disabled={saving() === purpose.id || (purpose.required && granted(purpose.id))}
                    onChange={(value) => void changeConsent(purpose, value)}
                  />
                </li>
              )}
            </For>
          </ul>
        </Show>
      </Show>
    </Block>
  );
}
