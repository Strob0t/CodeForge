import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Conversation } from "~/api/types";

const apiMock = vi.hoisted(() => ({ get: vi.fn<(id: string) => Promise<Conversation>>() }));

vi.mock("~/api/client", () => ({ api: { conversations: { get: apiMock.get } } }));

import { conversationInProject } from "./linkedConversation";

function conversation(id: string, projectId: string): Conversation {
  return { id, tenant_id: "t-1", project_id: projectId, title: "", created_at: "", updated_at: "" };
}

// S7-G review: /projects/A?conversation=<a conversation of B> opened B's
// conversation in A's chat. A linked conversation opens only in its project.
describe("conversationInProject", () => {
  beforeEach(() => {
    apiMock.get.mockReset();
  });

  it("accepts a conversation of the project", async () => {
    apiMock.get.mockResolvedValue(conversation("c-1", "p-a"));
    await expect(conversationInProject("p-a", "c-1")).resolves.toBe(true);
    expect(apiMock.get).toHaveBeenCalledWith("c-1");
  });

  it("refuses a conversation of another project", async () => {
    apiMock.get.mockResolvedValue(conversation("c-2", "p-b"));
    await expect(conversationInProject("p-a", "c-2")).resolves.toBe(false);
  });

  it("passes on a failed load (unknown or another tenant's conversation)", async () => {
    apiMock.get.mockRejectedValue(new Error("conversation not found"));
    await expect(conversationInProject("p-a", "c-3")).rejects.toThrow("conversation not found");
  });
});
