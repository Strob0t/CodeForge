import { useParams } from "@solidjs/router";
import {
  createEffect,
  createResource,
  createSignal,
  ErrorBoundary,
  For,
  onCleanup,
  onMount,
  Show,
} from "solid-js";
import { Portal } from "solid-js/web";

import { api } from "~/api/client";
import {
  DEFAULT_SPLIT,
  MAX_SPLIT,
  MIN_SPLIT,
  ROADMAP_COLLAPSED_KEY,
  SPLIT_RATIO_KEY,
} from "~/config/constants";
import { useBreakpoint } from "~/hooks/useBreakpoint";
import { useI18n } from "~/i18n";
import { Alert, Badge, Button, ErrorBanner } from "~/ui";

import AuditTable from "../audit/AuditTable";
import { CanvasModal } from "../canvas/CanvasModal";
import ActiveWorkPanel from "./ActiveWorkPanel";
import AgentNetwork from "./AgentNetwork";
import AgentPanel from "./AgentPanel";
import ArchitectureGraph from "./ArchitectureGraph";
import AutoAgentButton from "./AutoAgentButton";
import BoundariesPanel from "./BoundariesPanel";
import ChatPanel from "./ChatPanel";
import CompactSettingsPopover from "./CompactSettingsPopover";
import CostBreakdown from "./CostBreakdown";
import FeatureMapPanel from "./FeatureMapPanel";
import FilePanel from "./FilePanel";
import GoalsPanel from "./GoalsPanel";
import LiveOutput from "./LiveOutput";
import LSPPanel from "./LSPPanel";
import MultiTerminal from "./MultiTerminal";
import OnboardingProgress from "./OnboardingProgress";
import PlanPanel from "./PlanPanel";
import PolicyPanel from "./PolicyPanel";
import RefactorApproval from "./RefactorApproval";
import RepoMapPanel from "./RepoMapPanel";
import RetrievalPanel from "./RetrievalPanel";
import RoadmapPanel from "./RoadmapPanel";
import RunPanel from "./RunPanel";
import SearchSimulator from "./SearchSimulator";
import SessionPanel from "./SessionPanel";
import TaskPanel from "./TaskPanel";
import TrajectoryPanel from "./TrajectoryPanel";
import { useProjectDetail } from "./useProjectDetail";
import WarRoom from "./WarRoom";

// ---------------------------------------------------------------------------
// Grouped Panel Selector (custom dropdown with optgroup-style headers + tooltips)
// ---------------------------------------------------------------------------

const PANEL_GROUPS = [
  {
    label: "Planning",
    items: [
      { value: "goals", label: "Goals", tip: "Define project goals and requirements" },
      { value: "roadmap", label: "Roadmap", tip: "Milestones and feature breakdown" },
      { value: "featuremap", label: "Feature Map", tip: "Visual feature board with drag-and-drop" },
      { value: "tasks", label: "Tasks & Roadmap", tip: "Task list synced with external PM tools" },
      { value: "plans", label: "Plans", tip: "Step-by-step execution plans for features" },
    ],
  },
  {
    label: "Execution",
    items: [
      { value: "warroom", label: "War Room", tip: "Live multi-agent coordination view" },
      { value: "agents", label: "Agents & Runs", tip: "Agent configuration and run management" },
      { value: "sessions", label: "Sessions", tip: "Agent session history and continuity" },
      { value: "trajectory", label: "Trajectory", tip: "Step-by-step replay of agent actions" },
    ],
  },
  {
    label: "Intelligence",
    items: [
      {
        value: "code",
        label: "Code Intelligence",
        tip: "Repo map, architecture graph, and LSP servers",
      },
      {
        value: "retrieval",
        label: "Retrieval",
        tip: "Search simulator for agent context retrieval",
      },
      {
        value: "boundaries",
        label: "Boundaries",
        tip: "Cross-layer boundary detection for contract review",
      },
    ],
  },
  {
    label: "Governance",
    items: [
      { value: "policy", label: "Policy", tip: "Permission rules and policy presets" },
      { value: "audit", label: "Audit Trail", tip: "Chronological log of all project actions" },
    ],
  },
] as const;

