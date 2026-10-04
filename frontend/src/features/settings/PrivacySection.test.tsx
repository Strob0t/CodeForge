import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ConsentPurpose, ConsentStatus, User, UserDataExport } from "~/api/types";

// KI-93 / KI-121: the privacy page pointed to a Settings > Privacy screen that
// did not exist, and GET /me/export, DELETE /me/data and /me/consent had no UI.

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
  useLocation: () => ({ pathname: "/settings" }),
}));

const privacy = vi.hoisted(() => ({
  exportMyData: vi.fn<() => Promise<UserDataExport>>(),
  deleteMyData: vi.fn<() => Promise<undefined>>(),
  consentPurposes: vi.fn<() => Promise<ConsentPurpose[]>>(),
  consentStatus: vi.fn<() => Promise<ConsentStatus[]>>(),
  setConsent: vi.fn<(purposeId: string, granted: boolean) => Promise<undefined>>(),
}));

vi.mock("~/api/client", () => ({ api: { privacy } }));

const auth = vi.hoisted(() => ({ logout: vi.fn<() => Promise<void>>() }));

const USER: User = {
  id: "u-1",
  email: "Ada@Example.org",
  name: "Ada",
  role: "viewer",
  tenant_id: "t-1",
  enabled: true,
  is_platform_admin: false,
  created_at: "",
  updated_at: "",
};

vi.mock("~/components/AuthProvider", () => ({
  useAuth: () => ({ user: () => USER, logout: auth.logout }),
}));

import { ToastProvider } from "~/components/Toast";
import { I18nProvider } from "~/i18n";

import PrivacySection from "./PrivacySection";

const LLM: ConsentPurpose = {
  id: "llm-external",
  label: "External LLM processing",
  description: "Prompts go to the configured LLM providers.",
  legal_basis: "consent",
  required: false,
  version: 1,
};

const SERVICE: ConsentPurpose = {
  id: "service",
  label: "Service delivery",
  description: "Running the service.",
  legal_basis: "contract",
  required: true,
  version: 2,
};

function renderSection(): void {
  render(() => (
    <I18nProvider>
      <ToastProvider>
        <PrivacySection />
      </ToastProvider>
    </I18nProvider>
  ));
}

beforeEach(() => {
  vi.clearAllMocks();
  privacy.consentPurposes.mockResolvedValue([LLM, SERVICE]);
  privacy.consentStatus.mockResolvedValue([{ purpose_id: "service", granted: true }]);
  privacy.setConsent.mockResolvedValue(undefined);
  privacy.deleteMyData.mockResolvedValue(undefined);
  auth.logout.mockResolvedValue(undefined);
});

describe("PrivacySection export", () => {
  const createObjectURL = vi.fn<(blob: Blob) => string>(() => "blob:export");
  const revokeObjectURL = vi.fn<(href: string) => void>();
  let saved: { download: string; href: string }[];

  beforeEach(() => {
    saved = [];
    Object.assign(URL, { createObjectURL, revokeObjectURL });
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (
      this: HTMLAnchorElement,
    ) {
      saved.push({ download: this.download, href: this.href });
    });
  });

  afterEach(() => vi.restoreAllMocks());

  it("saves the export as the user's JSON file", async () => {
    privacy.exportMyData.mockResolvedValue({
      exported_at: "2026-10-04T00:00:00Z",
      format_version: "1.0",
      user: USER,
      api_keys: [],
      llm_keys: [],
      sessions: [],
      conversations: [],
      cost_records: [],
      audit_trail: [],
    });
    renderSection();

    fireEvent.click(await screen.findByRole("button", { name: "Download my data" }));

    await screen.findByText("Your data was downloaded.");
    expect(saved).toEqual([{ download: "codeforge-export-u-1.json", href: "blob:export" }]);
    const blob = createObjectURL.mock.calls[0][0];
    expect(blob.type).toBe("application/json");
    expect(JSON.parse(await blob.text())).toMatchObject({ format_version: "1.0", user: USER });
    await waitFor(() => expect(revokeObjectURL).toHaveBeenCalledWith("blob:export"));
  });

  it("reports a failed export and saves nothing", async () => {
    privacy.exportMyData.mockRejectedValue(new Error("boom"));
    renderSection();

    fireEvent.click(await screen.findByRole("button", { name: "Download my data" }));

    await screen.findByText("boom");
    expect(saved).toEqual([]);
  });
});

