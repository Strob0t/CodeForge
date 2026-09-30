import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMock = vi.hoisted(() => ({
  approve: vi.fn<(runId: string, callId: string, decision: string) => Promise<void>>(),
  allowAlways:
    vi.fn<(projectId: string, tool: string, command?: string, profile?: string) => Promise<void>>(),
}));

vi.mock("~/api/client", () => ({
  api: {
    runs: { approve: apiMock.approve },
    policies: { allowAlways: apiMock.allowAlways },
  },
}));

import PermissionRequestCard from "./PermissionRequestCard";

describe("PermissionRequestCard", () => {
  beforeEach(() => {
    apiMock.approve.mockReset().mockResolvedValue(undefined);
    apiMock.allowAlways.mockReset().mockResolvedValue(undefined);
  });

  it("extends the policy profile that asked when allowing always", async () => {
    render(() => (
      <PermissionRequestCard
        projectId="p1"
        runId="r1"
        callId="c1"
        tool="bash"
        command="make build"
        profile="headless-safe-sandbox"
      />
    ));
    fireEvent.click(screen.getByText("Allow Always"));
    await waitFor(() =>
      expect(apiMock.allowAlways).toHaveBeenCalledWith(
        "p1",
        "bash",
        "make build",
        "headless-safe-sandbox",
      ),
    );
    expect(apiMock.approve).toHaveBeenCalledWith("r1", "c1", "allow");
  });
});