function PanelSelector(props: { value: string; onChange: (v: string) => void }) {
  const [open, setOpen] = createSignal(false);
  const [pos, setPos] = createSignal({ top: 0, left: 0 });
  let containerRef: HTMLDivElement | undefined;
  let btnRef: HTMLButtonElement | undefined;

  const selectedLabel = () => {
    for (const g of PANEL_GROUPS) {
      for (const item of g.items) {
        if (item.value === props.value) return item.label;
      }
    }
    return "More panels...";
  };

  const handleSelect = (value: string) => {
    props.onChange(value);
    setOpen(false);
  };

  const toggleOpen = () => {
    if (!open() && btnRef) {
      const rect = btnRef.getBoundingClientRect();
      setPos({ top: rect.bottom + 4, left: rect.left });
    }
    setOpen(!open());
  };

  // Close on outside click
  const onDocClick = (e: MouseEvent) => {
    if (containerRef && !containerRef.contains(e.target as Node)) setOpen(false);
  };
  onMount(() => document.addEventListener("mousedown", onDocClick));
  onCleanup(() => document.removeEventListener("mousedown", onDocClick));

  return (
    <div ref={containerRef} class="relative">
      <button
        ref={btnRef}
        type="button"
        class="h-8 rounded-md border border-cf-border bg-cf-bg px-2 pr-7 text-sm text-cf-text cursor-pointer focus:outline-none focus:ring-1 focus:ring-cf-accent text-left min-w-[140px]"
        style={{
          "background-image": `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='currentColor' stroke-width='2'%3E%3Cpath d='M6 9l6 6 6-6'/%3E%3C/svg%3E")`,
          "background-repeat": "no-repeat",
          "background-position": "right 0.5rem center",
        }}
        onClick={toggleOpen}
        aria-haspopup="listbox"
        aria-expanded={open()}
      >
        {props.value ? selectedLabel() : "More panels..."}
      </button>
      <Show when={open()}>
        <Portal>
          <div
            role="listbox"
            style={{
              position: "fixed",
              "z-index": "99999",
              top: `${pos().top}px`,
              left: `${pos().left}px`,
              "min-width": "260px",
              "max-height": "70vh",
              "overflow-y": "auto",
              "border-radius": "0.5rem",
              border: "1px solid var(--cf-border)",
              "background-color": "var(--cf-bg-surface)",
              "box-shadow": "0 10px 25px rgba(0,0,0,0.15)",
              padding: "0.25rem 0",
            }}
          >
            <For each={PANEL_GROUPS}>
              {(group) => (
                <>
                  <div class="px-3 pt-2.5 pb-1 text-[10px] font-bold uppercase tracking-wider text-cf-text-tertiary select-none">
                    {group.label}
                  </div>
                  <For each={group.items}>
                    {(item) => (
                      <button
                        type="button"
                        role="option"
                        aria-selected={props.value === item.value}
                        class={`w-full text-left px-3 py-1.5 text-sm cursor-pointer hover:bg-cf-accent/10 flex flex-col gap-0 ${props.value === item.value ? "bg-cf-accent/15 text-cf-accent font-medium" : "text-cf-text"}`}
                        onClick={() => handleSelect(item.value)}
                        title={item.tip}
                      >
                        <span>{item.label}</span>
                        <span class="text-[11px] text-cf-text-tertiary leading-tight">
                          {item.tip}
                        </span>
                      </button>
                    )}
                  </For>
                </>
              )}
            </For>
          </div>
        </Portal>
      </Show>
    </div>
  );
}

/** Fallback UI for sub-panel ErrorBoundary: shows an inline alert with retry */
function PanelErrorFallback(props: { error: Error; reset: () => void }) {
  return (
    <div class="px-4 py-3">
      <Alert variant="error">
        <div class="flex items-center justify-between gap-2">
          <span>{props.error.message || "Failed to load this panel"}</span>
          <Button variant="secondary" size="xs" onClick={props.reset}>
            Retry
          </Button>
        </div>
      </Alert>
    </div>
  );
}

