import { type JSX, splitProps } from "solid-js";

import { clickable } from "~/utils/a11y";
import { cx } from "~/utils/cx";

// ---------------------------------------------------------------------------
// Card
// ---------------------------------------------------------------------------

export interface CardProps {
  class?: string;
  children: JSX.Element;
  onClick?: (e: MouseEvent) => void;
}

function CardRoot(props: CardProps): JSX.Element {
  const [local] = splitProps(props, ["class", "children", "onClick"]);
  return (
    <div
      class={cx(
        "rounded-cf-lg border border-cf-border bg-cf-bg-surface shadow-cf-sm hover:-translate-y-0.5 transition-transform duration-200",
        local.onClick && "cursor-pointer",
        local.class,
      )}
      {...(local.onClick ? clickable((e) => local.onClick?.(e as MouseEvent)) : {})}
    >
      {local.children}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Card.Header
// ---------------------------------------------------------------------------

function CardHeader(props: CardProps): JSX.Element {
  const [local] = splitProps(props, ["class", "children"]);
  return (
    <div class={cx("border-b border-cf-border px-3 py-3 sm:px-4", local.class)}>
      {local.children}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Card.Body
// ---------------------------------------------------------------------------

function CardBody(props: CardProps): JSX.Element {
  const [local] = splitProps(props, ["class", "children"]);
  return <div class={cx("px-3 py-3 sm:px-4 sm:py-4", local.class)}>{local.children}</div>;
}

// ---------------------------------------------------------------------------
// Card.Footer
// ---------------------------------------------------------------------------

function CardFooter(props: CardProps): JSX.Element {
  const [local] = splitProps(props, ["class", "children"]);
  return (
    <div class={cx("border-t border-cf-border px-3 py-3 sm:px-4", local.class)}>
      {local.children}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Compound export
// ---------------------------------------------------------------------------

export const Card = Object.assign(CardRoot, {
  Header: CardHeader,
  Body: CardBody,
  Footer: CardFooter,
});
