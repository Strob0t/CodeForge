import { fireEvent, render } from "@solidjs/testing-library";
import { describe, expect, it, vi } from "vitest";

import type { BenchmarkExecMode } from "~/api/types";
import { I18nProvider } from "~/i18n";

import { EXEC_MODE_OPTIONS, ExecModeSelect } from "./ExecModeSelect";

function renderSelect(onChange: (mode: BenchmarkExecMode) => void = () => undefined) {
  return render(() => (
    <I18nProvider>
      <ExecModeSelect value="mount" onChange={onChange} />
    </I18nProvider>
  ));
}

function option(container: HTMLElement, value: BenchmarkExecMode): HTMLOptionElement {
  const el = container.querySelector<HTMLOptionElement>(`option[value="${value}"]`);
  if (!el) throw new Error(`option ${value} not rendered`);
  return el;
}

describe("ExecModeSelect", () => {
  it("offers only mount until sandbox isolation exists (KI-13)", () => {
    expect(EXEC_MODE_OPTIONS.filter((o) => o.available).map((o) => o.value)).toEqual(["mount"]);
  });

  it("keeps sandbox and hybrid visible but disabled and marked unavailable", () => {
    const { container } = renderSelect();

    expect(option(container, "mount").disabled).toBe(false);
    for (const value of ["sandbox", "hybrid"] as const) {
      const el = option(container, value);
      expect(el.disabled).toBe(true);
      expect(el.textContent).toContain("not available yet");
    }
  });

  it("explains why sandbox and hybrid are unavailable", () => {
    const { container } = renderSelect();

    expect(container.textContent).toContain("tools would run without isolation (KI-13)");
  });

  it("never reports an unavailable mode as the selection", () => {
    const onChange = vi.fn();
    const { container } = renderSelect(onChange);
    const select = container.querySelector("select");
    if (!select) throw new Error("select not rendered");

    for (const value of ["sandbox", "hybrid"]) {
      select.value = value;
      fireEvent.change(select);
    }
    expect(onChange).not.toHaveBeenCalled();

    select.value = "mount";
    fireEvent.change(select);
    expect(onChange).toHaveBeenCalledWith("mount");
  });
});
