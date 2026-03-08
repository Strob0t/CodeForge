import { type JSX, Show } from "solid-js";

interface ResourceViewProps<T> {
  loading: boolean;
  data: T | undefined;
  loadingFallback?: JSX.Element;
  emptyFallback?: JSX.Element;
  children: (item: T) => JSX.Element;
}

export function ResourceView<T>(props: ResourceViewProps<T>) {
  return (
    <Show
      when={!props.loading}
      fallback={
        props.loadingFallback ?? <div class="py-8 text-center text-cf-text-muted">Loading...</div>
      }
    >
      <Show
        when={props.data}
        fallback={
          props.emptyFallback ?? <div class="py-8 text-center text-cf-text-muted">No data</div>
        }
      >
        {(item) => props.children(item())}
      </Show>
    </Show>
  );
}
