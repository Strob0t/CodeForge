import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { RepoMap } from "~/api/types";
import { useI18n } from "~/i18n";
import { Alert, Badge, Button, Card } from "~/ui";

interface RepoMapPanelProps {
  projectId: string;
  onStatusUpdate?: (status: string) => void;
}

export default function RepoMapPanel(props: RepoMapPanelProps) {
  const { t, fmt } = useI18n();
  const [repoMap, { refetch }] = createResource(
    () => props.projectId || undefined,
    async (id: string): Promise<RepoMap | null> => {
      try {
        return await api.repomap.get(id);
      } catch {
        return null;
      }
    },
  );

  const [expanded, setExpanded] = createSignal(false);
  const [generating, setGenerating] = createSignal(false);
  const [error, setError] = createSignal("");

  const handleGenerate = async () => {
    setGenerating(true);
    setError("");
    try {
      await api.repomap.generate(props.projectId);
      props.onStatusUpdate?.("generating");
      // Refetch after a short delay to pick up the result
      setTimeout(() => refetch(), 2000);
    } catch (e) {
      setError(e instanceof Error ? e.message : t("repomap.toast.generateFailed"));
    } finally {
      setGenerating(false);
    }
  };

  return (
    <Card>
      <Card.Header>
        <div class="flex items-center justify-between">
          <h3 class="text-lg font-semibold">{t("repomap.title")}</h3>
          <Button
            variant="primary"
            size="sm"
            onClick={handleGenerate}
            disabled={generating()}
            loading={generating()}
          >
            {generating()
              ? t("repomap.generating")
              : repoMap()
                ? t("repomap.regenerate")
                : t("repomap.generate")}
          </Button>
        </div>
      </Card.Header>

      <Card.Body>
        <Show when={error()}>
          <div class="mb-3">
            <Alert variant="error">{error()}</Alert>
          </div>
        </Show>

        <Show
          when={!repoMap.loading}
          fallback={<p class="text-sm text-cf-text-muted">{t("common.loading")}</p>}
        >
          <Show
            when={repoMap()}
            fallback={<p class="text-sm text-cf-text-tertiary">{t("repomap.empty")}</p>}
          >
            {(rm) => (
              <>
                {/* Stats */}
                <div class="mb-3 grid grid-cols-3 gap-3">
                  <div class="rounded-cf-sm border border-cf-border-subtle bg-cf-bg-surface-alt p-2 text-center">
                    <div class="text-lg font-semibold text-cf-text-primary">
                      {fmt.compact(rm().file_count)}
                    </div>
                    <div class="text-xs text-cf-text-tertiary">{t("repomap.files")}</div>
                  </div>
                  <div class="rounded-cf-sm border border-cf-border-subtle bg-cf-bg-surface-alt p-2 text-center">
                    <div class="text-lg font-semibold text-cf-text-primary">
                      {fmt.compact(rm().symbol_count)}
                    </div>
                    <div class="text-xs text-cf-text-tertiary">{t("repomap.symbols")}</div>
                  </div>
                  <div class="rounded-cf-sm border border-cf-border-subtle bg-cf-bg-surface-alt p-2 text-center">
                    <div class="text-lg font-semibold text-cf-text-primary">
                      {fmt.compact(rm().token_count)}
                    </div>
                    <div class="text-xs text-cf-text-tertiary">{t("repomap.tokens")}</div>
                  </div>
                </div>

                {/* Languages */}
                <Show when={rm().languages.length > 0}>
                  <div class="mb-3">
                    <span class="mr-2 text-xs text-cf-text-tertiary">{t("repomap.languages")}</span>
                    <div class="inline-flex flex-wrap gap-1">
                      <For each={rm().languages}>
                        {(lang) => <Badge variant="info">{lang}</Badge>}
                      </For>
                    </div>
                  </div>
                </Show>

                {/* Version and timestamp */}
                <div class="mb-3 text-xs text-cf-text-muted">
                  {t("repomap.version", {
                    version: rm().version,
                    date: fmt.dateTime(rm().updated_at),
                  })}
                </div>

                {/* Collapsible map text */}
                <div>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setExpanded((v) => !v)}
                    aria-expanded={expanded()}
                    aria-label={expanded() ? t("repomap.hideMapAria") : t("repomap.showMapAria")}
                  >
                    <span class="font-mono text-xs" aria-hidden="true">
                      {expanded() ? "v" : ">"}
                    </span>{" "}
                    {expanded() ? t("repomap.hideMap") : t("repomap.showMap")}
                  </Button>
                  <Show when={expanded()}>
                    <pre class="mt-2 max-h-96 overflow-auto rounded-cf-sm border border-cf-border bg-cf-bg-surface-alt p-3 text-xs leading-relaxed text-cf-text-secondary">
                      {rm().map_text}
                    </pre>
                  </Show>
                </div>
              </>
            )}
          </Show>
        </Show>
      </Card.Body>
    </Card>
  );
}

export { RepoMapPanel };
