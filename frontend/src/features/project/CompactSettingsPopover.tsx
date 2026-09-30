import { createResource, createSignal, For, onCleanup, Show } from "solid-js";

import { api } from "~/api/client";
import type { MCPServer } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { getErrorMessage } from "~/utils/getErrorMessage";

import { ProjectCostSection } from "../costs/CostDashboardPage";

interface CompactSettingsPopoverProps {
  projectId: string;
  open: boolean;
  onClose: () => void;
}

export default function CompactSettingsPopover(props: CompactSettingsPopoverProps) {
  const { t } = useI18n();
  const { show: toast } = useToast();

  const [assignedIds, setAssignedIds] = createSignal<Set<string>>(new Set());
  const [togglingId, setTogglingId] = createSignal<string | null>(null);

  const [allServers] = createResource(
    () => props.open,
    async (open) => {
      if (!open) return [] as MCPServer[];
      return api.mcp.listServers();
    },
  );

  const [projectServers] = createResource(
    () => (props.open ? props.projectId : false),
    async (projectId) => {
      const servers = await api.mcp.listProjectServers(projectId);
      setAssignedIds(new Set(servers.map((s) => s.id)));
      return servers;
    },
  );

  const toggleServer = async (serverId: string) => {
    setTogglingId(serverId);
    try {
      const assigned = assignedIds();
      if (assigned.has(serverId)) {
        await api.mcp.unassignFromProject(props.projectId, serverId);
        const next = new Set(assigned);
        next.delete(serverId);
        setAssignedIds(next);
      } else {
        await api.mcp.assignToProject(props.projectId, serverId);
        const next = new Set(assigned);
        next.add(serverId);
        setAssignedIds(next);
      }
    } catch (err) {
      toast("error", getErrorMessage(err, "Failed to update MCP server assignment"));
    } finally {
      setTogglingId(null);
    }
  };

  let popoverRef: HTMLDivElement | undefined;

  // Dismiss: click-outside and Escape key.
  // Listeners are registered once on mount and check props.open in the handler
  // to avoid SolidJS createEffect timing issues with addEventListener/removeEventListener.
  const handleClickOutside = (e: MouseEvent) => {
    // Check the parent container (which also holds the toggle button) so that
    // clicking the gear icon counts as "inside" and lets the toggle handler work.
    const container = popoverRef?.parentElement;
    if (props.open && container && !container.contains(e.target as Node)) {
      props.onClose();
    }
  };

  const handleKeyDown = (e: KeyboardEvent) => {
    if (props.open && e.key === "Escape") {
      e.preventDefault();
      props.onClose();
    }
  };

  document.addEventListener("mousedown", handleClickOutside);
  document.addEventListener("keydown", handleKeyDown);

  onCleanup(() => {
    document.removeEventListener("mousedown", handleClickOutside);
    document.removeEventListener("keydown", handleKeyDown);
  });

  return (
    <Show when={props.open}>
      <div
        ref={popoverRef}
        class="absolute right-0 top-full mt-2 w-[calc(100vw-2rem)] sm:w-96 max-w-96 rounded-cf-md border border-cf-border bg-cf-bg-surface shadow-cf-lg z-50 p-4"
      >
        <h3 class="text-sm font-semibold text-cf-text-primary mb-3">
          {t("detail.settings.title")}
        </h3>

        {/* Autonomy is a property of the mode a run or conversation uses. */}
        <p class="text-xs text-cf-text-tertiary mb-3">{t("detail.settings.autonomyFromMode")}</p>

        {/* MCP Servers */}
        <div class="mb-3">
          <h4 class="text-xs font-medium text-cf-text-tertiary mb-1">MCP Servers</h4>
          <p class="text-xs text-cf-text-tertiary mb-2">
            Assign documentation and tool servers to this project.
          </p>
          <Show
            when={!allServers.loading && !projectServers.loading}
            fallback={<p class="text-xs text-cf-text-tertiary">Loading...</p>}
          >
            <Show
              when={(allServers() ?? []).length > 0}
              fallback={
                <p class="text-xs text-cf-text-tertiary italic">No MCP servers registered.</p>
              }
            >
              <div class="space-y-1.5 max-h-32 overflow-y-auto">
                <For each={allServers() ?? []}>
                  {(server) => (
                    <label class="flex items-center gap-2 text-xs cursor-pointer">
                      <input
                        type="checkbox"
                        class="rounded border-cf-border text-cf-accent focus:ring-cf-accent"
                        checked={assignedIds().has(server.id)}
                        disabled={togglingId() === server.id}
                        onChange={() => void toggleServer(server.id)}
                      />
                      <span class="text-cf-text-primary truncate">{server.name}</span>
                      <span
                        class={`ml-auto inline-block h-2 w-2 rounded-full flex-shrink-0 ${
                          server.enabled ? "bg-green-500" : "bg-red-400"
                        }`}
                        title={server.enabled ? "Enabled" : "Disabled"}
                      />
                    </label>
                  )}
                </For>
              </div>
              <p class="text-xs text-cf-text-tertiary mt-1">
                {assignedIds().size} of {(allServers() ?? []).length} servers assigned
              </p>
            </Show>
          </Show>
        </div>

        {/* Cost Summary */}
        <div class="border-t border-cf-border pt-3">
          <h4 class="text-xs font-medium text-cf-text-tertiary mb-2">
            {t("detail.settings.costSummary")}
          </h4>
          <ProjectCostSection projectId={props.projectId} />
        </div>
      </div>
    </Show>
  );
}
