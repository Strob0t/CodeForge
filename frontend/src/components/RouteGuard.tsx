import { useNavigate } from "@solidjs/router";
import { createEffect, type JSX, Show } from "solid-js";

import { useAuth } from "./AuthProvider";

/**
 * RouteGuard redirects unauthenticated users to /login.
 * Wrap protected page content with this component.
 */
export function RouteGuard(props: { children: JSX.Element }): JSX.Element {
  const { isAuthenticated } = useAuth();
  const navigate = useNavigate();

  createEffect(() => {
    if (!isAuthenticated()) {
      navigate("/login", { replace: true });
    }
  });

  return <Show when={isAuthenticated()}>{props.children}</Show>;
}
