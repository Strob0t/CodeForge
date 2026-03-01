import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { DiscoveredModel, LLMModel } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useAsyncAction, useFormState } from "~/hooks";
import { useI18n } from "~/i18n";
import {
  Alert,
  Badge,
  Button,
  Card,
  EmptyState,
  ErrorBanner,
  FormField,
  Input,
  LoadingState,
  PageLayout,
} from "~/ui";

const MODEL_FORM_DEFAULTS = {
  modelName: "",
  litellmModel: "",
  apiBase: "",
  apiKey: "",
};

export default function ModelsPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [models, { refetch }] = createResource(() => api.llm.models());
  const [health] = createResource(() => api.llm.health());
  const [showForm, setShowForm] = createSignal(false);
  const [discoveredModels, setDiscoveredModels] = createSignal<DiscoveredModel[]>([]);
  const [showDiscovered, setShowDiscovered] = createSignal(false);

  const form = useFormState(MODEL_FORM_DEFAULTS);

  const {
    run: handleAdd,
    error,
    clearError,
  } = useAsyncAction(
    async () => {
      if (!form.state.modelName.trim() || !form.state.litellmModel.trim()) return;

      const params: Record<string, string> = { model: form.state.litellmModel };
      if (form.state.apiBase.trim()) params.api_base = form.state.apiBase;
      if (form.state.apiKey.trim()) params.api_key = form.state.apiKey;

      await api.llm.addModel({
        model_name: form.state.modelName,
        litellm_params: params,
      });
      form.reset();
      setShowForm(false);
      refetch();
      toast("success", t("models.toast.added"));
    },
    {
      onError: (err) => {
        const msg = err instanceof Error ? err.message : t("models.toast.addFailed");
        toast("error", msg);
      },
    },
  );

  const { run: handleDelete } = useAsyncAction(
    async (modelId: string) => {
      await api.llm.deleteModel(modelId);
      refetch();
      toast("success", t("models.toast.deleted"));
    },
    {
      onError: (err) => {
        const msg = err instanceof Error ? err.message : t("models.toast.deleteFailed");
        toast("error", msg);
      },
    },
  );

  const { run: handleDiscover, loading: discovering } = useAsyncAction(
    async () => {
      const result = await api.llm.discover();
      setDiscoveredModels(result.models);
      setShowDiscovered(true);
    },
    {
      onError: (err) => {
        const msg = err instanceof Error ? err.message : t("models.toast.discoverFailed");
        toast("error", msg);
      },
    },
  );

  return (
    <PageLayout
      title={t("models.title")}
      action={
        <div class="flex items-center gap-3">
          <Show when={health()}>
            <Badge variant={health()?.status === "healthy" ? "success" : "danger"} pill>
              LiteLLM: {health()?.status ?? "unknown"}
            </Badge>
          </Show>
          <Button
            variant="secondary"
            onClick={() => void handleDiscover()}
            disabled={discovering()}
          >
            {discovering() ? t("models.discovering") : t("models.discover")}
          </Button>
          <Button onClick={() => setShowForm((v) => !v)}>
            {showForm() ? t("common.cancel") : t("models.addModel")}
          </Button>
        </div>
      }
    >
      <ErrorBanner error={error} onDismiss={clearError} />

      <Show when={showForm()}>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void handleAdd();
          }}
          class="mb-6"
          aria-label="Add model"
        >
          <Card>
            <Card.Body>
              <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <FormField label={t("models.form.displayName")} id="model-display-name" required>
                  <Input
                    id="model-display-name"
                    type="text"
                    value={form.state.modelName}
                    onInput={(e) => form.setState("modelName", e.currentTarget.value)}
                    placeholder={t("models.form.namePlaceholder")}
                    aria-required="true"
                  />
                </FormField>
                <FormField label={t("models.form.litellmModel")} id="model-litellm-id" required>
                  <Input
                    id="model-litellm-id"
                    type="text"
                    value={form.state.litellmModel}
                    onInput={(e) => form.setState("litellmModel", e.currentTarget.value)}
                    placeholder={t("models.form.modelPlaceholder")}
                    aria-required="true"
                  />
                </FormField>
                <FormField label={t("models.form.apiBase")} id="model-api-base">
                  <Input
                    id="model-api-base"
                    type="text"
                    value={form.state.apiBase}
                    onInput={(e) => form.setState("apiBase", e.currentTarget.value)}
                    placeholder={t("models.form.apiBasePlaceholder")}
                  />
                </FormField>
                <FormField label={t("models.form.apiKey")} id="model-api-key">
                  <Input
                    id="model-api-key"
                    type="password"
                    value={form.state.apiKey}
                    onInput={(e) => form.setState("apiKey", e.currentTarget.value)}
                    placeholder={t("models.form.apiKeyPlaceholder")}
                  />
                </FormField>
              </div>
              <div class="mt-4 flex justify-end">
                <Button type="submit">{t("models.form.add")}</Button>
              </div>
            </Card.Body>
          </Card>
        </form>
      </Show>

      {/* Discovered Models Section */}
      <Show when={showDiscovered()}>
        <div class="mb-6">
          <div class="mb-3 flex items-center justify-between">
            <h2 class="text-lg font-semibold text-cf-text-primary">{t("models.discovered")}</h2>
            <div class="flex items-center gap-2">
              <Badge variant="info">
                {t("models.discoveredCount", { count: String(discoveredModels().length) })}
              </Badge>
              <Button variant="ghost" size="sm" onClick={() => setShowDiscovered(false)}>
                {t("common.close")}
              </Button>
            </div>
          </div>
          <Show
            when={discoveredModels().length > 0}
            fallback={<EmptyState title={t("models.discoveredEmpty")} />}
          >
            <div class="grid grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-3">
              <For each={discoveredModels()}>
                {(model) => <DiscoveredModelCard model={model} />}
              </For>
            </div>
          </Show>
        </div>
      </Show>

      <Show when={models.loading}>
        <LoadingState message={t("models.loading")} />
      </Show>

      <Show when={models.error}>
        <Alert variant="error">{t("models.loadError")}</Alert>
      </Show>

      <Show when={!models.loading && !models.error}>
        <Show when={models()?.length} fallback={<EmptyState title={t("models.empty")} />}>
          <div class="grid grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-3">
            <For each={models() ?? []}>
              {(model) => <ModelCard model={model} onDelete={handleDelete} />}
            </For>
          </div>
        </Show>
      </Show>
    </PageLayout>
  );
}

