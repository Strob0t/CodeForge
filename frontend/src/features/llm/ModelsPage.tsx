import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { LLMModel } from "~/api/types";
import { useToast } from "~/components/Toast";

export default function ModelsPage() {
  const { show: toast } = useToast();
  const [models, { refetch }] = createResource(() => api.llm.models());
  const [health] = createResource(() => api.llm.health());
  const [showForm, setShowForm] = createSignal(false);
  const [modelName, setModelName] = createSignal("");
  const [litellmModel, setLitellmModel] = createSignal("");
  const [apiBase, setApiBase] = createSignal("");
  const [apiKey, setApiKey] = createSignal("");
  const [error, setError] = createSignal("");

  const handleAdd = async (e: SubmitEvent) => {
    e.preventDefault();
    if (!modelName().trim() || !litellmModel().trim()) return;

    setError("");
    try {
      const params: Record<string, string> = { model: litellmModel() };
      if (apiBase().trim()) params.api_base = apiBase();
      if (apiKey().trim()) params.api_key = apiKey();

      await api.llm.addModel({
        model_name: modelName(),
        litellm_params: params,
      });
      setModelName("");
      setLitellmModel("");
      setApiBase("");
      setApiKey("");
      setShowForm(false);
      refetch();
      toast("success", "Model added");
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to add model";
      setError(msg);
      toast("error", msg);
    }
  };

  const handleDelete = async (modelId: string) => {
    setError("");
    try {
      await api.llm.deleteModel(modelId);
      refetch();
      toast("success", "Model deleted");
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to delete model";
      setError(msg);
      toast("error", msg);
    }
  };

  return (
    <div>
      <div class="mb-6 flex items-center justify-between">
        <div class="flex items-center gap-3">
          <h2 class="text-2xl font-bold text-gray-900 dark:text-gray-100">LLM Models</h2>
          <Show when={health()}>
            <span
              class={`rounded-full px-2 py-0.5 text-xs ${
                health()?.status === "healthy"
                  ? "bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400"
                  : "bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400"
              }`}
            >
              LiteLLM: {health()?.status ?? "unknown"}
            </span>
          </Show>
        </div>
        <button
          class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
          onClick={() => setShowForm((v) => !v)}
        >
          {showForm() ? "Cancel" : "Add Model"}
        </button>
      </div>

      <Show when={error()}>
        <div class="mb-4 rounded-md bg-red-50 dark:bg-red-900/20 p-3 text-sm text-red-700 dark:text-red-400">
          {error()}
        </div>
      </Show>

      <Show when={showForm()}>
        <form
          onSubmit={handleAdd}
          class="mb-6 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-5"
        >
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
                Display Name *
              </label>
              <input
                type="text"
                value={modelName()}
                onInput={(e) => setModelName(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-700 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                placeholder="gpt-4o"
              />
            </div>
            <div>
              <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
                LiteLLM Model *
              </label>
              <input
                type="text"
                value={litellmModel()}
                onInput={(e) => setLitellmModel(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-700 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                placeholder="openai/gpt-4o"
              />
            </div>
            <div>
              <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
                API Base (optional)
              </label>
              <input
                type="text"
                value={apiBase()}
                onInput={(e) => setApiBase(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-700 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                placeholder="https://api.openai.com/v1"
              />
            </div>
            <div>
              <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
                API Key (optional)
              </label>
              <input
                type="password"
                value={apiKey()}
                onInput={(e) => setApiKey(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-700 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                placeholder="sk-..."
              />
            </div>
          </div>
          <div class="mt-4 flex justify-end">
            <button
              type="submit"
              class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
            >
              Add Model
            </button>
          </div>
        </form>
      </Show>

      <Show when={models.loading}>
        <p class="text-sm text-gray-500 dark:text-gray-400">Loading models...</p>
      </Show>

      <Show when={models.error}>
        <p class="text-sm text-red-500 dark:text-red-400">Failed to load models.</p>
      </Show>

      <Show when={!models.loading && !models.error}>
        <Show
          when={models()?.length}
          fallback={
            <p class="text-sm text-gray-500 dark:text-gray-400">
              No models configured. Click "Add Model" to get started.
            </p>
          }
        >
          <div class="grid grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-3">
            <For each={models() ?? []}>
              {(model) => <ModelCard model={model} onDelete={handleDelete} />}
            </For>
          </div>
        </Show>
      </Show>
    </div>
  );
}

interface ModelCardProps {
  model: LLMModel;
  onDelete: (id: string) => void;
}

function ModelCard(props: ModelCardProps) {
  return (
    <div class="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-5 shadow-sm dark:shadow-gray-900/30 transition-shadow hover:shadow-md dark:hover:shadow-gray-900/30">
      <div class="flex items-start justify-between">
        <div>
          <h3 class="text-lg font-semibold text-gray-900 dark:text-gray-100">
            {props.model.model_name}
          </h3>
          <Show when={props.model.litellm_provider}>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {props.model.litellm_provider}
            </p>
          </Show>
        </div>
        <Show when={props.model.model_id}>
          <button
            class="rounded px-2 py-1 text-sm text-red-500 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 hover:text-red-700 dark:hover:text-red-400"
            onClick={() => props.onDelete(props.model.model_id ?? "")}
          >
            Delete
          </button>
        </Show>
      </div>

      <div class="mt-3 flex flex-wrap gap-2 text-xs">
        <Show when={props.model.model_id}>
          <span class="rounded bg-gray-100 dark:bg-gray-700 px-2 py-0.5 font-mono text-gray-600 dark:text-gray-400">
            {props.model.model_id}
          </span>
        </Show>
        <Show when={props.model.model_info}>
          <For each={Object.entries(props.model.model_info ?? {})}>
            {([key, value]) => (
              <Show when={typeof value === "string" || typeof value === "number"}>
                <span class="rounded bg-blue-50 dark:bg-blue-900/30 px-2 py-0.5 text-blue-600 dark:text-blue-400">
                  {key}: {String(value)}
                </span>
              </Show>
            )}
          </For>
        </Show>
      </div>
    </div>
  );
}
