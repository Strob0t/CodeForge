import { type JSX, splitProps } from "solid-js";

import { Card } from "../composites/Card";
import { SectionHeader } from "../composites/SectionHeader";

export interface SectionProps {
  title: string;
  description?: string;
  action?: JSX.Element;
  class?: string;
  children: JSX.Element;
}

export function Section(props: SectionProps): JSX.Element {
  const [local] = splitProps(props, ["title", "description", "action", "class", "children"]);

  return (
    <div class={local.class}>
      <SectionHeader
        title={local.title}
        description={local.description}
        action={local.action}
        class="mb-3"
      />
      <Card>
        <Card.Body>{local.children}</Card.Body>
      </Card>
    </div>
  );
}
