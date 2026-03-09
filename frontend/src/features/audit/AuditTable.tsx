import { createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { AuditEntry } from "~/api/types";
import { useAsyncAction } from "~/hooks/useAsyncAction";
import { useI18n } from "~/i18n";
import { Button } from "~/ui";

interface AuditTableProps {
  projectId?: string;
}

export default function AuditTable(props: AuditTableProps) {
  const { t } = useI18n();

  const [entries, setEntries] = createSignal<AuditEntry[]>([]);
  const [cursor, setCursor] = createSignal<string>("");
  const [hasMore, setHasMore] = createSignal(false);
  const [total, setTotal] = createSignal(0);
  const [expandedId, setExpandedId] = createSignal<string | null>(null);
  const [filterAction, setFilterAction] = createSignal("");
  const [initialLoaded, setInitialLoaded] = createSignal(false);

  const PAGE_SIZE = 50;

  const {
    run: fetchPage,
    loading,
    error,
  } = useAsyncAction(async (reset: boolean) => {
    const opts: { action?: string; cursor?: string; limit: number } = {
      limit: PAGE_SIZE,
    };
    const action = filterAction();
    if (action) opts.action = action;
    if (!reset && cursor()) opts.cursor = cursor();

    const page = props.projectId
      ? await api.audit.listByProject(props.projectId, opts)
      : await api.audit.list(opts);

    if (reset) {
      setEntries(page.entries);
    } else {
      setEntries((prev) => [...prev, ...page.entries]);
    }
    setCursor(page.cursor);
    setHasMore(page.has_more);
    if (reset || total() === 0) setTotal(page.total);
    setInitialLoaded(true);
  });

  // Initial fetch
  void fetchPage(true);

  function handleFilterChange(action: string) {
    setFilterAction(action);
    void fetchPage(true);
  }

  function uniqueActions(): string[] {
    const seen = new Set<string>();
    for (const e of entries()) {
      if (e.action) seen.add(e.action);
    }
    return Array.from(seen).sort();
  }

  function shortId(id: string): string {
    return id ? id.slice(0, 8) : "-";
  }

  function toggleRow(id: string) {
    setExpandedId(expandedId() === id ? null : id);
  }

  return (
    <div class="space-y-4">
      {/* Filter + Total */}
      <div class="flex items-center justify-between gap-4">
        <div class="flex items-center gap-2">
          <label class="text-xs text-cf-text-muted" for="audit-action-filter">
            {t("audit.filter.label")}
          </label>
          <select
            id="audit-action-filter"
            class="rounded border border-cf-border bg-cf-bg-surface px-2 py-1 text-sm text-cf-text-primary"
            value={filterAction()}
            onChange={(e) => handleFilterChange(e.currentTarget.value)}
          >
            <option value="">{t("audit.filter.all")}</option>
            <For each={uniqueActions()}>{(action) => <option value={action}>{action}</option>}</For>
          </select>
        </div>
        <Show when={total() > 0}>
          <span class="text-xs text-cf-text-muted">
            {t("audit.total", { count: String(total()) })}
          </span>
        </Show>
      </div>

      {/* Loading */}
      <Show when={loading() && !initialLoaded()}>
        <p class="py-8 text-center text-sm text-cf-text-muted">{t("audit.loading")}</p>
      </Show>

      {/* Error */}
      <Show when={error()}>
        <p class="py-4 text-center text-sm text-red-500">{error()}</p>
      </Show>

      {/* Empty */}
      <Show when={initialLoaded() && entries().length === 0 && !loading()}>
        <div class="flex flex-col items-center justify-center gap-3 py-16 text-center">
          <p class="text-sm text-cf-text-muted">{t("empty.audit")}</p>
        </div>
      </Show>

      {/* Table */}
      <Show when={entries().length > 0}>
        <div class="overflow-x-auto rounded-lg border border-cf-border">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-cf-border bg-cf-bg-surface-alt text-left text-xs font-medium uppercase text-cf-text-muted">
                <th class="px-3 py-2">{t("audit.table.timestamp")}</th>
                <th class="px-3 py-2">{t("audit.table.action")}</th>
                <Show when={!props.projectId}>
                  <th class="px-3 py-2">{t("audit.table.project")}</th>
                </Show>
                <th class="px-3 py-2">{t("audit.table.run")}</th>
                <th class="px-3 py-2">{t("audit.table.agent")}</th>
                <th class="px-3 py-2 w-8" />
              </tr>
            </thead>
            <tbody>
              <For each={entries()}>
                {(entry) => (
                  <>
                    <tr
                      class="cursor-pointer border-b border-cf-border transition-colors hover:bg-cf-bg-surface-alt/50"
                      onClick={() => toggleRow(entry.id)}
                    >
                      <td class="whitespace-nowrap px-3 py-2 font-mono text-xs">
                        {new Date(entry.created_at).toLocaleString()}
                      </td>
                      <td class="px-3 py-2">
                        <span class="inline-block rounded bg-cf-accent/10 px-1.5 py-0.5 text-xs font-medium text-cf-accent">
                          {entry.action}
                        </span>
                      </td>
                      <Show when={!props.projectId}>
                        <td class="px-3 py-2 font-mono text-xs text-cf-text-secondary">
                          {shortId(entry.project_id)}
                        </td>
                      </Show>
                      <td class="px-3 py-2 font-mono text-xs text-cf-text-secondary">
                        {shortId(entry.run_id)}
                      </td>
                      <td class="px-3 py-2 font-mono text-xs text-cf-text-secondary">
                        {shortId(entry.agent_id)}
                      </td>
                      <td class="px-3 py-2 text-center text-xs text-cf-text-muted">
                        {expandedId() === entry.id ? "\u25B2" : "\u25BC"}
                      </td>
                    </tr>
                    <Show when={expandedId() === entry.id}>
                      <tr class="border-b border-cf-border bg-cf-bg-surface">
                        <td colSpan={props.projectId ? 5 : 6} class="px-4 py-3">
                          <div class="text-xs text-cf-text-muted mb-1">
                            {t("audit.table.details")}
                          </div>
                          <pre class="whitespace-pre-wrap text-sm text-cf-text-primary font-mono bg-cf-bg-surface-alt rounded p-2">
                            {entry.details || "-"}
                          </pre>
                        </td>
                      </tr>
                    </Show>
                  </>
                )}
              </For>
            </tbody>
          </table>
        </div>
      </Show>

      {/* Load More */}
      <Show when={hasMore()}>
        <div class="flex justify-center pt-2">
          <Button
            variant="secondary"
            size="sm"
            onClick={() => void fetchPage(false)}
            disabled={loading()}
            loading={loading()}
          >
            {t("audit.loadMore")}
          </Button>
        </div>
      </Show>
    </div>
  );
}
