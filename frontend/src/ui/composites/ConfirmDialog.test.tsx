import { fireEvent, render, screen } from "@solidjs/testing-library";
import { describe, expect, it, vi } from "vitest";

import { ConfirmDialog } from "./ConfirmDialog";

// S9-D review: a confirmed action that runs (a webhook rotation) must not be
// cancelled half-way; its answer would find no dialog to report to.
describe("ConfirmDialog", () => {
  function renderDialog(busy: boolean): { onConfirm: () => void; onCancel: () => void } {
    const onConfirm = vi.fn();
    const onCancel = vi.fn();
    render(() => (
      <ConfirmDialog
        open
        title="Rotate"
        message="Sure?"
        confirmLabel="Rotate"
        busy={busy}
        onConfirm={onConfirm}
        onCancel={onCancel}
      />
    ));
    return { onConfirm, onCancel };
  }

  it("cancels with the button and with Escape", () => {
    const { onCancel } = renderDialog(false);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(onCancel).toHaveBeenCalledTimes(2);
  });

  it("can be neither cancelled nor confirmed again while busy", () => {
    const { onConfirm, onCancel } = renderDialog(true);
    const cancel = screen.getByRole("button", { name: "Cancel" }) as HTMLButtonElement;
    expect(cancel.disabled).toBe(true);
    fireEvent.click(cancel);
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(onCancel).not.toHaveBeenCalled();

    const confirm = screen.getByRole("button", { name: /Rotate/ }) as HTMLButtonElement;
    expect(confirm.disabled).toBe(true);
    fireEvent.click(confirm);
    expect(onConfirm).not.toHaveBeenCalled();
  });
});
