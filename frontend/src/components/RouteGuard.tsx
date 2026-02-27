import { useNavigate } from "@solidjs/router";
import { createEffect, type JSX, Show } from "solid-js";

import { useAuth } from "./AuthProvider";

/**
 * RouteGuard redirects unauthenticated users to /login.
 * Waits for auth initialization (refresh cookie check) before deciding.
 * Wrap protected page content with this component.
 */
export function RouteGuard(props: { children: JSX.Element }): JSX.Element {
  const { isAuthenticated, initializing, mustChangePassword } = useAuth();
  const navigate = useNavigate();

  createEffect(() => {
    // Wait until the initial session restore attempt completes.
    if (initializing()) return;
    if (!isAuthenticated()) {
      navigate("/login", { replace: true });
      return;
    }
    // Force password change before allowing access to protected pages.
    if (mustChangePassword()) {
      navigate("/change-password", { replace: true });
    }
  });

  return (
    <Show when={!initializing() && isAuthenticated() && !mustChangePassword()}>
      {props.children}
    </Show>
  );
}
