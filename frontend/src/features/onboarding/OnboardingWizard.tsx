import { createSignal, For, type JSX, Match, Show, Switch } from "solid-js";

import { Button } from "~/ui";
import { cx } from "~/utils/cx";

import ConfigureAIStep from "./steps/ConfigureAIStep";
import ConnectCodeStep from "./steps/ConnectCodeStep";
import CreateProjectStep from "./steps/CreateProjectStep";

const STEPS = [
  { title: "Connect your code", description: "Link a version control account" },
  { title: "Configure AI", description: "Set up your AI model provider" },
  { title: "Create a project", description: "Start your first project" },
] as const;

const STORAGE_KEY = "codeforge-onboarding-completed";

interface OnboardingWizardProps {
  onComplete: () => void;
}

export function OnboardingWizard(props: OnboardingWizardProps): JSX.Element {
  const [currentStep, setCurrentStep] = createSignal(0);

  const finish = () => {
    localStorage.setItem(STORAGE_KEY, "true");
    props.onComplete();
  };

  const handleNext = () => {
    if (currentStep() >= STEPS.length - 1) {
      finish();
    } else {
      setCurrentStep(currentStep() + 1);
    }
  };

  const handleBack = () => {
    if (currentStep() > 0) setCurrentStep(currentStep() - 1);
  };

  return (
    <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
      <div class="w-full max-w-lg rounded-cf-lg border border-cf-border bg-cf-bg-surface p-6 shadow-cf-lg">
        {/* Step indicator dots */}
        <div class="mb-6 flex items-center justify-center gap-2">
          <For each={[...STEPS]}>
            {(_, i) => (
              <div
                class={cx(
                  "h-2 w-2 rounded-full transition-colors",
                  i() === currentStep() ? "bg-cf-accent" : "bg-cf-border",
                )}
              />
            )}
          </For>
        </div>

        {/* Step title */}
        <h2 class="mb-1 text-center text-xl font-bold text-cf-text-primary">
          {STEPS[currentStep()].title}
        </h2>
        <p class="mb-6 text-center text-sm text-cf-text-muted">
          {STEPS[currentStep()].description}
        </p>

        {/* Step content */}
        <div class="min-h-[200px]">
          <Switch>
            <Match when={currentStep() === 0}>
              <ConnectCodeStep onNext={handleNext} />
            </Match>
            <Match when={currentStep() === 1}>
              <ConfigureAIStep onNext={handleNext} onBack={handleBack} />
            </Match>
            <Match when={currentStep() === 2}>
              <CreateProjectStep onNext={handleNext} onBack={handleBack} />
            </Match>
          </Switch>
        </div>

        {/* Navigation */}
        <div class="mt-6 flex items-center justify-between">
          <Show when={currentStep() > 0} fallback={<div />}>
            <Button variant="ghost" onClick={handleBack}>
              Back
            </Button>
          </Show>
          <div class="flex items-center gap-3">
            <button
              type="button"
              class="text-sm text-cf-text-muted hover:text-cf-text-secondary"
              onClick={finish}
            >
              Skip setup
            </button>
            <Button variant="primary" onClick={handleNext}>
              {currentStep() >= STEPS.length - 1 ? "Finish" : "Next"}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
