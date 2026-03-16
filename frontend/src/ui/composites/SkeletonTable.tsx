import { For, type JSX, splitProps } from "solid-js";

import { Skeleton } from "../primitives/Skeleton";

export interface SkeletonTableProps {
  columns?: number;
  rows?: number;
  class?: string;
}

const cellWidths = ["75%", "60%", "90%", "50%", "80%", "65%", "70%", "55%"];

export function SkeletonTable(props: SkeletonTableProps): JSX.Element {
  const [local] = splitProps(props, ["columns", "rows", "class"]);

  const cols = (): number => local.columns ?? 4;
  const rowCount = (): number => local.rows ?? 5;

  return (
    <div
      role="presentation"
      aria-hidden="true"
      class={
        "overflow-x-auto rounded-cf-lg border border-cf-border" +
        (local.class ? " " + local.class : "")
      }
    >
      <table class="w-full text-left text-sm">
        <thead>
          <tr class="border-b border-cf-border bg-cf-bg-surface-alt">
            <For each={Array.from({ length: cols() })}>
              {() => (
                <th class="px-3 py-2 sm:px-4">
                  <Skeleton variant="rect" width="60%" height="0.625rem" class="opacity-70" />
                </th>
              )}
            </For>
          </tr>
        </thead>
        <tbody>
          <For each={Array.from({ length: rowCount() })}>
            {(_, ri) => (
              <tr class="border-b border-cf-border last:border-b-0">
                <For each={Array.from({ length: cols() })}>
                  {(_, ci) => (
                    <td class="px-3 py-2 sm:px-4">
                      <Skeleton
                        variant="rect"
                        width={cellWidths[(ri() * cols() + ci()) % cellWidths.length]}
                        height="0.75rem"
                      />
                    </td>
                  )}
                </For>
              </tr>
            )}
          </For>
        </tbody>
      </table>
    </div>
  );
}
