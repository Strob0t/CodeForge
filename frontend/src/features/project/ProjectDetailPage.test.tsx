import { render, screen, waitFor } from "@solidjs/testing-library";
import { ErrorBoundary } from "solid-js";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { GitStatus } from "~/api/types";
import type { WSMessage } from "~/api/websocket";

// KI-129 review: the branch badge refreshes after agent activity; a failed
// refresh must not throw where the page reads the status (the error
// boundary would replace the whole page).

vi.hoisted(() => {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => undefined,
      removeListener: () => undefined,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      dispatchEvent: () => false,
    }),
  });
});

vi.mock("@solidjs/router", () => ({
  A: (props: Record<string, unknown>) => props,
  useNavigate: () => () => undefined,
  useLocation: () => ({ pathname: "/" }),
  useParams: () => ({ id: "p-1" }),
  useSearchParams: () => [{}, () => undefined],
}));

const ws = vi.hoisted(() => {
  const handlers = new Set<(msg: WSMessage) => void>();
  return {
    handlers,
    emit(type: string, payload: Record<string, unknown>): void {
      for (const h of [...handlers]) h({ type, payload });
    },
  };
});

vi.mock("~/components/WebSocketProvider", () => ({
  useWebSocket: () => ({
    onMessage: (handler: (msg: WSMessage) => void) => {
      ws.handlers.add(handler);
      return () => ws.handlers.delete(handler);
    },
  }),
}));

const gitStatus = vi.hoisted(() => vi.fn<(projectId: string) => Promise<GitStatus>>());

vi.mock("~/api/client", () => ({
  api: {
    projects: {
      get: (id: string) =>
        Promise.resolve({ id, name: "Badge project", config: {}, workspace_path: "/ws" }),
      gitStatus,
      branches: () => Promise.resolve([]),
    },
    tasks: { list: () => Promise.resolve([]) },
    agents: { list: () => Promise.resolve([]) },
    goals: { list: () => Promise.resolve([]) },
    roadmap: { get: () => Promise.resolve(null) },
    sessions: { list: () => Promise.resolve([]) },
  },
}));

// The panels are not under test.
vi.mock("../canvas/CanvasModal", () => ({ CanvasModal: () => null }));
vi.mock("./PanelSelector", () => ({ PanelSelector: () => null }));
for (const panel of [
  "./ActiveWorkPanel",
  "./AgentPanel",
  "./AutoAgentButton",
  "./ChatPanel",
  "./CompactSettingsPopover",
  "./ConsolidatedPlanView",
  "./CostBreakdown",
  "./FeatureMapPanel",
  "./FilePanel",
  "./GoalsPanel",
  "./LiveOutput",
  "./MultiTerminal",
  "./OnboardingProgress",
  "./PlanPanel",
  "./PolicyPanel",
  "./RefactorApproval",
  "./RepoMapPanel",
  "./RoadmapPanel",
  "./RunPanel",
  "./SessionPanel",
  "./TaskPanel",
]) {
  vi.doMock(panel, () => ({ default: () => null }));
}

const onMain: GitStatus = {
  branch: "main",
  commit_hash: "abc",
  commit_message: "",
  dirty: false,
  ahead: 0,
  behind: 0,
};

async function renderPage(): Promise<void> {
  const [{ ToastProvider }, { I18nProvider }, { default: ProjectDetailPage }] = await Promise.all([
    import("~/components/Toast"),
    import("~/i18n"),
    import("./ProjectDetailPage"),
  ]);
  render(() => (
    <ErrorBoundary fallback={(err: Error) => <p>page crashed: {err.message}</p>}>
      <I18nProvider>
        <ToastProvider>
          <ProjectDetailPage />
        </ToastProvider>
      </I18nProvider>
    </ErrorBoundary>
  ));
}

describe("ProjectDetailPage branch badge", () => {
  beforeEach(() => {
    ws.handlers.clear();
    gitStatus.mockReset().mockResolvedValue(onMain);
  });

  it("keeps the page when a refresh of the git status fails", async () => {
    await renderPage();
    expect(await screen.findByText("main")).toBeTruthy();

    gitStatus.mockRejectedValue(new Error("git status failed"));
    ws.emit("run.status", {
      run_id: "r-1",
      task_id: "t-1",
      project_id: "p-1",
      status: "completed",
    });
    await waitFor(() => expect(gitStatus).toHaveBeenCalledTimes(2));
    await new Promise((resolve) => setTimeout(resolve, 20));

    expect(document.body.textContent).not.toContain("page crashed");
    expect(screen.getByText("Badge project")).toBeTruthy();
    expect(screen.queryByText("main")).toBeNull();

    // The next refresh shows the badge again.
    gitStatus.mockResolvedValue({ ...onMain, branch: "agent/fix" });
    ws.emit("run.status", { run_id: "r-2", task_id: "t-1", project_id: "p-1", status: "failed" });
    expect(await screen.findByText("agent/fix")).toBeTruthy();
  });
});
