import { createSignal, For, type JSX, Show } from "solid-js";

import { api } from "~/api/client";
import type { DiscoveredModel } from "~/api/types";
import { useAsyncAction } from "~/hooks";
import { Alert, Button, FormField, Input } from "~/ui";

export interface StepProps {
  onNext: () => void;
  onBack?: () => void;
}

export default function ConfigureAIStep(props: StepProps): JSX.Element {
  const [models, setModels] = createSignal<DiscoveredModel[]>([]);
  const [manualModel, setManualModel] = createSignal("");
  const [addedModel, setAddedModel] = createSignal(false);

  const {
    run: handleDiscover,
    loading: discovering,
    error: discoverError,
  } = useAsyncAction(async () => {
    const result = await api.llm.discover();
    setModels(result.models);
  });

  const {
    run: handleAddModel,
    loading: adding,
    error: addError,
  } = useAsyncAction(async () => {
    await api.llm.addModel({
      model_name: manualModel().trim(),
      litellm_params: {},
    });
    setAddedModel(true);
    setManualModel("");
  });

  const onSkip = () => props.onNext();

  return (
    <div class="space-y-4">
      <Show when={discoverError()}>
        <Alert variant="error">{discoverError()}</Alert>
      </Show>
      <Show when={addError()}>
        <Alert variant="error">{addError()}</Alert>
      </Show>
      <Show when={addedModel()}>
        <Alert variant="success">Model added successfully.</Alert>
      </Show>

      <div class="flex items-center gap-3">
        <Button variant="secondary" onClick={() => void handleDiscover()} loading={discovering()}>
          Discover Models
        </Button>
      </div>

      <Show when={models().length > 0}>
        <div class="max-h-40 overflow-y-auto rounded-cf-sm border border-cf-border">
          <ul class="divide-y divide-cf-border">
            <For each={models()}>
              {(m) => (
                <li class="px-3 py-2 text-sm text-cf-text-primary">
                  <span class="font-medium">{m.model_name}</span>
                  <Show when={m.source}>
                    <span class="ml-2 text-xs text-cf-text-muted">({m.source})</span>
                  </Show>
                </li>
              )}
            </For>
          </ul>
        </div>
      </Show>

      <div class="border-t border-cf-border pt-3">
        <p class="mb-2 text-xs text-cf-text-muted">Or add a model manually:</p>
        <div class="flex items-center gap-2">
          <FormField label="Model name" id="onboard-model-name" class="flex-1">
            <Input
              id="onboard-model-name"
              type="text"
              value={manualModel()}
              onInput={(e) => setManualModel(e.currentTarget.value)}
              placeholder="e.g. openai/gpt-4o"
            />
          </FormField>
          <Button
            size="sm"
            class="mt-5"
            onClick={() => void handleAddModel()}
            loading={adding()}
            disabled={!manualModel().trim()}
          >
            Add
          </Button>
        </div>
      </div>

      <div class="flex items-center gap-3 pt-1">
        <button
          type="button"
          class="text-sm text-cf-text-muted hover:text-cf-text-secondary"
          onClick={onSkip}
        >
          Skip
        </button>
      </div>
    </div>
  );
}
