import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { CreateMCPServerRequest, MCPServer, MCPServerTool } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";

// ---------------------------------------------------------------------------
// MCP Servers Page
// ---------------------------------------------------------------------------

export default function MCPServersPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [servers, { refetch }] = createResource(() => api.mcp.listServers());
  const [showForm, setShowForm] = createSignal(false);
  const [editingId, setEditingId] = createSignal<string | null>(null);
  const [error, setError] = createSignal("");

  // -- Form state --
  const [formName, setFormName] = createSignal("");
  const [formDesc, setFormDesc] = createSignal("");
  const [formTransport, setFormTransport] = createSignal<"stdio" | "sse">("stdio");
  const [formCommand, setFormCommand] = createSignal("");
  const [formArgs, setFormArgs] = createSignal("");
  const [formUrl, setFormUrl] = createSignal("");
  const [formEnv, setFormEnv] = createSignal<{ key: string; value: string }[]>([]);
  const [formEnabled, setFormEnabled] = createSignal(true);

  function isEditing(): boolean {
    return editingId() !== null;
  }

  function resetForm(): void {
    setFormName("");
    setFormDesc("");
    setFormTransport("stdio");
    setFormCommand("");
    setFormArgs("");
    setFormUrl("");
    setFormEnv([]);
    setFormEnabled(true);
    setEditingId(null);
  }

  function handleCancelForm(): void {
    setShowForm(false);
    resetForm();
    setError("");
  }

  function handleEdit(server: MCPServer): void {
    setFormName(server.name);
    setFormDesc(server.description);
    setFormTransport(server.transport);
    setFormCommand(server.command);
    setFormArgs((server.args ?? []).join("\n"));
    setFormUrl(server.url);
    const envEntries = Object.entries(server.env ?? {}).map(([key, value]) => ({ key, value }));
    setFormEnv(envEntries);
    setFormEnabled(server.enabled);
    setEditingId(server.id);
    setShowForm(true);
  }

  function addEnvRow(): void {
    setFormEnv([...formEnv(), { key: "", value: "" }]);
  }

  function removeEnvRow(index: number): void {
    setFormEnv(formEnv().filter((_, i) => i !== index));
  }

  function updateEnvRow(index: number, field: "key" | "value", val: string): void {
    setFormEnv(formEnv().map((row, i) => (i === index ? { ...row, [field]: val } : row)));
  }

  const handleSubmit = async (e: SubmitEvent) => {
    e.preventDefault();
    const name = formName().trim();
    if (!name) {
      toast("error", t("mcp.toast.nameRequired"));
      return;
    }
    setError("");
    try {
      const envObj: Record<string, string> = {};
      for (const row of formEnv()) {
        const k = row.key.trim();
        if (k) envObj[k] = row.value;
      }

      const req: CreateMCPServerRequest = {
        name,
        description: formDesc().trim() || undefined,
        transport: formTransport(),
        command: formTransport() === "stdio" ? formCommand().trim() || undefined : undefined,
        args:
          formTransport() === "stdio"
            ? formArgs()
                .split("\n")
                .map((s) => s.trim())
                .filter(Boolean)
            : undefined,
        url: formTransport() === "sse" ? formUrl().trim() || undefined : undefined,
        env: Object.keys(envObj).length > 0 ? envObj : undefined,
        enabled: formEnabled(),
      };

      const eid = editingId();
      if (isEditing() && eid) {
        await api.mcp.updateServer(eid, req);
        toast("success", t("mcp.toast.updated"));
      } else {
        await api.mcp.createServer(req);
        toast("success", t("mcp.toast.created"));
      }
      resetForm();
      setShowForm(false);
      refetch();
    } catch (err) {
      const msg =
        err instanceof Error
          ? err.message
          : isEditing()
            ? t("mcp.toast.updateFailed")
            : t("mcp.toast.createFailed");
      setError(msg);
      toast("error", msg);
    }
  };

  const handleDelete = async (server: MCPServer) => {
    if (!confirm(t("mcp.deleteConfirm"))) return;
    try {
      await api.mcp.deleteServer(server.id);
      toast("success", t("mcp.toast.deleted"));
      refetch();
    } catch {
      toast("error", t("mcp.toast.deleteFailed"));
    }
  };

  return (
    <div>
      <div class="mb-6 flex items-center justify-between">
        <div>
          <h2 class="text-2xl font-bold text-gray-900 dark:text-gray-100">{t("mcp.title")}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{t("mcp.description")}</p>
        </div>
        <button
          type="button"
          class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
          onClick={() => {
            if (showForm()) {
              handleCancelForm();
            } else {
              setShowForm(true);
            }
          }}
        >
          {showForm() ? t("common.cancel") : t("mcp.addServer")}
        </button>
      </div>

      <Show when={error()}>
        <div
          class="mb-4 rounded-md bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-400"
          role="alert"
        >
          {error()}
        </div>
      </Show>

      {/* Add / Edit Form */}
      <Show when={showForm()}>
        <form
          onSubmit={handleSubmit}
          class="mb-6 rounded-lg border border-gray-200 bg-white p-5 dark:border-gray-700 dark:bg-gray-800"
          aria-label={isEditing() ? t("mcp.editServer") : t("mcp.addServer")}
        >
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            {/* Name */}
            <div>
              <label
                for="mcp-name"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("mcp.form.name")} <span aria-hidden="true">*</span>
                <span class="sr-only">(required)</span>
              </label>
              <input
                id="mcp-name"
                type="text"
                value={formName()}
                onInput={(e) => setFormName(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
                placeholder={t("mcp.form.namePlaceholder")}
                aria-required="true"
              />
            </div>

            {/* Transport */}
            <div>
              <label
                for="mcp-transport"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("mcp.form.transport")}
              </label>
              <select
                id="mcp-transport"
                value={formTransport()}
                onChange={(e) => setFormTransport(e.currentTarget.value as "stdio" | "sse")}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
              >
                <option value="stdio">{t("mcp.transport.stdio")}</option>
                <option value="sse">{t("mcp.transport.sse")}</option>
              </select>
            </div>

            {/* Description */}
            <div class="sm:col-span-2">
              <label
                for="mcp-desc"
                class="block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                {t("mcp.form.description")}
              </label>
              <input
                id="mcp-desc"
                type="text"
                value={formDesc()}
                onInput={(e) => setFormDesc(e.currentTarget.value)}
                class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
                placeholder={t("mcp.form.descriptionPlaceholder")}
              />
            </div>

            {/* Command (stdio only) */}
            <Show when={formTransport() === "stdio"}>
              <div class="sm:col-span-2">
                <label
                  for="mcp-command"
                  class="block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {t("mcp.form.command")}
                </label>
                <input
                  id="mcp-command"
                  type="text"
                  value={formCommand()}
                  onInput={(e) => setFormCommand(e.currentTarget.value)}
                  class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm font-mono focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
                  placeholder={t("mcp.form.commandPlaceholder")}
                />
              </div>
              <div class="sm:col-span-2">
                <label
                  for="mcp-args"
                  class="block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {t("mcp.form.args")}
                </label>
                <textarea
                  id="mcp-args"
                  value={formArgs()}
                  onInput={(e) => setFormArgs(e.currentTarget.value)}
                  rows={3}
                  class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm font-mono focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
                  placeholder={t("mcp.form.argsPlaceholder")}
                />
              </div>
            </Show>

            {/* URL (sse only) */}
            <Show when={formTransport() === "sse"}>
              <div class="sm:col-span-2">
                <label
                  for="mcp-url"
                  class="block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {t("mcp.form.url")}
                </label>
                <input
                  id="mcp-url"
                  type="text"
                  value={formUrl()}
                  onInput={(e) => setFormUrl(e.currentTarget.value)}
                  class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm font-mono focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
                  placeholder={t("mcp.form.urlPlaceholder")}
                />
              </div>
            </Show>

            {/* Environment Variables */}
            <div class="sm:col-span-2">
              <div class="mb-2 flex items-center justify-between">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {t("mcp.form.env")}
                </span>
                <button
                  type="button"
                  class="text-xs text-blue-600 hover:underline dark:text-blue-400"
                  onClick={addEnvRow}
                >
                  {t("mcp.form.addEnv")}
                </button>
              </div>
              <For each={formEnv()}>
                {(row, index) => (
                  <div class="mb-2 flex gap-2">
                    <input
                      type="text"
                      value={row.key}
                      onInput={(e) => updateEnvRow(index(), "key", e.currentTarget.value)}
                      class="flex-1 rounded-md border border-gray-300 px-3 py-2 text-sm font-mono focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
                      placeholder={t("mcp.form.envKey")}
                      aria-label={`${t("mcp.form.envKey")} ${index() + 1}`}
                    />
                    <input
                      type="text"
                      value={row.value}
                      onInput={(e) => updateEnvRow(index(), "value", e.currentTarget.value)}
                      class="flex-1 rounded-md border border-gray-300 px-3 py-2 text-sm font-mono focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700"
                      placeholder={t("mcp.form.envValue")}
                      aria-label={`${t("mcp.form.envValue")} ${index() + 1}`}
                    />
                    <button
                      type="button"
                      class="rounded-md px-2 py-1 text-xs text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
                      onClick={() => removeEnvRow(index())}
                      aria-label={`Remove variable ${index() + 1}`}
                    >
                      {t("common.delete")}
                    </button>
                  </div>
                )}
              </For>
            </div>

            {/* Enabled toggle */}
            <div class="flex items-center gap-3 sm:col-span-2">
              <input
                id="mcp-enabled"
                type="checkbox"
                checked={formEnabled()}
                onChange={(e) => setFormEnabled(e.currentTarget.checked)}
                class="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              <label for="mcp-enabled" class="text-sm font-medium text-gray-700 dark:text-gray-300">
                {t("mcp.form.enabled")}
              </label>
            </div>
          </div>

          <div class="mt-4 flex justify-end gap-2">
            <button
              type="button"
              class="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
              onClick={handleCancelForm}
            >
              {t("common.cancel")}
            </button>
            <button
              type="submit"
              class="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
            >
              {isEditing() ? t("mcp.form.update") : t("mcp.form.create")}
            </button>
          </div>
        </form>
      </Show>

      {/* Loading state */}
      <Show when={servers.loading}>
        <p class="text-sm text-gray-500 dark:text-gray-400">{t("mcp.loading")}</p>
      </Show>

      {/* Error state */}
      <Show when={servers.error}>
        <p class="text-sm text-red-500 dark:text-red-400">{t("mcp.loadError")}</p>
      </Show>

      {/* Server list */}
      <Show when={!servers.loading && !servers.error}>
        <Show
          when={(servers() ?? []).length > 0}
          fallback={<p class="text-sm text-gray-500 dark:text-gray-400">{t("mcp.empty")}</p>}
        >
          <div class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800">
            <table class="w-full text-left text-sm">
              <thead>
                <tr class="border-b border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-800/50">
                  <th scope="col" class="px-4 py-3 font-medium text-gray-700 dark:text-gray-300">
                    {t("mcp.table.name")}
                  </th>
                  <th scope="col" class="px-4 py-3 font-medium text-gray-700 dark:text-gray-300">
                    {t("mcp.table.transport")}
                  </th>
                  <th scope="col" class="px-4 py-3 font-medium text-gray-700 dark:text-gray-300">
                    {t("mcp.table.status")}
                  </th>
                  <th scope="col" class="px-4 py-3 font-medium text-gray-700 dark:text-gray-300">
                    {t("mcp.table.enabled")}
                  </th>
                  <th scope="col" class="px-4 py-3 font-medium text-gray-700 dark:text-gray-300">
                    {t("mcp.table.actions")}
                  </th>
                </tr>
              </thead>
              <tbody>
                <For each={servers() ?? []}>
                  {(server) => (
                    <MCPServerRow
                      server={server}
                      onEdit={handleEdit}
                      onDelete={handleDelete}
                      onRefetch={refetch}
                    />
                  )}
                </For>
              </tbody>
            </table>
          </div>
        </Show>
      </Show>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Status badge component
// ---------------------------------------------------------------------------

function StatusBadge(props: { status: MCPServer["status"] }) {
  const { t } = useI18n();

  const config = () => {
    switch (props.status) {
      case "connected":
        return {
          label: t("mcp.status.connected"),
          classes: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
        };
      case "disconnected":
        return {
          label: t("mcp.status.disconnected"),
          classes: "bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400",
        };
      case "error":
        return {
          label: t("mcp.status.error"),
          classes: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
        };
      case "registered":
      default:
        return {
          label: t("mcp.status.registered"),
          classes: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
        };
    }
  };

  return (
    <span class={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${config().classes}`}>
      {config().label}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Server table row with expandable tools section
// ---------------------------------------------------------------------------

function MCPServerRow(props: {
  server: MCPServer;
  onEdit: (server: MCPServer) => void;
  onDelete: (server: MCPServer) => void;
  onRefetch: () => void;
}) {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [testing, setTesting] = createSignal(false);
  const [showTools, setShowTools] = createSignal(false);
  const [tools, setTools] = createSignal<MCPServerTool[] | null>(null);
  const [toolsLoading, setToolsLoading] = createSignal(false);

  const handleTest = async () => {
    setTesting(true);
    try {
      await api.mcp.testServer(props.server.id);
      toast("success", t("mcp.testSuccess"));
      props.onRefetch();
    } catch {
      toast("error", t("mcp.testFailed"));
    } finally {
      setTesting(false);
    }
  };

  const handleToggleTools = async () => {
    if (showTools()) {
      setShowTools(false);
      return;
    }
    if (tools() === null) {
      setToolsLoading(true);
      try {
        const result = await api.mcp.listTools(props.server.id);
        setTools(result);
      } catch {
        setTools([]);
      } finally {
        setToolsLoading(false);
      }
    }
    setShowTools(true);
  };

  return (
    <>
      <tr class="border-b border-gray-100 dark:border-gray-700/50">
        <td class="px-4 py-3">
          <div>
            <span class="font-medium text-gray-900 dark:text-gray-100">{props.server.name}</span>
            <Show when={props.server.description}>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                {props.server.description}
              </p>
            </Show>
          </div>
        </td>
        <td class="px-4 py-3">
          <span class="rounded bg-gray-100 px-1.5 py-0.5 font-mono text-xs text-gray-600 dark:bg-gray-700 dark:text-gray-400">
            {props.server.transport}
          </span>
        </td>
        <td class="px-4 py-3">
          <StatusBadge status={props.server.status} />
        </td>
        <td class="px-4 py-3">
          <span
            class={`text-xs font-medium ${
              props.server.enabled
                ? "text-green-600 dark:text-green-400"
                : "text-gray-400 dark:text-gray-500"
            }`}
          >
            {props.server.enabled ? t("mcp.table.enabled") : "Disabled"}
          </span>
        </td>
        <td class="px-4 py-3">
          <div class="flex items-center gap-2">
            <button
              type="button"
              class="rounded border border-gray-300 px-2 py-1 text-xs text-gray-600 hover:bg-gray-50 disabled:opacity-50 dark:border-gray-600 dark:text-gray-400 dark:hover:bg-gray-700"
              onClick={handleTest}
              disabled={testing()}
              aria-label={t("mcp.testAria", { name: props.server.name })}
            >
              {testing() ? t("mcp.testing") : t("mcp.test")}
            </button>
            <button
              type="button"
              class="rounded px-2 py-1 text-xs text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
              onClick={handleToggleTools}
              aria-label={
                showTools()
                  ? t("mcp.tools.hideToolsAria", { name: props.server.name })
                  : t("mcp.tools.showToolsAria", { name: props.server.name })
              }
            >
              {showTools() ? t("mcp.tools.hideTools") : t("mcp.tools.showTools")}
            </button>
            <button
              type="button"
              class="rounded px-2 py-1 text-xs text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
              onClick={() => props.onEdit(props.server)}
              aria-label={t("mcp.editAria", { name: props.server.name })}
            >
              {t("mcp.editServer")}
            </button>
            <button
              type="button"
              class="text-xs text-red-600 hover:underline dark:text-red-400"
              onClick={() => props.onDelete(props.server)}
              aria-label={t("mcp.deleteAria", { name: props.server.name })}
            >
              {t("common.delete")}
            </button>
          </div>
        </td>
      </tr>

      {/* Expandable tools row */}
      <Show when={showTools()}>
        <tr class="border-b border-gray-100 dark:border-gray-700/50">
          <td colspan="5" class="bg-gray-50 px-4 py-3 dark:bg-gray-800/50">
            <div class="ml-4">
              <h4 class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
                {t("mcp.tools")}
              </h4>
              <Show when={toolsLoading()}>
                <p class="text-xs text-gray-500 dark:text-gray-400">{t("mcp.tools.loading")}</p>
              </Show>
              <Show when={!toolsLoading()}>
                <Show
                  when={(tools() ?? []).length > 0}
                  fallback={
                    <p class="text-xs text-gray-500 dark:text-gray-400">{t("mcp.tools.empty")}</p>
                  }
                >
                  <div class="space-y-2">
                    <For each={tools() ?? []}>{(tool) => <ToolCard tool={tool} />}</For>
                  </div>
                </Show>
              </Show>
            </div>
          </td>
        </tr>
      </Show>
    </>
  );
}

// ---------------------------------------------------------------------------
// Tool card within expandable row
// ---------------------------------------------------------------------------

function ToolCard(props: { tool: MCPServerTool }) {
  const { t } = useI18n();
  const [showSchema, setShowSchema] = createSignal(false);

  return (
    <div class="rounded border border-gray-200 bg-white p-3 dark:border-gray-600 dark:bg-gray-700">
      <div class="flex items-start justify-between">
        <div>
          <span class="font-mono text-sm font-medium text-gray-900 dark:text-gray-100">
            {props.tool.name}
          </span>
          <Show when={props.tool.description}>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{props.tool.description}</p>
          </Show>
        </div>
        <Show when={props.tool.input_schema && Object.keys(props.tool.input_schema).length > 0}>
          <button
            type="button"
            class="text-xs text-blue-600 hover:underline dark:text-blue-400"
            onClick={() => setShowSchema((v) => !v)}
          >
            {showSchema() ? t("common.close") : t("mcp.tools.inputSchema")}
          </button>
        </Show>
      </div>
      <Show when={showSchema()}>
        <pre class="mt-2 max-h-48 overflow-auto rounded bg-gray-50 p-2 text-xs text-gray-600 dark:bg-gray-900 dark:text-gray-400">
          {JSON.stringify(props.tool.input_schema, null, 2)}
        </pre>
      </Show>
    </div>
  );
}
