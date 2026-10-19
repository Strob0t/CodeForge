import { type JSX, splitProps } from "solid-js";

import { Button } from "../primitives/Button";
import { Modal } from "./Modal";

export interface ConfirmDialogProps {
  open: boolean;
  title: string;
  message: string | JSX.Element;
  confirmLabel?: string;
  cancelLabel?: string;
  variant?: "danger" | "primary";
  /** The confirmed action runs: the dialog can be neither confirmed again nor cancelled. */
  busy?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ConfirmDialog(props: ConfirmDialogProps): JSX.Element {
  const [local] = splitProps(props, [
    "open",
    "title",
    "message",
    "confirmLabel",
    "cancelLabel",
    "variant",
    "busy",
    "onConfirm",
    "onCancel",
  ]);

  const cancel = (): void => {
    if (!local.busy) local.onCancel();
  };

  return (
    <Modal open={local.open} onClose={cancel} title={local.title}>
      <div class="text-sm text-cf-text-secondary">{local.message}</div>
      <div class="mt-4 flex justify-end gap-2">
        <Button variant="secondary" disabled={local.busy} onClick={cancel}>
          {local.cancelLabel ?? "Cancel"}
        </Button>
        <Button
          variant={local.variant === "danger" ? "danger" : "primary"}
          loading={local.busy}
          onClick={local.onConfirm}
        >
          {local.confirmLabel ?? "Confirm"}
        </Button>
      </div>
    </Modal>
  );
}
