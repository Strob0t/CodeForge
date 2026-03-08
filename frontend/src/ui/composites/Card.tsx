import { type JSX, splitProps } from "solid-js";

import { cx } from "~/utils/cx";

// ---------------------------------------------------------------------------
// Card
// ---------------------------------------------------------------------------

export interface CardProps {
  class?: string;
  children: JSX.Element;
}

function CardRoot(props: CardProps): JSX.Element {
  const [local] = splitProps(props, ["class", "children"]);
  return (
    <div
      class={cx("rounded-cf-lg border border-cf-border bg-cf-bg-surface shadow-cf-sm", local.class)}
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
