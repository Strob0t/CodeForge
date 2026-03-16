import { createSignal } from "solid-js";

import { api } from "~/api/client";
import type { Project } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { Button, ErrorBanner, FormField, Input, Modal, Textarea } from "~/ui";

export interface EditProjectModalProps {
  project: Project | null;
  onClose: () => void;
  onUpdated: () => void;
}

export function EditProjectModal(props: EditProjectModalProps) {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [error, setError] = createSignal("");
  const [name, setName] = createSignal("");
  const [description, setDescription] = createSignal("");

  // Sync form fields when project changes
  const project = () => {
    const p = props.project;
    if (p) {
      setName(p.name);
      setDescription(p.description ?? "");
      setError("");
    }
    return p;
  };

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    setError("");
    const p = props.project;
    if (!p) return;

    const trimmedName = name().trim();
    if (!trimmedName) {
      setError(t("dashboard.toast.nameRequired"));
      return;
    }

    try {
      await api.projects.update(p.id, {
        name: trimmedName,
        description: description(),
      });
      toast("success", t("dashboard.toast.updated"));
      props.onUpdated();
    } catch (err) {
      const msg = err instanceof Error ? err.message : t("dashboard.toast.updateFailed");
      setError(msg);
      toast("error", msg);
    }
  }

  return (
    <Modal open={!!project()} onClose={props.onClose} title={t("dashboard.form.edit")}>
      <ErrorBanner error={error} onDismiss={() => setError("")} />

      <form onSubmit={handleSubmit}>
        <div class="grid grid-cols-1 gap-4">
          <FormField label={t("dashboard.form.name")} id="edit_name" required>
            <Input
              id="edit_name"
              type="text"
              value={name()}
              onInput={(e) => setName(e.currentTarget.value)}
              aria-required="true"
            />
          </FormField>

          <FormField label={t("dashboard.form.description")} id="edit_description">
            <Textarea
              id="edit_description"
              value={description()}
              onInput={(e) => setDescription(e.currentTarget.value)}
              rows={3}
            />
          </FormField>
        </div>

        <div class="mt-4 flex justify-end gap-2">
          <Button variant="secondary" onClick={props.onClose}>
            {t("common.cancel")}
          </Button>
          <Button type="submit">{t("common.save")}</Button>
        </div>
      </form>
    </Modal>
  );
}
