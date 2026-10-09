import { render, screen } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { KnowledgeBase } from "~/api/types";

// KI-146: the tenant's admins create, index and delete knowledge bases (the
// Go Core answers 403 to everyone else since KI-105), so the page offers
// those actions to admins only, as the MCP page does (KI-98).

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

const kb: KnowledgeBase = {
  id: "kb-1",
  name: "Go style",
  description: "Effective Go",
  category: "framework",
  tags: [],
  content_path: "go-style",
  status: "indexed",
  chunk_count: 12,
  created_at: "",
  updated_at: "",
};

vi.mock("~/api/client", () => ({
  api: { knowledgeBases: { list: () => Promise.resolve([kb]) } },
}));

/** The signed-in user's role (the auth context); the Go Core stays the authority. */
const auth = vi.hoisted(() => ({ role: "admin" as "admin" | "editor" | "viewer" }));

vi.mock("~/components/AuthProvider", () => ({
  useAuth: () => ({
    hasRole: (...roles: string[]) => roles.includes(auth.role),
  }),
}));

import { ConfirmProvider } from "~/components/ConfirmProvider";
import { ToastProvider } from "~/components/Toast";
import { I18nProvider } from "~/i18n";

import { KnowledgeBasesContent } from "./KnowledgeBasesPage";

const ADMIN_ONLY = /Only admins of your organization create/;

function renderPage(): void {
  render(() => (
    <I18nProvider>
      <ToastProvider>
        <ConfirmProvider>
          <KnowledgeBasesContent />
        </ConfirmProvider>
      </ToastProvider>
    </I18nProvider>
  ));
}

describe("KnowledgeBasesPage", () => {
  beforeEach(() => {
    auth.role = "admin";
  });

  it("offers create, index and delete to admins", async () => {
    renderPage();
    await screen.findByText("Go style");
    expect(screen.getByRole("button", { name: "Create Knowledge Base" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Re-index" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Delete" })).toBeTruthy();
    expect(screen.queryByText(ADMIN_ONLY)).toBeNull();
  });

  it.each(["editor", "viewer"] as const)("offers no admin action to a %s", async (role) => {
    auth.role = role;
    renderPage();
    await screen.findByText("Go style");
    expect(screen.queryByRole("button", { name: "Create Knowledge Base" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Re-index" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Delete" })).toBeNull();
    expect(screen.getByText(ADMIN_ONLY)).toBeTruthy();
  });
});
