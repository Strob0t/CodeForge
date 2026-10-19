import { createSignal, For, type JSX, Show } from "solid-js";

import { api } from "~/api/client";
import type { ProviderInfo, RoadmapSyncDirection, RoadmapSyncResult } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { extractErrorMessage } from "~/lib/errorUtils";
import { Alert, Button, Checkbox, FormField, Input, Select } from "~/ui";

const DIRECTIONS: RoadmapSyncDirection[] = ["pull", "push", "bidi"];

/** Whether the provider can write to the PM tool (push, and the push half of both directions). */
function canWrite(provider: ProviderInfo | undefined): boolean {
  return provider?.capabilities.create_item === true || provider?.capabilities.update_item === true;
}

/** The provider's credential key: Plane reads api_token, the others token (internal/adapter/*). */
function credentials(provider: string, token: string): Record<string, string> | undefined {
  if (!token) return undefined;
  return provider === "plane" ? { api_token: token } : { token };
}

/**
 * Syncs the roadmap with a PM tool (POST /projects/{id}/roadmap/sync): the user
 * picks the direction, what may change, and can preview the result first.
 */
export default function RoadmapSyncForm(props: {
  projectId: string;
  providers: ProviderInfo[];
  /** Called after a sync that changed something (not a preview). */
  onSynced: (result: RoadmapSyncResult) => void;
  onCancel: () => void;
}): JSX.Element {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [provider, setProvider] = createSignal("");
  const [projectRef, setProjectRef] = createSignal("");
  const [direction, setDirection] = createSignal<RoadmapSyncDirection>("pull");
  const [createNew, setCreateNew] = createSignal(true);
  const [updateExisting, setUpdateExisting] = createSignal(true);
  // A preview first: a push creates and changes items in the PM tool.
  const [dryRun, setDryRun] = createSignal(true);
  const [token, setToken] = createSignal("");
  const [syncing, setSyncing] = createSignal(false);
  const [result, setResult] = createSignal<RoadmapSyncResult | null>(null);

  const selected = (): ProviderInfo | undefined =>
    props.providers.find((p) => p.name === provider());
  const writable = (): boolean => canWrite(selected());
  const ready = (): boolean => provider() !== "" && projectRef().trim() !== "" && !syncing();

  function chooseProvider(name: string): void {
    // A token belongs to the provider it was typed for.
    if (name !== provider()) setToken("");
    setProvider(name);
    setResult(null);
    if (!canWrite(props.providers.find((p) => p.name === name))) setDirection("pull");
  }

  async function handleSync(): Promise<void> {
    if (!ready()) return;
    setSyncing(true);
    setResult(null);
    try {
      const res = await api.roadmap.sync(props.projectId, {
        provider: provider(),
        project_ref: projectRef().trim(),
        direction: direction(),
        dry_run: dryRun(),
        create_new: createNew(),
        update_exist: updateExisting(),
        provider_config: credentials(provider(), token()),
      });
      setResult(res);
      if (!res.dry_run) {
        // Kept for applying a preview, forgotten once the sync ran.
        setToken("");
        toast("success", t("roadmap.sync.done"));
        props.onSynced(res);
      }
    } catch (err) {
      toast("error", extractErrorMessage(err, t("roadmap.sync.failed")));
    } finally {
      setSyncing(false);
    }
  }

  return (
    <div class="mb-4 rounded-cf-sm border border-cf-border bg-cf-bg-surface-alt p-3">
      <div class="mb-2 text-xs font-medium text-cf-text-secondary">{t("roadmap.sync.title")}</div>
      <div class="flex flex-col gap-2">
        <Select
          value={provider()}
          onChange={(e) => chooseProvider(e.currentTarget.value)}
          aria-label={t("roadmap.pmProviderLabel")}
        >
          <option value="">{t("roadmap.selectProvider")}</option>
          <For each={props.providers}>{(p) => <option value={p.name}>{p.name}</option>}</For>
        </Select>
        <Input
          type="text"
          placeholder={t("roadmap.projectRefPlaceholder")}
          value={projectRef()}
          onInput={(e) => setProjectRef(e.currentTarget.value)}
          aria-label={t("roadmap.pmProjectRefLabel")}
        />

        <fieldset class="flex flex-col gap-1">
          <legend class="mb-1 text-xs font-medium text-cf-text-secondary">
            {t("roadmap.sync.direction")}
          </legend>
          <For each={DIRECTIONS}>
            {(d) => (
              <label class="flex items-center gap-2 text-sm text-cf-text-primary">
                <input
                  type="radio"
                  name={`roadmap-sync-direction-${props.projectId}`}
                  value={d}
                  checked={direction() === d}
                  disabled={d !== "pull" && provider() !== "" && !writable()}
                  onChange={() => setDirection(d)}
                  class="h-4 w-4 text-cf-accent focus-visible:ring-2 focus-visible:ring-cf-focus-ring"
                />
                {t(`roadmap.sync.direction.${d}`)}
              </label>
            )}
          </For>
          <Show when={provider() !== "" && !writable()}>
            <p class="text-xs text-cf-text-muted">
              {t("roadmap.sync.readOnlyProvider", { provider: provider() })}
            </p>
          </Show>
        </fieldset>

        <div class="flex flex-col gap-1">
          <Checkbox
            label={t("roadmap.sync.createNew")}
            checked={createNew()}
            onChange={setCreateNew}
          />
          <Checkbox
            label={t("roadmap.sync.updateExisting")}
            checked={updateExisting()}
            onChange={setUpdateExisting}
          />
          <Checkbox label={t("roadmap.sync.dryRun")} checked={dryRun()} onChange={setDryRun} />
        </div>

        <FormField
          id={`roadmap-sync-token-${props.projectId}`}
          label={t("roadmap.sync.token")}
          help={t("roadmap.sync.tokenHint")}
        >
          <Input
            id={`roadmap-sync-token-${props.projectId}`}
            type="password"
            autocomplete="off"
            value={token()}
            onInput={(e) => setToken(e.currentTarget.value)}
          />
        </FormField>

        <div class="flex gap-2">
          <Button
            variant="primary"
            size="sm"
            onClick={() => void handleSync()}
            disabled={!ready()}
            loading={syncing()}
          >
            {dryRun() ? t("roadmap.sync.preview") : t("roadmap.sync.run")}
          </Button>
          <Button variant="ghost" size="sm" onClick={() => props.onCancel()}>
            {t("common.cancel")}
          </Button>
        </div>

        <Show when={result()}>
          {(res) => (
            <Alert variant={(res().errors ?? []).length > 0 ? "warning" : "success"}>
              <p>
                {t("roadmap.sync.result", {
                  direction: res().direction,
                  created: res().created,
                  updated: res().updated,
                  skipped: res().skipped,
                })}
              </p>
              <Show when={res().dry_run}>
                <p>{t("roadmap.sync.previewNote")}</p>
              </Show>
              <For each={res().errors ?? []}>{(err) => <p class="text-xs">{err}</p>}</For>
            </Alert>
          )}
        </Show>
      </div>
    </div>
  );
}
