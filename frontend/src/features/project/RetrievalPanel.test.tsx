import { render, screen, waitFor } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { RetrievalIndexStatus } from "~/api/types";

// KI-138: a BM25-only index (no usable embedding model) looked like a full
// index; the panel says that semantic search is off.

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
}));

const mocks = vi.hoisted(() => ({
  indexStatus: vi.fn<(projectId: string) => Promise<RetrievalIndexStatus>>(),
}));

vi.mock("~/api/client", () => ({
  api: {
    retrieval: { indexStatus: mocks.indexStatus },
    graph: { status: () => Promise.reject(new Error("no graph")) },
  },
}));

import { I18nProvider } from "~/i18n";

import RetrievalPanel from "./RetrievalPanel";

const NOTICE = /Semantic search is off/;

function index(bm25Only?: boolean): RetrievalIndexStatus {
  return {
    project_id: "p1",
    status: "ready",
    file_count: 12,
    chunk_count: 80,
    embedding_model: "",
    bm25_only: bm25Only,
  };
}

function renderPanel(): void {
  render(() => (
    <I18nProvider>
      <RetrievalPanel projectId="p1" />
    </I18nProvider>
  ));
}

describe("RetrievalPanel", () => {
  beforeEach(() => {
    mocks.indexStatus.mockReset();
  });

  it("says that semantic search is off for a BM25-only index", async () => {
    mocks.indexStatus.mockResolvedValue(index(true));
    renderPanel();
    expect(await screen.findByText(NOTICE)).toBeTruthy();
  });

  it.each([
    ["a full index", false],
    ["an index without the flag", undefined],
  ])("shows no notice for %s", async (_name, bm25Only) => {
    mocks.indexStatus.mockResolvedValue(index(bm25Only));
    renderPanel();
    await waitFor(() => expect(screen.getByText("80")).toBeTruthy());
    expect(screen.queryByText(NOTICE)).toBeNull();
  });
});
