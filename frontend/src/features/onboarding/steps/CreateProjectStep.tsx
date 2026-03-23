import { createSignal, type JSX, Show } from "solid-js";

import { api } from "~/api/client";
import { useAsyncAction } from "~/hooks";
import { Alert, Button, FormField, Input } from "~/ui";

export interface StepProps {
  onNext: () => void;
  onBack?: () => void;
}

// eslint-disable-next-line @typescript-eslint/no-unused-vars
export default function CreateProjectStep(_props: StepProps): JSX.Element {
  const [name, setName] = createSignal("");
  const [repoUrl, setRepoUrl] = createSignal("");
  const [success, setSuccess] = createSignal(false);

  const {
    run: handleCreate,
    loading,
    error,
  } = useAsyncAction(async () => {
    const created = await api.projects.create({
      name: name().trim(),
      description: "",
      repo_url: repoUrl().trim(),
      provider: "",
      config: {},
    });
    setSuccess(true);

    // Auto-setup if repo URL was provided
    if (created.repo_url) {
      api.projects.setup(created.id).catch(() => {
        // best-effort: setup runs in background, errors are non-blocking and logged server-side
      });
    }
  });

  return (
    <div class="space-y-4">
      <Show when={success()}>
        <Alert variant="success">Project created successfully.</Alert>
      </Show>

      <Show when={error()}>
        <Alert variant="error">{error()}</Alert>
      </Show>

      <FormField label="Project name" id="onboard-project-name" required>
        <Input
          id="onboard-project-name"
          type="text"
          value={name()}
          onInput={(e) => setName(e.currentTarget.value)}
          placeholder="my-project"
          aria-required="true"
        />
      </FormField>

      <FormField label="Repository URL" id="onboard-repo-url">
        <Input
          id="onboard-repo-url"
          type="text"
          value={repoUrl()}
          onInput={(e) => setRepoUrl(e.currentTarget.value)}
          placeholder="https://github.com/user/repo (optional)"
        />
      </FormField>

      <Button onClick={() => void handleCreate()} loading={loading()} disabled={!name().trim()}>
        Create Project
      </Button>
    </div>
  );
}
