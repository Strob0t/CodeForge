import { type JSX, Show, splitProps } from "solid-js";

import { Skeleton } from "../primitives/Skeleton";
import { Card } from "./Card";

export type SkeletonCardVariant = "stat" | "project";

export interface SkeletonCardProps {
  variant?: SkeletonCardVariant;
  class?: string;
}

export function SkeletonCard(props: SkeletonCardProps): JSX.Element {
  const [local] = splitProps(props, ["variant", "class"]);
  const variant = (): SkeletonCardVariant => local.variant ?? "project";

  return (
    <>
      <Show when={variant() === "stat"}>
        <Card class={"p-3" + (local.class ? " " + local.class : "")}>
          <Skeleton variant="rect" width="40%" height="0.75rem" />
          <div class="mt-2">
            <Skeleton variant="rect" width="60%" height="1.25rem" />
          </div>
        </Card>
      </Show>

      <Show when={variant() === "project"}>
        <Card class={local.class}>
          <Card.Body>
            <Skeleton variant="rect" width="70%" height="1rem" />
            <div class="mt-3">
              <Skeleton variant="text" lines={2} />
            </div>
            <div class="mt-3 flex gap-2">
              <Skeleton variant="rect" width="3rem" height="1.25rem" class="rounded-full" />
              <Skeleton variant="rect" width="4rem" height="1.25rem" class="rounded-full" />
            </div>
          </Card.Body>
        </Card>
      </Show>
    </>
  );
}
