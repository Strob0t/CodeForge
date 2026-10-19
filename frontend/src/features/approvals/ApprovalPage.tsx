import { useParams } from "@solidjs/router";
import { createResource, createSignal, type JSX, onMount, Show } from "solid-js";

import { api } from "~/api/client";
import { useI18n } from "~/i18n";
import { extractErrorMessage } from "~/lib/errorUtils";
import { Button, Card, ErrorBanner, LoadingState, PageLayout } from "~/ui";

/**
 * Approval page for one tool call awaiting a decision. Approval emails link
 * here (KI-57): the signed-in user sees what the agent asks and decides with
 * an authenticated POST - the link itself changes nothing.
 */
export default function ApprovalPage(): JSX.Element {
  const { t } = useI18n();
  const params = useParams<{ runId: string; callId: string }>();
  onMount(() => {
    document.title = `${t("approval.title")} - CodeForge`;
  });

  const [request] = createResource(
    () => ({ runId: params.runId, callId: params.callId }),
    (p) => api.runs.pendingApproval(p.runId, p.callId),
  );
  const [decided, setDecided] = createSignal<"allow" | "deny" | null>(null);
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal("");

  async function decide(decision: "allow" | "deny"): Promise<void> {
    setBusy(true);
    setError("");
    try {
      await api.runs.decideApproval(params.runId, params.callId, decision);
      setDecided(decision);
    } catch (err) {
      setError(extractErrorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <PageLayout title={t("approval.title")}>
      <Show when={!request.loading} fallback={<LoadingState />}>
        <Show
          when={request.state === "ready" ? request() : undefined}
          fallback={<p class="text-sm text-cf-text-muted">{t("approval.notPending")}</p>}
        >
          {(req) => (
            <Card class="max-w-2xl">
              <Card.Body>
                <dl class="mb-4 grid grid-cols-[8rem_1fr] gap-2 text-sm">
                  <dt class="text-cf-text-muted">{t("approval.run")}</dt>
                  <dd class="font-mono break-all">{req().run_id}</dd>
                  <dt class="text-cf-text-muted">{t("approval.tool")}</dt>
                  <dd class="font-mono">{req().tool}</dd>
                  <Show when={req().command}>
                    <dt class="text-cf-text-muted">{t("approval.command")}</dt>
                    <dd class="font-mono break-all">{req().command}</dd>
                  </Show>
                  <Show when={req().path}>
                    <dt class="text-cf-text-muted">{t("approval.path")}</dt>
                    <dd class="font-mono break-all">{req().path}</dd>
                  </Show>
                  <Show when={req().profile}>
                    <dt class="text-cf-text-muted">{t("approval.profile")}</dt>
                    <dd class="font-mono">{req().profile}</dd>
                  </Show>
                  <Show when={req().arguments_preview}>
                    <dt class="text-cf-text-muted">{t("approval.arguments")}</dt>
                    <dd>
                      <pre class="whitespace-pre-wrap break-all font-mono text-xs">
                        {req().arguments_preview}
                      </pre>
                    </dd>
                  </Show>
                </dl>
                <ErrorBanner error={error} onDismiss={() => setError("")} />
                <Show
                  when={decided()}
                  fallback={
                    <div class="flex gap-2">
                      <Button
                        variant="primary"
                        disabled={busy()}
                        onClick={() => void decide("allow")}
                      >
                        {t("approval.approve")}
                      </Button>
                      <Button
                        variant="danger"
                        disabled={busy()}
                        onClick={() => void decide("deny")}
                      >
                        {t("approval.deny")}
                      </Button>
                    </div>
                  }
                >
                  {(d) => (
                    <p class="text-sm font-medium">
                      {d() === "allow" ? t("approval.approved") : t("approval.denied")}
                    </p>
                  )}
                </Show>
              </Card.Body>
            </Card>
          )}
        </Show>
      </Show>
    </PageLayout>
  );
}
