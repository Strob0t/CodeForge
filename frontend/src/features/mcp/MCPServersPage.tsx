import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { CreateMCPServerRequest, MCPServer, MCPServerTool, MCPTestResult } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useAsyncAction, useConfirmAction, useFormState } from "~/hooks";
import { useI18n } from "~/i18n";
import {
  Alert,
  Badge,
  Button,
  Card,
  Checkbox,
  ConfirmDialog,
  EmptyState,
  ErrorBanner,
  FormField,
  Input,
  LoadingState,
  PageLayout,
  Select,
  Table,
  Textarea,
} from "~/ui";
import type { TableColumn } from "~/ui/composites/Table";

// ---------------------------------------------------------------------------
// MCP Servers Page
// ---------------------------------------------------------------------------

interface MCPFormState {
  name: string;
  desc: string;
  transport: "stdio" | "sse" | "streamable_http";
  command: string;
  args: string;
  url: string;
  env: { key: string; value: string }[];
  enabled: boolean;
}

const FORM_DEFAULTS: MCPFormState = {
  name: "",
  desc: "",
  transport: "stdio",
  command: "",
  args: "",
  url: "",
  env: [],
  enabled: true,
};

export default function MCPServersPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [servers, { refetch }] = createResource(() => api.mcp.listServers());
  const [showForm, setShowForm] = createSignal(false);
  const [editingId, setEditingId] = createSignal<string | null>(null);

  // Pre-save test state
  const [testFailError, setTestFailError] = createSignal("");
  const [pendingRequest, setPendingRequest] = createSignal<CreateMCPServerRequest | null>(null);

  const form = useFormState(FORM_DEFAULTS);

  const del = useConfirmAction(async (server: MCPServer) => {
    await api.mcp.deleteServer(server.id);
    toast("success", t("mcp.toast.deleted"));
    refetch();
  });

  function isEditing(): boolean {
    return editingId() !== null;
  }

  function handleCancelForm(): void {
    setShowForm(false);
    form.reset();
    setEditingId(null);
    clearError();
  }

  function handleEdit(server: MCPServer): void {
    const envEntries = Object.entries(server.env ?? {}).map(([key, value]) => ({ key, value }));
    form.populate({
      name: server.name,
      desc: server.description,
      transport: server.transport,
      command: server.command,
      args: (server.args ?? []).join("\n"),
      url: server.url,
      env: envEntries,
      enabled: server.enabled,
    });
    setEditingId(server.id);
    setShowForm(true);
  }

  function addEnvRow(): void {
    form.setState("env", [...form.state.env, { key: "", value: "" }]);
  }

  function removeEnvRow(index: number): void {
    form.setState(
      "env",
      form.state.env.filter((_, i) => i !== index),
    );
  }

  function updateEnvRow(index: number, field: "key" | "value", val: string): void {
    form.setState(
      "env",
      form.state.env.map((row, i) => (i === index ? { ...row, [field]: val } : row)),
    );
  }

  function buildRequest(): CreateMCPServerRequest | null {
    const name = form.state.name.trim();
    if (!name) {
      toast("error", t("mcp.toast.nameRequired"));
      return null;
    }
    const envObj: Record<string, string> = {};
    for (const row of form.state.env) {
      const k = row.key.trim();
      if (k) envObj[k] = row.value;
    }
    return {
      name,
      description: form.state.desc.trim() || undefined,
      transport: form.state.transport,
      command:
        form.state.transport === "stdio" ? form.state.command.trim() || undefined : undefined,
      args:
        form.state.transport === "stdio"
          ? form.state.args
              .split("\n")
              .map((s) => s.trim())
              .filter(Boolean)
          : undefined,
      url:
        form.state.transport === "sse" || form.state.transport === "streamable_http"
          ? form.state.url.trim() || undefined
          : undefined,
      env: Object.keys(envObj).length > 0 ? envObj : undefined,
      enabled: form.state.enabled,
    };
  }

  async function saveServer(req: CreateMCPServerRequest, toolCount?: number): Promise<void> {
    const eid = editingId();
    if (isEditing() && eid) {
      await api.mcp.updateServer(eid, req);
      toast("success", t("mcp.toast.updated"));
    } else {
      await api.mcp.createServer(req);
      const msg = toolCount
        ? t("mcp.toast.createdWithTools", { count: String(toolCount) })
        : t("mcp.toast.created");
      toast("success", msg);
    }
    form.reset();
    setEditingId(null);
    setShowForm(false);
    refetch();
  }

  const {
    run: handleSubmit,
    loading: testingConnection,
    error,
    clearError,
  } = useAsyncAction(
    async () => {
      const req = buildRequest();
      if (!req) return;

      // Pre-save connection test.
      let testResult: MCPTestResult | null = null;
      try {
        testResult = await api.mcp.testConnection(req);
      } catch {
        testResult = { success: false, error: t("mcp.testFailed") };
      }

      if (testResult.success) {
        await saveServer(req, testResult.tools?.length);
      } else {
        // Test failed — show confirmation dialog.
        setPendingRequest(req);
        setTestFailError(testResult.error ?? t("mcp.testFailed"));
      }
    },
    {
      onError: (err) => {
        const msg =
          err instanceof Error
            ? err.message
            : isEditing()
              ? t("mcp.toast.updateFailed")
              : t("mcp.toast.createFailed");
        toast("error", msg);
      },
    },
  );

  const { run: handleTestFailConfirm } = useAsyncAction(
    async () => {
      const req = pendingRequest();
      if (!req) return;
      setPendingRequest(null);
      setTestFailError("");
      await saveServer(req);
    },
    {
      onError: (err) => {
        const msg = err instanceof Error ? err.message : t("mcp.toast.createFailed");
        toast("error", msg);
      },
    },
  );

  const handleTestFailCancel = () => {
    setPendingRequest(null);
    setTestFailError("");
  };

  const serverColumns: TableColumn<MCPServer>[] = [
    {
      key: "name",
      header: t("mcp.table.name"),
      render: (server) => (
        <div>
          <span class="font-medium text-cf-text-primary">{server.name}</span>
          <Show when={server.description}>
            <p class="mt-0.5 text-xs text-cf-text-muted">{server.description}</p>
          </Show>
        </div>
      ),
    },
    {
      key: "transport",
      header: t("mcp.table.transport"),
      render: (server) => <Badge class="font-mono">{server.transport}</Badge>,
    },
    {
      key: "status",
      header: t("mcp.table.status"),
      render: (server) => <StatusBadge status={server.status} />,
    },
    {
      key: "enabled",
      header: t("mcp.table.enabled"),
      render: (server) => (
        <Badge variant={server.enabled ? "success" : "default"}>
          {server.enabled ? t("mcp.table.enabled") : "Disabled"}
        </Badge>
      ),
    },
    {
      key: "actions",
      header: t("mcp.table.actions"),
      render: (server) => (
        <MCPServerActions
          server={server}
          onEdit={handleEdit}
          onDelete={(s) => del.requestConfirm(s)}
          onRefetch={refetch}
        />
      ),
    },
  ];

  return (
    <PageLayout
      title={t("mcp.title")}
      description={t("mcp.description")}
      action={
        <Button
          variant={showForm() ? "secondary" : "primary"}
          onClick={() => {
            if (showForm()) {
              handleCancelForm();
            } else {
              setShowForm(true);
            }
          }}
        >
          {showForm() ? t("common.cancel") : t("mcp.addServer")}
        </Button>
      }
    >
      <ErrorBanner error={error} onDismiss={clearError} />

      {/* Add / Edit Form */}
      <Show when={showForm()}>
        <Card class="mb-6">
          <Card.Body>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void handleSubmit();
              }}
              aria-label={isEditing() ? t("mcp.editServer") : t("mcp.addServer")}
            >
              <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* Name */}
                <FormField label={t("mcp.form.name")} id="mcp-name" required>
                  <Input
                    id="mcp-name"
                    type="text"
                    value={form.state.name}
                    onInput={(e) => form.setState("name", e.currentTarget.value)}
                    placeholder={t("mcp.form.namePlaceholder")}
                    aria-required="true"
                  />
                </FormField>

                {/* Transport */}
                <FormField label={t("mcp.form.transport")} id="mcp-transport">
                  <Select
                    id="mcp-transport"
                    value={form.state.transport}
                    onChange={(e) =>
                      form.setState(
                        "transport",
                        e.currentTarget.value as "stdio" | "sse" | "streamable_http",
                      )
                    }
                  >
                    <option value="stdio">{t("mcp.transport.stdio")}</option>
                    <option value="sse">{t("mcp.transport.sse")}</option>
                    <option value="streamable_http">{t("mcp.transport.streamable_http")}</option>
                  </Select>
                </FormField>

                {/* Description */}
                <FormField label={t("mcp.form.description")} id="mcp-desc" class="sm:col-span-2">
                  <Input
                    id="mcp-desc"
                    type="text"
                    value={form.state.desc}
                    onInput={(e) => form.setState("desc", e.currentTarget.value)}
                    placeholder={t("mcp.form.descriptionPlaceholder")}
                  />
                </FormField>

                {/* Command (stdio only) */}
                <Show when={form.state.transport === "stdio"}>
                  <FormField label={t("mcp.form.command")} id="mcp-command" class="sm:col-span-2">
                    <Input
                      id="mcp-command"
                      type="text"
                      value={form.state.command}
                      onInput={(e) => form.setState("command", e.currentTarget.value)}
                      mono
                      placeholder={t("mcp.form.commandPlaceholder")}
                    />
                  </FormField>
                  <FormField label={t("mcp.form.args")} id="mcp-args" class="sm:col-span-2">
                    <Textarea
                      id="mcp-args"
                      value={form.state.args}
                      onInput={(e) => form.setState("args", e.currentTarget.value)}
                      rows={3}
                      mono
                      placeholder={t("mcp.form.argsPlaceholder")}
                    />
                  </FormField>
                </Show>

                {/* URL (sse / streamable_http) */}
                <Show
                  when={
                    form.state.transport === "sse" || form.state.transport === "streamable_http"
                  }
                >
                  <FormField label={t("mcp.form.url")} id="mcp-url" class="sm:col-span-2">
                    <Input
                      id="mcp-url"
                      type="text"
                      value={form.state.url}
                      onInput={(e) => form.setState("url", e.currentTarget.value)}
                      mono
                      placeholder={t("mcp.form.urlPlaceholder")}
                    />
                  </FormField>
                </Show>

                {/* Environment Variables */}
                <div class="sm:col-span-2">
                  <div class="mb-2 flex items-center justify-between">
                    <span class="text-sm font-medium text-cf-text-secondary">
                      {t("mcp.form.env")}
                    </span>
                    <Button variant="ghost" size="sm" onClick={addEnvRow}>
                      {t("mcp.form.addEnv")}
                    </Button>
                  </div>
                  <For each={form.state.env}>
                    {(row, index) => (
                      <div class="mb-2 flex gap-2">
                        <Input
                          type="text"
                          value={row.key}
                          onInput={(e) => updateEnvRow(index(), "key", e.currentTarget.value)}
                          mono
                          placeholder={t("mcp.form.envKey")}
                          aria-label={`${t("mcp.form.envKey")} ${index() + 1}`}
                        />
                        <Input
                          type="text"
                          value={row.value}
                          onInput={(e) => updateEnvRow(index(), "value", e.currentTarget.value)}
                          mono
                          placeholder={t("mcp.form.envValue")}
                          aria-label={`${t("mcp.form.envValue")} ${index() + 1}`}
                        />
                        <Button
                          variant="danger"
                          size="sm"
                          onClick={() => removeEnvRow(index())}
                          aria-label={`Remove variable ${index() + 1}`}
                        >
                          {t("common.delete")}
                        </Button>
                      </div>
                    )}
                  </For>
                </div>

                {/* Enabled toggle */}
                <div class="flex items-center gap-3 sm:col-span-2">
                  <Checkbox
                    id="mcp-enabled"
                    checked={form.state.enabled}
                    onChange={(checked) => form.setState("enabled", checked)}
                  />
                  <label for="mcp-enabled" class="text-sm font-medium text-cf-text-secondary">
                    {t("mcp.form.enabled")}
                  </label>
                </div>
              </div>

              <div class="mt-4 flex justify-end gap-2">
                <Button variant="secondary" onClick={handleCancelForm}>
                  {t("common.cancel")}
                </Button>
                <Button type="submit" disabled={testingConnection()} loading={testingConnection()}>
                  {testingConnection()
                    ? t("mcp.testingConnection")
                    : isEditing()
                      ? t("mcp.form.update")
                      : t("mcp.form.create")}
                </Button>
              </div>
            </form>
          </Card.Body>
        </Card>
      </Show>

      {/* Loading state */}
      <Show when={servers.loading}>
        <LoadingState message={t("mcp.loading")} />
      </Show>

      {/* Error state */}
      <Show when={servers.error}>
        <Alert variant="error">{t("mcp.loadError")}</Alert>
      </Show>

      {/* Server list */}
      <Show when={!servers.loading && !servers.error}>
        <Show when={(servers() ?? []).length > 0} fallback={<EmptyState title={t("mcp.empty")} />}>
          <Table<MCPServer> columns={serverColumns} data={servers() ?? []} rowKey={(s) => s.id} />

          {/* Expandable tools sections below the table */}
          <For each={servers() ?? []}>{(server) => <MCPServerToolsPanel server={server} />}</For>
        </Show>
      </Show>

      {/* Delete confirm dialog */}
      <ConfirmDialog
        open={del.target() !== null}
        title={t("common.delete")}
        message={t("mcp.deleteConfirm")}
        variant="danger"
        confirmLabel={t("common.delete")}
        cancelLabel={t("common.cancel")}
        onConfirm={() => void del.confirm()}
        onCancel={del.cancel}
      />

      {/* Test-failed confirm dialog */}
      <ConfirmDialog
        open={pendingRequest() !== null}
        title={t("mcp.testFailedTitle")}
        message={t("mcp.testFailedMessage", { error: testFailError() })}
        variant="danger"
        confirmLabel={t("mcp.testFailedSaveAnyway")}
        cancelLabel={t("common.cancel")}
        onConfirm={() => void handleTestFailConfirm()}
        onCancel={handleTestFailCancel}
      />
    </PageLayout>
  );
}

