import { A } from "@solidjs/router";
import { type JSX, splitProps } from "solid-js";

export interface NavLinkProps {
  href: string;
  end?: boolean;
  class?: string;
  children: JSX.Element;
}

export function NavLink(props: NavLinkProps): JSX.Element {
  const [local, rest] = splitProps(props, ["href", "end", "class", "children"]);

  return (
    <A
      {...rest}
      href={local.href}
      end={local.end}
      class={
        "block rounded-cf-md px-3 py-2 text-sm font-medium text-cf-text-secondary hover:bg-cf-bg-surface-alt transition-colors" +
        (local.class ? " " + local.class : "")
      }
      activeClass="bg-cf-bg-surface-alt text-cf-accent"
    >
      {local.children}
    </A>
  );
}