/** Inline trajectory tab content with run selector + TrajectoryPanel */
function TrajectoryTabContent(props: {
  projectId: string;
  selectedRunId: string | null;
  onSelectRun: (id: string | null) => void;
  onNavigate?: (target: string) => void;
}) {
  const { t } = useI18n();
  const [runs] = createResource(
    () => props.projectId,
    (pid) => api.costs.recentRuns(pid, 50),
  );

  return (
    <div class="space-y-3">
      <Show
        when={!runs.loading && (runs() ?? []).length > 0}
        fallback={
          <Show
            when={!runs.loading}
            fallback={<p class="text-sm text-cf-text-muted">Loading...</p>}
          >
            <div class="flex flex-col items-center justify-center gap-3 py-16 text-center">
              <p class="text-sm text-cf-text-muted">{t("empty.trajectory")}</p>
              <button
                class="text-sm text-cf-accent hover:underline"
                onClick={() => props.onNavigate?.("sessions")}
              >
                {t("empty.trajectory.action")}
              </button>
            </div>
          </Show>
        }
      >
        <label class="flex items-center gap-2">
          <span class="text-sm font-medium text-cf-text-primary">{t("trajectory.runLabel")}:</span>
          <select
            class="rounded border border-cf-border bg-cf-bg-surface px-2 py-1 text-sm text-cf-text-primary"
            value={props.selectedRunId ?? ""}
            onChange={(e) => props.onSelectRun(e.currentTarget.value || null)}
          >
            <option value="">{t("trajectory.selectRun")}</option>
            <For each={runs() ?? []}>
              {(run) => (
                <option value={run.id}>
                  {run.id.slice(0, 8)} — {run.status} ({run.model || "?"})
                </option>
              )}
            </For>
          </select>
        </label>
        <Show when={props.selectedRunId}>{(runId) => <TrajectoryPanel runId={runId()} />}</Show>
      </Show>
    </div>
  );
}

