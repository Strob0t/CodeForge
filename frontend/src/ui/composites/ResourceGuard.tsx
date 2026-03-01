import type { JSX, Resource } from "solid-js";
import { Show } from "solid-js";

import { Alert } from "../primitives/Alert";
import { LoadingState } from "./LoadingState";

interface ResourceGuardProps<T> {
  resource: Resource<T>;
  loadingMessage?: string;
  errorMessage?: string;
  children: (data: () => NonNullable<T>) => JSX.Element;
}

/**
 * Replaces the loading/error/data Show nesting pattern used across pages.
 *
 * Usage:
 * ```tsx
 * <ResourceGuard resource={models} loadingMessage="Loading..." errorMessage="Failed to load">
 *   {(data) => <For each={data()}>{...}</For>}
 * </ResourceGuard>
 * ```
 */
export function ResourceGuard<T>(props: ResourceGuardProps<T>): JSX.Element {
  return (
    <>
      <Show when={props.resource.loading}>
        <LoadingState message={props.loadingMessage} />
      </Show>

      <Show when={props.resource.error}>
        <Alert variant="error">{props.errorMessage ?? "An error occurred"}</Alert>
      </Show>

      <Show when={!props.resource.loading && !props.resource.error && props.resource()}>
        {(data) => props.children(data as () => NonNullable<T>)}
      </Show>
    </>
  );
}
