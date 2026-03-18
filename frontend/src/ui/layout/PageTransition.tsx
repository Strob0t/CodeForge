import { type JSX, createSignal, onMount } from "solid-js";

import { cx } from "~/utils/cx";

interface PageTransitionProps {
  children: JSX.Element;
  class?: string;
}

export function PageTransition(props: PageTransitionProps): JSX.Element {
  const [mounted, setMounted] = createSignal(false);
  onMount(() => setMounted(true));

  return (
    <div class={cx(
      mounted() && "animate-[cf-fade-in_0.15s_ease-out]",
      props.class
    )}>
      {props.children}
    </div>
  );
}
