import { createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { CreateTaskRequest, Task, TaskStatus } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { Badge, Button, Card, FormField, Input, Textarea } from "~/ui";

interface TaskPanelProps {
  projectId: string;
  tasks: Task[];
  onRefetch: () => void;
  onError: (msg: string) => void;
}

function statusBadgeVariant(
  status: TaskStatus,
): "success" | "info" | "danger" | "warning" | "default" {
  switch (status) {
    case "completed":
      return "success";
    case "running":
      return "info";
    case "failed":
      return "danger";
    case "queued":
      return "warning";
    case "cancelled":
    default:
      return "default";
  }
}

export default function TaskPanel(props: TaskPanelProps) {
  const { t, fmt } = useI18n();
  const { show: toast } = useToast();
  const [showForm, setShowForm] = createSignal(false);
  const [title, setTitle] = createSignal("");
  const [prompt, setPrompt] = createSignal("");
  const [expanded, setExpanded] = createSignal<string | null>(null);

  const handleCreate = async (e: SubmitEvent) => {
    e.preventDefault();
    if (!title().trim() || !prompt().trim()) return;

    const data: CreateTaskRequest = { title: title(), prompt: prompt() };
    try {
      await api.tasks.create(props.projectId, data);
      setTitle("");
      setPrompt("");
      setShowForm(false);
      props.onRefetch();
      toast("success", t("task.toast.created"));
    } catch (err) {
      const msg = err instanceof Error ? err.message : t("task.toast.createFailed");
      props.onError(msg);
      toast("error", msg);
    }
  };

  const totalCost = () => props.tasks.reduce((sum, task) => sum + task.cost_usd, 0);

  return (
    <Card>
      <Card.Header>
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-3">
            <h3 class="text-lg font-semibold">{t("task.title")}</h3>
            <Show when={totalCost() > 0}>
              <Badge variant="info">
                {t("task.total")} {fmt.currency(totalCost())}
              </Badge>
            </Show>
          </div>
          <Button
            variant={showForm() ? "secondary" : "primary"}
            size="sm"
            onClick={() => setShowForm((v) => !v)}
          >
            {showForm() ? t("common.cancel") : t("task.newTask")}
          </Button>
        </div>
      </Card.Header>

      <Card.Body>
        <Show when={showForm()}>
          <form
            onSubmit={handleCreate}
            class="mb-4 rounded-cf-sm border border-cf-border-subtle bg-cf-bg-primary p-3"
          >
            <div class="space-y-3">
              <FormField id="task-title" label={t("task.form.title")} required>
                <Input
                  id="task-title"
                  type="text"
                  value={title()}
                  onInput={(e) => setTitle(e.currentTarget.value)}
                  placeholder={t("task.form.titlePlaceholder")}
                  aria-required="true"
                />
              </FormField>
              <FormField id="task-prompt" label={t("task.form.prompt")} required>
                <Textarea
                  id="task-prompt"
                  value={prompt()}
                  onInput={(e) => setPrompt(e.currentTarget.value)}
                  rows={3}
                  placeholder={t("task.form.promptPlaceholder")}
                  aria-required="true"
                />
              </FormField>
            </div>
            <div class="mt-3 flex justify-end">
              <Button type="submit" variant="primary" size="sm">
                {t("task.form.create")}
              </Button>
            </div>
          </form>
        </Show>

        <Show
          when={props.tasks.length > 0}
          fallback={<p class="text-sm text-cf-text-tertiary">{t("task.empty")}</p>}
        >
          <div class="space-y-2">
            <For each={props.tasks}>
              {(task) => (
                <div class="rounded-cf-sm border border-cf-border-subtle">
                  <div
                    class="flex cursor-pointer items-center justify-between p-3"
                    role="button"
                    tabIndex={0}
                    aria-expanded={expanded() === task.id}
                    aria-label={`Task: ${task.title}, status: ${task.status}`}
                    onClick={() => setExpanded((prev) => (prev === task.id ? null : task.id))}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault();
                        setExpanded((prev) => (prev === task.id ? null : task.id));
                      }
                    }}
                  >
                    <div class="flex items-center gap-2">
                      <span class="font-medium">{task.title}</span>
                      <span class="text-xs text-cf-text-muted">{task.id.slice(0, 8)}</span>
                      <Show when={task.cost_usd > 0}>
                        <Badge variant="info">{fmt.currency(task.cost_usd)}</Badge>
                      </Show>
                    </div>
                    <Badge variant={statusBadgeVariant(task.status)} pill>
                      {task.status}
                    </Badge>
                  </div>

                  <Show when={expanded() === task.id}>
                    <div class="border-t border-cf-border-subtle bg-cf-bg-primary p-3 text-sm">
                      <div class="mb-2 text-cf-text-tertiary">
                        <span class="font-medium">{t("task.prompt")}</span> {task.prompt}
                      </div>
                      <Show when={task.agent_id}>
                        <div class="text-xs text-cf-text-muted">
                          {t("task.agent")} {task.agent_id?.slice(0, 8) ?? ""}
                        </div>
                      </Show>
                      <Show when={task.result}>
                        {(result) => (
                          <div class="mt-2">
                            <Show when={result().output}>
                              <div class="mb-1">
                                <span class="text-xs font-medium text-cf-text-tertiary">
                                  {t("task.output")}
                                </span>
                                <pre class="mt-1 max-h-40 overflow-auto rounded-cf-sm bg-cf-bg-primary p-2 text-xs text-cf-success-fg">
                                  {result().output}
                                </pre>
                              </div>
                            </Show>
                            <Show when={result().error}>
                              <div class="mt-1 text-xs text-cf-danger-fg">
                                {t("task.errorLabel")} {result().error}
                              </div>
                            </Show>
                            <Show
                              when={(result().tokens_in ?? 0) > 0 || (result().tokens_out ?? 0) > 0}
                            >
                              <div class="mt-1 text-xs text-cf-text-muted">
                                {t("task.tokens")} {result().tokens_in ?? 0} in /{" "}
                                {result().tokens_out ?? 0} out
                              </div>
                            </Show>
                            <Show when={(result().files ?? []).length > 0}>
                              <div class="mt-1 text-xs text-cf-text-muted">
                                {t("task.files")} {(result().files ?? []).join(", ")}
                              </div>
                            </Show>
                          </div>
                        )}
                      </Show>
                    </div>
                  </Show>
                </div>
              )}
            </For>
          </div>
        </Show>
      </Card.Body>
    </Card>
  );
}
