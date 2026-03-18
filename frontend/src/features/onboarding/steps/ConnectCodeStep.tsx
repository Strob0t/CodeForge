import { createSignal, For, type JSX, Show } from "solid-js";

import { api } from "~/api/client";
import type { VCSProvider } from "~/api/types";
import { useAsyncAction } from "~/hooks";
import { Alert, Button, FormField, Input, Select } from "~/ui";

const VCS_PROVIDERS: { value: VCSProvider; label: string }[] = [
  { value: "github", label: "GitHub" },
  { value: "gitlab", label: "GitLab" },
  { value: "gitea", label: "Gitea" },
  { value: "bitbucket", label: "Bitbucket" },
];

export interface StepProps {
  onNext: () => void;
  onBack?: () => void;
}

export default function ConnectCodeStep(props: StepProps): JSX.Element {
  const [provider, setProvider] = createSignal<VCSProvider>("github");
  const [token, setToken] = createSignal("");
  const [label, setLabel] = createSignal("");
  const [success, setSuccess] = createSignal(false);

  const {
    run: handleAdd,
    loading,
    error,
  } = useAsyncAction(async () => {
    await api.vcsAccounts.create({
      provider: provider(),
      label: label().trim(),
      token: token().trim(),
    });
    setSuccess(true);
  });

  const onSkip = () => props.onNext();

  return (
    <div class="space-y-4">
      <Show when={success()}>
        <Alert variant="success">Account added successfully.</Alert>
      </Show>

      <Show when={error()}>
        <Alert variant="error">{error()}</Alert>
      </Show>

      <FormField label="Provider" id="onboard-vcs-provider">
        <Select
          id="onboard-vcs-provider"
          value={provider()}
          onChange={(e) => setProvider(e.currentTarget.value as VCSProvider)}
        >
          <For each={VCS_PROVIDERS}>{(p) => <option value={p.value}>{p.label}</option>}</For>
        </Select>
      </FormField>

      <FormField label="Token" id="onboard-vcs-token">
        <Input
          id="onboard-vcs-token"
          type="password"
          value={token()}
          onInput={(e) => setToken(e.currentTarget.value)}
          placeholder="ghp_... or glpat-..."
        />
      </FormField>

      <FormField label="Label" id="onboard-vcs-label">
        <Input
          id="onboard-vcs-label"
          type="text"
          value={label()}
          onInput={(e) => setLabel(e.currentTarget.value)}
          placeholder="e.g. my-github"
        />
      </FormField>

      <div class="flex items-center gap-3">
        <Button
          onClick={() => void handleAdd()}
          loading={loading()}
          disabled={!label().trim() || !token().trim()}
        >
          Add Account
        </Button>
        <button
          type="button"
          class="text-sm text-cf-text-muted hover:text-cf-text-secondary"
          onClick={onSkip}
        >
          Skip
        </button>
      </div>
    </div>
  );
}
