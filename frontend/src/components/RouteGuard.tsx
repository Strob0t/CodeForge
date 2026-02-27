import { useNavigate } from "@solidjs/router";
import { createEffect, type JSX, Show } from "solid-js";

import { useAuth } from "./AuthProvider";

/**
 * RouteGuard redirects unauthenticated users to /login.
 * Waits for auth initialization (refresh cookie check) before deciding.
 * Wrap protected page content with this component.
 */
export function RouteGuard(props: { children: JSX.Element }): JSX.Element {
  const { isAuthenticated, initializing } = useAuth();
  const navigate = useNavigate();

  createEffect(() => {
    // Wait until the initial session restore attempt completes.
    if (initializing()) return;
    if (!isAuthenticated()) {
      navigate("/login", { replace: true });
    }
  });

  return <Show when={!initializing() && isAuthenticated()}>{props.children}</Show>;
}
