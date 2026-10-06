import { render, screen } from "@solidjs/testing-library";
import { describe, expect, it } from "vitest";

import type { HealthFactors } from "~/api/types";
import { I18nProvider } from "~/i18n";

import HealthDot from "./HealthDot";

/** The factors of a project without runs: no errors and a stable cost. */
const noRuns: HealthFactors = {
  success_rate: 0,
  error_rate_inv: 100,
  activity_freshness: 0,
  task_velocity: 0,
  cost_stability: 100,
};

describe("HealthDot", () => {
  // KI-129: a new project without runs was shown as critical (score 35).
  it("shows a project without runs as unknown, not critical", () => {
    render(() => (
      <I18nProvider>
        <HealthDot score={35} level="unknown" factors={noRuns} />
      </I18nProvider>
    ));
    const dot = screen.getByTitle("No runs in the last 7 days");
    expect(dot.className).not.toContain("--cf-danger");
    expect(dot.className).toContain("--cf-text-muted");
  });

  it("shows a critical project in the danger color with its score", () => {
    render(() => (
      <I18nProvider>
        <HealthDot score={12} level="critical" factors={noRuns} />
      </I18nProvider>
    ));
    expect(screen.getByTitle("Health: 12").className).toContain("--cf-danger");
  });
});