interface ModelCardProps {
  model: LLMModel;
  onDelete: (id: string) => Promise<void>;
}

function ModelCard(props: ModelCardProps) {
  const { t } = useI18n();
  return (
    <Card class="transition-shadow hover:shadow-md">
      <Card.Body>
        <div class="flex items-start justify-between">
          <div>
            <h3 class="text-lg font-semibold text-cf-text-primary">{props.model.model_name}</h3>
            <Show when={props.model.litellm_provider}>
              <p class="mt-1 text-sm text-cf-text-muted">{props.model.litellm_provider}</p>
            </Show>
          </div>
          <Show when={props.model.model_id}>
            <Button
              variant="danger"
              size="sm"
              onClick={() => void props.onDelete(props.model.model_id ?? "")}
              aria-label={t("models.deleteAria", { name: props.model.model_name })}
            >
              {t("common.delete")}
            </Button>
          </Show>
        </div>

        <div class="mt-3 flex flex-wrap gap-2 text-xs">
          <Show when={props.model.model_id}>
            <Badge variant="default">
              <span class="font-mono">{props.model.model_id}</span>
            </Badge>
          </Show>
          <Show when={props.model.model_info}>
            <For each={Object.entries(props.model.model_info ?? {})}>
              {([key, value]) => (
                <Show when={typeof value === "string" || typeof value === "number"}>
                  <Badge variant="info">
                    {key}: {String(value)}
                  </Badge>
                </Show>
              )}
            </For>
          </Show>
        </div>
      </Card.Body>
    </Card>
  );
}

interface DiscoveredModelCardProps {
  model: DiscoveredModel;
}

function DiscoveredModelCard(props: DiscoveredModelCardProps) {
  const { t } = useI18n();
  return (
    <Card class="transition-shadow hover:shadow-md">
      <Card.Body>
        <div class="flex items-start justify-between">
          <div>
            <h3 class="text-lg font-semibold text-cf-text-primary">{props.model.model_name}</h3>
            <Show when={props.model.provider}>
              <p class="mt-1 text-sm text-cf-text-muted">{props.model.provider}</p>
            </Show>
          </div>
          <div class="flex items-center gap-2">
            <Badge variant={props.model.status === "reachable" ? "success" : "danger"} pill>
              {props.model.status === "reachable"
                ? t("models.status.reachable")
                : t("models.status.unreachable")}
            </Badge>
            <Badge variant={props.model.source === "ollama" ? "warning" : "info"} pill>
              {props.model.source === "ollama"
                ? t("models.source.ollama")
                : t("models.source.litellm")}
            </Badge>
          </div>
        </div>

        <div class="mt-3 flex flex-wrap gap-2 text-xs">
          <Show when={props.model.model_id}>
            <Badge variant="default">
              <span class="font-mono">{props.model.model_id}</span>
            </Badge>
          </Show>
          <Show when={props.model.max_tokens}>
            <Badge variant="info">max_tokens: {props.model.max_tokens?.toLocaleString()}</Badge>
          </Show>
          <For each={props.model.tags ?? []}>{(tag) => <Badge variant="default">{tag}</Badge>}</For>
          <Show when={props.model.input_cost_per_token}>
            <Badge variant="info">
              in: ${((props.model.input_cost_per_token ?? 0) * 1_000_000).toFixed(2)}/M
            </Badge>
          </Show>
          <Show when={props.model.output_cost_per_token}>
            <Badge variant="info">
              out: ${((props.model.output_cost_per_token ?? 0) * 1_000_000).toFixed(2)}/M
            </Badge>
          </Show>
        </div>
      </Card.Body>
    </Card>
  );
}