export default function ProjectDetailPage() {
  const { t, fmt } = useI18n();
  const params = useParams<{ id: string }>();

  // Extract data-fetching, WS events, and state into a custom hook
  const pd = useProjectDetail(() => params.id);

  // Destructure for template readability
  const {
    project,
    refetchProject,
    tasks,
    refetchTasks,
    gitStatus,
    agents,
    onboardGoals,
    onboardRoadmap,
    onboardSessions,
    cloning,
    pulling,
    error,
    setError,
    budgetAlert,
    setBudgetAlert,
    settingsOpen,
    setSettingsOpen,
    showCanvas,
    setShowCanvas,
    autoAgentStatus,
    liveOutputTaskId,
    liveOutputLines,
    agentTerminals,
    activeRunCost,
    handleClone,
    handlePull,
  } = pd;

  // Left panel tab
  type LeftTab =
    | "roadmap"
    | "featuremap"
    | "files"
    | "warroom"
    | "goals"
    | "audit"
    | "sessions"
    | "trajectory"
    | "boundaries"
    | "agents"
    | "code"
    | "retrieval"
    | "plans"
    | "tasks"
    | "policy";
  const [leftTab, setLeftTab] = createSignal<LeftTab>("files");
  const [selectedRunId, setSelectedRunId] = createSignal<string | null>(null);

  // Unified navigation handler: "chat" switches mobile view, other tabs switch left panel
  function handleNavigate(target: string) {
    if (target === "chat") {
      if (isMobile()) {
        setMobileView("chat");
      }
      // On desktop the chat panel is always visible — no action needed
      return;
    }
    setLeftTab(target as LeftTab);
  }

  // Resizable split + collapsible roadmap
  const [splitRatio, setSplitRatio] = createSignal(DEFAULT_SPLIT);
  const [roadmapCollapsed, setRoadmapCollapsed] = createSignal(false);
  const [dragging, setDragging] = createSignal(false);
  const { isMobile, isDesktop } = useBreakpoint();
  type MobileView = "panels" | "chat";
  const [mobileView, setMobileView] = createSignal<MobileView>("panels");
  let containerRef: HTMLDivElement | undefined;

  onMount(() => {
    document.title = "Project - CodeForge";
    const savedRatio = localStorage.getItem(SPLIT_RATIO_KEY);
    if (savedRatio) {
      const n = Number(savedRatio);
      if (n >= MIN_SPLIT && n <= MAX_SPLIT) setSplitRatio(n);
    }
    setRoadmapCollapsed(localStorage.getItem(ROADMAP_COLLAPSED_KEY) === "true");
  });

  createEffect(() => {
    const p = project();
    if (p) {
      document.title = p.name + " - CodeForge";
    }
  });

  function persistSplit(ratio: number) {
    setSplitRatio(ratio);
    localStorage.setItem(SPLIT_RATIO_KEY, String(ratio));
  }

  function toggleRoadmap() {
    const next = !roadmapCollapsed();
    setRoadmapCollapsed(next);
    localStorage.setItem(ROADMAP_COLLAPSED_KEY, String(next));
  }

  function handlePointerDown(e: PointerEvent) {
    e.preventDefault();
    setDragging(true);
    const el = e.currentTarget as HTMLElement;
    el.setPointerCapture(e.pointerId);
  }

  function handlePointerMove(e: PointerEvent) {
    if (!dragging() || !containerRef) return;
    const rect = containerRef.getBoundingClientRect();
    const pct = ((e.clientX - rect.left) / rect.width) * 100;
    persistSplit(Math.max(MIN_SPLIT, Math.min(MAX_SPLIT, Math.round(pct))));
  }

  function handlePointerUp() {
    setDragging(false);
  }

  return (
    <div class="flex flex-col h-full">
      <Show
        when={project()}
        fallback={
          <Show
            when={project.error}
            fallback={<p class="text-cf-text-tertiary p-4">{t("detail.loading")}</p>}
          >
            <div class="flex flex-col items-center justify-center py-20 text-center">
              <h2 class="mb-2 text-xl font-bold text-cf-text-primary">
                {t("notFound.projectTitle")}
              </h2>
              <p class="mb-6 text-cf-text-tertiary">{t("notFound.projectMessage")}</p>
              <Button variant="primary" onClick={() => window.location.assign("/")}>
                {t("notFound.backToDashboard")}
              </Button>
            </div>
          </Show>
        }
      >
        {(p) => (
          <>
            {/* Header Bar */}
            <div class="flex flex-col gap-2 px-3 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-4 border-b border-cf-border flex-shrink-0">
              <div class="flex items-center gap-3">
                <h2 class="text-lg font-bold text-cf-text-primary">{p().name}</h2>

                {/* Git Status Badge */}
                <Show when={gitStatus()}>
                  {(gs) => (
                    <Badge variant={gs().dirty ? "warning" : "success"} pill>
                      <span class="font-mono">{gs().branch}</span>
                      <span>{gs().dirty ? t("detail.dirty") : t("detail.clean")}</span>
                    </Badge>
                  )}
                </Show>
              </div>

              <div class="flex flex-wrap items-center gap-2">
                {/* Clone Button */}
                <Show when={!p().workspace_path && p().repo_url}>
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={handleClone}
                    disabled={cloning()}
                    loading={cloning()}
                    aria-label={t("detail.cloneAria")}
                  >
                    {cloning() ? t("detail.cloning") : t("detail.cloneRepo")}
                  </Button>
                </Show>

                {/* Pull Button */}
                <Show when={p().workspace_path}>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={handlePull}
                    disabled={pulling()}
                    loading={pulling()}
                    aria-label={t("detail.pullAria")}
                  >
                    {pulling() ? t("detail.pulling") : t("detail.pull")}
                  </Button>
                </Show>

                {/* Auto-Agent Toggle */}
                <Show when={p().workspace_path}>
                  <AutoAgentButton projectId={params.id} wsStatus={autoAgentStatus} />
                </Show>

                {/* Design Canvas */}
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setShowCanvas(true)}
                  aria-label="Open Design Canvas"
                  title="Design Canvas"
                  data-testid="project-canvas-btn"
                >
                  <svg
                    class="h-5 w-5"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                    stroke-width="1.5"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M9.53 16.122a3 3 0 0 0-5.78 1.128 2.25 2.25 0 0 1-2.4 2.245 4.5 4.5 0 0 0 8.4-2.245c0-.399-.078-.78-.22-1.128Zm0 0a15.998 15.998 0 0 0 3.388-1.62m-5.043-.025a15.994 15.994 0 0 1 1.622-3.395m3.42 3.42a15.995 15.995 0 0 0 4.764-4.648l3.876-5.814a1.151 1.151 0 0 0-1.597-1.597L14.146 6.32a15.996 15.996 0 0 0-4.649 4.763m3.42 3.42a6.776 6.776 0 0 0-3.42-3.42"
                    />
                  </svg>
                </Button>

                {/* Settings Gear Icon */}
                <div class="relative">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setSettingsOpen(!settingsOpen())}
                    aria-label={t("detail.settings.gearTooltip")}
                    title={t("detail.settings.gearTooltip")}
                  >
                    <svg
                      class="h-5 w-5"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                      stroke-width="1.5"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.325.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 0 1 1.37.49l1.296 2.247a1.125 1.125 0 0 1-.26 1.431l-1.003.827c-.293.241-.438.613-.43.992a7.723 7.723 0 0 1 0 .255c-.008.378.137.75.43.991l1.004.827c.424.35.534.955.26 1.43l-1.298 2.247a1.125 1.125 0 0 1-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.47 6.47 0 0 1-.22.128c-.331.183-.581.495-.644.869l-.213 1.281c-.09.543-.56.94-1.11.94h-2.594c-.55 0-1.019-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 0 1-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 0 1-1.369-.49l-1.297-2.247a1.125 1.125 0 0 1 .26-1.431l1.004-.827c.292-.24.437-.613.43-.991a6.932 6.932 0 0 1 0-.255c.007-.38-.138-.751-.43-.992l-1.004-.827a1.125 1.125 0 0 1-.26-1.43l1.297-2.247a1.125 1.125 0 0 1 1.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.086.22-.128.332-.183.582-.495.644-.869l.214-1.28Z"
                      />
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        d="M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z"
                      />
                    </svg>
                  </Button>
                  <CompactSettingsPopover
                    projectId={params.id}
                    config={p().config ?? {}}
                    open={settingsOpen()}
                    onClose={() => setSettingsOpen(false)}
                    onSaved={() => {
                      refetchProject();
                      setSettingsOpen(false);
                    }}
                  />
                </div>
              </div>
            </div>

            {/* Onboarding Progress */}
            <OnboardingProgress
              projectId={params.id}
              hasWorkspace={!!p().workspace_path}
              hasStack={!!p().config?.detected_languages}
              hasGoals={(onboardGoals() ?? []).length > 0}
              hasRoadmap={(onboardRoadmap()?.milestones ?? []).length > 0}
              hasRuns={(onboardSessions() ?? []).length > 0}
              onNavigate={handleNavigate}
            />

            {/* Error Banner */}
            <Show when={error()}>
              <div class="mx-4 mt-2 flex-shrink-0">
                <ErrorBanner error={error} onDismiss={() => setError("")} class="" />
              </div>
            </Show>

            {/* Budget Alert Banner */}
            <Show when={budgetAlert()}>
              {(alert) => (
                <div class="mx-4 mt-2 flex-shrink-0">
                  <Alert variant="warning" onDismiss={() => setBudgetAlert(null)}>
                    {t("detail.budgetAlert", {
                      runId: alert().run_id.slice(0, 8),
                      pct: fmt.percent(alert().percentage),
                      cost: fmt.currency(alert().cost_usd),
                      max: fmt.currency(alert().max_cost),
                    })}
                  </Alert>
                </div>
              )}
            </Show>

            {/* Side-by-side Layout: Roadmap (left) | Chat (right) */}
            <div
              ref={containerRef}
              class={`flex flex-1 min-h-0 ${!isDesktop() ? "flex-col" : "flex-row"}`}
              onPointerMove={handlePointerMove}
              onPointerUp={handlePointerUp}
            >
              <Show when={!isMobile() || mobileView() === "panels"}>
                <Show when={!roadmapCollapsed()}>
                  <div
                    class={`flex flex-col min-h-0 overflow-hidden ${["featuremap", "files", "warroom", "goals", "audit", "sessions", "trajectory", "boundaries", "agents", "code", "retrieval", "plans", "tasks", "policy"].includes(leftTab()) ? "" : "overflow-y-auto"}`}
                    style={
                      isMobile()
                        ? { height: "100%" }
                        : !isDesktop()
                          ? { height: "50%", "border-bottom": "1px solid var(--cf-border)" }
                          : {
                              width: `${splitRatio()}%`,
                              "border-right": "1px solid var(--cf-border)",
                            }
                    }
                  >
                    <div class="flex items-center justify-between px-4 pt-3 pb-2 flex-shrink-0">
                      <div class="flex items-center gap-1.5">
                        <Show when={p().workspace_path}>
                          <Button
                            variant="ghost"
                            size="sm"
                            class={leftTab() === "files" ? "bg-cf-accent/15 text-cf-accent" : ""}
                            onClick={() => setLeftTab("files")}
                          >
                            Files
                          </Button>
                        </Show>
                        <PanelSelector
                          value={leftTab() === "files" ? "" : leftTab()}
                          onChange={(val) => setLeftTab(val as LeftTab)}
                        />
                      </div>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={toggleRoadmap}
                        title={t("detail.roadmap.collapse")}
                      >
                        <svg
                          class="h-4 w-4"
                          fill="none"
                          stroke="currentColor"
                          viewBox="0 0 24 24"
                          stroke-width="2"
                        >
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            d="M11 19l-7-7 7-7m8 14l-7-7 7-7"
                          />
                        </svg>
                      </Button>
                    </div>
                    <Show when={leftTab() === "files"}>
                      <ErrorBoundary
                        fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}
                      >
                        <div class="flex-1 min-h-0">
                          <FilePanel
                            projectId={params.id}
                            hasWorkspace={!!p().workspace_path}
                            onNavigate={handleNavigate}
                          />
                        </div>
                      </ErrorBoundary>
                    </Show>
                    <Show when={leftTab() === "goals"}>
                      <ErrorBoundary
                        fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}
                      >
                        <div class="flex-1 min-h-0">
                          <GoalsPanel projectId={params.id} onNavigate={handleNavigate} />
                        </div>
                      </ErrorBoundary>
                    </Show>
                    <Show when={leftTab() === "roadmap"}>
                      <ErrorBoundary
                        fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}
                      >
                        <div class="flex-1 overflow-y-auto px-4 pb-4">
                          <RoadmapPanel
                            projectId={params.id}
                            onError={setError}
                            onNavigate={handleNavigate}
                          />
                        </div>
                      </ErrorBoundary>
                    </Show>
                    <Show when={leftTab() === "featuremap"}>
                      <ErrorBoundary
                        fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}
                      >
                        <div class="flex-1 min-h-0">
                          <FeatureMapPanel
                            projectId={params.id}
                            onError={setError}
                            onNavigate={handleNavigate}
                          />
                        </div>
                      </ErrorBoundary>
                    </Show>
                    <Show when={leftTab() === "warroom"}>
                      <ErrorBoundary
                        fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}
                      >
                        <div class="flex-1 min-h-0">
                          <WarRoom projectId={params.id} onNavigate={handleNavigate} />
                        </div>
                      </ErrorBoundary>
                    </Show>
                    <Show when={leftTab() === "sessions"}>
                      <ErrorBoundary
                        fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}
                      >
                        <div class="flex-1 min-h-0 overflow-y-auto px-4 pb-4">
                          <SessionPanel projectId={params.id} onNavigate={handleNavigate} />
                        </div>
                      </ErrorBoundary>
                    </Show>
                    <Show when={leftTab() === "trajectory"}>
                      <ErrorBoundary
                        fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}
                      >
                        <div class="flex-1 min-h-0 overflow-y-auto px-4 pb-4">
                          <TrajectoryTabContent
                            projectId={params.id}
                            selectedRunId={selectedRunId()}
                            onSelectRun={setSelectedRunId}
                            onNavigate={handleNavigate}
                          />
                        </div>
                      </ErrorBoundary>
                    </Show>
                    <Show when={leftTab() === "audit"}>
                      <ErrorBoundary
                        fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}
                      >
                        <div class="flex-1 min-h-0 overflow-y-auto px-4 pb-4">
                          <AuditTable projectId={params.id} />
                        </div>
                      </ErrorBoundary>
                    </Show>
                    <Show when={leftTab() === "boundaries"}>
                      <ErrorBoundary
                        fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}
                      >
                        <div class="flex-1 min-h-0 overflow-y-auto px-4 pb-4">
                          <BoundariesPanel projectId={params.id} />
                        </div>
                      </ErrorBoundary>
                    </Show>
                    <Show when={leftTab() === "agents"}>
                      <ErrorBoundary
                        fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}
                      >
                        <div class="flex-1 min-h-0 overflow-y-auto px-4 pb-4 space-y-4">
                          <AgentPanel
                            projectId={params.id}
                            tasks={tasks() ?? []}
                            onError={setError}
                          />
                          <RunPanel
                            projectId={params.id}
                            tasks={tasks() ?? []}
                            agents={agents() ?? []}
                            onError={setError}
                          />
                          <AgentNetwork projectId={params.id} />
                          <Show when={liveOutputLines().length > 0}>
                            <LiveOutput taskId={liveOutputTaskId()} lines={liveOutputLines()} />
                          </Show>
                          <Show when={agentTerminals().length > 0}>
                            <MultiTerminal terminals={agentTerminals()} />
                          </Show>
                          <Show when={activeRunCost()}>
                            {(cost) => (
                              <CostBreakdown
                                costUsd={cost().costUsd}
                                tokensIn={cost().tokensIn}
                                tokensOut={cost().tokensOut}
                                steps={cost().steps}
                                model={cost().model}
                              />
                            )}
                          </Show>
                        </div>
                      </ErrorBoundary>
                    </Show>
                    <Show when={leftTab() === "code"}>
                      <ErrorBoundary
                        fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}
                      >
                        <div class="flex-1 min-h-0 overflow-y-auto px-4 pb-4 space-y-4">
                          <RepoMapPanel projectId={params.id} />
                          <ArchitectureGraph projectId={params.id} />
                          <LSPPanel projectId={params.id} />
                        </div>
                      </ErrorBoundary>
                    </Show>
                    <Show when={leftTab() === "retrieval"}>
                      <ErrorBoundary
                        fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}
                      >
                        <div class="flex-1 min-h-0 overflow-y-auto px-4 pb-4 space-y-4">
                          <RetrievalPanel projectId={params.id} />
                          <SearchSimulator projectId={params.id} />
                        </div>
                      </ErrorBoundary>
                    </Show>
                    <Show when={leftTab() === "plans"}>
                      <ErrorBoundary
                        fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}
                      >
                        <div class="flex-1 min-h-0 overflow-y-auto px-4 pb-4">
                          <PlanPanel
                            projectId={params.id}
                            tasks={tasks() ?? []}
                            agents={agents() ?? []}
                            onError={setError}
                          />
                        </div>
                      </ErrorBoundary>
                    </Show>
                    <Show when={leftTab() === "tasks"}>
                      <ErrorBoundary
                        fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}
                      >
                        <div class="flex-1 min-h-0 overflow-y-auto px-4 pb-4">
                          <TaskPanel
                            projectId={params.id}
                            tasks={tasks() ?? []}
                            onRefetch={refetchTasks}
                            onError={setError}
                          />
                        </div>
                      </ErrorBoundary>
                    </Show>
                    <Show when={leftTab() === "policy"}>
                      <ErrorBoundary
                        fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}
                      >
                        <div class="flex-1 min-h-0 overflow-y-auto px-4 pb-4">
                          <PolicyPanel projectId={params.id} onError={setError} />
                        </div>
                      </ErrorBoundary>
                    </Show>
                  </div>

                  {/* Draggable Divider */}
                  <Show when={isDesktop()}>
                    <div
                      class={`w-1.5 flex-shrink-0 cursor-col-resize hover:bg-cf-accent/30 transition-colors ${dragging() ? "bg-cf-accent/40" : "bg-transparent"}`}
                      onPointerDown={handlePointerDown}
                    />
                  </Show>
                </Show>
              </Show>

              <Show when={!isMobile() || mobileView() === "chat"}>
                <div
                  class="flex flex-col min-h-0 overflow-hidden"
                  style={
                    isMobile()
                      ? { height: "100%" }
                      : !isDesktop()
                        ? { height: roadmapCollapsed() ? "100%" : "50%" }
                        : { width: roadmapCollapsed() ? "100%" : `${100 - splitRatio()}%` }
                  }
                >
                  <Show when={roadmapCollapsed()}>
                    <div class="flex items-center px-4 py-1 border-b border-cf-border flex-shrink-0">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={toggleRoadmap}
                        title={t("detail.roadmap.expand")}
                      >
                        <svg
                          class="h-4 w-4"
                          fill="none"
                          stroke="currentColor"
                          viewBox="0 0 24 24"
                          stroke-width="2"
                        >
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            d="M13 5l7 7-7 7M5 5l7 7-7 7"
                          />
                        </svg>
                      </Button>
                      <span class="text-xs text-cf-text-muted ml-1">{t("detail.tab.roadmap")}</span>
                    </div>
                  </Show>
                  <ActiveWorkPanel projectId={params.id} />
                  <ErrorBoundary
                    fallback={(err, reset) => <PanelErrorFallback error={err} reset={reset} />}
                  >
                    <ChatPanel projectId={params.id} activeTab={leftTab()} />
                  </ErrorBoundary>
                </div>
              </Show>

              {/* Global overlay: refactor approval dialog */}
              <RefactorApproval />

              {/* Design Canvas modal overlay */}
              <CanvasModal
                open={showCanvas()}
                onClose={() => setShowCanvas(false)}
                onExport={() => setShowCanvas(false)}
              />

              {/* Mobile bottom tab bar */}
              <Show when={isMobile()}>
                <div class="flex border-t border-cf-border flex-shrink-0">
                  <button
                    type="button"
                    class={`flex-1 py-3 text-sm font-medium text-center min-h-[48px] transition-colors ${
                      mobileView() === "panels"
                        ? "text-cf-accent border-t-2 border-cf-accent bg-cf-bg-surface"
                        : "text-cf-text-muted hover:text-cf-text-secondary"
                    }`}
                    onClick={() => setMobileView("panels")}
                  >
                    Panels
                  </button>
                  <button
                    type="button"
                    class={`flex-1 py-3 text-sm font-medium text-center min-h-[48px] transition-colors ${
                      mobileView() === "chat"
                        ? "text-cf-accent border-t-2 border-cf-accent bg-cf-bg-surface"
                        : "text-cf-text-muted hover:text-cf-text-secondary"
                    }`}
                    onClick={() => setMobileView("chat")}
                  >
                    Chat
                  </button>
                </div>
              </Show>
            </div>
          </>
        )}
      </Show>
    </div>
  );
}
