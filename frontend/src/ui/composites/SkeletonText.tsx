import { type JSX, splitProps } from "solid-js";

import { Skeleton } from "../primitives/Skeleton";

export interface SkeletonTextProps {
  lines?: number;
  class?: string;
}

export function SkeletonText(props: SkeletonTextProps): JSX.Element {
  const [local] = splitProps(props, ["lines", "class"]);

  return (
    <div class={local.class}>
      <Skeleton variant="text" lines={local.lines ?? 3} />
    </div>
  );
}
