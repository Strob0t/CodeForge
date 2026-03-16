import { For, type JSX, splitProps } from "solid-js";

import { Skeleton } from "../primitives/Skeleton";

export interface SkeletonChatProps {
  messages?: number;
  class?: string;
}

export function SkeletonChat(props: SkeletonChatProps): JSX.Element {
  const [local] = splitProps(props, ["messages", "class"]);
  const count = (): number => local.messages ?? 4;

  return (
    <div
      role="presentation"
      aria-hidden="true"
      class={"flex flex-col gap-4" + (local.class ? " " + local.class : "")}
    >
      <For each={Array.from({ length: count() })}>
        {(_, i) => (
          <div class={i() % 2 === 1 ? "flex justify-end" : "flex justify-start"}>
            <div
              class={
                "rounded-cf-md px-4 py-3 " +
                (i() % 2 === 1 ? "bg-cf-accent/10 max-w-[50%]" : "bg-cf-bg-surface-alt max-w-[75%]")
              }
            >
              <Skeleton variant="text" lines={i() % 2 === 1 ? 1 : 2} />
            </div>
          </div>
        )}
      </For>
    </div>
  );
}
