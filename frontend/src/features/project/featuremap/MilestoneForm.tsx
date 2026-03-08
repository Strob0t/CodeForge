import { createSignal } from "solid-js";

import { api } from "~/api/client";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { Button, Input } from "~/ui";

interface MilestoneFormProps {
  projectId: string;
  onSave: () => void;
  onCancel: () => void;
}

export default function MilestoneForm(props: MilestoneFormProps) {
  const { t } = useI18n();
  const { show: toast } = useToast();

  const [title, setTitle] = createSignal("");
  const [saving, setSaving] = createSignal(false);

  const handleSave = async () => {
    const trimmed = title().trim();
    if (!trimmed) return;

    setSaving(true);
    try {
      await api.roadmap.createMilestone(props.projectId, { title: trimmed });
      toast("success", t("featuremap.milestoneCreated"));
      props.onSave();
    } catch (e) {
      const msg = e instanceof Error ? e.message : t("featuremap.createFailed");
      toast("error", msg);
    } finally {
      setSaving(false);
    }
  };

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Enter") {
      e.preventDefault();
      void handleSave();
    } else if (e.key === "Escape") {
      props.onCancel();
    }
  };

  return (
    <div class="flex flex-col gap-2 p-3">
      <Input
        type="text"
        placeholder={t("featuremap.milestonePlaceholder")}
        value={title()}
        onInput={(e) => setTitle(e.currentTarget.value)}
        onKeyDown={handleKeyDown}
        autofocus
      />
      <div class="flex gap-2 justify-end">
        <Button variant="ghost" size="sm" onClick={props.onCancel}>
          {t("featuremap.cancel")}
        </Button>
        <Button
          variant="primary"
          size="sm"
          onClick={handleSave}
          disabled={saving() || !title().trim()}
          loading={saving()}
        >
          {t("featuremap.save")}
        </Button>
      </div>
    </div>
  );
}
