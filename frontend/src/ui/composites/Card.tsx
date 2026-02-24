import { type JSX, splitProps } from "solid-js";

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
      class={
        "rounded-cf-lg border border-cf-border bg-cf-bg-surface shadow-cf-sm" +
        (local.class ? " " + local.class : "")
      }
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
    <div class={"border-b border-cf-border px-4 py-3" + (local.class ? " " + local.class : "")}>
      {local.children}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Card.Body
// ---------------------------------------------------------------------------

function CardBody(props: CardProps): JSX.Element {
  const [local] = splitProps(props, ["class", "children"]);
  return <div class={"px-4 py-4" + (local.class ? " " + local.class : "")}>{local.children}</div>;
}

// ---------------------------------------------------------------------------
// Card.Footer
// ---------------------------------------------------------------------------

function CardFooter(props: CardProps): JSX.Element {
  const [local] = splitProps(props, ["class", "children"]);
  return (
    <div class={"border-t border-cf-border px-4 py-3" + (local.class ? " " + local.class : "")}>
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
