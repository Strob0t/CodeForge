import { For, type JSX, Show, splitProps } from "solid-js";

export type SkeletonVariant = "text" | "rect" | "circle";

export interface SkeletonProps {
  variant?: SkeletonVariant;
  width?: string;
  height?: string;
  lines?: number;
  class?: string;
}

export function Skeleton(props: SkeletonProps): JSX.Element {
  const [local] = splitProps(props, ["variant", "width", "height", "lines", "class"]);

  const variant = (): SkeletonVariant => local.variant ?? "rect";
  const count = (): number => local.lines ?? 3;

  return (
    <>
      <Show when={variant() === "text"}>
        <div role="presentation" aria-hidden="true" class={local.class}>
          <For each={Array.from({ length: count() })}>
            {(_, i) => (
              <div
                class={
                  "cf-shimmer h-3 rounded-cf-sm mb-2 last:mb-0" +
                  (i() === count() - 1 ? " w-3/4" : " w-full")
                }
              />
            )}
          </For>
        </div>
      </Show>

      <Show when={variant() === "circle"}>
        <div
          role="presentation"
          aria-hidden="true"
          class={"cf-shimmer rounded-full" + (local.class ? " " + local.class : "")}
          style={{ width: local.width ?? "2.5rem", height: local.height ?? "2.5rem" }}
        />
      </Show>

      <Show when={variant() === "rect"}>
        <div
          role="presentation"
          aria-hidden="true"
          class={"cf-shimmer rounded-cf-md" + (local.class ? " " + local.class : "")}
          style={{ width: local.width ?? "100%", height: local.height ?? "1rem" }}
        />
      </Show>
    </>
  );
}
