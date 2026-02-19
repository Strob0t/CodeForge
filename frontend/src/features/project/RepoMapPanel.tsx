import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { RepoMap } from "~/api/types";
import { useI18n } from "~/i18n";

interface RepoMapPanelProps {
  projectId: string;
  onStatusUpdate?: (status: string) => void;
}

export default function RepoMapPanel(props: RepoMapPanelProps) {
  const { t, fmt } = useI18n();
  const [repoMap, { refetch }] = createResource<RepoMap | null>(
    () => props.projectId,
    async (id) => {
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
    <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <div class="mb-3 flex items-center justify-between">
        <h3 class="text-lg font-semibold">{t("repomap.title")}</h3>
        <button
          class="rounded bg-teal-600 px-3 py-1.5 text-sm text-white hover:bg-teal-700 disabled:opacity-50"
          onClick={handleGenerate}
          disabled={generating()}
        >
          {generating()
            ? t("repomap.generating")
            : repoMap()
              ? t("repomap.regenerate")
              : t("repomap.generate")}
        </button>
      </div>

      <Show when={error()}>
        <div class="mb-3 rounded bg-red-50 p-2 text-sm text-red-600 dark:bg-red-900/30 dark:text-red-400">
          {error()}
        </div>
      </Show>

      <Show
        when={!repoMap.loading}
        fallback={<p class="text-sm text-gray-400 dark:text-gray-500">{t("common.loading")}</p>}
      >
        <Show
          when={repoMap()}
          fallback={<p class="text-sm text-gray-500 dark:text-gray-400">{t("repomap.empty")}</p>}
        >
          {(rm) => (
            <>
              {/* Stats */}
              <div class="mb-3 grid grid-cols-3 gap-3">
                <div class="rounded border border-gray-100 bg-gray-50 p-2 text-center dark:border-gray-600 dark:bg-gray-700">
                  <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">
                    {fmt.compact(rm().file_count)}
                  </div>
                  <div class="text-xs text-gray-500 dark:text-gray-400">{t("repomap.files")}</div>
                </div>
                <div class="rounded border border-gray-100 bg-gray-50 p-2 text-center dark:border-gray-600 dark:bg-gray-700">
                  <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">
                    {fmt.compact(rm().symbol_count)}
                  </div>
                  <div class="text-xs text-gray-500 dark:text-gray-400">{t("repomap.symbols")}</div>
                </div>
                <div class="rounded border border-gray-100 bg-gray-50 p-2 text-center dark:border-gray-600 dark:bg-gray-700">
                  <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">
                    {fmt.compact(rm().token_count)}
                  </div>
                  <div class="text-xs text-gray-500 dark:text-gray-400">{t("repomap.tokens")}</div>
                </div>
              </div>

              {/* Languages */}
              <Show when={rm().languages.length > 0}>
                <div class="mb-3">
                  <span class="mr-2 text-xs text-gray-500 dark:text-gray-400">
                    {t("repomap.languages")}
                  </span>
                  <div class="inline-flex flex-wrap gap-1">
                    <For each={rm().languages}>
                      {(lang) => (
                        <span class="rounded bg-teal-50 px-2 py-0.5 text-xs text-teal-700 dark:bg-teal-900/30 dark:text-teal-400">
                          {lang}
                        </span>
                      )}
                    </For>
                  </div>
                </div>
              </Show>

              {/* Version and timestamp */}
              <div class="mb-3 text-xs text-gray-400 dark:text-gray-500">
                {t("repomap.version", {
                  version: rm().version,
                  date: fmt.dateTime(rm().updated_at),
                })}
              </div>

              {/* Collapsible map text */}
              <div>
                <button
                  type="button"
                  class="flex items-center gap-1 text-sm text-gray-600 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200"
                  onClick={() => setExpanded((v) => !v)}
                  aria-expanded={expanded()}
                  aria-label={expanded() ? t("repomap.hideMapAria") : t("repomap.showMapAria")}
                >
                  <span class="font-mono text-xs" aria-hidden="true">
                    {expanded() ? "v" : ">"}
                  </span>
                  {expanded() ? t("repomap.hideMap") : t("repomap.showMap")}
                </button>
                <Show when={expanded()}>
                  <pre class="mt-2 max-h-96 overflow-auto rounded border border-gray-200 bg-gray-50 p-3 text-xs leading-relaxed text-gray-700 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-300">
                    {rm().map_text}
                  </pre>
                </Show>
              </div>
            </>
          )}
        </Show>
      </Show>
    </div>
  );
}

export { RepoMapPanel };
