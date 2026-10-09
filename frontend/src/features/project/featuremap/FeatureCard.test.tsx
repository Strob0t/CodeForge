import { render, screen } from "@solidjs/testing-library";
import { describe, expect, it, vi } from "vitest";

import type { RoadmapFeature } from "~/api/types";
import { I18nProvider } from "~/i18n";

// jsdom has no matchMedia; the UI modules read it when they are loaded.
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

// ~/ui links through the router, whose .jsx sources vitest cannot load.
vi.mock("@solidjs/router", () => ({
  A: (props: Record<string, unknown>) => props,
  useNavigate: () => () => undefined,
  useLocation: () => ({ pathname: "/" }),
}));

import FeatureCard from "./FeatureCard";

function feature(result?: string, title = "Add CLI"): RoadmapFeature {
  return {
    id: "f1",
    milestone_id: "m1",
    roadmap_id: "r1",
    title,
    description: "",
    status: result?.startsWith("failed") ? "cancelled" : "done",
    sort_order: 0,
    labels: [],
    spec_ref: "",
    external_ids: {},
    result,
    version: 1,
    created_at: "",
    updated_at: "",
  };
}

function renderCard(f: RoadmapFeature) {
  return render(() => (
    <I18nProvider>
      <FeatureCard
        feature={f}
        index={0}
        milestoneId="m1"
        onStatusToggle={() => undefined}
        onEdit={() => undefined}
      />
    </I18nProvider>
  ));
}

// KI-152: the auto-agent records how its verification of a feature ended.
describe("FeatureCard", () => {
  it("shows the auto-agent's verification result", () => {
    const result = "failed: verification failed after 2 fix attempts: tests failed (`pytest`)";
    renderCard(feature(result));
    expect(screen.getByText(result)).toBeTruthy();
  });

  it("shows no result line for a feature the auto-agent did not run", () => {
    const { container } = renderCard(feature());
    expect(container.querySelector("p")).toBeNull();
  });

  // KI-129: long titles were cut off after one line.
  it("wraps a long title to two lines and shows it in full on hover", () => {
    const title =
      "Support importing OpenSpec change proposals from nested monorepo packages with a preview";
    renderCard(feature(undefined, title));
    const el = screen.getByText(title);
    expect(el.getAttribute("title")).toBe(title);
    expect(el.className).toContain("line-clamp-2");
    expect(el.className).not.toContain("truncate");
  });
});
