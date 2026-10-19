import {
  batch,
  createEffect,
  createResource,
  createSignal,
  For,
  type JSX,
  on,
  onCleanup,
  Show,
} from "solid-js";

import { api } from "~/api/client";
import { FetchError } from "~/api/core";
import type {
  CreateWebhookRequest,
  WebhookEndpoint,
  WebhookKind,
  WebhookProvider,
  WebhookRegistered,
} from "~/api/types";
import { useAuth } from "~/components/AuthProvider";
import { useToast } from "~/components/Toast";
import {
  WEBHOOK_MIN_PROVIDED_SECRET_LENGTH,
  WEBHOOK_PROVIDERS,
  webhookProviderGeneratesSecret,
} from "~/config/domain-constants";
import { type TranslationKey, useI18n } from "~/i18n";
import { extractErrorMessage, logError } from "~/lib/errorUtils";
import {
  Alert,
  Badge,
  Button,
  ConfirmDialog,
  EmptyState,
  ErrorBanner,
  FormField,
  Input,
  LoadingState,
  Modal,
  Select,
} from "~/ui";

// ---------------------------------------------------------------------------
// Webhooks panel (KI-109): the inbound webhooks of a project (KI-85)
// ---------------------------------------------------------------------------

const KINDS: readonly WebhookKind[] = ["vcs", "pm"];

const KIND_LABELS: Record<WebhookKind, TranslationKey> = {
  vcs: "webhooks.kind.vcs",
  pm: "webhooks.kind.pm",
};

const KIND_HELP: Record<WebhookKind, TranslationKey> = {
  vcs: "webhooks.kindHelp.vcs",
  pm: "webhooks.kindHelp.pm",
};

const PROVIDER_LABELS: Record<WebhookProvider, string> = {
  github: "GitHub",
  gitlab: "GitLab",
  plane: "Plane",
};

/** Secret and token inputs: neither the browser nor a password manager may
 * offer to save them (not every manager honors autocomplete alone). */
const SECRET_INPUT = {
  type: "password",
  autocomplete: "new-password",
  "data-1p-ignore": "",
  "data-lpignore": "true",
} as const;

/** The toasts for an answer that arrives after the panel moved on. */
interface UnseenMessages {
  /** The project changed: the webhook belongs to the previous one. */
  elsewhere: TranslationKey;
  /** The panel closed. */
  closed: TranslationKey;
  /** Nothing was lost: a Plane secret is the admin's own. */
  plain: TranslationKey;
}

const CREATED_UNSEEN: UnseenMessages = {
  elsewhere: "webhooks.toast.createdElsewhere",
  closed: "webhooks.toast.createdClosed",
  plain: "webhooks.toast.created",
};

const ROTATED_UNSEEN: UnseenMessages = {
  elsewhere: "webhooks.toast.rotatedElsewhere",
  closed: "webhooks.toast.rotatedClosed",
  plain: "webhooks.toast.rotated",
};

function asKind(value: string): WebhookKind | undefined {
  return KINDS.find((k) => k === value);
}

function asProvider(kind: WebhookKind, value: string): WebhookProvider | undefined {
  const providers: readonly WebhookProvider[] = WEBHOOK_PROVIDERS[kind];
  return providers.find((p) => p === value);
}

/** The URL to enter at the provider. The API answers a path on its own
 * origin and has no public base URL setting; the frontend reaches the API on
 * this page's origin (/api/v1), so that origin is used. */
function inboundURL(path: string): string {
  return new URL(path, window.location.origin).href;
}

/** A secret shown once, after a registration or a rotation, on the project
 * it was issued for. It lives only in this panel's memory: never in browser
 * storage, never in the API cache. */
interface RevealedSecret {
  projectId: string;
  name: string;
  provider: WebhookProvider;
  url: string;
  secret: string;
}

