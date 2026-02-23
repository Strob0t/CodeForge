import { render } from "@solidjs/testing-library";
import { describe, expect, it } from "vitest";

import { I18nProvider } from "~/i18n";

import { StepProgress } from "./StepProgress";

/** Render helper that wraps component in required providers. */
function renderWithProviders(props: { current: number; max?: number; label?: string }) {
  return render(() => (
    <I18nProvider>
      <StepProgress {...props} />
    </I18nProvider>
  ));
}

describe("StepProgress", () => {
  it("renders current and max step count", () => {
    const { container } = renderWithProviders({ current: 3, max: 50 });
    expect(container.textContent).toContain("3");
    expect(container.textContent).toContain("50");
  });

  it("renders a progressbar role element", () => {
    const { container } = renderWithProviders({ current: 10, max: 100 });
    const bar = container.querySelector('[role="progressbar"]');
    expect(bar).not.toBeNull();
    expect(bar?.getAttribute("aria-valuenow")).toBe("10");
    expect(bar?.getAttribute("aria-valuemax")).toBe("100");
  });

  it("shows only current count when max is not provided", () => {
    const { container } = renderWithProviders({ current: 7 });
    // Without max, it should show just the current number
    expect(container.textContent).toContain("7");
    // Should not contain a "/" separator since there's no max
    const spans = container.querySelectorAll("span");
    const textContent = Array.from(spans)
      .map((s) => s.textContent)
      .join("");
    // The fraction "current / max" should not appear
    expect(textContent).not.toMatch(/7\s*\/\s*\d+/);
  });

  it("uses custom label when provided", () => {
    const { container } = renderWithProviders({ current: 5, max: 20, label: "Custom Label" });
    expect(container.textContent).toContain("Custom Label");
  });

  it("clamps percent to 100 when current exceeds max", () => {
    const { container } = renderWithProviders({ current: 150, max: 100 });
    const bar = container.querySelector('[role="progressbar"]');
    expect(bar).not.toBeNull();
    // The fill bar width should be clamped to 100%
    const fillBar = bar?.querySelector("div");
    if (fillBar) {
      const width = fillBar.style.width;
      expect(width).toBe("100%");
    }
  });

  it("shows 0% width when current is 0", () => {
    const { container } = renderWithProviders({ current: 0, max: 50 });
    const bar = container.querySelector('[role="progressbar"]');
    expect(bar).not.toBeNull();
    const fillBar = bar?.querySelector("div");
    if (fillBar) {
      expect(fillBar.style.width).toBe("0%");
    }
  });
});
