import type { JSX } from "solid-js";
import { Show } from "solid-js";

import type { UserRole } from "~/api/types";

import { useAuth } from "./AuthProvider";

/**
 * RoleGate conditionally renders children based on the user's role.
 * If the user does not have one of the required roles, nothing is rendered.
 */
export function RoleGate(props: {
  roles: UserRole[];
  fallback?: JSX.Element;
  children: JSX.Element;
}): JSX.Element {
  const { hasRole } = useAuth();

  return (
    <Show when={hasRole(...props.roles)} fallback={props.fallback}>
      {props.children}
    </Show>
  );
}