export default function WebhooksPanel(props: { projectId: string }): JSX.Element {
  const { t, fmt } = useI18n();
  const { show: toast } = useToast();
  const { hasRole } = useAuth();
  // The Go Core lists a project's webhooks to admins and editors and lets only
  // admins change them (routes.go); the UI offers nobody more than that.
  const canView = (): boolean => hasRole("admin", "editor");
  const isAdmin = (): boolean => hasRole("admin");

  // The panel stays mounted when the project changes. Each change starts a
  // new generation: the answer to a request sent before it changes nothing
  // here, and neither does one that arrives after the panel closed.
  let generation = 0;
  let disposed = false;
  onCleanup(() => {
    disposed = true;
  });
  const isCurrent = (gen: number): boolean => !disposed && gen === generation;
  /** Whether the panel shows projectId: also after a switch away and back,
   * when the forms and dialogs are new ones but its secrets still belong. */
  const showsProject = (projectId: string): boolean => !disposed && projectId === props.projectId;

  const [webhooks, { refetch }] = createResource(
    () => (canView() ? props.projectId : false),
    async (projectId) => ({ projectId, items: await api.webhooks.list(projectId) }),
  );
  /** The webhooks of the project shown. They stay visible while they are
   * reloaded; another project's never show. */
  const listed = (): WebhookEndpoint[] | undefined => {
    if (webhooks.error || webhooks.state === "unresolved" || webhooks.state === "pending") {
      return undefined;
    }
    const latest = webhooks.latest;
    return latest?.projectId === props.projectId ? latest.items : undefined;
  };

  const [error, setError] = createSignal("");
  const [revealed, setRevealed] = createSignal<RevealedSecret | null>(null);
  const shownSecret = (): RevealedSecret | null => {
    const r = revealed();
    return r !== null && r.projectId === props.projectId ? r : null;
  };

  const nameOf = (w: { kind: WebhookKind; provider: WebhookProvider }): string =>
    t("webhooks.name", { provider: PROVIDER_LABELS[w.provider], kind: t(KIND_LABELS[w.kind]) });

  const secretTooShort = (): string =>
    t("webhooks.form.secretTooShort", { min: WEBHOOK_MIN_PROVIDED_SECRET_LENGTH });

  /** Shows a CodeForge-generated secret once. A secret Plane generated is
   * not echoed: the admin just pasted it. */
  function reveal(projectId: string, reg: WebhookRegistered): void {
    if (webhookProviderGeneratesSecret(reg.provider)) return;
    setRevealed({
      projectId,
      name: nameOf(reg),
      provider: reg.provider,
      url: inboundURL(reg.url),
      secret: reg.secret,
    });
  }

  /** Reports an answer that came after the panel moved on. A secret
   * CodeForge generated is lost then: only another rotation shows one. */
  function reportUnseen(reg: WebhookRegistered, messages: UnseenMessages): void {
    if (webhookProviderGeneratesSecret(reg.provider)) {
      toast("success", t(messages.plain));
      return;
    }
    toast("warning", t(disposed ? messages.closed : messages.elsewhere, { name: nameOf(reg) }), 0);
  }

  /** The message for a failed change. Without an answer (a network error)
   * the change may still have gone through. */
  function changeError(err: unknown, failed: TranslationKey, noAnswer: TranslationKey): string {
    if (err instanceof FetchError) return extractErrorMessage(err, t(failed));
    logError("webhooks.change", err);
    return t(noAnswer);
  }

  // -- Register --------------------------------------------------------------

  const [showForm, setShowForm] = createSignal(false);
  const [kind, setKind] = createSignal<WebhookKind>("vcs");
  const [provider, setProvider] = createSignal<WebhookProvider>("github");
  const [givenSecret, setGivenSecret] = createSignal("");
  const [apiToken, setAPIToken] = createSignal("");
  const [formError, setFormError] = createSignal("");
  const [saving, setSaving] = createSignal(false);

  function closeForm(): void {
    batch(() => {
      setShowForm(false);
      setKind("vcs");
      setProvider("github");
      setGivenSecret("");
      setAPIToken("");
      setFormError("");
    });
  }

  function chooseKind(next: WebhookKind): void {
    batch(() => {
      setKind(next);
      const p = asProvider(next, provider()) ?? WEBHOOK_PROVIDERS[next][0];
      setProvider(p);
      if (!webhookProviderGeneratesSecret(p)) setGivenSecret("");
      if (next !== "pm") setAPIToken("");
    });
  }

  function chooseProvider(next: WebhookProvider): void {
    setProvider(next);
    if (!webhookProviderGeneratesSecret(next)) setGivenSecret("");
  }

  function createError(err: unknown, req: CreateWebhookRequest): string {
    // The Go Core answers a second webhook of one kind and provider with 409
    // and a generic conflict message (writeDomainError).
    if (err instanceof FetchError && err.status === 409) {
      return t("webhooks.form.exists", { name: nameOf(req) });
    }
    return changeError(err, "webhooks.toast.createFailed", "webhooks.error.createNoAnswer");
  }

  async function handleCreate(): Promise<void> {
    if (saving()) return;
    setFormError("");
    const req: CreateWebhookRequest = { kind: kind(), provider: provider() };
    if (webhookProviderGeneratesSecret(provider())) {
      if (givenSecret().length < WEBHOOK_MIN_PROVIDED_SECRET_LENGTH) {
        setFormError(secretTooShort());
        return;
      }
      req.secret = givenSecret();
    }
    if (kind() === "pm" && apiToken() !== "") req.api_token = apiToken();
    const gen = generation;
    const projectId = props.projectId;
    setSaving(true);
    try {
      const reg = await api.webhooks.create(projectId, req);
      if (!showsProject(projectId)) {
        reportUnseen(reg, CREATED_UNSEEN);
        return;
      }
      if (isCurrent(gen)) closeForm();
      reveal(projectId, reg);
      toast(
        "success",
        webhookProviderGeneratesSecret(reg.provider)
          ? t("webhooks.toast.createdPlane")
          : t("webhooks.toast.created"),
      );
      void refetch();
    } catch (err) {
      const message = createError(err, req);
      if (isCurrent(gen)) setFormError(message);
      else toast("error", message);
    } finally {
      if (isCurrent(gen)) setSaving(false);
    }
  }

  // -- Rotate ----------------------------------------------------------------

  const [rotateTarget, setRotateTarget] = createSignal<WebhookEndpoint | null>(null);
  const [rotateSecret, setRotateSecret] = createSignal("");
  const [rotateError, setRotateError] = createSignal("");
  const [rotating, setRotating] = createSignal(false);
  const rotatesGivenSecret = (): boolean => {
    const w = rotateTarget();
    return w !== null && webhookProviderGeneratesSecret(w.provider);
  };

  /** Opens and closes the dialog with nothing typed or failed before. */
  function setRotateDialog(w: WebhookEndpoint | null): void {
    batch(() => {
      setRotateTarget(w);
      setRotateSecret("");
      setRotateError("");
    });
  }

  async function confirmRotate(): Promise<void> {
    const target = rotateTarget();
    if (!target || rotating()) return;
    const given = rotatesGivenSecret();
    if (given && rotateSecret().length < WEBHOOK_MIN_PROVIDED_SECRET_LENGTH) {
      setRotateError(secretTooShort());
      return;
    }
    const gen = generation;
    const projectId = props.projectId;
    setRotateError("");
    // The dialog cannot be cancelled while the rotation runs (busy): its
    // answer always finds it, unless the project changed or the panel closed.
    setRotating(true);
    try {
      const reg = await api.webhooks.rotate(
        projectId,
        target.id,
        given ? rotateSecret() : undefined,
      );
      if (!showsProject(projectId)) {
        reportUnseen(reg, ROTATED_UNSEEN);
        return;
      }
      if (isCurrent(gen)) setRotateDialog(null);
      reveal(projectId, reg);
      toast("success", t("webhooks.toast.rotated"));
      void refetch();
    } catch (err) {
      const message = changeError(
        err,
        "webhooks.toast.rotateFailed",
        "webhooks.error.rotateNoAnswer",
      );
      if (isCurrent(gen)) setRotateError(message);
      else toast("error", message);
    } finally {
      if (isCurrent(gen)) setRotating(false);
    }
  }

  // -- Delete ----------------------------------------------------------------

  const [deleteTarget, setDeleteTarget] = createSignal<WebhookEndpoint | null>(null);
  const [deleting, setDeleting] = createSignal(false);

  async function confirmDelete(): Promise<void> {
    const target = deleteTarget();
    if (!target || deleting()) return;
    const gen = generation;
    const projectId = props.projectId;
    setDeleting(true);
    try {
      await api.webhooks.delete(projectId, target.id);
      toast("success", t("webhooks.toast.deleted"));
      if (showsProject(projectId)) void refetch();
    } catch (err) {
      const message = extractErrorMessage(err, t("webhooks.toast.deleteFailed"));
      if (isCurrent(gen)) setError(message);
      else toast("error", message);
    } finally {
      if (isCurrent(gen)) {
        // Only the dialog of this delete closes.
        if (deleteTarget() === target) setDeleteTarget(null);
        setDeleting(false);
      }
    }
  }

  // -- API token (PM webhooks) -----------------------------------------------

  const [tokenTarget, setTokenTarget] = createSignal<WebhookEndpoint | null>(null);
  const [newToken, setNewToken] = createSignal("");
  const [tokenError, setTokenError] = createSignal("");
  const [savingToken, setSavingToken] = createSignal(false);

  /** Opens and closes the dialog with nothing typed or failed before. */
  function setTokenDialog(w: WebhookEndpoint | null): void {
    batch(() => {
      setTokenTarget(w);
      setNewToken("");
      setTokenError("");
    });
  }

  function cancelToken(): void {
    if (!savingToken()) setTokenDialog(null);
  }

  /** Sets the token, or removes it with "". */
  async function applyToken(token: string): Promise<void> {
    const target = tokenTarget();
    if (!target || savingToken()) return;
    const gen = generation;
    const projectId = props.projectId;
    setSavingToken(true);
    try {
      await api.webhooks.setAPIToken(projectId, target.id, token);
      toast(
        "success",
        token === "" ? t("webhooks.toast.apiTokenRemoved") : t("webhooks.toast.apiTokenSaved"),
      );
      if (isCurrent(gen)) setTokenDialog(null);
      if (showsProject(projectId)) void refetch();
    } catch (err) {
      const message = extractErrorMessage(err, t("webhooks.toast.apiTokenFailed"));
      if (isCurrent(gen)) setTokenError(message);
      else toast("error", message);
    } finally {
      if (isCurrent(gen)) setSavingToken(false);
    }
  }

  function saveToken(): void {
    if (newToken() === "") {
      setTokenError(t("webhooks.apiToken.required"));
      return;
    }
    void applyToken(newToken());
  }

  // -- Project switch --------------------------------------------------------

  // Nothing typed, opened or failed for one project carries over to the next.
  createEffect(
    on(
      () => props.projectId,
      () => {
        generation += 1;
        batch(() => {
          setRevealed(null);
          setError("");
          closeForm();
          setRotateDialog(null);
          setDeleteTarget(null);
          setTokenDialog(null);
          setSaving(false);
          setRotating(false);
          setDeleting(false);
          setSavingToken(false);
        });
      },
      { defer: true },
    ),
  );

  // -- View ------------------------------------------------------------------

  return (
    <div class="space-y-4">
      <div class="flex flex-wrap items-start justify-between gap-2">
        <div class="min-w-0">
          <h3 class="text-sm font-semibold text-cf-text-secondary">{t("webhooks.title")}</h3>
          <p class="text-xs text-cf-text-muted">{t("webhooks.description")}</p>
        </div>
        <Show when={isAdmin()}>
          <Button
            variant={showForm() ? "secondary" : "primary"}
            size="sm"
            onClick={() => (showForm() ? closeForm() : setShowForm(true))}
          >
            {showForm() ? t("common.cancel") : t("webhooks.add")}
          </Button>
        </Show>
      </div>

      <Show when={canView()} fallback={<Alert variant="info">{t("webhooks.viewerHidden")}</Alert>}>
        <Show when={!isAdmin()}>
          <Alert variant="info">{t("webhooks.adminOnly")}</Alert>
        </Show>

        <ErrorBanner error={error} onDismiss={() => setError("")} class="" />

        <Show when={shownSecret()}>
          {(r) => (
            // Not an Alert: role="alert" would make screen readers read the
            // secret aloud. The status line announces the box without it.
            <section
              data-testid="webhook-secret-box"
              aria-labelledby="webhook_secret_title"
              class="space-y-2 rounded-cf-md border border-cf-warning-border bg-cf-warning-bg p-3 text-sm text-cf-warning-fg"
            >
              <p id="webhook_secret_title" role="status" class="font-medium">
                {t("webhooks.secret.title")}: {r().name}
              </p>
              <p>{t("webhooks.secret.once", { provider: PROVIDER_LABELS[r().provider] })}</p>
              <CopyField
                id="webhook_revealed_url"
                label={t("webhooks.field.url")}
                value={r().url}
                copyLabel={t("webhooks.copyURL", { name: r().name })}
              />
              <CopyField
                id="webhook_revealed_secret"
                testId="webhook-secret"
                label={t("webhooks.secret.label")}
                value={r().secret}
                copyLabel={t("webhooks.secret.copy")}
              />
              <Button variant="secondary" size="xs" onClick={() => setRevealed(null)}>
                {t("webhooks.secret.done")}
              </Button>
            </section>
          )}
        </Show>

        <Show when={isAdmin() && showForm()}>
          <form
            aria-label={t("webhooks.form.title")}
            class="space-y-3 rounded-cf-md border border-cf-border bg-cf-bg-surface p-3"
            onSubmit={(e) => {
              e.preventDefault();
              void handleCreate();
            }}
          >
            <ErrorBanner error={formError} onDismiss={() => setFormError("")} class="" />
            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <FormField
                label={t("webhooks.form.kind")}
                id="webhook_kind"
                help={t(KIND_HELP[kind()])}
              >
                <Select
                  id="webhook_kind"
                  value={kind()}
                  onChange={(e) => {
                    const next = asKind(e.currentTarget.value);
                    if (next) chooseKind(next);
                  }}
                >
                  <For each={KINDS}>{(k) => <option value={k}>{t(KIND_LABELS[k])}</option>}</For>
                </Select>
              </FormField>
              <FormField label={t("webhooks.form.provider")} id="webhook_provider">
                <Select
                  id="webhook_provider"
                  value={provider()}
                  onChange={(e) => {
                    const next = asProvider(kind(), e.currentTarget.value);
                    if (next) chooseProvider(next);
                  }}
                >
                  <For each={WEBHOOK_PROVIDERS[kind()]}>
                    {(p) => <option value={p}>{PROVIDER_LABELS[p]}</option>}
                  </For>
                </Select>
              </FormField>
            </div>
            <Show when={webhookProviderGeneratesSecret(provider())}>
              <FormField
                label={t("webhooks.form.planeSecret")}
                id="webhook_given_secret"
                required
                help={t("webhooks.form.planeSecretHelp")}
              >
                <Input
                  id="webhook_given_secret"
                  {...SECRET_INPUT}
                  mono
                  value={givenSecret()}
                  onInput={(e) => setGivenSecret(e.currentTarget.value)}
                />
              </FormField>
            </Show>
            <Show when={kind() === "pm"}>
              <FormField
                label={t("webhooks.form.apiToken")}
                id="webhook_api_token"
                help={t("webhooks.form.apiTokenHelp")}
              >
                <Input
                  id="webhook_api_token"
                  {...SECRET_INPUT}
                  mono
                  value={apiToken()}
                  onInput={(e) => setAPIToken(e.currentTarget.value)}
                />
              </FormField>
            </Show>
            <div class="flex justify-end">
              <Button type="submit" size="sm" loading={saving()}>
                {t("webhooks.form.submit")}
              </Button>
            </div>
          </form>
        </Show>

        <Show when={listed() === undefined && webhooks.loading}>
          <LoadingState message={t("webhooks.loading")} />
        </Show>

        <Show when={webhooks.error && !webhooks.loading}>
          <Alert variant="error">
            <div class="flex flex-1 flex-wrap items-center justify-between gap-2">
              <span>{t("webhooks.loadError")}</span>
              <Button variant="secondary" size="xs" onClick={() => void refetch()}>
                {t("webhooks.retry")}
              </Button>
            </div>
          </Alert>
        </Show>

        <Show when={listed()}>
          {(items) => (
            <Show
              when={items().length > 0}
              fallback={
                <EmptyState
                  title={t("webhooks.empty")}
                  description={
                    isAdmin()
                      ? t("webhooks.emptyDescription")
                      : t("webhooks.emptyDescriptionReadOnly")
                  }
                />
              }
            >
              <ul class="space-y-3">
                <For each={items()}>
                  {(w) => (
                    <li
                      data-testid={`webhook-${w.id}`}
                      class="space-y-2 rounded-cf-md border border-cf-border bg-cf-bg-surface p-3"
                    >
                      <div class="flex flex-wrap items-center gap-2">
                        <Badge variant={w.kind === "vcs" ? "info" : "primary"}>
                          {t(KIND_LABELS[w.kind])}
                        </Badge>
                        <span class="text-sm font-medium text-cf-text-primary">
                          {PROVIDER_LABELS[w.provider]}
                        </span>
                        <Show when={isAdmin()}>
                          <div class="ml-auto flex flex-wrap gap-1">
                            <Show when={w.kind === "pm"}>
                              <Button
                                variant="ghost"
                                size="xs"
                                aria-label={t("webhooks.apiTokenLabel", { name: nameOf(w) })}
                                onClick={() => setTokenDialog(w)}
                              >
                                {t("webhooks.apiToken.edit")}
                              </Button>
                            </Show>
                            <Button
                              variant="ghost"
                              size="xs"
                              aria-label={t("webhooks.rotateLabel", { name: nameOf(w) })}
                              onClick={() => setRotateDialog(w)}
                            >
                              {t("webhooks.rotate")}
                            </Button>
                            <Button
                              variant="ghost"
                              size="xs"
                              class="text-cf-danger-fg"
                              aria-label={t("webhooks.deleteLabel", { name: nameOf(w) })}
                              onClick={() => setDeleteTarget(w)}
                            >
                              {t("webhooks.delete")}
                            </Button>
                          </div>
                        </Show>
                      </div>
                      <CopyField
                        id={`webhook_url_${w.id}`}
                        label={t("webhooks.field.url")}
                        value={inboundURL(w.url)}
                        copyLabel={t("webhooks.copyURL", { name: nameOf(w) })}
                      />
                      <dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-xs">
                        <dt class="text-cf-text-muted">{t("webhooks.field.created")}</dt>
                        <dd class="text-cf-text-secondary">{fmt.dateTime(w.created_at)}</dd>
                        <dt class="text-cf-text-muted">{t("webhooks.field.secretSince")}</dt>
                        <dd class="text-cf-text-secondary">{fmt.dateTime(w.secret_rotated_at)}</dd>
                        <Show when={w.kind === "pm"}>
                          <dt class="text-cf-text-muted">{t("webhooks.field.apiToken")}</dt>
                          <dd class="text-cf-text-secondary">
                            {w.has_api_token
                              ? t("webhooks.apiToken.own")
                              : t("webhooks.apiToken.none")}
                          </dd>
                        </Show>
                      </dl>
                    </li>
                  )}
                </For>
              </ul>
              <p class="text-xs text-cf-text-muted">
                {t("webhooks.urlHint", { origin: window.location.origin })}
              </p>
            </Show>
          )}
        </Show>
      </Show>

      <ConfirmDialog
        open={rotateTarget() !== null}
        title={t("webhooks.rotate.title")}
        message={
          <div class="space-y-3">
            <p>
              {rotatesGivenSecret()
                ? t("webhooks.rotate.planeMessage")
                : t("webhooks.rotate.message", {
                    provider: PROVIDER_LABELS[rotateTarget()?.provider ?? "github"],
                  })}
            </p>
            <Show when={rotatesGivenSecret()}>
              <FormField
                label={t("webhooks.rotate.planeSecret")}
                id="webhook_rotate_secret"
                required
              >
                <Input
                  id="webhook_rotate_secret"
                  {...SECRET_INPUT}
                  mono
                  value={rotateSecret()}
                  onInput={(e) => setRotateSecret(e.currentTarget.value)}
                />
              </FormField>
            </Show>
            <Show when={rotateError()}>
              <p class="text-xs text-cf-danger-fg" role="alert">
                {rotateError()}
              </p>
            </Show>
          </div>
        }
        variant="danger"
        busy={rotating()}
        confirmLabel={t("webhooks.rotate.confirm")}
        cancelLabel={t("common.cancel")}
        onConfirm={() => void confirmRotate()}
        onCancel={() => setRotateDialog(null)}
      />

      <ConfirmDialog
        open={deleteTarget() !== null}
        title={t("webhooks.delete.title")}
        message={t("webhooks.delete.message", {
          provider: PROVIDER_LABELS[deleteTarget()?.provider ?? "github"],
        })}
        variant="danger"
        busy={deleting()}
        confirmLabel={t("webhooks.delete")}
        cancelLabel={t("common.cancel")}
        onConfirm={() => void confirmDelete()}
        onCancel={() => setDeleteTarget(null)}
      />

      <Modal
        open={tokenTarget() !== null}
        onClose={cancelToken}
        title={t("webhooks.apiToken.title", {
          name: nameOf(tokenTarget() ?? { kind: "pm", provider: "github" }),
        })}
      >
        <form
          class="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            saveToken();
          }}
        >
          <FormField
            label={t("webhooks.apiToken.new")}
            id="webhook_new_api_token"
            help={t("webhooks.apiToken.help")}
            error={tokenError() || undefined}
          >
            <Input
              id="webhook_new_api_token"
              {...SECRET_INPUT}
              mono
              value={newToken()}
              onInput={(e) => setNewToken(e.currentTarget.value)}
            />
          </FormField>
          <div class="flex flex-wrap justify-end gap-2">
            <Show when={tokenTarget()?.has_api_token}>
              <Button
                type="button"
                variant="danger"
                disabled={savingToken()}
                onClick={() => void applyToken("")}
              >
                {t("webhooks.apiToken.remove")}
              </Button>
            </Show>
            <Button
              type="button"
              variant="secondary"
              disabled={savingToken()}
              onClick={cancelToken}
            >
              {t("common.cancel")}
            </Button>
            <Button type="submit" loading={savingToken()}>
              {t("webhooks.apiToken.save")}
            </Button>
          </div>
        </form>
      </Modal>
    </div>
  );
}

/** A read-only value with a copy button (a webhook's URL or its secret). */
function CopyField(props: {
  id: string;
  label: string;
  value: string;
  copyLabel: string;
  testId?: string;
}): JSX.Element {
  const { t } = useI18n();
  const { show: toast } = useToast();

  async function copy(): Promise<void> {
    try {
      await navigator.clipboard.writeText(props.value);
      toast("success", t("webhooks.copied"));
    } catch (err) {
      // No clipboard (insecure origin, denied permission): the field stays
      // selectable for copying by hand.
      toast("error", t("webhooks.copyFailed"));
      logError("webhooks.copy", err);
    }
  }

  return (
    <div class="min-w-0">
      <label for={props.id} class="text-xs text-cf-text-muted">
        {props.label}
      </label>
      <div class="mt-0.5 flex items-center gap-2">
        <Input
          id={props.id}
          data-testid={props.testId}
          readOnly
          mono
          autocomplete="off"
          class="text-xs"
          value={props.value}
          onFocus={(e) => e.currentTarget.select()}
        />
        <Button
          variant="secondary"
          size="xs"
          aria-label={props.copyLabel}
          onClick={() => void copy()}
        >
          {t("webhooks.copy")}
        </Button>
      </div>
    </div>
  );
}