describe("PrivacySection delete", () => {
  async function openDialog(): Promise<HTMLInputElement> {
    renderSection();
    fireEvent.click(await screen.findByRole("button", { name: "Delete my account..." }));
    return (await screen.findByLabelText(
      "Type your email address Ada@Example.org to confirm",
    )) as HTMLInputElement;
  }

  const deleteButton = (): HTMLButtonElement =>
    screen.getByRole("button", { name: "Delete my account" }) as HTMLButtonElement;

  it("says what is deleted, what is kept and what stays with the organization", async () => {
    await openDialog();

    for (const text of [
      "Your account: name, email address, password and role",
      "Your API keys and LLM keys",
      "Your sign-in sessions and password reset links",
      "Your channel memberships and read markers",
      "Audit log entries of your actions (your email address and IP address are removed)",
      'Your channel messages (the sender becomes "Deleted user")',
      "Projects, conversations, runs and costs belong to your organization and are not deleted.",
      "Database backups that still contain your data are rotated out after about five weeks.",
    ]) {
      expect(screen.getByText(text)).toBeDefined();
    }
  });

  it.each(["", "ada@example.co", "someone@example.org", "Ada@Example.org!"])(
    "keeps the delete button disabled for %j",
    async (typed) => {
      const input = await openDialog();
      fireEvent.input(input, { target: { value: typed } });

      expect(deleteButton().disabled).toBe(true);
      fireEvent.click(deleteButton());
      expect(privacy.deleteMyData).not.toHaveBeenCalled();
    },
  );

  it("deletes after the email address is typed, then signs out", async () => {
    const input = await openDialog();
    fireEvent.input(input, { target: { value: "  ada@example.org " } });

    expect(deleteButton().disabled).toBe(false);
    fireEvent.click(deleteButton());

    await waitFor(() => expect(auth.logout).toHaveBeenCalledTimes(1));
    expect(privacy.deleteMyData).toHaveBeenCalledTimes(1);
    expect(await screen.findByText("Your account and data were deleted.")).toBeDefined();
  });

  it("signs out even when the server can no longer end the deleted user's session", async () => {
    auth.logout.mockRejectedValue(new Error("401"));
    const input = await openDialog();
    fireEvent.input(input, { target: { value: "Ada@Example.org" } });
    fireEvent.click(deleteButton());

    await waitFor(() => expect(auth.logout).toHaveBeenCalledTimes(1));
  });

  it("shows a failed deletion and keeps the user signed in", async () => {
    privacy.deleteMyData.mockRejectedValue(new Error("deletion failed"));
    const input = await openDialog();
    fireEvent.input(input, { target: { value: "Ada@Example.org" } });
    fireEvent.click(deleteButton());

    expect(await screen.findByText("deletion failed")).toBeDefined();
    expect(auth.logout).not.toHaveBeenCalled();
    expect(deleteButton().disabled).toBe(false);
  });

  it("deletes nothing when cancelled", async () => {
    const input = await openDialog();
    fireEvent.input(input, { target: { value: "Ada@Example.org" } });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(screen.queryByText("Delete your account?")).toBeNull());
    expect(privacy.deleteMyData).not.toHaveBeenCalled();
  });
});

describe("PrivacySection consent", () => {
  const toggle = (label: string): HTMLInputElement =>
    screen.getByRole("checkbox", { name: `Consent to ${label}` }) as HTMLInputElement;

  it("shows each purpose with its legal basis and the user's consent", async () => {
    renderSection();

    expect(await screen.findByText("External LLM processing")).toBeDefined();
    expect(screen.getByText("Consent (Art. 6(1)(a) GDPR)")).toBeDefined();
    expect(screen.getByText("Contract (Art. 6(1)(b) GDPR)")).toBeDefined();
    await waitFor(() => expect(toggle("Service delivery").checked).toBe(true));
    // A purpose without a record has no consent.
    expect(toggle("External LLM processing").checked).toBe(false);
  });

  it("records a consent and its withdrawal", async () => {
    renderSection();
    await screen.findByText("External LLM processing");

    fireEvent.click(toggle("External LLM processing"));
    await waitFor(() => expect(privacy.setConsent).toHaveBeenCalledWith("llm-external", true));
    await screen.findByText("Consent saved.");
    expect(toggle("External LLM processing").checked).toBe(true);

    fireEvent.click(toggle("External LLM processing"));
    await waitFor(() => expect(privacy.setConsent).toHaveBeenLastCalledWith("llm-external", false));
    expect(toggle("External LLM processing").checked).toBe(false);
  });

  it("shows the stored consent again when saving fails", async () => {
    privacy.setConsent.mockRejectedValue(new Error("consent refused"));
    renderSection();
    await screen.findByText("External LLM processing");

    fireEvent.click(toggle("External LLM processing"));

    await screen.findByText("consent refused");
    expect(toggle("External LLM processing").checked).toBe(false);
  });

  it("offers no withdrawal of a required purpose", async () => {
    renderSection();
    await waitFor(() => expect(toggle("Service delivery").checked).toBe(true));

    expect(toggle("Service delivery").disabled).toBe(true);
    expect(screen.getByText("A required purpose cannot be withdrawn.")).toBeDefined();
  });

  it("lets the user grant a required purpose not granted yet", async () => {
    privacy.consentStatus.mockResolvedValue([]);
    renderSection();
    await screen.findByText("Service delivery");

    expect(toggle("Service delivery").disabled).toBe(false);
  });

  it("says when the instance asks for no consent", async () => {
    privacy.consentPurposes.mockResolvedValue([]);
    renderSection();

    expect(await screen.findByText("This instance asks for no consent.")).toBeDefined();
  });

  it("says when the consent settings cannot be loaded", async () => {
    privacy.consentStatus.mockRejectedValue(new Error("down"));
    renderSection();

    expect(await screen.findByText("The consent settings could not be loaded.")).toBeDefined();
  });
});
