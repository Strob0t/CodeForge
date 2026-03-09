import { createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { FeatureStatus, RoadmapFeature } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useAsyncAction } from "~/hooks";
import { useI18n } from "~/i18n";
import { Button, Input, Select, Textarea } from "~/ui";
import { getErrorMessage } from "~/utils/getErrorMessage";

interface FeatureCardFormProps {
  milestoneId: string;
  feature?: RoadmapFeature;
  onSave: () => void;
  onCancel: () => void;
}

const FEATURE_STATUSES: FeatureStatus[] = [
  "backlog",
  "planned",
  "in_progress",
  "done",
  "cancelled",
];

export default function FeatureCardForm(props: FeatureCardFormProps) {
  const { t } = useI18n();
  const { show: toast } = useToast();

  // eslint-disable-next-line solid/reactivity -- intentional one-time initialization
  const [title, setTitle] = createSignal(props.feature?.title ?? "");
  // eslint-disable-next-line solid/reactivity -- intentional one-time initialization
  const [status, setStatus] = createSignal<FeatureStatus>(props.feature?.status ?? "backlog");
  // eslint-disable-next-line solid/reactivity -- intentional one-time initialization
  const [description, setDescription] = createSignal(props.feature?.description ?? "");

  const { run: handleSave, loading: saving } = useAsyncAction(
    async () => {
      const trimmed = title().trim();
      if (!trimmed) return;

      if (props.feature) {
        await api.roadmap.updateFeature(props.feature.id, {
          title: trimmed,
          description: description().trim(),
          status: status(),
          version: props.feature.version,
        });
        toast("success", t("featuremap.featureUpdated"));
      } else {
        await api.roadmap.createFeature(props.milestoneId, {
          title: trimmed,
          description: description().trim() || undefined,
        });
        toast("success", t("featuremap.featureCreated"));
      }
      props.onSave();
    },
    {
      onError: (err) => {
        toast("error", getErrorMessage(err, t("featuremap.createFailed")));
      },
    },
  );

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Enter") {
      e.preventDefault();
      void handleSave();
    } else if (e.key === "Escape") {
      props.onCancel();
    }
  };

  return (
    <div class="flex flex-col gap-2 rounded-cf-sm border border-cf-border bg-cf-bg-surface p-2">
      <Input
        type="text"
        placeholder={t("featuremap.featurePlaceholder")}
        value={title()}
        onInput={(e) => setTitle(e.currentTarget.value)}
        onKeyDown={handleKeyDown}
        autofocus
      />
      <Textarea
        placeholder={t("featuremap.descriptionPlaceholder")}
        value={description()}
        onInput={(e) => setDescription(e.currentTarget.value)}
        rows={3}
      />
      {/* Status selector only shown in edit mode (backend create has no status field) */}
      <Show when={props.feature}>
        <Select
          value={status()}
          onChange={(e) => setStatus(e.currentTarget.value as FeatureStatus)}
        >
          <For each={FEATURE_STATUSES}>{(s) => <option value={s}>{s}</option>}</For>
        </Select>
      </Show>
      <div class="flex gap-2 justify-end">
        <Button variant="ghost" size="sm" onClick={props.onCancel}>
          {t("featuremap.cancel")}
        </Button>
        <Button
          variant="primary"
          size="sm"
          onClick={() => void handleSave()}
          disabled={saving() || !title().trim()}
          loading={saving()}
        >
          {t("featuremap.save")}
        </Button>
      </div>
    </div>
  );
}