// ---------------------------------------------------------------------------
// Status badge component
// ---------------------------------------------------------------------------

function StatusBadge(props: { status: MCPServer["status"] }) {
  const { t } = useI18n();

  const config = (): { label: string; variant: "success" | "default" | "danger" | "info" } => {
    switch (props.status) {
      case "connected":
        return { label: t("mcp.status.connected"), variant: "success" };
      case "disconnected":
        return { label: t("mcp.status.disconnected"), variant: "default" };
      case "error":
        return { label: t("mcp.status.error"), variant: "danger" };
      case "registered":
      default:
        return { label: t("mcp.status.registered"), variant: "info" };
    }
  };

  return (
    <Badge variant={config().variant} pill>
      {config().label}
    </Badge>
  );
}

// ---------------------------------------------------------------------------
// Server action buttons (used inside table row)
// ---------------------------------------------------------------------------

function MCPServerActions(props: {
  server: MCPServer;
  onEdit: (server: MCPServer) => void;
  onDelete: (server: MCPServer) => void;
  onRefetch: () => void;
}) {
  const { t } = useI18n();
  const { show: toast } = useToast();

  const { run: handleTest, loading: testing } = useAsyncAction(
    async () => {
      const result = await api.mcp.testServer(props.server.id);
      if (result.success) {
        const toolCount = result.tools?.length ?? 0;
        toast("success", t("mcp.testSuccessTools", { count: String(toolCount) }));
      } else {
        toast("error", result.error ?? t("mcp.testFailed"));
      }
      props.onRefetch();
    },
    {
      onError: () => {
        toast("error", t("mcp.testFailed"));
      },
    },
  );

  return (
    <div class="flex items-center gap-2">
      <Button
        variant="secondary"
        size="sm"
        onClick={() => void handleTest()}
        disabled={testing()}
        loading={testing()}
        aria-label={t("mcp.testAria", { name: props.server.name })}
      >
        {testing() ? t("mcp.testing") : t("mcp.test")}
      </Button>
      <Button
        variant="ghost"
        size="sm"
        onClick={() => props.onEdit(props.server)}
        aria-label={t("mcp.editAria", { name: props.server.name })}
      >
        {t("mcp.editServer")}
      </Button>
      <Button
        variant="ghost"
        size="sm"
        class="text-cf-danger-fg hover:text-cf-danger-fg"
        onClick={() => props.onDelete(props.server)}
        aria-label={t("mcp.deleteAria", { name: props.server.name })}
      >
        {t("common.delete")}
      </Button>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Expandable tools panel per server
// ---------------------------------------------------------------------------

function MCPServerToolsPanel(props: { server: MCPServer }) {
  const { t } = useI18n();
  const [showTools, setShowTools] = createSignal(false);
  const [tools, setTools] = createSignal<MCPServerTool[] | null>(null);

  const { run: loadTools, loading: toolsLoading } = useAsyncAction(
    async () => {
      const result = await api.mcp.listTools(props.server.id);
      setTools(result);
    },
    {
      onError: () => {
        setTools([]);
      },
    },
  );

  const handleToggleTools = () => {
    if (showTools()) {
      setShowTools(false);
      return;
    }
    if (tools() === null) {
      void loadTools();
    }
    setShowTools(true);
  };

  return (
    <Show when={true}>
      <div class="mt-2">
        <Button
          variant="ghost"
          size="sm"
          onClick={handleToggleTools}
          aria-label={
            showTools()
              ? t("mcp.tools.hideToolsAria", { name: props.server.name })
              : t("mcp.tools.showToolsAria", { name: props.server.name })
          }
        >
          {showTools()
            ? t("mcp.tools.hideTools") + " - " + props.server.name
            : t("mcp.tools.showTools") + " - " + props.server.name}
        </Button>
        <Show when={showTools()}>
          <Card class="mt-2">
            <Card.Body>
              <h4 class="mb-2 text-sm font-medium text-cf-text-secondary">{t("mcp.tools")}</h4>
              <Show when={toolsLoading()}>
                <LoadingState message={t("mcp.tools.loading")} />
              </Show>
              <Show when={!toolsLoading()}>
                <Show
                  when={(tools() ?? []).length > 0}
                  fallback={<EmptyState title={t("mcp.tools.empty")} />}
                >
                  <div class="space-y-2">
                    <For each={tools() ?? []}>{(tool) => <ToolCard tool={tool} />}</For>
                  </div>
                </Show>
              </Show>
            </Card.Body>
          </Card>
        </Show>
      </div>
    </Show>
  );
}

// ---------------------------------------------------------------------------
// Tool card within expandable panel
// ---------------------------------------------------------------------------

function ToolCard(props: { tool: MCPServerTool }) {
  const { t } = useI18n();
  const [showSchema, setShowSchema] = createSignal(false);

  return (
    <Card>
      <Card.Body class="p-3">
        <div class="flex items-start justify-between">
          <div>
            <span class="font-mono text-sm font-medium text-cf-text-primary">
              {props.tool.name}
            </span>
            <Show when={props.tool.description}>
              <p class="mt-0.5 text-xs text-cf-text-muted">{props.tool.description}</p>
            </Show>
          </div>
          <Show when={props.tool.input_schema && Object.keys(props.tool.input_schema).length > 0}>
            <Button variant="ghost" size="sm" onClick={() => setShowSchema((v) => !v)}>
              {showSchema() ? t("common.close") : t("mcp.tools.inputSchema")}
            </Button>
          </Show>
        </div>
        <Show when={showSchema()}>
          <pre class="mt-2 max-h-48 overflow-auto rounded-cf-md bg-cf-bg-surface-alt p-2 text-xs text-cf-text-secondary">
            {JSON.stringify(props.tool.input_schema, null, 2)}
          </pre>
        </Show>
      </Card.Body>
    </Card>
  );
}
