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
    "onConfirm",
    "onCancel",
  ]);

  return (
    <Modal open={local.open} onClose={local.onCancel} title={local.title}>
      <div class="text-sm text-cf-text-secondary">{local.message}</div>
      <div class="mt-4 flex justify-end gap-2">
        <Button variant="secondary" onClick={local.onCancel}>
          {local.cancelLabel ?? "Cancel"}
        </Button>
        <Button
          variant={local.variant === "danger" ? "danger" : "primary"}
          onClick={local.onConfirm}
        >
          {local.confirmLabel ?? "Confirm"}
        </Button>
      </div>
    </Modal>
  );
}
