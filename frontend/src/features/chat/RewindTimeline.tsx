import type { Component } from "solid-js";
import { createSignal, For, Show } from "solid-js";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface StepEntry {
  stepId: string;
  name: string;
  timestamp?: string;
  status: "running" | "completed" | "failed" | "cancelled" | "skipped";
}

type RewindMode = "all" | "code_only" | "conversation_only";

interface RewindTimelineProps {
  conversationId: string;
  steps: StepEntry[];
  visible: boolean;
  onClose: () => void;
  onRewind: (stepId: string, mode: RewindMode) => void;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Map step status to a colored dot class. */
function statusDotClass(status: StepEntry["status"]): string {
  switch (status) {
    case "completed":
      return "bg-cf-success";
    case "failed":
      return "bg-cf-danger";
    case "running":
      return "bg-cf-info animate-pulse";
    case "cancelled":
    case "skipped":
      return "bg-cf-text-muted";
  }
}

/** Format an ISO timestamp string as HH:MM. */
function formatTime(timestamp: string): string {
  try {
    const date = new Date(timestamp);
    if (isNaN(date.getTime())) return "";
    const hours = String(date.getHours()).padStart(2, "0");
    const minutes = String(date.getMinutes()).padStart(2, "0");
    return `${hours}:${minutes}`;
  } catch {
    return "";
  }
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

const RewindTimeline: Component<RewindTimelineProps> = (props) => {
  const [confirmStepId, setConfirmStepId] = createSignal<string | null>(null);

  const handleStepClick = (stepId: string) => {
    setConfirmStepId((prev) => (prev === stepId ? null : stepId));
  };

  const handleRewind = (stepId: string, mode: RewindMode) => {
    setConfirmStepId(null);
    props.onRewind(stepId, mode);
  };

  const handleCancel = () => {
    setConfirmStepId(null);
  };

  return (
    <Show when={props.visible}>
      <div class="absolute inset-x-0 top-0 z-30 rounded-b-cf-md border-b border-cf-border bg-cf-bg-surface shadow-cf-md transition-all duration-200">
        {/* Header */}
        <div class="flex items-center justify-between px-4 py-2 border-b border-cf-border-subtle">
          <span class="text-sm font-semibold text-cf-text-primary">Rewind Timeline</span>
          <button
            type="button"
            class="flex h-6 w-6 items-center justify-center rounded-cf-sm text-cf-text-muted hover:text-cf-text-primary hover:bg-cf-bg-inset transition-colors"
            aria-label="Close timeline"
            onClick={() => props.onClose()}
          >
            &times;
          </button>
        </div>

        {/* Timeline entries */}
        <div class="flex items-start gap-1 overflow-x-auto px-4 py-3 scrollbar-none">
          <For each={props.steps}>
            {(step) => (
              <div class="relative flex flex-col items-center">
                {/* Step button */}
                <button
                  type="button"
                  class={`group flex flex-col items-center gap-1 rounded-cf-sm px-3 py-2 transition-colors hover:bg-cf-bg-inset ${
                    confirmStepId() === step.stepId ? "bg-cf-bg-inset" : ""
                  }`}
                  onClick={() => handleStepClick(step.stepId)}
                  aria-label={`Step: ${step.name}`}
                >
                  {/* Status dot */}
                  <span
                    class={`h-2.5 w-2.5 shrink-0 rounded-full ${statusDotClass(step.status)}`}
                    aria-hidden="true"
                  />
                  {/* Step name */}
                  <span class="max-w-[100px] truncate text-xs text-cf-text-secondary group-hover:text-cf-text-primary">
                    {step.name}
                  </span>
                  {/* Timestamp */}
                  <Show when={step.timestamp}>
                    {(ts) => <span class="text-[10px] text-cf-text-muted">{formatTime(ts())}</span>}
                  </Show>
                </button>

                {/* Confirmation popover */}
                <Show when={confirmStepId() === step.stepId}>
                  <div class="absolute top-full left-1/2 z-40 mt-1 -translate-x-1/2 rounded-cf-md border border-cf-border bg-cf-bg-surface p-3 shadow-cf-lg">
                    <p class="mb-2 whitespace-nowrap text-xs font-medium text-cf-text-primary">
                      Rewind to this step?
                    </p>
                    <div class="flex gap-1.5">
                      <button
                        type="button"
                        class="rounded-cf-sm bg-cf-accent px-2 py-1 text-[11px] font-medium text-cf-accent-fg hover:bg-cf-accent-hover transition-colors"
                        onClick={() => handleRewind(step.stepId, "all")}
                      >
                        Code + Chat
                      </button>
                      <button
                        type="button"
                        class="rounded-cf-sm border border-cf-border px-2 py-1 text-[11px] font-medium text-cf-text-secondary hover:bg-cf-bg-inset transition-colors"
                        onClick={() => handleRewind(step.stepId, "code_only")}
                      >
                        Code Only
                      </button>
                      <button
                        type="button"
                        class="rounded-cf-sm border border-cf-border px-2 py-1 text-[11px] font-medium text-cf-text-secondary hover:bg-cf-bg-inset transition-colors"
                        onClick={() => handleRewind(step.stepId, "conversation_only")}
                      >
                        Chat Only
                      </button>
                    </div>
                    <button
                      type="button"
                      class="mt-1.5 block w-full text-center text-[11px] text-cf-text-muted hover:text-cf-text-secondary transition-colors"
                      onClick={handleCancel}
                    >
                      Cancel
                    </button>
                  </div>
                </Show>
              </div>
            )}
          </For>

          {/* Empty state */}
          <Show when={props.steps.length === 0}>
            <p class="w-full py-2 text-center text-xs text-cf-text-muted">No steps recorded yet.</p>
          </Show>
        </div>
      </div>
    </Show>
  );
};

export default RewindTimeline;
