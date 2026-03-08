import type { JSX } from "solid-js";

import { cx } from "~/utils/cx";

interface GridLayoutProps {
  children: JSX.Element;
  class?: string;
  /** Number of columns at lg breakpoint (default: 2). */
  lg?: 1 | 2 | 3 | 4;
  /** Number of columns at xl breakpoint (default: 3). */
  xl?: 1 | 2 | 3 | 4;
}

const LG_COLS: Record<number, string> = {
  1: "lg:grid-cols-1",
  2: "lg:grid-cols-2",
  3: "lg:grid-cols-3",
  4: "lg:grid-cols-4",
};

const XL_COLS: Record<number, string> = {
  1: "xl:grid-cols-1",
  2: "xl:grid-cols-2",
  3: "xl:grid-cols-3",
  4: "xl:grid-cols-4",
};

/** Responsive card grid: 1 column on mobile, configurable at lg/xl breakpoints. */
export function GridLayout(props: GridLayoutProps): JSX.Element {
  return (
    <div
      class={cx(
        "grid grid-cols-1 gap-4",
        LG_COLS[props.lg ?? 2],
        XL_COLS[props.xl ?? 3],
        props.class,
      )}
    >
      {props.children}
    </div>
  );
}
