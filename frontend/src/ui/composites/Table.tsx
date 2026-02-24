import { For, type JSX, Show, splitProps } from "solid-js";

import { Spinner } from "../primitives/Spinner";

export interface TableColumn<T> {
  key: string;
  header: string;
  render?: (row: T) => JSX.Element;
  class?: string;
}

export interface TableProps<T> {
  columns: TableColumn<T>[];
  data: T[];
  rowKey: (row: T) => string | number;
  emptyMessage?: string;
  loading?: boolean;
  class?: string;
}

export function Table<T>(props: TableProps<T>): JSX.Element {
  const [local] = splitProps(props, [
    "columns",
    "data",
    "rowKey",
    "emptyMessage",
    "loading",
    "class",
  ]);

  return (
    <div
      class={
        "overflow-auto rounded-cf-lg border border-cf-border" +
        (local.class ? " " + local.class : "")
      }
    >
      <table class="w-full text-left text-sm">
        <thead>
          <tr class="border-b border-cf-border bg-cf-bg-surface-alt">
            <For each={local.columns}>
              {(col) => (
                <th
                  class={
                    "px-4 py-2 text-xs font-medium uppercase tracking-wider text-cf-text-tertiary" +
                    (col.class ? " " + col.class : "")
                  }
                >
                  {col.header}
                </th>
              )}
            </For>
          </tr>
        </thead>
        <tbody>
          <Show when={local.loading}>
            <tr>
              <td colspan={local.columns.length} class="px-4 py-8 text-center">
                <Spinner size="md" />
              </td>
            </tr>
          </Show>
          <Show when={!local.loading && local.data.length === 0}>
            <tr>
              <td colspan={local.columns.length} class="px-4 py-8 text-center text-cf-text-muted">
                {local.emptyMessage ?? "No data"}
              </td>
            </tr>
          </Show>
          <Show when={!local.loading && local.data.length > 0}>
            <For each={local.data}>
              {(row) => (
                <tr class="border-b border-cf-border last:border-b-0 hover:bg-cf-bg-surface-alt transition-colors">
                  <For each={local.columns}>
                    {(col) => (
                      <td
                        class={
                          "px-4 py-2 text-cf-text-primary" + (col.class ? " " + col.class : "")
                        }
                      >
                        {col.render
                          ? col.render(row)
                          : String((row as Record<string, unknown>)[col.key] ?? "")}
                      </td>
                    )}
                  </For>
                </tr>
              )}
            </For>
          </Show>
        </tbody>
      </table>
    </div>
  );
}
