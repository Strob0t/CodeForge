import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import type { JSX } from "solid-js";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Conversation, Project, RetrievalIndexStatus } from "~/api/types";

// KI-121: SearchPage was routed nowhere (POST /search and /search/conversations
// were used only by the agent tool), and a conversation hit linked to /chat,
// which does not exist. It is routed at /search now, and a conversation hit
// opens the conversation in its project's chat.

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

const router = vi.hoisted(() => ({ navigate: vi.fn<(to: string) => void>() }));

vi.mock("@solidjs/router", () => ({
  A: (props: { href: string; class?: string; children?: JSX.Element }) => (
    <a href={props.href} class={props.class}>
      {props.children}
    </a>
  ),
  useNavigate: () => router.navigate,
  useLocation: () => ({ pathname: "/search" }),
}));

interface CodeHit {
  project_id: string;
  file: string;
  start_line: number;
  end_line: number;
  snippet: string;
  score: number;
}

interface ConversationHit {
  conversation_id: string;
  message_id: string;
  role: string;
  content: string;
  model: string;
  created_at: string;
}

const apiMock = vi.hoisted(() => ({
  projects: vi.fn<() => Promise<Project[]>>(),
  global: vi.fn<
    (
      q: string,
      ids?: string[],
      limit?: number,
    ) => Promise<{
      query: string;
      total: number;
      results: CodeHit[];
      indexes?: RetrievalIndexStatus[];
    }>
  >(),
  conversations:
    vi.fn<
      (
        q: string,
        ids?: string[],
        limit?: number,
      ) => Promise<{ query: string; total: number; results: ConversationHit[] }>
    >(),
  getConversation: vi.fn<(id: string) => Promise<Conversation>>(),
}));

vi.mock("~/api/client", () => ({
  api: {
    projects: { list: apiMock.projects },
    search: { global: apiMock.global, conversations: apiMock.conversations },
    conversations: { get: apiMock.getConversation },
  },
}));

import { ToastProvider } from "~/components/Toast";
import { I18nProvider } from "~/i18n";

import SearchPage from "./SearchPage";

function renderPage(): void {
  render(() => (
    <I18nProvider>
      <ToastProvider>
        <SearchPage />
      </ToastProvider>
    </I18nProvider>
  ));
}

function search(query: string): void {
  fireEvent.input(screen.getByLabelText("Search across all projects..."), {
    target: { value: query },
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  apiMock.projects.mockResolvedValue([{ id: "p-1", name: "shop" } as Project]);
  apiMock.global.mockResolvedValue({
    query: "cart",
    total: 1,
    results: [
      {
        project_id: "p-1",
        file: "src/cart.go",
        start_line: 12,
        end_line: 20,
        snippet: "func AddToCart()",
        score: 0.9,
      },
    ],
  });
  apiMock.conversations.mockResolvedValue({
    query: "cart",
    total: 1,
    results: [
      {
        conversation_id: "c-1",
        message_id: "m-1",
        role: "assistant",
        content: "The cart total is computed in src/cart.go.",
        model: "",
        created_at: "2026-10-04T10:00:00Z",
      },
    ],
  });
  apiMock.getConversation.mockResolvedValue({
    id: "c-1",
    tenant_id: "t-1",
    project_id: "p-1",
    title: "Cart",
    created_at: "",
    updated_at: "",
  });
});

describe("SearchPage", () => {
  it("searches the code of the tenant's projects and links a hit to its project", async () => {
    renderPage();
    search("cart");

    const file = await screen.findByText("src/cart.go");
    expect(apiMock.global).toHaveBeenCalledWith("cart", undefined, 30);
    expect(file.closest("a")?.getAttribute("href")).toBe("/projects/p-1");
  });

  it("opens a conversation hit in its project's chat", async () => {
    renderPage();
    search("cart");
    fireEvent.click(await screen.findByText("Conversations"));

    const hit = await screen.findByText("The cart total is computed in src/cart.go.");
    expect(apiMock.conversations).toHaveBeenCalledWith("cart", undefined, 30);
    fireEvent.click(hit);

    await waitFor(() =>
      expect(router.navigate).toHaveBeenCalledWith("/projects/p-1?conversation=c-1"),
    );
    expect(apiMock.getConversation).toHaveBeenCalledWith("c-1");
  });

  it("says so when a conversation hit cannot be opened", async () => {
    apiMock.getConversation.mockRejectedValue(new Error("conversation not found"));
    renderPage();
    search("cart");
    fireEvent.click(await screen.findByText("Conversations"));
    fireEvent.click(await screen.findByText("The cart total is computed in src/cart.go."));

    expect(await screen.findByText("conversation not found")).toBeDefined();
    expect(router.navigate).not.toHaveBeenCalled();
  });

  // KI-150: without an embedding key the index ended in "error" and the page
  // said only "No results found.".
  it("shows why the searched indexes find less", async () => {
    apiMock.projects.mockResolvedValue([
      { id: "p-1", name: "shop" } as Project,
      { id: "p-2", name: "blog" } as Project,
    ]);
    apiMock.global.mockResolvedValue({
      query: "cart",
      total: 0,
      results: [],
      indexes: [
        {
          project_id: "p-1",
          status: "ready",
          bm25_only: true,
          file_count: 3,
          chunk_count: 9,
          embedding_model: "text-embedding-3-small",
        },
        {
          project_id: "p-2",
          status: "error",
          error: "embedding call failed: 503",
          file_count: 0,
          chunk_count: 0,
          embedding_model: "",
        },
      ],
    });
    renderPage();
    search("cart");

    expect(await screen.findByText("No results found.")).toBeDefined();
    expect(await screen.findByText(/shop: the index ranks by keywords only/)).toBeDefined();
    expect(
      await screen.findByText("blog: the index failed: embedding call failed: 503"),
    ).toBeDefined();
  });

  it("does not search for an empty query", async () => {
    renderPage();
    search("   ");
    await new Promise((resolve) => setTimeout(resolve, 400));

    expect(apiMock.global).not.toHaveBeenCalled();
  });
});
