import { createResource, createSignal, For, onMount, Show } from "solid-js";

import { api } from "~/api/client";
import type { Project, QuarantineMessage, QuarantineStats, QuarantineStatus } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useAsyncAction } from "~/hooks";
import { useI18n } from "~/i18n";
import { extractErrorMessage } from "~/lib/errorUtils";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  ErrorBanner,
  FormField,
  Input,
  LoadingState,
  Modal,
  PageLayout,
  Select,
  StatCard,
  Table,
  Textarea,
} from "~/ui";
import type { TableColumn } from "~/ui/composites/Table";

// ---------------------------------------------------------------------------
// Quarantine Admin Dashboard
// ---------------------------------------------------------------------------

/** Map quarantine status to Badge variant. */
function statusVariant(status: QuarantineStatus): "warning" | "success" | "error" | "neutral" {
  switch (status) {
    case "pending":
      return "warning";
    case "approved":
      return "success";
    case "rejected":
      return "error";
    case "expired":
      return "neutral";
  }
}

export default function QuarantinePage() {
  onMount(() => {
    document.title = "Quarantine - CodeForge";
  });
  const { t } = useI18n();
  const { show: toast } = useToast();

  // -- State ----------------------------------------------------------------

  const [selectedProjectId, setSelectedProjectId] = createSignal("");
  const [statusFilter, setStatusFilter] = createSignal<QuarantineStatus | "">("");
  const [selectedMessage, setSelectedMessage] = createSignal<QuarantineMessage | null>(null);

  // Review form state (shared by inline actions and modal)
  const [reviewTarget, setReviewTarget] = createSignal<{
    id: string;
    action: "approve" | "reject";
  } | null>(null);
  const [reviewerName, setReviewerName] = createSignal("");
  const [reviewNote, setReviewNote] = createSignal("");

  // -- Data fetching --------------------------------------------------------

  const [projects] = createResource(() => api.projects.list());

  const [stats, { refetch: refetchStats }] = createResource(
    () => selectedProjectId(),
    (pid) => (pid ? api.quarantine.stats(pid) : undefined),
  );

  const [messages, { refetch: refetchMessages }] = createResource(
    () => ({ pid: selectedProjectId(), status: statusFilter() }),
    ({ pid, status }) =>
      pid ? api.quarantine.list(pid, status || undefined) : Promise.resolve([]),
  );

  // -- Review actions -------------------------------------------------------

  function startReview(id: string, action: "approve" | "reject") {
    setReviewTarget({ id, action });
    setReviewerName("");
    setReviewNote("");
  }

  function cancelReview() {
    setReviewTarget(null);
    setReviewerName("");
    setReviewNote("");
  }

  const {
    run: submitReview,
    loading: submitting,
    error: reviewError,
    clearError: clearReviewError,
  } = useAsyncAction(async () => {
    const target = reviewTarget();
    if (!target) return;

    const data = { reviewed_by: reviewerName().trim(), note: reviewNote().trim() };

    if (target.action === "approve") {
      await api.quarantine.approve(target.id, data);
      toast("success", t("quarantine.toast.approved"));
    } else {
      await api.quarantine.reject(target.id, data);
      toast("success", t("quarantine.toast.rejected"));
    }

    cancelReview();
    setSelectedMessage(null);
    refetchMessages();
    refetchStats();
  });

  // -- Table columns --------------------------------------------------------

  const columns: TableColumn<QuarantineMessage>[] = [
    {
      key: "subject",
      header: t("quarantine.col.subject"),
      render: (row) => (
        <button
          type="button"
          class="text-left font-medium text-cf-accent hover:underline"
          onClick={() => setSelectedMessage(row)}
        >
          {row.subject}
        </button>
      ),
    },
    {
      key: "trust_origin",
      header: t("quarantine.col.trustOrigin"),
    },
    {
      key: "trust_level",
      header: t("quarantine.col.trustLevel"),
    },
    {
      key: "risk_score",
      header: t("quarantine.col.riskScore"),
      render: (row) => (
        <Badge
          variant={row.risk_score > 0.7 ? "error" : row.risk_score > 0.4 ? "warning" : "success"}
        >
          {row.risk_score.toFixed(2)}
        </Badge>
      ),
    },
    {
      key: "status",
      header: t("quarantine.col.status"),
      render: (row) => <Badge variant={statusVariant(row.status)}>{row.status}</Badge>,
    },
    {
      key: "created_at",
      header: t("quarantine.col.createdAt"),
      render: (row) => <span>{new Date(row.created_at).toLocaleString()}</span>,
    },
    {
      key: "actions",
      header: "",
      render: (row) => (
        <Show when={row.status === "pending"}>
          <div class="flex items-center gap-1">
            <Button
              variant="primary"
              size="sm"
              onClick={(e: MouseEvent) => {
                e.stopPropagation();
                startReview(row.id, "approve");
              }}
            >
              {t("quarantine.action.approve")}
            </Button>
            <Button
              variant="danger"
              size="sm"
              onClick={(e: MouseEvent) => {
                e.stopPropagation();
                startReview(row.id, "reject");
              }}
            >
              {t("quarantine.action.reject")}
            </Button>
          </div>
        </Show>
      ),
    },
  ];

  // -- Render ---------------------------------------------------------------

  return (
    <PageLayout title={t("quarantine.title")} description={t("quarantine.description")}>
      {/* Project selector + status filter */}
      <div class="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-2">
        <FormField label={t("quarantine.selectProject")} id="q-project">
          <Select
            id="q-project"
            value={selectedProjectId()}
            onChange={(e) => setSelectedProjectId(e.currentTarget.value)}
          >
            <option value="">{t("quarantine.selectProject")}</option>
            <For each={projects() ?? []}>
              {(p: Project) => <option value={p.id}>{p.name}</option>}
            </For>
          </Select>
        </FormField>

        <FormField label={t("quarantine.filterStatus")} id="q-status">
          <Select
            id="q-status"
            value={statusFilter()}
            onChange={(e) => setStatusFilter(e.currentTarget.value as QuarantineStatus | "")}
          >
            <option value="">{t("quarantine.all")}</option>
            <option value="pending">{t("quarantine.stats.pending")}</option>
            <option value="approved">{t("quarantine.stats.approved")}</option>
            <option value="rejected">{t("quarantine.stats.rejected")}</option>
            <option value="expired">{t("quarantine.stats.expired")}</option>
          </Select>
        </FormField>
      </div>

      {/* Stats cards */}
      <Show when={selectedProjectId() && stats()}>
        <div class="mb-6 grid grid-cols-2 gap-4 sm:grid-cols-4">
          <StatCard
            label={t("quarantine.stats.pending")}
            value={(stats() as QuarantineStats).pending}
          />
          <StatCard
            label={t("quarantine.stats.approved")}
            value={(stats() as QuarantineStats).approved}
          />
          <StatCard
            label={t("quarantine.stats.rejected")}
            value={(stats() as QuarantineStats).rejected}
          />
          <StatCard
            label={t("quarantine.stats.expired")}
            value={(stats() as QuarantineStats).expired}
          />
        </div>
      </Show>

      {/* Loading / error */}
      <Show when={messages.loading}>
        <LoadingState message={t("common.loading")} />
      </Show>
      <Show when={messages.error}>
        <ErrorBanner error={extractErrorMessage(messages.error, String(messages.error))} />
      </Show>

      {/* Messages table */}
      <Show when={!messages.loading && !messages.error && selectedProjectId()}>
        <Show
          when={(messages() ?? []).length > 0}
          fallback={<EmptyState title={t("quarantine.empty")} />}
        >
          <Table<QuarantineMessage>
            columns={columns}
            data={messages() ?? []}
            rowKey={(m) => m.id}
          />
        </Show>
      </Show>

      {/* Review form (inline card) */}
      <Show when={reviewTarget()}>
        <Card class="mt-4">
          <Card.Body>
            <h3 class="mb-3 text-sm font-semibold text-cf-text-primary">
              {reviewTarget()?.action === "approve"
                ? t("quarantine.action.approve")
                : t("quarantine.action.reject")}
            </h3>
            <ErrorBanner error={reviewError} onDismiss={clearReviewError} />
            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <FormField label={t("quarantine.action.reviewerName")} id="q-reviewer">
                <Input
                  id="q-reviewer"
                  type="text"
                  value={reviewerName()}
                  onInput={(e) => setReviewerName(e.currentTarget.value)}
                />
              </FormField>
              <FormField label={t("quarantine.action.note")} id="q-note">
                <Textarea
                  id="q-note"
                  value={reviewNote()}
                  onInput={(e) => setReviewNote(e.currentTarget.value)}
                  rows={2}
                />
              </FormField>
            </div>
            <div class="mt-3 flex justify-end gap-2">
              <Button variant="secondary" onClick={cancelReview}>
                {t("common.cancel")}
              </Button>
              <Button
                variant={reviewTarget()?.action === "approve" ? "primary" : "danger"}
                onClick={() => void submitReview()}
                loading={submitting()}
                disabled={submitting()}
              >
                {reviewTarget()?.action === "approve"
                  ? t("quarantine.action.approve")
                  : t("quarantine.action.reject")}
              </Button>
            </div>
          </Card.Body>
        </Card>
      </Show>

      {/* Detail modal */}
      <Modal
        open={selectedMessage() !== null}
        onClose={() => setSelectedMessage(null)}
        title={t("quarantine.detail.title")}
        class="max-w-2xl"
      >
        <Show when={selectedMessage()}>
          {(msg) => (
            <div class="space-y-4">
              {/* Status + risk */}
              <div class="flex flex-wrap items-center gap-2">
                <Badge variant={statusVariant(msg().status)} pill>
                  {msg().status}
                </Badge>
                <Badge
                  variant={
                    msg().risk_score > 0.7
                      ? "error"
                      : msg().risk_score > 0.4
                        ? "warning"
                        : "success"
                  }
                  pill
                >
                  Risk: {msg().risk_score.toFixed(2)}
                </Badge>
              </div>

              {/* Subject */}
              <div>
                <div class="text-xs font-medium uppercase text-cf-text-muted">
                  {t("quarantine.col.subject")}
                </div>
                <div class="mt-1 text-sm text-cf-text-primary">{msg().subject}</div>
              </div>

              {/* Trust info */}
              <div class="grid grid-cols-2 gap-4">
                <div>
                  <div class="text-xs font-medium uppercase text-cf-text-muted">
                    {t("quarantine.col.trustOrigin")}
                  </div>
                  <div class="mt-1 text-sm text-cf-text-primary">{msg().trust_origin}</div>
                </div>
                <div>
                  <div class="text-xs font-medium uppercase text-cf-text-muted">
                    {t("quarantine.col.trustLevel")}
                  </div>
                  <div class="mt-1 text-sm text-cf-text-primary">{msg().trust_level}</div>
                </div>
              </div>

              {/* Payload */}
              <div>
                <div class="text-xs font-medium uppercase text-cf-text-muted">
                  {t("quarantine.detail.payload")}
                </div>
                <pre class="mt-1 max-h-48 overflow-auto rounded-cf-md bg-cf-bg-surface-alt p-3 text-xs text-cf-text-secondary">
                  {formatPayload(msg().payload)}
                </pre>
              </div>

              {/* Risk factors */}
              <Show when={msg().risk_factors.length > 0}>
                <div>
                  <div class="text-xs font-medium uppercase text-cf-text-muted">
                    {t("quarantine.detail.riskFactors")}
                  </div>
                  <div class="mt-1 flex flex-wrap gap-1">
                    <For each={msg().risk_factors}>
                      {(factor) => <Badge variant="warning">{factor}</Badge>}
                    </For>
                  </div>
                </div>
              </Show>

              {/* Timestamps */}
              <div class="grid grid-cols-2 gap-4 text-sm">
                <div>
                  <div class="text-xs font-medium uppercase text-cf-text-muted">
                    {t("quarantine.col.createdAt")}
                  </div>
                  <div class="mt-1 text-cf-text-primary">
                    {new Date(msg().created_at).toLocaleString()}
                  </div>
                </div>
                <div>
                  <div class="text-xs font-medium uppercase text-cf-text-muted">
                    {t("quarantine.col.expiresAt")}
                  </div>
                  <div class="mt-1 text-cf-text-primary">
                    {new Date(msg().expires_at).toLocaleString()}
                  </div>
                </div>
              </div>

              {/* Review info (if reviewed) */}
              <Show when={msg().reviewed_at}>
                <div class="rounded-cf-md border border-cf-border bg-cf-bg-surface-alt p-3">
                  <div class="grid grid-cols-2 gap-4 text-sm">
                    <div>
                      <div class="text-xs font-medium uppercase text-cf-text-muted">
                        {t("quarantine.detail.reviewedBy")}
                      </div>
                      <div class="mt-1 text-cf-text-primary">{msg().reviewed_by}</div>
                    </div>
                    <div>
                      <div class="text-xs font-medium uppercase text-cf-text-muted">
                        {t("quarantine.detail.reviewedAt")}
                      </div>
                      <div class="mt-1 text-cf-text-primary">
                        {new Date(msg().reviewed_at ?? "").toLocaleString()}
                      </div>
                    </div>
                  </div>
                  <Show when={msg().review_note}>
                    <div class="mt-2">
                      <div class="text-xs font-medium uppercase text-cf-text-muted">
                        {t("quarantine.detail.reviewNote")}
                      </div>
                      <div class="mt-1 text-sm text-cf-text-primary">{msg().review_note}</div>
                    </div>
                  </Show>
                </div>
              </Show>

              {/* Approve/Reject from modal */}
              <Show when={msg().status === "pending"}>
                <div class="flex justify-end gap-2 border-t border-cf-border pt-3">
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={() => {
                      setSelectedMessage(null);
                      startReview(msg().id, "approve");
                    }}
                  >
                    {t("quarantine.action.approve")}
                  </Button>
                  <Button
                    variant="danger"
                    size="sm"
                    onClick={() => {
                      setSelectedMessage(null);
                      startReview(msg().id, "reject");
                    }}
                  >
                    {t("quarantine.action.reject")}
                  </Button>
                </div>
              </Show>
            </div>
          )}
        </Show>
      </Modal>
    </PageLayout>
  );
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Try to pretty-print JSON payloads, fall back to raw string. */
function formatPayload(payload: string): string {
  try {
    return JSON.stringify(JSON.parse(payload), null, 2);
  } catch {
    return payload;
  }
}
