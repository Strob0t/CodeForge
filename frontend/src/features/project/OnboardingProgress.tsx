import { createMemo, createSignal, For, Show } from "solid-js";

import { useI18n } from "~/i18n";
import type { TranslationKey } from "~/i18n/en";

interface OnboardingStep {
  key: TranslationKey;
  done: boolean;
  action?: () => void;
}

interface OnboardingProgressProps {
  projectId: string;
  hasWorkspace: boolean;
  hasStack: boolean;
  hasGoals: boolean;
  hasRoadmap: boolean;
  hasRuns: boolean;
  onNavigate: (tab: string) => void;
}

export default function OnboardingProgress(props: OnboardingProgressProps) {
  const { t } = useI18n();

  const dismissKey = () => `codeforge:onboarding-dismissed:${props.projectId}`;
  const [dismissed, setDismissed] = createSignal(localStorage.getItem(dismissKey()) === "true");

  const steps = createMemo<OnboardingStep[]>(() => [
    { key: "onboarding.repoCloned", done: props.hasWorkspace },
    { key: "onboarding.stackDetected", done: props.hasStack },
    {
      key: "onboarding.goalsDefined",
      done: props.hasGoals,
      action: () => props.onNavigate("goals"),
    },
    {
      key: "onboarding.roadmapCreated",
      done: props.hasRoadmap,
      action: () => props.onNavigate("roadmap"),
    },
    {
      key: "onboarding.firstRun",
      done: props.hasRuns,
      action: () => props.onNavigate("chat"),
    },
  ]);

  const allDone = createMemo(() => steps().every((s) => s.done));

  function handleDismiss() {
    localStorage.setItem(dismissKey(), "true");
    setDismissed(true);
  }

  return (
    <Show when={!dismissed() && !allDone()}>
      <div class="flex items-center gap-3 px-4 py-2 border-b border-cf-border bg-cf-bg-secondary text-xs">
        <For each={steps()}>
          {(step, i) => (
            <>
              <Show when={i() > 0}>
                <span class="text-cf-text-muted">&rarr;</span>
              </Show>
              <button
                class={
                  "flex items-center gap-1 rounded px-1.5 py-0.5 transition-colors " +
                  (step.done
                    ? "text-green-600"
                    : step.action
                      ? "text-cf-text-muted hover:text-cf-accent cursor-pointer"
                      : "text-cf-text-muted cursor-default")
                }
                onClick={() => step.action?.()}
                disabled={step.done || !step.action}
              >
                <span>{step.done ? "\u2713" : "\u25CB"}</span>
                <span>{t(step.key)}</span>
              </button>
            </>
          )}
        </For>
        <button
          class="ml-auto text-cf-text-muted hover:text-cf-text-primary"
          onClick={handleDismiss}
          title={t("onboarding.dismiss")}
        >
          &times;
        </button>
      </div>
    </Show>
  );
}
